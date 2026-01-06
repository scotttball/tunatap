package bastion

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/scotttball/tunatap/internal/audit"
	"github.com/scotttball/tunatap/internal/client"
	"github.com/scotttball/tunatap/internal/config"
	"github.com/scotttball/tunatap/internal/health"
	"github.com/scotttball/tunatap/internal/tunnel"
	"github.com/scotttball/tunatap/pkg/utils"
	"golang.org/x/crypto/ssh"
)

// TunnelOptions configures optional features for the tunnel.
type TunnelOptions struct {
	// AuditLogger logs tunnel connect/disconnect events
	AuditLogger *audit.Logger
	// OnReady is called when the tunnel is ready with the actual port
	OnReady ReadyCallback
}

// bastionBackoffConfig returns the backoff configuration for bastion retries.
func bastionBackoffConfig() *utils.BackoffConfig {
	return &utils.BackoffConfig{
		InitialInterval: 5 * time.Second,
		MaxInterval:     2 * time.Minute,
		Multiplier:      1.5,
		JitterFactor:    0.3,
		MaxAttempts:     15,
	}
}

// ReadyCallback is called when the tunnel is ready with the actual port.
type ReadyCallback func(port int)

// TunnelThroughBastion establishes an SSH tunnel through a bastion service.
func TunnelThroughBastion(ctx context.Context, ociClient *client.OCIClient, cfg *config.Config, cluster *config.Cluster, endpoint *config.ClusterEndpoint) error {
	return TunnelThroughBastionWithOptions(ctx, ociClient, cfg, cluster, endpoint, nil)
}

// TunnelThroughBastionWithCallback establishes an SSH tunnel and calls the callback when ready.
func TunnelThroughBastionWithCallback(ctx context.Context, ociClient *client.OCIClient, cfg *config.Config, cluster *config.Cluster, endpoint *config.ClusterEndpoint, onReady ReadyCallback) error {
	return TunnelThroughBastionWithOptions(ctx, ociClient, cfg, cluster, endpoint, &TunnelOptions{OnReady: onReady})
}

// TunnelThroughBastionWithOptions establishes an SSH tunnel with full options.
func TunnelThroughBastionWithOptions(ctx context.Context, ociClient *client.OCIClient, cfg *config.Config, cluster *config.Cluster, endpoint *config.ClusterEndpoint, opts *TunnelOptions) error {
	if opts == nil {
		opts = &TunnelOptions{}
	}

	backoff := utils.NewBackoff(bastionBackoffConfig())

	// Default bastion type to STANDARD if not set
	bastionType := "STANDARD"
	if cluster.BastionType != nil {
		bastionType = *cluster.BastionType
	}

	// Generate a session ID for audit/health tracking
	sessionID := fmt.Sprintf("%d-%d", time.Now().UnixNano(), os.Getpid())

	// Prepare audit session info (but don't start until tunnel is up)
	bastionID := ""
	if cluster.BastionId != nil {
		bastionID = *cluster.BastionId
	}
	auditSession := &audit.Session{
		ID:          sessionID,
		ClusterName: cluster.ClusterName,
		Region:      cluster.Region,
		LocalPort:   *cluster.LocalPort,
		RemoteHost:  endpoint.Ip,
		RemotePort:  endpoint.Port,
		BastionID:   bastionID,
	}

	// Register with health registry (starts unhealthy)
	healthRegistry := health.GetRegistry()
	tunnelStatus := &health.TunnelStatus{
		ID:         sessionID,
		Cluster:    cluster.ClusterName,
		Region:     cluster.Region,
		LocalPort:  *cluster.LocalPort,
		RemoteHost: endpoint.Ip,
		RemotePort: endpoint.Port,
		Healthy:    false, // Will be set to true once tunnel is ready
	}
	healthRegistry.Register(tunnelStatus)

	// Track whether tunnel was ever healthy (for audit logging)
	var tunnelWasHealthy bool
	var lastError error

	// Ensure cleanup on exit
	defer func() {
		healthRegistry.Deregister(sessionID)
		// Only log audit disconnect if tunnel was ever connected
		if opts.AuditLogger != nil && tunnelWasHealthy {
			errMsg := ""
			if lastError != nil {
				errMsg = lastError.Error()
			}
			if err := opts.AuditLogger.EndSession(sessionID, errMsg); err != nil {
				log.Warn().Err(err).Msg("Failed to end audit session")
			}
		}
	}()

	for {
		log.Debug().Msgf("Connection attempt %d", backoff.Attempt()+1)

		var err error
		if bastionType == "INTERNAL" {
			err = handleInternalBastionWithOptions(ctx, cluster, endpoint, sessionID, opts, healthRegistry, auditSession, &tunnelWasHealthy)
		} else {
			err = handleStandardBastionWithOptions(ctx, ociClient, cfg, cluster, endpoint, sessionID, opts, healthRegistry, auditSession, &tunnelWasHealthy)
		}

		if err == nil {
			return nil
		}

		// Track the error for audit logging
		lastError = err

		// Update health status on error
		healthRegistry.UpdateHealth(sessionID, false, err.Error())

		log.Error().Err(err).Msgf("%s bastion tunnel failed", bastionType)

		// Check for context cancellation before sleeping
		select {
		case <-ctx.Done():
			lastError = ctx.Err()
			return ctx.Err()
		default:
		}

		// Get next backoff duration
		duration, shouldRetry := backoff.Next()
		if !shouldRetry {
			lastError = fmt.Errorf("max retry attempts (%d) exceeded: %w", backoff.Attempt(), err)
			return lastError
		}

		log.Info().Msgf("Retrying in %s (attempt %d/%d)",
			duration.Round(time.Millisecond),
			backoff.Attempt(),
			bastionBackoffConfig().MaxAttempts)

		// Sleep with context awareness
		select {
		case <-ctx.Done():
			lastError = ctx.Err()
			return ctx.Err()
		case <-time.After(duration):
		}
	}
}

// handleInternalBastionWithOptions handles tunneling through an internal bastion with full options.
func handleInternalBastionWithOptions(ctx context.Context, cluster *config.Cluster, endpoint *config.ClusterEndpoint, sessionID string, opts *TunnelOptions, healthRegistry *health.Registry, auditSession *audit.Session, tunnelWasHealthy *bool) error {
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

	// Mark tunnel as healthy, start audit session, and call ready callback
	healthRegistry.UpdateHealth(sessionID, true, "")
	*tunnelWasHealthy = true
	if opts.AuditLogger != nil {
		if err := opts.AuditLogger.StartSession(auditSession); err != nil {
			log.Warn().Err(err).Msg("Failed to start audit session")
		}
	}
	if opts.OnReady != nil {
		opts.OnReady(*cluster.LocalPort)
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", sshCmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// handleStandardBastionWithOptions handles tunneling through a standard bastion service with full options.
func handleStandardBastionWithOptions(ctx context.Context, ociClient *client.OCIClient, cfg *config.Config, cluster *config.Cluster, endpoint *config.ClusterEndpoint, auditSessionID string, opts *TunnelOptions, healthRegistry *health.Registry, auditSession *audit.Session, tunnelWasHealthy *bool) error {
	var bastionSessionID string
	var sshConfig ssh.ClientConfig

	log.Info().Msg("Getting bastion session...")
	err := UpdateBastionConnection(ctx, &bastionSessionID, &sshConfig, ociClient, cfg, cluster, endpoint)
	if err != nil {
		return fmt.Errorf("failed to get session from Bastion: %w", err)
	}

	log.Info().Msgf("Using session: %s", bastionSessionID)

	sshCmd := GetTunnelCommand(
		cfg.SshPrivateKeyFile,
		*cluster.LocalPort,
		endpoint.Port,
		endpoint.Ip,
		bastionSessionID,
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
				if err := UpdateBastionConnection(ctx, &bastionSessionID, &sshConfig, ociClient, cfg, cluster, endpoint); err != nil {
					log.Error().Err(err).Msg("Failed to update bastion connection")
				} else if opts.AuditLogger != nil {
					// Log session refresh event (ignore errors as this is non-critical)
					_ = opts.AuditLogger.LogSessionRefresh(auditSessionID, bastionSessionID)
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
		// Tunnel is ready - mark healthy, start audit session, call callback
		healthRegistry.UpdateHealth(auditSessionID, true, "")
		*tunnelWasHealthy = true
		if opts.AuditLogger != nil {
			if err := opts.AuditLogger.StartSession(auditSession); err != nil {
				log.Warn().Err(err).Msg("Failed to start audit session")
			}
		}
		if opts.OnReady != nil {
			opts.OnReady(tun.GetActualLocalPort())
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
