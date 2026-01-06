package tunnel

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/scotttball/tunatap/internal/pool"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/proxy"
)

// SSHTunnel represents an SSH tunnel configuration.
type SSHTunnel struct {
	Local                         *Endpoint
	Server                        *Endpoint
	Remote                        *Endpoint
	SocksProxy                    *Endpoint
	Config                        *ssh.ClientConfig
	SshConnectionMaxConcurrentUse int
	SshConnectionPoolSize         int
	SshWarmupConnectionCount      int

	// ActualLocalPort is set after Start() binds to the local port.
	// Useful when Local.Port is 0 (ephemeral port allocation).
	ActualLocalPort int

	// Ready is closed when the tunnel is ready to accept connections.
	Ready chan struct{}

	// listener holds the TCP listener for graceful shutdown.
	listener net.Listener
}

// NewSSHTunnel creates a new SSH tunnel configuration.
func NewSSHTunnel(localListener, server string, sshConfig *ssh.ClientConfig, destination string, poolSize, warmupCount, maxConcurrent int, socksProxy string) *SSHTunnel {
	tunnel := &SSHTunnel{
		Config:                        sshConfig,
		Local:                         NewEndpoint(localListener),
		Server:                        NewEndpoint(server),
		Remote:                        NewEndpoint(destination),
		SshConnectionPoolSize:         poolSize,
		SshWarmupConnectionCount:      warmupCount,
		SshConnectionMaxConcurrentUse: maxConcurrent,
		Ready:                         make(chan struct{}),
	}

	if socksProxy != "" {
		tunnel.SocksProxy = NewEndpoint(socksProxy)
	}

	return tunnel
}

// GetActualLocalPort returns the actual local port the tunnel is listening on.
// This is useful when the configured port is 0 (ephemeral port).
func (tunnel *SSHTunnel) GetActualLocalPort() int {
	if tunnel.ActualLocalPort != 0 {
		return tunnel.ActualLocalPort
	}
	return tunnel.Local.Port
}

// Close gracefully shuts down the tunnel.
func (tunnel *SSHTunnel) Close() error {
	if tunnel.listener != nil {
		return tunnel.listener.Close()
	}
	return nil
}

// establishServerConnection creates a new SSH connection to the server.
func (tunnel *SSHTunnel) establishServerConnection() (*ssh.Client, error) {
	if tunnel.SocksProxy != nil {
		return tunnel.connectViaProxy()
	}
	log.Info().Msgf("Establishing SSH connection to %s", tunnel.Server.String())
	return ssh.Dial("tcp", tunnel.Server.String(), tunnel.Config)
}

// connectViaProxy connects to the SSH server through a SOCKS proxy.
func (tunnel *SSHTunnel) connectViaProxy() (*ssh.Client, error) {
	log.Info().Msgf("Establishing SSH connection via SOCKS proxy to %s", tunnel.Server.String())
	dialer, err := proxy.SOCKS5("tcp", tunnel.SocksProxy.String(), nil, proxy.Direct)
	if err != nil {
		return nil, fmt.Errorf("failed to create SOCKS dialer: %w", err)
	}

	conn, err := dialer.Dial("tcp", tunnel.Server.String())
	if err != nil {
		return nil, fmt.Errorf("failed to dial SSH server via SOCKS proxy: %w", err)
	}

	c, chans, reqs, err := ssh.NewClientConn(conn, tunnel.Server.String(), tunnel.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSH client connection: %w", err)
	}

	return ssh.NewClient(c, chans, reqs), nil
}

// NewConnectionPoolForRemote creates a connection pool for this tunnel.
func (tunnel *SSHTunnel) NewConnectionPoolForRemote() (*pool.ConnectionPool, error) {
	return pool.NewConnectionPool(
		tunnel.SshConnectionPoolSize,
		tunnel.SshConnectionMaxConcurrentUse,
		func() (*ssh.Client, error) {
			return tunnel.establishServerConnection()
		},
		tunnel.SshWarmupConnectionCount,
	)
}

// startHealthCheck periodically checks the health of the connection pool.
func (tunnel *SSHTunnel) startHealthCheck(ctx context.Context, connPool *pool.ConnectionPool) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Debug().Msg("Health check stopped due to context cancellation")
			return
		case <-ticker.C:
			log.Debug().Msg("Performing connection pool health check")
			connPool.HealthCheck(pool.CheckSSHClientHealth)
		}
	}
}

// Start starts the tunnel, listening for local connections and forwarding them.
func (tunnel *SSHTunnel) Start() error {
	log.Debug().Msgf("Setup local listener: %s", tunnel.Local)
	listener, err := net.Listen("tcp", tunnel.Local.String())
	if err != nil {
		log.Error().Err(err).Msgf("Failed to setup local listener: %s", tunnel.Local)
		return err
	}
	tunnel.listener = listener
	defer listener.Close()

	// Extract actual port (important for ephemeral port allocation)
	if addr, ok := listener.Addr().(*net.TCPAddr); ok {
		tunnel.ActualLocalPort = addr.Port
		log.Debug().Msgf("Bound to actual port: %d", tunnel.ActualLocalPort)
	}

	connPool, err := tunnel.NewConnectionPoolForRemote()
	if err != nil {
		log.Error().Err(err).Msgf("Failed to setup connection pool: %s", tunnel.Remote)
		return err
	}
	defer connPool.Close()

	errors := make(chan error, 10)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Health check goroutine
	go tunnel.startHealthCheck(ctx, connPool)

	// Signal that tunnel is ready
	close(tunnel.Ready)

	log.Info().Msgf("Tunnel ready. Listening on localhost:%d, forwarding to %s via %s",
		tunnel.ActualLocalPort, tunnel.Remote.String(), tunnel.Server.String())

	for {
		select {
		case err := <-errors:
			log.Error().Err(err).Msg("received error from forwarder")

			cancel()
			ctx, cancel = context.WithCancel(context.Background())
			go tunnel.startHealthCheck(ctx, connPool)

		default:
		}

		localConnections := make(chan net.Conn, 100)

		go func() {
			for localConn := range localConnections {
				go tunnel.forward(ctx, localConn, connPool, errors)
			}
		}()

		localConn, err := listener.Accept()
		if err != nil {
			errors <- fmt.Errorf("listener.Accept() error: %w", err)
			continue
		}

		select {
		case localConnections <- localConn:
			log.Debug().Msg("Queued new connection for forwarding")
		default:
			log.Warn().Msg("Too many connections; closing new connection")
			localConn.Close()
		}
	}
}

// forward forwards a local connection through the SSH tunnel.
func (tunnel *SSHTunnel) forward(ctx context.Context, localConn net.Conn, connPool *pool.ConnectionPool, ch chan error) {
	defer localConn.Close()

	trackedConn, err := connPool.Get()
	if err != nil {
		ch <- fmt.Errorf("failed to get connection from pool: %w", err)
		return
	}

	defer trackedConn.Decrement()

	remoteConn, err := trackedConn.Client.Dial("tcp", tunnel.Remote.String())
	if err != nil {
		trackedConn.Invalidate()
		log.Error().Err(err).Msg("remote dial error")
		return
	}
	defer remoteConn.Close()

	log.Debug().Msgf("Connected to remote endpoint: %s", tunnel.Remote.String())

	pipe := func(ctx context.Context, writer, reader net.Conn, done chan<- struct{}) {
		defer func() {
			done <- struct{}{}
			writer.Close()
		}()

		buf := make([]byte, 32*1024)
		for {
			select {
			case <-ctx.Done():
				log.Debug().Msg("Pipe routine canceled due to context cancellation")
				return
			default:
				n, err := reader.Read(buf)
				if n > 0 {
					if _, writeErr := writer.Write(buf[:n]); writeErr != nil {
						log.Debug().Err(writeErr).Msg("Error writing to connection during piping")
						return
					}
				}
				if err != nil {
					if err != io.EOF {
						log.Debug().Err(err).Msg("Data transfer error during piping")
					}
					return
				}
			}
		}
	}

	done := make(chan struct{}, 2)

	go pipe(ctx, localConn, remoteConn, done)
	go pipe(ctx, remoteConn, localConn, done)

	select {
	case <-done:
	case <-ctx.Done():
		log.Debug().Msg("Forward routine canceled")
	}
}

// StartAsync starts the tunnel in a goroutine and returns immediately.
// Use the Ready channel to wait for the tunnel to be ready.
// Returns an error channel that will receive any errors from the tunnel.
func (tunnel *SSHTunnel) StartAsync() <-chan error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- tunnel.Start()
	}()
	return errCh
}

// FindAvailablePort finds an available TCP port on localhost.
func FindAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port, nil
}

// IsPortAvailable checks if a TCP port is available on localhost.
func IsPortAvailable(port int) bool {
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return false
	}
	listener.Close()
	return true
}
