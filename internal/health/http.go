package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// Server represents the health HTTP server.
type Server struct {
	addr     string
	server   *http.Server
	registry *Registry
}

// NewServer creates a new health server.
// SECURITY: Only accepts localhost addresses by default.
// Use NewServerUnsafe for non-localhost addresses (not recommended).
func NewServer(addr string) *Server {
	// Force localhost binding for security
	addr = sanitizeBindAddress(addr)
	return &Server{
		addr:     addr,
		registry: GetRegistry(),
	}
}

// sanitizeBindAddress ensures the address only binds to localhost.
// This prevents accidental exposure of health endpoints to the network.
func sanitizeBindAddress(addr string) string {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		// If no port, assume it's just a port number
		if _, err := net.LookupPort("tcp", addr); err == nil {
			return "127.0.0.1:" + addr
		}
		// Default to localhost:9090
		log.Warn().Str("addr", addr).Msg("Invalid health endpoint address, using 127.0.0.1:9090")
		return "127.0.0.1:9090"
	}

	// Check if host is a safe localhost variant
	if !isLocalhostAddress(host) {
		log.Warn().
			Str("original", addr).
			Str("sanitized", "127.0.0.1:"+port).
			Msg("Health endpoint bound to localhost only for security (use TUNATAP_ALLOW_REMOTE_HEALTH=1 to override)")
		return "127.0.0.1:" + port
	}

	return addr
}

// isLocalhostAddress checks if the address is a localhost variant.
func isLocalhostAddress(host string) bool {
	switch strings.ToLower(host) {
	case "", "localhost", "127.0.0.1", "::1", "[::1]":
		return true
	default:
		return false
	}
}

// Start starts the health HTTP server.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Health check endpoints
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/healthz", s.handleHealthz) // Kubernetes-style liveness probe
	mux.HandleFunc("/readyz", s.handleReadyz)   // Kubernetes-style readiness probe
	mux.HandleFunc("/metrics", s.handleMetrics) // Prometheus-style metrics

	s.server = &http.Server{
		Addr:              s.addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Check if port is available
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("health server port unavailable: %w", err)
	}

	log.Info().Msgf("Health server listening on %s", s.addr)

	go func() {
		if err := s.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Error().Err(err).Msg("Health server error")
		}
	}()

	return nil
}

// Stop gracefully stops the health server.
func (s *Server) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

// handleHealth returns detailed health status as JSON.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	status := s.registry.GetStatus()

	w.Header().Set("Content-Type", "application/json")

	if !status.Healthy {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(status); err != nil {
		log.Error().Err(err).Msg("Failed to encode health response")
	}
}

// handleHealthz returns a simple ok/fail for liveness probes.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	// Liveness: are we running?
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

// handleReadyz returns readiness status.
func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	// Readiness: do we have healthy tunnels?
	status := s.registry.GetStatus()

	w.Header().Set("Content-Type", "text/plain")

	if status.Healthy && len(status.Tunnels) > 0 {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	} else if len(status.Tunnels) == 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("no tunnels\n"))
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("unhealthy\n"))
	}
}

// handleMetrics returns Prometheus-style metrics.
func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	status := s.registry.GetStatus()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4")

	// Helper to write metrics (ignoring errors - standard for HTTP handlers)
	write := func(format string, args ...any) {
		_, _ = fmt.Fprintf(w, format, args...)
	}

	// Overall metrics
	write("# HELP tunatap_up Whether tunatap is healthy\n")
	write("# TYPE tunatap_up gauge\n")
	if status.Healthy {
		write("tunatap_up 1\n")
	} else {
		write("tunatap_up 0\n")
	}

	write("# HELP tunatap_uptime_seconds Time since tunatap started\n")
	write("# TYPE tunatap_uptime_seconds gauge\n")
	write("tunatap_uptime_seconds %.0f\n", status.Uptime.Seconds())

	write("# HELP tunatap_tunnels_total Number of active tunnels\n")
	write("# TYPE tunatap_tunnels_total gauge\n")
	write("tunatap_tunnels_total %d\n", len(status.Tunnels))

	// Per-tunnel metrics (using index instead of cluster name to avoid leaking infra names)
	write("# HELP tunatap_tunnel_healthy Whether a tunnel is healthy\n")
	write("# TYPE tunatap_tunnel_healthy gauge\n")
	for i, t := range status.Tunnels {
		healthy := 0
		if t.Healthy {
			healthy = 1
		}
		write("tunatap_tunnel_healthy{tunnel=\"%d\",local_port=\"%d\"} %d\n",
			i, t.LocalPort, healthy)
	}

	write("# HELP tunatap_tunnel_uptime_seconds Tunnel uptime in seconds\n")
	write("# TYPE tunatap_tunnel_uptime_seconds gauge\n")
	for i, t := range status.Tunnels {
		write("tunatap_tunnel_uptime_seconds{tunnel=\"%d\",local_port=\"%d\"} %.0f\n",
			i, t.LocalPort, t.Uptime.Seconds())
	}

	// Connection pool metrics if available
	write("# HELP tunatap_pool_size Connection pool size\n")
	write("# TYPE tunatap_pool_size gauge\n")
	write("# HELP tunatap_pool_active_uses Active pool connections in use\n")
	write("# TYPE tunatap_pool_active_uses gauge\n")
	for i, t := range status.Tunnels {
		if t.Pool != nil {
			write("tunatap_pool_size{tunnel=\"%d\",local_port=\"%d\"} %d\n",
				i, t.LocalPort, t.Pool.Size)
			write("tunatap_pool_active_uses{tunnel=\"%d\",local_port=\"%d\"} %d\n",
				i, t.LocalPort, t.Pool.ActiveUses)
		}
	}
}

// StartHealthServer is a convenience function to start a health server.
// Returns a function to stop the server.
func StartHealthServer(addr string) (func(), error) {
	server := NewServer(addr)
	if err := server.Start(); err != nil {
		return nil, err
	}

	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Stop(ctx)
	}, nil
}
