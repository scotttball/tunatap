package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/scotttball/tunatap/internal/bastion"
	"github.com/scotttball/tunatap/internal/client"
	"github.com/scotttball/tunatap/internal/cluster"
	"github.com/scotttball/tunatap/internal/config"
	"github.com/scotttball/tunatap/internal/discovery"
	"github.com/scotttball/tunatap/internal/kubeconfig"
	"github.com/scotttball/tunatap/pkg/utils"
	"github.com/spf13/cobra"
)

var (
	execClusterName  string
	execEndpointName string
	execBastionName  string
	execNoOCIAuth    bool
	execOCIProfile   string
	execRegionHint   string
	execNoCache      bool
)

var execCmd = &cobra.Command{
	Use:   "exec [cluster] -- <command> [args...]",
	Short: "Run a command with tunnel and kubeconfig configured",
	Long: `Start a tunnel to the specified cluster and run a command with KUBECONFIG set.

The tunnel is established, a temporary kubeconfig is generated pointing to
localhost:<port>, and the command is executed. When the command exits,
the tunnel is torn down and the temporary kubeconfig is cleaned up.

Examples:
  tunatap exec my-cluster -- kubectl get nodes
  tunatap exec my-cluster -- helm list -A
  tunatap exec -c prod -- k9s`,
	RunE:               runExec,
	Args:               cobra.MinimumNArgs(1),
	DisableFlagParsing: false,
}

func init() {
	rootCmd.AddCommand(execCmd)

	execCmd.Flags().StringVarP(&execClusterName, "cluster", "c", "", "cluster name to connect to")
	execCmd.Flags().StringVarP(&execEndpointName, "endpoint", "e", "", "endpoint name (e.g., 'private', 'public')")
	execCmd.Flags().StringVarP(&execBastionName, "bastion", "b", "", "bastion name to use")
	execCmd.Flags().BoolVar(&execNoOCIAuth, "no-oci-auth", false, "disable OCI exec-auth in kubeconfig (use insecure mode)")
	execCmd.Flags().StringVar(&execOCIProfile, "oci-profile", "", "OCI config profile for exec-auth (overrides config)")
	execCmd.Flags().StringVarP(&execRegionHint, "region", "r", "", "region hint for cluster discovery (optional)")
	execCmd.Flags().BoolVar(&execNoCache, "no-cache", false, "skip cache and force fresh discovery")
}

func runExec(cmd *cobra.Command, args []string) error {
	// Parse args to find cluster name and command
	clusterArg := ""
	commandArgs := args

	// Check if first arg is cluster name (before --)
	if len(args) > 0 && args[0] != "--" && execClusterName == "" {
		clusterArg = args[0]
		commandArgs = args[1:]
	}

	// Remove leading "--" if present
	if len(commandArgs) > 0 && commandArgs[0] == "--" {
		commandArgs = commandArgs[1:]
	}

	if len(commandArgs) == 0 {
		return fmt.Errorf("no command specified")
	}

	// Try to load configuration (non-fatal if missing for zero-touch mode)
	cfg, cfgErr := config.ReadConfig(GetConfigFile())
	if cfgErr != nil {
		// Use default config for zero-touch mode
		log.Debug().Msg("No config file found, using zero-touch mode")
		cfg = config.DefaultConfig()
	} else {
		// Configure globals from config
		if err := config.ConfigureGlobals(cfg); err != nil {
			return fmt.Errorf("failed to configure globals: %w", err)
		}
	}

	// Determine cluster name
	clusterToUse := execClusterName
	if clusterToUse == "" {
		clusterToUse = clusterArg
	}

	var selectedCluster *config.Cluster
	var ociClient *client.OCIClient
	var err error

	// Try to find cluster in config first (if we have a config)
	if clusterToUse != "" && cfgErr == nil && !cfg.SkipDiscovery {
		selectedCluster = config.FindClusterByName(cfg, clusterToUse)
	}

	// If not found in config, try discovery
	if selectedCluster == nil && clusterToUse != "" {
		log.Info().Msgf("Cluster '%s' not found in config, attempting discovery...", clusterToUse)

		// Create OCI client with auto-detection for discovery
		ociClient, err = createOCIClientForDiscovery(cfg)
		if err != nil {
			return fmt.Errorf("failed to create OCI client: %w", err)
		}

		// Initialize cache
		var cache *discovery.Cache
		if !execNoCache {
			ttl := time.Duration(cfg.GetCacheTTLHours()) * time.Hour
			cache, _ = discovery.NewCache(utils.DefaultTunatapDir(), ttl)
		}

		// Perform discovery
		discoverer := discovery.NewDiscoverer(ociClient, cache)
		hints := &discovery.DiscoveryHints{Region: execRegionHint}

		discovered, err := discoverer.DiscoverClusterWithHints(cmd.Context(), clusterToUse, hints)
		if err != nil {
			if errors.Is(err, discovery.ErrMultipleClustersFound) {
				return err
			}
			return fmt.Errorf("discovery failed: %w", err)
		}

		// Discover bastion
		bastionInfo, err := discoverer.DiscoverBastion(cmd.Context(), discovered)
		if err != nil {
			return fmt.Errorf("failed to discover bastion: %w", err)
		}

		// Convert to config.Cluster
		selectedCluster, err = discoverer.ResolveToConfig(discovered, bastionInfo)
		if err != nil {
			return fmt.Errorf("failed to resolve cluster config: %w", err)
		}

		// Set region on OCI client
		ociClient.SetRegion(discovered.Region)
	} else if selectedCluster == nil {
		// Interactive selection from config (or error if no clusters)
		selectedCluster, err = selectCluster(cfg, clusterToUse)
		if err != nil {
			return err
		}
	}

	// Override bastion if specified
	if execBastionName != "" {
		selectedCluster.Bastion = &execBastionName
	}

	// Get endpoint
	endpoint := config.GetClusterEndpoint(selectedCluster, execEndpointName)
	if endpoint == nil {
		return fmt.Errorf("no endpoints configured for cluster '%s'", selectedCluster.ClusterName)
	}

	log.Info().Msgf("Connecting to cluster: %s", selectedCluster.ClusterName)

	// Create OCI client if not already created (for config-based flow)
	if ociClient == nil {
		ociClient, err = createOCIClient(cfg, selectedCluster.Region)
		if err != nil {
			return fmt.Errorf("failed to create OCI client: %w", err)
		}
	}

	// Validate cluster with auto port allocation
	if err := cluster.ValidateAndUpdateCluster(cmd.Context(), ociClient, selectedCluster, true, 0); err != nil {
		return fmt.Errorf("failed to validate cluster: %w", err)
	}

	// Create context with cancellation
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start tunnel in background
	tunnelErr := make(chan error, 1)
	tunnelReady := make(chan int, 1)

	go func() {
		err := bastion.TunnelThroughBastionWithCallback(ctx, ociClient, cfg, selectedCluster, endpoint, func(port int) {
			tunnelReady <- port
		})
		tunnelErr <- err
	}()

	// Wait for tunnel to be ready or error
	var actualPort int
	select {
	case actualPort = <-tunnelReady:
		log.Info().Msgf("Tunnel ready on port %d", actualPort)
	case err := <-tunnelErr:
		return fmt.Errorf("tunnel failed to start: %w", err)
	case <-sigChan:
		cancel()
		return fmt.Errorf("interrupted")
	}

	// Create temporary kubeconfig
	kubeconfigPath, err := createTempKubeconfig(cfg, selectedCluster, actualPort, execNoOCIAuth, execOCIProfile)
	if err != nil {
		cancel()
		return fmt.Errorf("failed to create kubeconfig: %w", err)
	}
	defer os.Remove(kubeconfigPath)

	log.Info().Msgf("Created temporary kubeconfig: %s", kubeconfigPath)
	log.Info().Msgf("Running: %v", commandArgs)

	// Execute command
	execCommand := exec.CommandContext(ctx, commandArgs[0], commandArgs[1:]...)
	execCommand.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	execCommand.Stdin = os.Stdin
	execCommand.Stdout = os.Stdout
	execCommand.Stderr = os.Stderr

	// Run command and wait
	cmdErr := execCommand.Run()

	// Cancel tunnel
	cancel()

	// Wait for tunnel to close
	<-tunnelErr

	if cmdErr != nil {
		if exitErr, ok := cmdErr.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return cmdErr
	}

	return nil
}

// createTempKubeconfig creates a temporary kubeconfig file for the cluster.
// If the cluster has an OCID and OCI auth is not disabled, it uses OCI exec-auth
// so kubectl can get short-lived tokens automatically via the OCI CLI.
func createTempKubeconfig(cfg *config.Config, cluster *config.Cluster, port int, noOCIAuth bool, profileOverride string) (string, error) {
	var kubecfg *kubeconfig.Kubeconfig

	// Determine OCI profile to use
	profile := profileOverride
	if profile == "" {
		profile = cfg.OCIProfile
	}

	// Use OCI exec-auth if cluster has OCID and OCI auth is not disabled
	if cluster.Ocid != nil && *cluster.Ocid != "" && !noOCIAuth {
		log.Debug().Msg("Using OCI exec-auth for kubeconfig (kubectl will get tokens via OCI CLI)")
		kubecfg = kubeconfig.NewOCIKubeconfigForTunnel(
			cluster.ClusterName,
			*cluster.Ocid,
			cluster.Region,
			port,
			profile,
		)
	} else {
		// Fall back to simple insecure kubeconfig
		log.Debug().Msg("Using insecure kubeconfig (no OCI exec-auth)")
		kubecfg = kubeconfig.NewInsecureKubeconfig(cluster.ClusterName, port)
	}

	// Create temp file
	tempDir := os.TempDir()
	kubeconfigPath := filepath.Join(tempDir, fmt.Sprintf("tunatap-kubeconfig-%s-%d.yaml", cluster.ClusterName, port))

	if err := kubecfg.WriteToFile(kubeconfigPath); err != nil {
		return "", err
	}

	return kubeconfigPath, nil
}
