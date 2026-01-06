package tunnel

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// GetHomeDir returns the user's home directory.
func GetHomeDir() string {
	userHomeDir, _ := os.UserHomeDir()
	return userHomeDir
}

// GetHostKeyFilePath returns the path to known_hosts.
func GetHostKeyFilePath() string {
	return filepath.Join(GetHomeDir(), ".ssh", "known_hosts")
}

// GetPrivateKey loads and parses a private key from file.
func GetPrivateKey(keyFilePath string) (ssh.Signer, error) {
	keyFilePath = strings.ReplaceAll(keyFilePath, "~", GetHomeDir())
	key, err := os.ReadFile(keyFilePath)
	if err != nil {
		return nil, err
	}
	return ssh.ParsePrivateKey(key)
}

// AddHostKey adds a host key to the known_hosts file.
func AddHostKey(host string, key ssh.PublicKey) error {
	khFilePath := GetHostKeyFilePath()
	f, err := os.OpenFile(khFilePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	knownHostLine := knownhosts.Line([]string{knownhosts.Normalize(host)}, key) + "\n"
	_, err = f.WriteString(knownHostLine)
	return err
}

// GetKnownHostsCallbackWithNewHost returns a host key callback that adds unknown hosts.
func GetKnownHostsCallbackWithNewHost() (ssh.HostKeyCallback, error) {
	khFilePath := GetHostKeyFilePath()

	// Ensure the known_hosts file exists
	if _, err := os.Stat(khFilePath); os.IsNotExist(err) {
		// Create the .ssh directory if it doesn't exist
		sshDir := filepath.Dir(khFilePath)
		if err := os.MkdirAll(sshDir, 0700); err != nil {
			return nil, err
		}
		// Create empty known_hosts file
		f, err := os.OpenFile(khFilePath, os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			return nil, err
		}
		f.Close()
	}

	knownHostsCallback, err := knownhosts.New(khFilePath)
	if err != nil {
		return nil, err
	}

	return func(dialAddr string, addr net.Addr, key ssh.PublicKey) error {
		err := knownHostsCallback(dialAddr, addr, key)
		if err != nil {
			var keyError *knownhosts.KeyError
			if errors.As(err, &keyError) && len(keyError.Want) == 0 {
				// Key missing; add the host key
				log.Info().Msgf("Adding new host key for %s", dialAddr)
				if addErr := AddHostKey(dialAddr, key); addErr != nil {
					return addErr
				}
				// Reload callback after adding the new host key
				knownHostsCallback, err = knownhosts.New(khFilePath)
				if err != nil {
					return err
				}
				// Revalidate with the updated known_hosts
				return knownHostsCallback(dialAddr, addr, key)
			}
			return err // Key mismatch or other errors
		}
		return nil
	}, nil
}

// CreateSSHClientConfig creates an SSH client config for bastion connections.
func CreateSSHClientConfig(username, keyFilePath string) (*ssh.ClientConfig, error) {
	customCallback, err := GetKnownHostsCallbackWithNewHost()
	if err != nil {
		return nil, err
	}

	signer, err := GetPrivateKey(keyFilePath)
	if err != nil {
		return nil, err
	}

	return &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: customCallback,
		Timeout:         0,
	}, nil
}

// CreateSSHClientConfigWithPublicKey creates an SSH config from a public key string.
func CreateSSHClientConfigWithPublicKey(username, publicKey string) (*ssh.ClientConfig, error) {
	customCallback, err := GetKnownHostsCallbackWithNewHost()
	if err != nil {
		return nil, err
	}

	// Parse the public key to get the signer
	signer, err := ssh.ParsePrivateKey([]byte(publicKey))
	if err != nil {
		return nil, err
	}

	return &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: customCallback,
		Timeout:         0,
	}, nil
}

// SSHAgentAvailable checks if SSH agent is available via SSH_AUTH_SOCK.
func SSHAgentAvailable() bool {
	socketPath := os.Getenv("SSH_AUTH_SOCK")
	return socketPath != ""
}

// GetSSHAgentAuthMethod returns an SSH auth method using the SSH agent.
func GetSSHAgentAuthMethod() (ssh.AuthMethod, error) {
	socketPath := os.Getenv("SSH_AUTH_SOCK")
	if socketPath == "" {
		return nil, fmt.Errorf("SSH_AUTH_SOCK not set")
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SSH agent: %w", err)
	}

	agentClient := agent.NewClient(conn)
	return ssh.PublicKeysCallback(agentClient.Signers), nil
}

// GetSSHAgentSigners returns signers from the SSH agent.
func GetSSHAgentSigners() ([]ssh.Signer, error) {
	socketPath := os.Getenv("SSH_AUTH_SOCK")
	if socketPath == "" {
		return nil, fmt.Errorf("SSH_AUTH_SOCK not set")
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to SSH agent: %w", err)
	}

	agentClient := agent.NewClient(conn)
	return agentClient.Signers()
}

// CreateSSHClientConfigWithAgent creates an SSH client config using SSH agent.
// Falls back to key file if agent is not available.
func CreateSSHClientConfigWithAgent(username, keyFilePath string) (*ssh.ClientConfig, error) {
	customCallback, err := GetKnownHostsCallbackWithNewHost()
	if err != nil {
		return nil, err
	}

	var authMethods []ssh.AuthMethod

	// Try SSH agent first
	if SSHAgentAvailable() {
		agentAuth, err := GetSSHAgentAuthMethod()
		if err != nil {
			log.Warn().Err(err).Msg("Failed to get SSH agent auth, will try key file")
		} else {
			log.Debug().Msg("Using SSH agent for authentication")
			authMethods = append(authMethods, agentAuth)
		}
	}

	// Add key file as fallback (if configured)
	if keyFilePath != "" {
		signer, err := GetPrivateKey(keyFilePath)
		if err != nil {
			// Only error if we have no auth methods
			if len(authMethods) == 0 {
				return nil, fmt.Errorf("no SSH agent and failed to load key: %w", err)
			}
			log.Warn().Err(err).Msg("Failed to load SSH key file, using agent only")
		} else {
			log.Debug().Msg("Adding SSH key file as authentication method")
			authMethods = append(authMethods, ssh.PublicKeys(signer))
		}
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no SSH authentication methods available")
	}

	return &ssh.ClientConfig{
		User:            username,
		Auth:            authMethods,
		HostKeyCallback: customCallback,
		Timeout:         0,
	}, nil
}

// CreateSSHClientConfigPreferAgent creates an SSH client config preferring agent over key file.
// If preferAgent is true, tries agent first. Otherwise, uses key file first.
func CreateSSHClientConfigPreferAgent(username, keyFilePath string, preferAgent bool) (*ssh.ClientConfig, error) {
	customCallback, err := GetKnownHostsCallbackWithNewHost()
	if err != nil {
		return nil, err
	}

	var authMethods []ssh.AuthMethod

	if preferAgent {
		// Try SSH agent first
		if SSHAgentAvailable() {
			agentAuth, err := GetSSHAgentAuthMethod()
			if err == nil {
				log.Debug().Msg("Using SSH agent for authentication (preferred)")
				authMethods = append(authMethods, agentAuth)
			}
		}

		// Add key file as fallback
		if keyFilePath != "" {
			if signer, err := GetPrivateKey(keyFilePath); err == nil {
				authMethods = append(authMethods, ssh.PublicKeys(signer))
			}
		}
	} else {
		// Try key file first
		if keyFilePath != "" {
			if signer, err := GetPrivateKey(keyFilePath); err == nil {
				log.Debug().Msg("Using SSH key file for authentication (preferred)")
				authMethods = append(authMethods, ssh.PublicKeys(signer))
			}
		}

		// Add agent as fallback
		if SSHAgentAvailable() {
			if agentAuth, err := GetSSHAgentAuthMethod(); err == nil {
				authMethods = append(authMethods, agentAuth)
			}
		}
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no SSH authentication methods available")
	}

	return &ssh.ClientConfig{
		User:            username,
		Auth:            authMethods,
		HostKeyCallback: customCallback,
		Timeout:         0,
	}, nil
}
