package cmd

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/scotttball/tunatap/internal/config"
	"github.com/scotttball/tunatap/internal/preflight"
	"github.com/spf13/cobra"
)

var (
	preflightCluster string
	preflightVerbose bool
	preflightTimeout int
)

var preflightCmd = &cobra.Command{
	Use:   "preflight [cluster]",
	Short: "Run preflight checks before connecting to a cluster",
	Long: `Run comprehensive preflight checks to verify connectivity and permissions.

Preflight checks include:
  - OCI authentication verification
  - OCI CLI availability
  - Bastion service health and accessibility
  - IAM permissions for bastion operations
  - Cluster access permissions
  - SSH agent availability
  - Network connectivity to bastion endpoint

Examples:
  # Check a specific cluster
  tunatap preflight my-cluster

  # Verbose output with suggestions
  tunatap preflight my-cluster -v

  # Specify timeout for network checks
  tunatap preflight my-cluster --timeout 15`,
	RunE: runPreflight,
	Args: cobra.MaximumNArgs(1),
}

func init() {
	rootCmd.AddCommand(preflightCmd)

	preflightCmd.Flags().StringVarP(&preflightCluster, "cluster", "c", "", "cluster name to check")
	preflightCmd.Flags().BoolVarP(&preflightVerbose, "verbose", "v", false, "show detailed output with suggestions")
	preflightCmd.Flags().IntVar(&preflightTimeout, "timeout", 10, "timeout in seconds for network checks")
}

func runPreflight(cmd *cobra.Command, args []string) error {
	// Determine cluster name
	clusterName := preflightCluster
	if clusterName == "" && len(args) > 0 {
		clusterName = args[0]
	}

	// Load configuration
	cfg, err := config.ReadConfig(GetConfigFile())
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	// Configure globals
	if err := config.ConfigureGlobals(cfg); err != nil {
		return fmt.Errorf("failed to configure globals: %w", err)
	}

	// Select cluster
	selectedCluster, err := selectCluster(cfg, clusterName)
	if err != nil {
		return err
	}

	fmt.Printf("Running preflight checks for cluster '%s'...\n", selectedCluster.ClusterName)

	// Create OCI client
	ociClient, err := createOCIClient(cfg, selectedCluster.Region)
	if err != nil {
		log.Warn().Err(err).Msg("Could not create OCI client - some checks will be skipped")
	}

	// Set up preflight options
	opts := &preflight.CheckOptions{
		Config:    cfg,
		Cluster:   selectedCluster,
		OCIClient: ociClient,
		Verbose:   preflightVerbose,
		Timeout:   time.Duration(preflightTimeout) * time.Second,
	}

	// Run all preflight checks
	checker := preflight.NewChecker(opts)
	results := checker.RunAll(cmd.Context())

	// Print results
	preflight.PrintResults(results, preflightVerbose)

	// Summary
	errorCount := 0
	warningCount := 0
	for _, r := range results {
		switch r.Status {
		case preflight.StatusError:
			errorCount++
		case preflight.StatusWarning:
			warningCount++
		}
	}

	fmt.Println("Summary:")
	fmt.Printf("  Total checks: %d\n", len(results))
	if errorCount > 0 {
		fmt.Printf("  ✗ Errors: %d\n", errorCount)
	}
	if warningCount > 0 {
		fmt.Printf("  ⚠ Warnings: %d\n", warningCount)
	}
	if errorCount == 0 && warningCount == 0 {
		fmt.Println("  ✓ All checks passed!")
	}

	// Show auto-fixable issues
	fixable := preflight.GetAutoFixable(results)
	if len(fixable) > 0 && preflightVerbose {
		fmt.Println("\nAuto-fixable issues:")
		for _, r := range fixable {
			fmt.Printf("  - %s: %s\n", r.Name, r.Message)
		}
		fmt.Println("Run 'tunatap doctor --auto-fix' to attempt automatic fixes")
	}

	if errorCount > 0 {
		return fmt.Errorf("preflight checks failed with %d error(s)", errorCount)
	}

	return nil
}
