package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRegistry_RegisterDeregister(t *testing.T) {
	r := &Registry{
		tunnels:   make(map[string]*TunnelStatus),
		startTime: time.Now(),
	}

	status := &TunnelStatus{
		ID:        "test-1",
		Cluster:   "my-cluster",
		LocalPort: 6443,
	}

	r.Register(status)

	if r.TunnelCount() != 1 {
		t.Errorf("TunnelCount() = %d, want 1", r.TunnelCount())
	}

	r.Deregister("test-1")

	if r.TunnelCount() != 0 {
		t.Errorf("TunnelCount() = %d, want 0", r.TunnelCount())
	}
}

func TestRegistry_UpdateHealth(t *testing.T) {
	r := &Registry{
		tunnels:   make(map[string]*TunnelStatus),
		startTime: time.Now(),
	}

	status := &TunnelStatus{
		ID:      "test-1",
		Cluster: "my-cluster",
		Healthy: true,
	}

	r.Register(status)
	r.UpdateHealth("test-1", false, "connection failed")

	s := r.GetTunnelStatus("test-1")
	if s.Healthy {
		t.Error("Tunnel should be unhealthy")
	}
	if s.LastError != "connection failed" {
		t.Errorf("LastError = %q, want %q", s.LastError, "connection failed")
	}
}

func TestRegistry_UpdatePoolStatus(t *testing.T) {
	r := &Registry{
		tunnels:   make(map[string]*TunnelStatus),
		startTime: time.Now(),
	}

	status := &TunnelStatus{
		ID:      "test-1",
		Cluster: "my-cluster",
	}

	r.Register(status)
	r.UpdatePoolStatus("test-1", &PoolStatus{Size: 5, ActiveUses: 2, Available: 3})

	s := r.GetTunnelStatus("test-1")
	if s.Pool == nil {
		t.Fatal("Pool should not be nil")
	}
	if s.Pool.Size != 5 {
		t.Errorf("Pool.Size = %d, want 5", s.Pool.Size)
	}
}

func TestRegistry_GetStatus(t *testing.T) {
	r := &Registry{
		tunnels:   make(map[string]*TunnelStatus),
		startTime: time.Now(),
	}

	// Empty registry should be healthy
	status := r.GetStatus()
	if !status.Healthy {
		t.Error("Empty registry should be healthy")
	}

	// Add healthy tunnel
	r.Register(&TunnelStatus{ID: "test-1", Cluster: "c1", Healthy: true})
	status = r.GetStatus()
	if !status.Healthy {
		t.Error("Should be healthy with one healthy tunnel")
	}
	if len(status.Tunnels) != 1 {
		t.Errorf("Tunnels count = %d, want 1", len(status.Tunnels))
	}

	// Add unhealthy tunnel
	r.Register(&TunnelStatus{ID: "test-2", Cluster: "c2", Healthy: false})
	status = r.GetStatus()
	if status.Healthy {
		t.Error("Should be unhealthy with one unhealthy tunnel")
	}
}

func TestServer_HandleHealth(t *testing.T) {
	// Create isolated registry for test
	r := &Registry{
		tunnels:   make(map[string]*TunnelStatus),
		startTime: time.Now(),
	}
	s := &Server{registry: r}

	// Test empty (healthy)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	s.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}

	var status HealthStatus
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if !status.Healthy {
		t.Error("Status should be healthy")
	}

	// Add unhealthy tunnel and test again
	r.Register(&TunnelStatus{ID: "test-1", Healthy: false})
	rec = httptest.NewRecorder()
	s.handleHealth(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestServer_HandleHealthz(t *testing.T) {
	s := &Server{registry: &Registry{tunnels: make(map[string]*TunnelStatus), startTime: time.Now()}}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.handleHealthz(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != "ok\n" {
		t.Errorf("Body = %q, want %q", body, "ok\n")
	}
}

func TestServer_HandleReadyz(t *testing.T) {
	r := &Registry{
		tunnels:   make(map[string]*TunnelStatus),
		startTime: time.Now(),
	}
	s := &Server{registry: r}

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)

	// Empty = not ready
	rec := httptest.NewRecorder()
	s.handleReadyz(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Empty: Status code = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	// With healthy tunnel = ready
	r.Register(&TunnelStatus{ID: "test-1", Healthy: true})
	rec = httptest.NewRecorder()
	s.handleReadyz(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("With healthy tunnel: Status code = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestServer_HandleMetrics(t *testing.T) {
	r := &Registry{
		tunnels:   make(map[string]*TunnelStatus),
		startTime: time.Now(),
	}
	r.Register(&TunnelStatus{
		ID:        "test-1",
		Cluster:   "my-cluster",
		LocalPort: 6443,
		Healthy:   true,
		Pool:      &PoolStatus{Size: 5, ActiveUses: 2},
	})

	s := &Server{registry: r}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	s.handleMetrics(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status code = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()

	// Check for expected metrics
	expectedMetrics := []string{
		"tunatap_up 1",
		"tunatap_uptime_seconds",
		"tunatap_tunnels_total 1",
		"tunatap_tunnel_healthy",
		"tunatap_pool_size",
	}

	for _, metric := range expectedMetrics {
		if !strings.Contains(body, metric) {
			t.Errorf("Missing metric: %s", metric)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{90 * time.Second, "2m0s"},
		{90 * time.Minute, "2h0m0s"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
