package sshkeys

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"

	"golang.org/x/crypto/ssh"
)

// EphemeralKeyPair holds an in-memory SSH key pair.
// The keys are never written to disk, providing enhanced security
// for bastion session authentication.
type EphemeralKeyPair struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	signer     ssh.Signer
}

// GenerateEphemeralKeyPair generates a new ED25519 key pair in memory.
// The keys are cryptographically secure and suitable for SSH authentication.
// Returns an error if key generation fails.
func GenerateEphemeralKeyPair() (*EphemeralKeyPair, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ED25519 key: %w", err)
	}

	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH signer: %w", err)
	}

	return &EphemeralKeyPair{
		privateKey: priv,
		publicKey:  pub,
		signer:     signer,
	}, nil
}

// Signer returns the SSH signer for this key pair.
// Use this to authenticate SSH connections.
func (e *EphemeralKeyPair) Signer() ssh.Signer {
	return e.signer
}

// PublicKeyString returns the public key in OpenSSH authorized_keys format.
// This format is suitable for registering with OCI Bastion sessions.
func (e *EphemeralKeyPair) PublicKeyString() string {
	return string(ssh.MarshalAuthorizedKey(e.signer.PublicKey()))
}

// AuthMethod returns an SSH authentication method using this key pair.
// Use this when creating SSH client configurations.
func (e *EphemeralKeyPair) AuthMethod() ssh.AuthMethod {
	return ssh.PublicKeys(e.signer)
}

// PublicKey returns the raw ED25519 public key.
func (e *EphemeralKeyPair) PublicKey() ed25519.PublicKey {
	return e.publicKey
}
