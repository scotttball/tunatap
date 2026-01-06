package health

import (
	"sync"
	"time"
)

// TunnelStatus represents the health status of a single tunnel.
type TunnelStatus struct {
	ID         string        `json:"id"`
	Cluster    string        `json:"cluster"`
	Region     string        `json:"region,omitempty"`
	LocalPort  int           `json:"local_port"`
	RemoteHost string        `json:"remote_host"`
	RemotePort int           `json:"remote_port"`
	SessionID  string        `json:"session_id,omitempty"`
	StartTime  time.Time     `json:"start_time"`
	Uptime     time.Duration `json:"uptime_ns"`
	Healthy    bool          `json:"healthy"`
	LastError  string        `json:"last_error,omitempty"`
	Pool       *PoolStatus   `json:"pool,omitempty"`
}

// PoolStatus represents the status of the connection pool.
type PoolStatus struct {
	Size       int `json:"size"`
	ActiveUses int `json:"active_uses"`
	Available  int `json:"available"`
}

// HealthStatus represents the overall health status.
type HealthStatus struct {
	Healthy   bool            `json:"healthy"`
	Uptime    time.Duration   `json:"uptime_ns"`
	UptimeStr string          `json:"uptime"`
	Tunnels   []*TunnelStatus `json:"tunnels"`
}

// Registry tracks all active tunnels for health reporting.
type Registry struct {
	mu        sync.RWMutex
	tunnels   map[string]*TunnelStatus
	startTime time.Time
}

var globalRegistry *Registry
var once sync.Once

// GetRegistry returns the global health registry.
func GetRegistry() *Registry {
	once.Do(func() {
		globalRegistry = &Registry{
			tunnels:   make(map[string]*TunnelStatus),
			startTime: time.Now(),
		}
	})
	return globalRegistry
}

// Register adds a tunnel to the registry.
// If StartTime is zero, it will be set to now.
// Healthy defaults to true only if not explicitly set (zero value).
func (r *Registry) Register(status *TunnelStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if status.StartTime.IsZero() {
		status.StartTime = time.Now()
	}
	// Don't override explicit Healthy setting - only default for new tunnels
	// Note: We can't distinguish unset from explicitly false, so we trust the caller
	r.tunnels[status.ID] = status
}

// Deregister removes a tunnel from the registry.
func (r *Registry) Deregister(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tunnels, id)
}

// UpdateHealth updates the health status of a tunnel.
func (r *Registry) UpdateHealth(id string, healthy bool, lastError string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if status, ok := r.tunnels[id]; ok {
		status.Healthy = healthy
		if lastError != "" {
			status.LastError = lastError
		}
	}
}

// UpdatePoolStatus updates the connection pool status for a tunnel.
func (r *Registry) UpdatePoolStatus(id string, pool *PoolStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if status, ok := r.tunnels[id]; ok {
		status.Pool = pool
	}
}

// GetStatus returns the overall health status.
func (r *Registry) GetStatus() *HealthStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	uptime := time.Since(r.startTime)
	tunnels := make([]*TunnelStatus, 0, len(r.tunnels))
	allHealthy := true

	for _, t := range r.tunnels {
		// Update uptime
		t.Uptime = time.Since(t.StartTime)
		tunnels = append(tunnels, t)
		if !t.Healthy {
			allHealthy = false
		}
	}

	return &HealthStatus{
		Healthy:   allHealthy || len(tunnels) == 0,
		Uptime:    uptime,
		UptimeStr: formatDuration(uptime),
		Tunnels:   tunnels,
	}
}

// GetTunnelStatus returns the status of a specific tunnel.
func (r *Registry) GetTunnelStatus(id string) *TunnelStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if status, ok := r.tunnels[id]; ok {
		status.Uptime = time.Since(status.StartTime)
		return status
	}
	return nil
}

// IsHealthy returns true if all tunnels are healthy.
func (r *Registry) IsHealthy() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, t := range r.tunnels {
		if !t.Healthy {
			return false
		}
	}
	return true
}

// TunnelCount returns the number of registered tunnels.
func (r *Registry) TunnelCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.tunnels)
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return d.Round(time.Second).String()
	}
	if d < time.Hour {
		return d.Round(time.Minute).String()
	}
	return d.Round(time.Hour).String()
}
