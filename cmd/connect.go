package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/koki-develop/go-fzf"
	"github.com/rs/zerolog/log"
	"github.com/scotttball/tunatap/internal/bastion"
	"github.com/scotttball/tunatap/internal/client"
	"github.com/scotttball/tunatap/internal/cluster"
	"github.com/scotttball/tunatap/internal/config"
	"github.com/scotttball/tunatap/internal/preflight"
	"github.com/scotttball/tunatap/pkg/utils"
	"github.com/spf13/cobra"
)

var (
	clusterName     string
	localPort       int
	bastionName     string
	endpointName    string
	noBastion       bool
	connectPreflight bool
	skipPreflight   bool
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
}

func runConnect(cmd *cobra.Command, args []string) error {
	// Handle cluster name from args
	if len(args) > 0 {
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

	// Select or find cluster
	selectedCluster, err := selectCluster(cfg, clusterName)
	if err != nil {
		return err
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

	// Create OCI client
	ociClient, err := createOCIClient(cfg, selectedCluster.Region)
	if err != nil {
		return fmt.Errorf("failed to create OCI client: %w", err)
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

	// Start the tunnel
	if useBastion {
		return bastion.TunnelThroughBastion(ctx, ociClient, cfg, selectedCluster, endpoint)
	}

	// Direct connection without bastion (for future use)
	return fmt.Errorf("direct connection without bastion not yet implemented")
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
