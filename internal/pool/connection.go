package pool

import (
	"sync"

	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/ssh"
)

// TrackedSSHConnection wraps an SSH client with usage tracking.
type TrackedSSHConnection struct {
	Client   *ssh.Client
	useCount int
	maxUses  int
	invalid  bool
	mu       sync.Mutex
}

// NewTrackedConnection creates a new tracked connection.
func NewTrackedConnection(client *ssh.Client, maxUses int) *TrackedSSHConnection {
	return &TrackedSSHConnection{
		Client:  client,
		maxUses: maxUses,
	}
}

// GetUseCount returns the current use count.
func (conn *TrackedSSHConnection) GetUseCount() int {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	return conn.useCount
}

// Increment increments the use count if under the limit.
// Returns true if successful, false if limit reached.
func (conn *TrackedSSHConnection) Increment() bool {
	conn.mu.Lock()
	defer conn.mu.Unlock()

	if conn.invalid {
		return false
	}

	if conn.useCount < conn.maxUses {
		conn.useCount++
		return true
	}
	return false
}

// Decrement decrements the use count.
func (conn *TrackedSSHConnection) Decrement() {
	conn.mu.Lock()
	defer conn.mu.Unlock()

	if conn.useCount > 0 {
		conn.useCount--
	}

	log.Debug().Msgf("Decremented use count for server connection, current use: %v", conn.useCount)
}

// Invalidate marks the connection as invalid.
func (conn *TrackedSSHConnection) Invalidate() {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	conn.invalid = true
}

// IsInvalid returns whether the connection is invalid.
func (conn *TrackedSSHConnection) IsInvalid() bool {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	return conn.invalid
}

// CanAcceptMore returns true if the connection can accept more uses.
func (conn *TrackedSSHConnection) CanAcceptMore() bool {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	return !conn.invalid && conn.useCount < conn.maxUses
}

// IsIdle returns true if the connection has no active uses.
func (conn *TrackedSSHConnection) IsIdle() bool {
	conn.mu.Lock()
	defer conn.mu.Unlock()
	return conn.useCount == 0
}

// Close closes the underlying SSH client.
func (conn *TrackedSSHConnection) Close() error {
	conn.mu.Lock()
	defer conn.mu.Unlock()

	conn.invalid = true
	if conn.Client != nil {
		return conn.Client.Close()
	}
	return nil
}
