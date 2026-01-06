package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/scotttball/tunatap/internal/autofix"
	"github.com/scotttball/tunatap/internal/client"
	"github.com/scotttball/tunatap/internal/config"
	"github.com/scotttball/tunatap/internal/preflight"
	"github.com/scotttball/tunatap/pkg/utils"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose configuration and connectivity issues",
	Long: `Check tunatap configuration, OCI credentials, SSH keys, and connectivity.

This command performs various checks to help diagnose issues with your setup.

Examples:
  # Basic diagnostics
  tunatap doctor

  # Verbose output with details
  tunatap doctor -v

  # Check specific cluster (includes OCI-aware preflight checks)
  tunatap doctor --cluster my-cluster

  # Full preflight checks for a cluster
  tunatap doctor --cluster my-cluster --preflight

  # Auto-fix safe issues
  tunatap doctor --auto-fix

  # Show what auto-fix would do
  tunatap doctor --auto-fix --dry-run`,
	RunE: runDoctor,
}

var (
	doctorVerbose   bool
	doctorCluster   string
	doctorPreflight bool
	doctorAutoFix   bool
	doctorDryRun    bool
)

func init() {
	rootCmd.AddCommand(doctorCmd)
	doctorCmd.Flags().BoolVarP(&doctorVerbose, "verbose", "v", false, "show detailed output")
	doctorCmd.Flags().StringVarP(&doctorCluster, "cluster", "c", "", "run cluster-specific diagnostics")
	doctorCmd.Flags().BoolVar(&doctorPreflight, "preflight", false, "run full preflight checks (requires --cluster)")
	doctorCmd.Flags().BoolVar(&doctorAutoFix, "auto-fix", false, "automatically fix safe issues")
	doctorCmd.Flags().BoolVar(&doctorDryRun, "dry-run", false, "show what auto-fix would do without making changes")
}

type checkResult struct {
	name    string
	status  string
	message string
}

func runDoctor(cmd *cobra.Command, args []string) error {
	fmt.Println("Running tunatap diagnostics...")
	fmt.Println()

	results := []checkResult{}

	// Check 1: Configuration file
	results = append(results, checkConfigFile())

	// Check 2: OCI configuration
	results = append(results, checkOCIConfig())

	// Check 3: SSH keys
	results = append(results, checkSSHKeys())

	// Check 4: OCI CLI
	results = append(results, checkOCICLI())

	// Check 5: OCI connectivity (if verbose)
	if doctorVerbose {
		results = append(results, checkOCIConnectivity())
	}

	// Check 6: Clusters configuration
	results = append(results, checkClustersConfig())

	// Print basic results
	fmt.Println("Basic Diagnostics:")
	fmt.Println("------------------")

	hasErrors := false
	for _, r := range results {
		statusIcon := "✓"
		if r.status == "error" {
			statusIcon = "✗"
			hasErrors = true
		} else if r.status == "warning" {
			statusIcon = "⚠"
		}

		fmt.Printf("%s %s: %s\n", statusIcon, r.name, r.message)
	}

	// Run cluster-specific preflight checks if requested
	if doctorCluster != "" || doctorPreflight {
		fmt.Println()
		preflightResults, preflightErr := runPreflightChecks(cmd.Context(), doctorCluster, doctorPreflight)
		if preflightErr != nil {
			return preflightErr
		}
		if preflight.HasErrors(preflightResults) {
			hasErrors = true
		}
	}

	// Run auto-fix if requested
	if doctorAutoFix {
		fmt.Println()
		if err := runAutoFix(doctorDryRun); err != nil {
			return err
		}
	}

	if hasErrors && !doctorAutoFix {
		fmt.Println("\nSome checks failed. Please review the errors above.")
		fmt.Println("Run 'tunatap doctor --auto-fix' to automatically fix safe issues.")
		return fmt.Errorf("diagnostics found issues")
	}

	if !doctorAutoFix {
		fmt.Println("\nAll checks passed!")
	}
	return nil
}

// runAutoFix runs the auto-fix process.
func runAutoFix(dryRun bool) error {
	fmt.Println("Auto-Fix:")
	fmt.Println("---------")

	fixer := autofix.NewFixer(GetConfigFile(), dryRun)
	fixes := fixer.Diagnose()

	if len(fixes) == 0 {
		fmt.Println("No issues found that can be auto-fixed.")
		return nil
	}

	// Show fixes
	safeFixes := autofix.GetSafeFixes(fixes)
	unsafeFixes := autofix.GetUnsafeFixes(fixes)

	if len(safeFixes) > 0 {
		fmt.Printf("\nSafe fixes (%d):\n", len(safeFixes))
		for _, fix := range safeFixes {
			fmt.Printf("  • %s\n", fix.Description)
		}
	}

	if len(unsafeFixes) > 0 {
		fmt.Printf("\nManual fixes required (%d):\n", len(unsafeFixes))
		for _, fix := range unsafeFixes {
			fmt.Printf("  • %s\n", fix.Description)
			if fix.Details != "" {
				fmt.Printf("    → %s\n", fix.Details)
			}
		}
	}

	// Apply safe fixes
	if len(safeFixes) > 0 {
		fmt.Println()
		if dryRun {
			fmt.Println("[DRY RUN] Would apply the following fixes:")
		} else {
			fmt.Println("Applying safe fixes...")
		}

		results := fixer.ApplySafe()
		for _, result := range results {
			fmt.Println(autofix.FormatResult(result))
		}
	}

	return nil
}

// runPreflightChecks runs OCI-aware preflight checks for a specific cluster.
func runPreflightChecks(ctx context.Context, clusterName string, fullPreflight bool) ([]preflight.CheckResult, error) {
	// Load config
	cfg, err := config.ReadConfig(GetConfigFile())
	if err != nil {
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	// Find cluster
	var cluster *config.Cluster
	if clusterName != "" {
		cluster = config.FindClusterByName(cfg, clusterName)
		if cluster == nil {
			return nil, fmt.Errorf("cluster '%s' not found in config", clusterName)
		}
	} else if len(cfg.Clusters) > 0 {
		cluster = cfg.Clusters[0]
		fmt.Printf("No cluster specified, using first cluster: %s\n", cluster.ClusterName)
	} else {
		return nil, fmt.Errorf("no clusters configured")
	}

	// Create OCI client
	var ociClient *client.OCIClient
	ociClient, err = createOCIClient(cfg, cluster.Region)
	if err != nil {
		log.Warn().Err(err).Msg("Could not create OCI client for preflight checks")
	}

	// Set up preflight options
	opts := &preflight.CheckOptions{
		Config:    cfg,
		Cluster:   cluster,
		OCIClient: ociClient,
		Verbose:   doctorVerbose,
		Timeout:   10 * time.Second,
	}

	// Create checker and run
	checker := preflight.NewChecker(opts)

	fmt.Printf("Preflight Checks for '%s':\n", cluster.ClusterName)
	fmt.Println("---------------------------")

	var results []preflight.CheckResult
	if fullPreflight {
		results = checker.RunAll(ctx)
	} else {
		results = checker.RunForCluster(ctx)
	}

	preflight.PrintResults(results, doctorVerbose)

	return results, nil
}

func checkConfigFile() checkResult {
	cfgPath := GetConfigFile()

	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		return checkResult{
			name:    "Config File",
			status:  "warning",
			message: fmt.Sprintf("Not found at %s. Run 'tunatap setup init' to create one.", cfgPath),
		}
	}

	cfg, err := config.ReadConfig(cfgPath)
	if err != nil {
		return checkResult{
			name:    "Config File",
			status:  "error",
			message: fmt.Sprintf("Failed to parse: %v", err),
		}
	}

	return checkResult{
		name:    "Config File",
		status:  "ok",
		message: fmt.Sprintf("Found at %s (%d clusters configured)", cfgPath, len(cfg.Clusters)),
	}
}

func checkOCIConfig() checkResult {
	ociConfigPath := utils.DefaultOCIConfigPath()

	if _, err := os.Stat(ociConfigPath); os.IsNotExist(err) {
		return checkResult{
			name:    "OCI Config",
			status:  "error",
			message: fmt.Sprintf("Not found at %s. Run 'oci setup config' to create one.", ociConfigPath),
		}
	}

	return checkResult{
		name:    "OCI Config",
		status:  "ok",
		message: fmt.Sprintf("Found at %s", ociConfigPath),
	}
}

func checkSSHKeys() checkResult {
	cfg, err := config.ReadConfig(GetConfigFile())
	if err != nil {
		cfg = config.DefaultConfig()
		cfg.SshPrivateKeyFile = utils.DefaultSSHPrivateKey()
	}

	keyPath := cfg.SshPrivateKeyFile
	if keyPath == "" {
		keyPath = utils.DefaultSSHPrivateKey()
	}

	// Expand path (handles ~ and normalizes separators)
	keyPath = utils.ExpandPath(keyPath)

	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		return checkResult{
			name:    "SSH Private Key",
			status:  "error",
			message: fmt.Sprintf("Not found at %s", keyPath),
		}
	}

	// Check for public key
	pubKeyPath := keyPath + ".pub"
	if _, err := os.Stat(pubKeyPath); os.IsNotExist(err) {
		return checkResult{
			name:    "SSH Private Key",
			status:  "warning",
			message: fmt.Sprintf("Found at %s, but public key (.pub) not found", keyPath),
		}
	}

	return checkResult{
		name:    "SSH Keys",
		status:  "ok",
		message: fmt.Sprintf("Found at %s (with public key)", keyPath),
	}
}

func checkOCICLI() checkResult {
	_, err := exec.LookPath("oci")
	if err != nil {
		return checkResult{
			name:    "OCI CLI",
			status:  "warning",
			message: "Not found in PATH. Some features may not work.",
		}
	}

	// Check OCI CLI version
	cmd := exec.Command("oci", "--version")
	output, err := cmd.Output()
	if err != nil {
		return checkResult{
			name:    "OCI CLI",
			status:  "warning",
			message: "Installed but could not determine version",
		}
	}

	return checkResult{
		name:    "OCI CLI",
		status:  "ok",
		message: fmt.Sprintf("Installed (%s)", string(output)[:len(output)-1]),
	}
}

func checkOCIConnectivity() checkResult {
	log.Info().Msg("Testing OCI connectivity...")

	configPath := utils.DefaultOCIConfigPath()
	ociClient, err := client.NewOCIClientWithProfile(configPath, "DEFAULT")
	if err != nil {
		return checkResult{
			name:    "OCI Connectivity",
			status:  "error",
			message: fmt.Sprintf("Failed to create client: %v", err),
		}
	}

	// Try to get namespace as a connectivity test
	ctx := context.Background()
	_, err = ociClient.GetNamespace(ctx, "")
	if err != nil {
		return checkResult{
			name:    "OCI Connectivity",
			status:  "error",
			message: fmt.Sprintf("Failed to connect: %v", err),
		}
	}

	return checkResult{
		name:    "OCI Connectivity",
		status:  "ok",
		message: "Successfully connected to OCI",
	}
}

func checkClustersConfig() checkResult {
	cfg, err := config.ReadConfig(GetConfigFile())
	if err != nil {
		return checkResult{
			name:    "Clusters",
			status:  "warning",
			message: "Could not read config to check clusters",
		}
	}

	if len(cfg.Clusters) == 0 {
		return checkResult{
			name:    "Clusters",
			status:  "warning",
			message: "No clusters configured. Run 'tunatap setup' to add clusters.",
		}
	}

	// Check each cluster for required fields
	issues := []string{}
	for _, c := range cfg.Clusters {
		if c.Region == "" {
			issues = append(issues, fmt.Sprintf("%s: missing region", c.ClusterName))
		}
		if len(c.Endpoints) == 0 && c.Ocid == nil {
			issues = append(issues, fmt.Sprintf("%s: no endpoints and no OCID configured", c.ClusterName))
		}
	}

	if len(issues) > 0 {
		return checkResult{
			name:    "Clusters",
			status:  "warning",
			message: fmt.Sprintf("%d configured, %d with issues", len(cfg.Clusters), len(issues)),
		}
	}

	return checkResult{
		name:    "Clusters",
		status:  "ok",
		message: fmt.Sprintf("%d clusters configured", len(cfg.Clusters)),
	}
}
