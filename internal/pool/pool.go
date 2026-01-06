package pool

import (
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/ssh"
)

// ConnectionFactory is a function that creates new SSH connections.
type ConnectionFactory func() (*ssh.Client, error)

// HealthCheckFunc checks if an SSH client is healthy.
type HealthCheckFunc func(*ssh.Client) bool

// ConnectionPool manages a pool of SSH connections.
type ConnectionPool struct {
	mu            sync.Mutex
	connections   []*TrackedSSHConnection
	maxSize       int
	maxConcurrent int
	factory       ConnectionFactory
}

// NewConnectionPool creates a new connection pool.
func NewConnectionPool(maxSize, maxConcurrent int, factory ConnectionFactory, warmupCount int) (*ConnectionPool, error) {
	pool := &ConnectionPool{
		connections:   make([]*TrackedSSHConnection, 0, maxSize),
		maxSize:       maxSize,
		maxConcurrent: maxConcurrent,
		factory:       factory,
	}

	// Warm up the pool with initial connections
	for i := 0; i < warmupCount && i < maxSize; i++ {
		client, err := factory()
		if err != nil {
			log.Warn().Err(err).Msgf("Failed to create warmup connection %d", i+1)
			continue
		}
		conn := NewTrackedConnection(client, maxConcurrent)
		pool.connections = append(pool.connections, conn)
		log.Debug().Msgf("Created warmup connection %d/%d", i+1, warmupCount)
	}

	if len(pool.connections) == 0 && warmupCount > 0 {
		return nil, fmt.Errorf("failed to create any warmup connections")
	}

	log.Info().Msgf("Connection pool initialized with %d connections (max: %d)", len(pool.connections), maxSize)
	return pool, nil
}

// Get retrieves an available connection from the pool.
func (p *ConnectionPool) Get() (*TrackedSSHConnection, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// First, try to find an existing connection with capacity
	for _, conn := range p.connections {
		if conn.CanAcceptMore() {
			if conn.Increment() {
				log.Debug().Msgf("Reusing existing connection, use count: %d", conn.GetUseCount())
				return conn, nil
			}
		}
	}

	// Remove invalid connections
	p.removeInvalidConnections()

	// If we can create a new connection, do so
	if len(p.connections) < p.maxSize {
		client, err := p.factory()
		if err != nil {
			return nil, fmt.Errorf("failed to create new connection: %w", err)
		}

		conn := NewTrackedConnection(client, p.maxConcurrent)
		conn.Increment()
		p.connections = append(p.connections, conn)
		log.Debug().Msgf("Created new connection, pool size: %d", len(p.connections))
		return conn, nil
	}

	// Pool is full, wait for a connection to become available
	// For now, return an error (could implement waiting later)
	return nil, fmt.Errorf("connection pool exhausted")
}

// removeInvalidConnections removes invalid connections from the pool.
func (p *ConnectionPool) removeInvalidConnections() {
	valid := make([]*TrackedSSHConnection, 0, len(p.connections))
	for _, conn := range p.connections {
		if !conn.IsInvalid() {
			valid = append(valid, conn)
		} else {
			conn.Close()
		}
	}
	p.connections = valid
}

// HealthCheck performs a health check on all connections.
func (p *ConnectionPool) HealthCheck(checkFunc HealthCheckFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conn := range p.connections {
		if conn.IsInvalid() {
			continue
		}

		if !checkFunc(conn.Client) {
			log.Warn().Msg("Connection failed health check, marking invalid")
			conn.Invalidate()
		}
	}

	// Clean up invalid connections that are idle
	p.removeIdleInvalidConnections()
}

// removeIdleInvalidConnections removes invalid connections that have no active uses.
func (p *ConnectionPool) removeIdleInvalidConnections() {
	valid := make([]*TrackedSSHConnection, 0, len(p.connections))
	for _, conn := range p.connections {
		if conn.IsInvalid() && conn.IsIdle() {
			conn.Close()
		} else {
			valid = append(valid, conn)
		}
	}
	p.connections = valid
}

// Size returns the current number of connections in the pool.
func (p *ConnectionPool) Size() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.connections)
}

// ActiveCount returns the total number of active uses across all connections.
func (p *ConnectionPool) ActiveCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	total := 0
	for _, conn := range p.connections {
		total += conn.GetUseCount()
	}
	return total
}

// Close closes all connections in the pool.
func (p *ConnectionPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conn := range p.connections {
		if err := conn.Close(); err != nil {
			log.Warn().Err(err).Msg("Error closing connection")
		}
	}
	p.connections = nil

	log.Info().Msg("Connection pool closed")
}

// CheckSSHClientHealth checks if an SSH client is healthy by sending a keepalive.
func CheckSSHClientHealth(client *ssh.Client) bool {
	if client == nil {
		return false
	}
	_, _, err := client.SendRequest("keepalive@openssh.com", true, nil)
	return err == nil
}
