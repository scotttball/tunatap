package bastion

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/scotttball/tunatap/internal/client"
	"github.com/scotttball/tunatap/internal/config"
	"github.com/scotttball/tunatap/internal/tunnel"
	"github.com/scotttball/tunatap/pkg/utils"
	"golang.org/x/crypto/ssh"
)

var (
	maxRetries         = 30
	sleepTimeInSeconds = 10
)

// incrementalSleep sleeps with exponential backoff.
func incrementalSleep(retry int) {
	sleepDuration := time.Duration(sleepTimeInSeconds*(maxRetries-retry+1)) * time.Second
	log.Info().Msgf("Waiting for %s", sleepDuration.String())
	time.Sleep(sleepDuration)
}

// ReadyCallback is called when the tunnel is ready with the actual port.
type ReadyCallback func(port int)

// TunnelThroughBastion establishes an SSH tunnel through a bastion service.
func TunnelThroughBastion(ctx context.Context, ociClient *client.OCIClient, cfg *config.Config, cluster *config.Cluster, endpoint *config.ClusterEndpoint) error {
	return TunnelThroughBastionWithCallback(ctx, ociClient, cfg, cluster, endpoint, nil)
}

// TunnelThroughBastionWithCallback establishes an SSH tunnel and calls the callback when ready.
func TunnelThroughBastionWithCallback(ctx context.Context, ociClient *client.OCIClient, cfg *config.Config, cluster *config.Cluster, endpoint *config.ClusterEndpoint, onReady ReadyCallback) error {
	retry := maxRetries

	// Default bastion type to STANDARD if not set
	bastionType := "STANDARD"
	if cluster.BastionType != nil {
		bastionType = *cluster.BastionType
	}

	for retry > 0 {
		log.Debug().Msgf("Retries left: %d", retry)

		if bastionType == "INTERNAL" {
			err := handleInternalBastionWithCallback(ctx, cluster, endpoint, onReady)
			if err != nil {
				log.Error().Err(err).Msg("Internal bastion tunnel failed")
				retry--
				incrementalSleep(retry)
				continue
			}
			return nil
		}

		err := handleStandardBastionWithCallback(ctx, ociClient, cfg, cluster, endpoint, onReady)
		if err != nil {
			log.Error().Err(err).Msg("Standard bastion tunnel failed")
			retry--
			incrementalSleep(retry)
			continue
		}
		return nil
	}

	return fmt.Errorf("too many failed attempts, giving up")
}

// handleInternalBastionWithCallback handles tunneling through an internal bastion with a ready callback.
func handleInternalBastionWithCallback(ctx context.Context, cluster *config.Cluster, endpoint *config.ClusterEndpoint, onReady ReadyCallback) error {
	log.Info().Msg("Using internal bastion service")

	if cluster.JumpBoxIP == nil {
		return fmt.Errorf("jumpbox_ip setting is required for internal bastion service")
	}

	bastionLB := fmt.Sprintf("ztb-internal.bastion.%s.oci.oracleiaas.com", cluster.Region)

	sshCmd := GetInternalTunnelCommand(
		*cluster.LocalPort,
		endpoint.Port,
		endpoint.Ip,
		*cluster.BastionId,
		*cluster.JumpBoxIP,
		cluster.Region,
		*cluster.CompartmentOcid,
		bastionLB,
	)

	log.Info().Msgf("Creating ssh tunnel. The equivalent ssh command is:\n%s\nYou can now use kubectl in another terminal", sshCmd)

	// Call ready callback with the port
	if onReady != nil {
		onReady(*cluster.LocalPort)
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", sshCmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// handleStandardBastionWithCallback handles tunneling through a standard bastion service with a ready callback.
func handleStandardBastionWithCallback(ctx context.Context, ociClient *client.OCIClient, cfg *config.Config, cluster *config.Cluster, endpoint *config.ClusterEndpoint, onReady ReadyCallback) error {
	var sessionID string
	var sshConfig ssh.ClientConfig

	log.Info().Msg("Getting bastion session...")
	err := UpdateBastionConnection(&sessionID, &sshConfig, ociClient, cfg, cluster, endpoint)
	if err != nil {
		return fmt.Errorf("failed to get session from Bastion: %w", err)
	}

	log.Info().Msgf("Using session: %s", sessionID)

	sshCmd := GetTunnelCommand(
		cfg.SshPrivateKeyFile,
		*cluster.LocalPort,
		endpoint.Port,
		endpoint.Ip,
		sessionID,
		cluster.Region,
		cfg.SshSocksProxy,
	)

	log.Info().Msgf("Creating ssh tunnel. The equivalent ssh command is:\n%s\nYou can now use kubectl in another terminal", sshCmd)

	// Start periodic session refresh
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				log.Debug().Msg("Periodic update check of bastion session...")
				if err := UpdateBastionConnection(&sessionID, &sshConfig, ociClient, cfg, cluster, endpoint); err != nil {
					log.Error().Err(err).Msg("Failed to update bastion connection")
				}
			}
		}
	}()

	// Establish SSH tunnel
	bastionAddr := GetBastionHostAddress(*cluster.BastionId, cluster.Region)
	localAddr := fmt.Sprintf("localhost:%d", *cluster.LocalPort)
	remoteTunnel := fmt.Sprintf("localhost:%d", endpoint.Port)

	tun := tunnel.NewSSHTunnel(
		localAddr,
		bastionAddr,
		&sshConfig,
		remoteTunnel,
		cfg.GetPoolSize(),
		cfg.GetWarmupCount(),
		cfg.GetMaxConcurrent(),
		cfg.SshSocksProxy,
	)

	// Start tunnel asynchronously and wait for it to be ready
	errCh := tun.StartAsync()

	// Wait for tunnel to be ready
	select {
	case <-tun.Ready:
		// Tunnel is ready, call callback with actual port
		if onReady != nil {
			onReady(tun.GetActualLocalPort())
		}
	case err := <-errCh:
		return err
	case <-ctx.Done():
		tun.Close()
		return ctx.Err()
	}

	// Wait for tunnel to complete or context cancellation
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		tun.Close()
		return ctx.Err()
	}
}

// StartTunnel is a convenience function to start a tunnel to a cluster.
func StartTunnel(ctx context.Context, configPath, clusterName string, localPort int) error {
	cfg, err := config.ReadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	cluster := config.FindClusterByName(cfg, clusterName)
	if cluster == nil {
		return fmt.Errorf("cluster '%s' not found in config", clusterName)
	}

	endpoint := config.GetClusterEndpoint(cluster, "")
	if endpoint == nil {
		return fmt.Errorf("no endpoints configured for cluster '%s'", clusterName)
	}

	// Set local port
	if localPort > 0 {
		cluster.LocalPort = &localPort
	}

	// Create OCI client
	ociClient, err := createOCIClient(cfg, cluster.Region)
	if err != nil {
		return fmt.Errorf("failed to create OCI client: %w", err)
	}

	// Configure globals
	if err := config.ConfigureGlobals(cfg); err != nil {
		return fmt.Errorf("failed to configure globals: %w", err)
	}

	return TunnelThroughBastion(ctx, ociClient, cfg, cluster, endpoint)
}

// createOCIClient creates an OCI client for the given region.
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
