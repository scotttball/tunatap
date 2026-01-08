package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/koki-develop/go-fzf"
	"github.com/rs/zerolog/log"
	"github.com/scotttball/tunatap/internal/audit"
	"github.com/scotttball/tunatap/internal/bastion"
	"github.com/scotttball/tunatap/internal/client"
	"github.com/scotttball/tunatap/internal/cluster"
	"github.com/scotttball/tunatap/internal/config"
	"github.com/scotttball/tunatap/internal/discovery"
	"github.com/scotttball/tunatap/internal/health"
	"github.com/scotttball/tunatap/internal/preflight"
	"github.com/scotttball/tunatap/internal/state"
	"github.com/scotttball/tunatap/pkg/utils"
	"github.com/spf13/cobra"
)

var (
	clusterName       string
	localPort         int
	bastionName       string
	endpointName      string
	noBastion         bool
	connectPreflight  bool
	skipPreflight     bool
	regionHint        string
	noCache           bool
	connectOCIProfile string
)

var connectCmd = &cobra.Command{
	Use:   "connect [cluster]",
	Short: "Connect to a cluster through bastion",
	Long: `Establish an SSH tunnel to a cluster through OCI Bastion service.

If no cluster name is provided, an interactive selector will be shown.`,
	RunE: runConnect,
}

func init() {
	rootCmd.AddCommand(connectCmd)

	connectCmd.Flags().StringVarP(&clusterName, "cluster", "c", "", "cluster name to connect to")
	connectCmd.Flags().IntVarP(&localPort, "port", "p", 0, "local port for the tunnel (0 or negative for auto)")
	connectCmd.Flags().StringVarP(&bastionName, "bastion", "b", "", "bastion name to use")
	connectCmd.Flags().StringVarP(&endpointName, "endpoint", "e", "", "endpoint name (e.g., 'private', 'public')")
	connectCmd.Flags().BoolVar(&noBastion, "no-bastion", false, "connect directly without bastion")
	connectCmd.Flags().BoolVar(&connectPreflight, "preflight", false, "run preflight checks before connecting")
	connectCmd.Flags().BoolVar(&skipPreflight, "skip-preflight", false, "skip quick preflight validation")
	connectCmd.Flags().StringVarP(&regionHint, "region", "r", "", "region hint for cluster discovery (optional)")
	connectCmd.Flags().BoolVar(&noCache, "no-cache", false, "skip cache and force fresh discovery")
	connectCmd.Flags().StringVar(&connectOCIProfile, "oci-profile", "", "OCI config profile to use (overrides config)")
}

func runConnect(cmd *cobra.Command, args []string) error {
	// Handle cluster name from args
	if len(args) > 0 {
		clusterName = args[0]
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

	// Override OCI profile if specified via flag
	if connectOCIProfile != "" {
		cfg.OCIProfile = connectOCIProfile
		log.Debug().Str("profile", connectOCIProfile).Msg("Using OCI profile from flag")
	}

	var selectedCluster *config.Cluster
	var ociClient *client.OCIClient
	var err error

	// Try to find cluster in config first (if we have a config)
	if clusterName != "" && cfgErr == nil && !cfg.SkipDiscovery {
		selectedCluster = config.FindClusterByName(cfg, clusterName)
	}

	// If not found in config, try discovery
	if selectedCluster == nil && clusterName != "" {
		// Create OCI client with auto-detection for discovery
		ociClient, err = createOCIClientForDiscovery(cfg)
		if err != nil {
			ociErr := client.ClassifyOCIError(err, "create OCI client")
			if ociErr.Suggestion != "" {
				return fmt.Errorf("failed to create OCI client: %s\n\n%s", ociErr.Message, ociErr.Suggestion)
			}
			return fmt.Errorf("failed to create OCI client: %w", err)
		}

		// Initialize cache
		var cache *discovery.Cache
		if !noCache {
			ttl := time.Duration(cfg.GetCacheTTLHours()) * time.Hour
			cache, _ = discovery.NewCache(utils.DefaultTunatapDir(), ttl)
		}

		discoverer := discovery.NewDiscoverer(ociClient, cache)

		var discovered *discovery.DiscoveredCluster

		// Check if the input is an OCID - use direct lookup if so
		if discovery.IsClusterOCID(clusterName) {
			log.Info().Msgf("Detected cluster OCID, performing direct lookup...")
			discovered, err = discoverer.DiscoverClusterByOCID(cmd.Context(), clusterName)
			if err != nil {
				// Error messages from DiscoverClusterByOCID are already well-formatted
				return err
			}
		} else {
			log.Info().Msgf("Cluster '%s' not found in config, attempting discovery...", clusterName)

			// Perform name-based discovery
			hints := &discovery.DiscoveryHints{Region: regionHint}
			discovered, err = discoverer.DiscoverClusterWithHints(cmd.Context(), clusterName, hints)
			if err != nil {
				// Check if multiple clusters found - offer interactive selection
				if errors.Is(err, discovery.ErrMultipleClustersFound) {
					return err // The error message already contains all matches
				}

				// Provide better error messages for common failures
				if errors.Is(err, discovery.ErrClusterNotFound) {
					return fmt.Errorf("cluster '%s' not found\n\n"+
						"To find available clusters, try:\n"+
						"  tunatap list\n\n"+
						"If the cluster exists, you may need to:\n"+
						"  - Check that you have IAM policies to list clusters\n"+
						"  - Specify the region with --region if searching is slow\n"+
						"  - Use the cluster OCID directly instead of the name", clusterName)
				}

				if errors.Is(err, discovery.ErrClusterAccessDenied) {
					return err // Already has good suggestion
				}

				// Check for auth errors
				ociErr := client.ClassifyOCIError(err, "cluster discovery")
				if ociErr.Type == client.ErrorTypeNotAuthenticated {
					return fmt.Errorf("authentication failed during discovery\n\n%s", ociErr.Suggestion)
				}

				return fmt.Errorf("discovery failed: %w", err)
			}
		}

		// Discover bastion
		bastionInfo, err := discoverer.DiscoverBastion(cmd.Context(), discovered)
		if err != nil {
			if errors.Is(err, discovery.ErrNoBastionFound) {
				return fmt.Errorf("no bastion found for cluster '%s'\n\n"+
					"A bastion is required to connect to private OKE clusters.\n"+
					"Please ensure:\n"+
					"  1. A bastion exists in the cluster's compartment\n"+
					"  2. The bastion is in ACTIVE state\n"+
					"  3. You have IAM policies to read bastions\n\n"+
					"To create a bastion, visit the OCI Console:\n"+
					"  https://cloud.oracle.com/bastion", discovered.Name)
			}

			ociErr := client.ClassifyOCIError(err, "bastion discovery")
			if ociErr.Suggestion != "" {
				return fmt.Errorf("failed to discover bastion: %s\n\n%s", ociErr.Message, ociErr.Suggestion)
			}
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
		selectedCluster, err = selectCluster(cfg, clusterName)
		if err != nil {
			return err
		}
	}

	// Override bastion if specified
	if bastionName != "" {
		selectedCluster.Bastion = &bastionName
	}

	// Get endpoint
	endpoint := config.GetClusterEndpoint(selectedCluster, endpointName)
	if endpoint == nil {
		return fmt.Errorf("no endpoints configured for cluster '%s'", selectedCluster.ClusterName)
	}

	log.Info().Msgf("Connecting to cluster: %s", selectedCluster.ClusterName)
	log.Info().Msgf("Endpoint: %s:%d", endpoint.Ip, endpoint.Port)

	// Create OCI client if not already created (for config-based flow)
	if ociClient == nil {
		ociClient, err = createOCIClient(cfg, selectedCluster.Region)
		if err != nil {
			return fmt.Errorf("failed to create OCI client: %w", err)
		}
	}

	// Validate and update cluster configuration
	useBastion := !noBastion
	if err := cluster.ValidateAndUpdateCluster(cmd.Context(), ociClient, selectedCluster, useBastion, localPort); err != nil {
		return fmt.Errorf("failed to validate cluster: %w", err)
	}

	// Run preflight checks if requested or do quick check unless skipped
	if connectPreflight {
		// Full preflight checks
		opts := &preflight.CheckOptions{
			Config:    cfg,
			Cluster:   selectedCluster,
			OCIClient: ociClient,
			Verbose:   true,
			Timeout:   10 * time.Second,
		}
		checker := preflight.NewChecker(opts)
		results := checker.RunAll(cmd.Context())
		preflight.PrintResults(results, true)

		if preflight.HasErrors(results) {
			return fmt.Errorf("preflight checks failed - fix errors before connecting")
		}
	} else if !skipPreflight {
		// Quick check - just verify bastion is healthy
		if err := preflight.RunQuickCheck(cmd.Context(), ociClient, selectedCluster); err != nil {
			log.Warn().Err(err).Msg("Quick preflight check failed (use --skip-preflight to ignore)")
		}
	}

	log.Info().Msgf("Local port: %d", *selectedCluster.LocalPort)

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Info().Msg("Received shutdown signal, closing tunnel...")
		cancel()
	}()

	// Start health server if configured
	if cfg.HealthEndpoint != "" {
		stopHealth, err := health.StartHealthServer(cfg.HealthEndpoint)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to start health server")
		} else {
			defer stopHealth()
		}
	}

	// Set up audit logging if enabled
	var auditLogger *audit.Logger
	if cfg.IsAuditLoggingEnabled() {
		// Use configured home path from state, fall back to default
		homePath := state.GetInstance().GetHomePath()
		if homePath == "" {
			homePath = utils.DefaultTunatapDir()
		}
		audit.SetHomePath(homePath)

		var err error
		auditLogger, err = audit.NewLogger(audit.DefaultLogDir())
		if err != nil {
			log.Warn().Err(err).Msg("Failed to create audit logger")
		} else {
			defer auditLogger.Close()
		}
	}

	// Start the tunnel
	if useBastion {
		opts := &bastion.TunnelOptions{
			AuditLogger: auditLogger,
		}
		return bastion.TunnelThroughBastionWithOptions(ctx, ociClient, cfg, selectedCluster, endpoint, opts)
	}

	// Direct connection without bastion (for future use)
	return fmt.Errorf("direct connection without bastion not yet implemented")
}

// createOCIClientForDiscovery creates an OCI client for discovery operations.
// Uses auto-detection of authentication without requiring config values.
func createOCIClientForDiscovery(cfg *config.Config) (*client.OCIClient, error) {
	configPath := cfg.OCIConfigPath
	if configPath == "" {
		configPath = utils.DefaultOCIConfigPath()
	}

	profile := cfg.OCIProfile
	if profile == "" {
		profile = "DEFAULT"
	}

	return client.NewOCIClientAuto(configPath, profile)
}

func selectCluster(cfg *config.Config, name string) (*config.Cluster, error) {
	if name != "" {
		c := config.FindClusterByName(cfg, name)
		if c == nil {
			return nil, fmt.Errorf("cluster '%s' not found in config", name)
		}
		return c, nil
	}

	// Interactive selection
	if len(cfg.Clusters) == 0 {
		return nil, fmt.Errorf("no clusters configured")
	}

	if len(cfg.Clusters) == 1 {
		return cfg.Clusters[0], nil
	}

	f, err := fzf.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create selector: %w", err)
	}

	idxs, err := f.Find(cfg.Clusters, func(i int) string {
		return fmt.Sprintf("%s (%s)", cfg.Clusters[i].ClusterName, cfg.Clusters[i].Region)
	})
	if err != nil || len(idxs) == 0 {
		return nil, fmt.Errorf("no cluster selected")
	}

	return cfg.Clusters[idxs[0]], nil
}

func createOCIClient(cfg *config.Config, region string) (*client.OCIClient, error) {
	// Determine auth type
	authType := client.AuthTypeAuto
	if cfg.OCIAuthType != "" {
		authType = client.AuthType(cfg.OCIAuthType)
	}

	// Determine config path and profile
	configPath := cfg.OCIConfigPath
	if configPath == "" {
		configPath = utils.DefaultOCIConfigPath()
	}

	profile := cfg.OCIProfile
	if profile == "" {
		profile = "DEFAULT"
	}

	// Create client with appropriate auth type
	ociClient, err := client.NewOCIClientWithAuthType(authType, configPath, profile)
	if err != nil {
		return nil, err
	}

	ociClient.SetRegion(region)
	return ociClient, nil
}
