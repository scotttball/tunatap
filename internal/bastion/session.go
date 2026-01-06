package bastion

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/oracle/oci-go-sdk/v65/bastion"
	"github.com/rs/zerolog/log"
	"github.com/scotttball/tunatap/internal/client"
	"github.com/scotttball/tunatap/internal/config"
	"github.com/scotttball/tunatap/internal/sshkeys"
	"github.com/scotttball/tunatap/internal/tunnel"
	"golang.org/x/crypto/ssh"
)

const (
	sessionMaxTTLHours     = 3
	sessionCheckBuffer     = 10 * time.Minute
	sessionRefreshBuffer   = 5 * time.Minute  // Start refresh 5 minutes before expiration
	sessionRefreshInterval = 30 * time.Second // How often to check for refresh
)

// SessionManager manages bastion sessions.
type SessionManager struct {
	ociClient *client.OCIClient
	config    *config.Config

	// Current session tracking
	currentSession    *bastion.Session
	sessionExpiration time.Time
	mu                sync.RWMutex

	// Ephemeral key support
	ephemeralKeyPair *sshkeys.EphemeralKeyPair
	useEphemeralKeys bool
}

// NewSessionManager creates a new session manager.
func NewSessionManager(ociClient *client.OCIClient, cfg *config.Config) *SessionManager {
	// Use ephemeral keys if no SSH key file is configured or if explicitly enabled
	useEphemeral := cfg.SshPrivateKeyFile == "" || cfg.UseEphemeralKeys
	return &SessionManager{
		ociClient:        ociClient,
		config:           cfg,
		useEphemeralKeys: useEphemeral,
	}
}

// NewSessionManagerWithEphemeralKeys creates a session manager that uses ephemeral keys.
func NewSessionManagerWithEphemeralKeys(ociClient *client.OCIClient, cfg *config.Config) *SessionManager {
	return &SessionManager{
		ociClient:        ociClient,
		config:           cfg,
		useEphemeralKeys: true,
	}
}

// GetEphemeralSigner returns the ephemeral signer if ephemeral keys are being used.
// Returns nil if ephemeral keys are not in use.
func (m *SessionManager) GetEphemeralSigner() ssh.Signer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.ephemeralKeyPair != nil {
		return m.ephemeralKeyPair.Signer()
	}
	return nil
}

// IsUsingEphemeralKeys returns true if the session manager is using ephemeral keys.
func (m *SessionManager) IsUsingEphemeralKeys() bool {
	return m.useEphemeralKeys
}

// GetCurrentSessionID returns the current session ID if available.
func (m *SessionManager) GetCurrentSessionID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.currentSession != nil && m.currentSession.Id != nil {
		return *m.currentSession.Id
	}
	return ""
}

// NeedsRefresh checks if the current session needs to be refreshed.
func (m *SessionManager) NeedsRefresh() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.currentSession == nil {
		return true
	}

	// Check if we're within the refresh buffer
	timeUntilExpiration := time.Until(m.sessionExpiration)
	return timeUntilExpiration <= sessionRefreshBuffer
}

// GetTimeUntilRefresh returns the duration until a session refresh is needed.
func (m *SessionManager) GetTimeUntilRefresh() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.currentSession == nil {
		return 0
	}

	timeUntilExpiration := time.Until(m.sessionExpiration)
	timeUntilRefresh := timeUntilExpiration - sessionRefreshBuffer

	if timeUntilRefresh < 0 {
		return 0
	}
	return timeUntilRefresh
}

// StartAutoRefresh starts a goroutine that automatically refreshes the session.
// Returns a channel that receives the new session ID when refreshed.
func (m *SessionManager) StartAutoRefresh(ctx context.Context, cluster *config.Cluster, endpoint *config.ClusterEndpoint) <-chan string {
	refreshChan := make(chan string, 1)

	go func() {
		ticker := time.NewTicker(sessionRefreshInterval)
		defer ticker.Stop()
		defer close(refreshChan)

		for {
			select {
			case <-ctx.Done():
				log.Debug().Msg("Auto-refresh stopped due to context cancellation")
				return
			case <-ticker.C:
				if m.NeedsRefresh() {
					log.Info().Msg("Session needs refresh, creating new session...")

					// Create new session in the background
					session, err := m.refreshSession(ctx, cluster, endpoint)
					if err != nil {
						log.Error().Err(err).Msg("Failed to refresh session")
						continue
					}

					log.Info().Msgf("Session refreshed: %s", *session.Id)

					// Notify of new session
					select {
					case refreshChan <- *session.Id:
					default:
						// Channel full, skip notification
					}
				}
			}
		}
	}()

	return refreshChan
}

// refreshSession creates a new session while keeping the old one active.
func (m *SessionManager) refreshSession(ctx context.Context, cluster *config.Cluster, endpoint *config.ClusterEndpoint) (*bastion.Session, error) {
	session, err := m.createSession(ctx, cluster, endpoint)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.currentSession = session
	if session.TimeCreated != nil {
		m.sessionExpiration = session.TimeCreated.Time.Add(time.Duration(sessionMaxTTLHours) * time.Hour)
	}
	m.mu.Unlock()

	return session, nil
}

// GetOrCreateSession gets an existing active session or creates a new one.
func (m *SessionManager) GetOrCreateSession(ctx context.Context, cluster *config.Cluster, endpoint *config.ClusterEndpoint) (*bastion.Session, error) {
	if cluster.BastionId == nil {
		return nil, fmt.Errorf("bastion ID not set for cluster")
	}

	// List existing sessions
	sessions, err := m.ociClient.ListSessions(ctx, *cluster.BastionId)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	// Find an active session for our target
	targetIP := endpoint.Ip
	for _, session := range sessions {
		if session.LifecycleState == bastion.SessionLifecycleStateActive {
			// Check if this session targets our endpoint
			if m.sessionMatchesTarget(session, targetIP, endpoint.Port) {
				log.Info().Msgf("Found existing active session: %s", *session.Id)

				// Get full session details
				fullSession, err := m.ociClient.GetSession(ctx, *cluster.BastionId, *session.Id)
				if err != nil {
					log.Warn().Err(err).Msg("Failed to get session details, will create new")
					continue
				}

				// Check if session has enough time remaining
				if m.sessionHasTimeRemaining(fullSession) {
					// Track this session
					m.trackSession(fullSession)
					return fullSession, nil
				}
				log.Info().Msg("Session expiring soon, will create new")
			}
		}
	}

	// Create new session
	session, err := m.createSession(ctx, cluster, endpoint)
	if err != nil {
		return nil, err
	}

	// Track the new session
	m.trackSession(session)
	return session, nil
}

// trackSession updates the session tracking information.
func (m *SessionManager) trackSession(session *bastion.Session) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.currentSession = session
	if session.TimeCreated != nil {
		m.sessionExpiration = session.TimeCreated.Time.Add(time.Duration(sessionMaxTTLHours) * time.Hour)
		log.Debug().Msgf("Session expires at: %s (in %s)",
			m.sessionExpiration.Format(time.RFC3339),
			time.Until(m.sessionExpiration).Round(time.Minute))
	}
}

// sessionMatchesTarget checks if a session targets the given IP and port.
func (m *SessionManager) sessionMatchesTarget(session bastion.SessionSummary, targetIP string, targetPort int) bool {
	if session.TargetResourceDetails == nil {
		return false
	}

	switch details := session.TargetResourceDetails.(type) {
	case bastion.PortForwardingSessionTargetResourceDetails:
		if details.TargetResourcePrivateIpAddress != nil &&
			*details.TargetResourcePrivateIpAddress == targetIP &&
			details.TargetResourcePort != nil &&
			*details.TargetResourcePort == targetPort {
			return true
		}
	}

	return false
}

// sessionHasTimeRemaining checks if a session has enough time before expiration.
func (m *SessionManager) sessionHasTimeRemaining(session *bastion.Session) bool {
	if session.TimeCreated == nil {
		return false
	}

	expirationTime := session.TimeCreated.Time.Add(time.Duration(sessionMaxTTLHours) * time.Hour)
	remainingTime := time.Until(expirationTime)

	return remainingTime > sessionCheckBuffer
}

// createSession creates a new bastion session.
func (m *SessionManager) createSession(ctx context.Context, cluster *config.Cluster, endpoint *config.ClusterEndpoint) (*bastion.Session, error) {
	log.Info().Msgf("Creating new bastion session for %s:%d", endpoint.Ip, endpoint.Port)

	var publicKey string
	var err error

	// Use ephemeral keys if configured
	if m.useEphemeralKeys {
		log.Info().Msg("Using ephemeral SSH keys (in-memory, never written to disk)")
		keyPair, keyErr := sshkeys.GenerateEphemeralKeyPair()
		if keyErr != nil {
			return nil, fmt.Errorf("failed to generate ephemeral keys: %w", keyErr)
		}
		publicKey = keyPair.PublicKeyString()

		// Store the key pair for use in SSH connections
		m.mu.Lock()
		m.ephemeralKeyPair = keyPair
		m.mu.Unlock()
	} else {
		// Use traditional key file
		publicKey, err = m.getPublicKey()
		if err != nil {
			return nil, fmt.Errorf("failed to read public key: %w", err)
		}
	}

	sessionTTL := sessionMaxTTLHours * 3600 // Convert to seconds

	targetIP := endpoint.Ip
	targetPort := endpoint.Port

	sessionDetails := bastion.CreateSessionDetails{
		BastionId: cluster.BastionId,
		TargetResourceDetails: bastion.CreatePortForwardingSessionTargetResourceDetails{
			TargetResourcePrivateIpAddress: &targetIP,
			TargetResourcePort:             &targetPort,
		},
		KeyDetails: &bastion.PublicKeyDetails{
			PublicKeyContent: &publicKey,
		},
		DisplayName:         stringPtr(fmt.Sprintf("tunatap-%s-%d", endpoint.Ip, endpoint.Port)),
		SessionTtlInSeconds: &sessionTTL,
	}

	session, err := m.ociClient.CreateSession(ctx, *cluster.BastionId, sessionDetails)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	log.Info().Msgf("Session created: %s, waiting for active state...", *session.Id)

	// Wait for session to become active
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	return m.ociClient.WaitForSessionActive(ctx, *cluster.BastionId, *session.Id)
}

// getPublicKey reads the public key from SSH agent or the configured private key file.
func (m *SessionManager) getPublicKey() (string, error) {
	// Try SSH agent first if available
	if tunnel.SSHAgentAvailable() {
		signers, err := tunnel.GetSSHAgentSigners()
		if err == nil && len(signers) > 0 {
			log.Debug().Msg("Using public key from SSH agent")
			publicKey := ssh.MarshalAuthorizedKey(signers[0].PublicKey())
			return string(publicKey), nil
		}
		log.Debug().Msg("SSH agent available but no keys, falling back to key file")
	}

	keyPath := m.config.SshPrivateKeyFile
	if keyPath == "" {
		keyPath = "~/.ssh/id_rsa"
	}

	// Replace ~ with home directory
	if len(keyPath) > 0 && keyPath[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		keyPath = home + keyPath[1:]
	}

	// Try to read the public key file
	pubKeyPath := keyPath + ".pub"
	pubKeyData, err := os.ReadFile(pubKeyPath)
	if err == nil {
		return string(pubKeyData), nil
	}

	// If public key file doesn't exist, derive from private key
	privateKeyData, err := os.ReadFile(keyPath)
	if err != nil {
		return "", fmt.Errorf("failed to read private key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey(privateKeyData)
	if err != nil {
		return "", fmt.Errorf("failed to parse private key: %w", err)
	}

	publicKey := ssh.MarshalAuthorizedKey(signer.PublicKey())
	return string(publicKey), nil
}

// UpdateBastionConnection updates session and SSH config for a connection.
func UpdateBastionConnection(
	sessionID *string,
	sshConfig *ssh.ClientConfig,
	ociClient *client.OCIClient,
	cfg *config.Config,
	cluster *config.Cluster,
	endpoint *config.ClusterEndpoint,
) error {
	return UpdateBastionConnectionWithManager(sessionID, sshConfig, nil, ociClient, cfg, cluster, endpoint)
}

// UpdateBastionConnectionWithManager updates session and SSH config using a provided session manager.
// If manager is nil, a new one is created.
func UpdateBastionConnectionWithManager(
	sessionID *string,
	sshConfig *ssh.ClientConfig,
	manager *SessionManager,
	ociClient *client.OCIClient,
	cfg *config.Config,
	cluster *config.Cluster,
	endpoint *config.ClusterEndpoint,
) error {
	ctx := context.Background()

	if manager == nil {
		manager = NewSessionManager(ociClient, cfg)
	}

	session, err := manager.GetOrCreateSession(ctx, cluster, endpoint)
	if err != nil {
		return fmt.Errorf("failed to get or create session: %w", err)
	}

	*sessionID = *session.Id

	var newConfig *ssh.ClientConfig

	// Use ephemeral signer if available
	if signer := manager.GetEphemeralSigner(); signer != nil {
		log.Debug().Msg("Using ephemeral key for SSH authentication")
		newConfig, err = tunnel.CreateSSHClientConfigWithSigner(*sessionID, signer)
	} else {
		// Fall back to SSH agent and key file
		newConfig, err = tunnel.CreateSSHClientConfigWithAgent(*sessionID, cfg.SshPrivateKeyFile)
	}

	if err != nil {
		return fmt.Errorf("failed to create SSH config: %w", err)
	}

	*sshConfig = *newConfig
	return nil
}

func stringPtr(s string) *string {
	return &s
}
