package discovery

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	// DefaultCacheTTL is the default time-to-live for cache entries.
	DefaultCacheTTL = 24 * time.Hour

	// CacheFileName is the name of the cache file.
	CacheFileName = "cache.json"
)

// CacheEntry represents a cached cluster or bastion entry.
type CacheEntry struct {
	OCID            string    `json:"ocid"`
	Region          string    `json:"region"`
	CompartmentOCID string    `json:"compartment_ocid"`
	VcnID           string    `json:"vcn_id,omitempty"`
	SubnetID        string    `json:"subnet_id,omitempty"`
	CachedAt        time.Time `json:"cached_at"`

	// Endpoint information for clusters
	EndpointIP   string `json:"endpoint_ip,omitempty"`
	EndpointPort int    `json:"endpoint_port,omitempty"`
}

// CacheData represents the full cache file structure.
type CacheData struct {
	Clusters map[string]*CacheEntry `json:"clusters"`
	Bastions map[string]*CacheEntry `json:"bastions"`
}

// Cache manages cluster and bastion discovery caching.
type Cache struct {
	mu   sync.RWMutex
	data CacheData
	path string
	ttl  time.Duration
}

// NewCache creates or loads a cache from the specified base directory.
func NewCache(basePath string, ttl time.Duration) (*Cache, error) {
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}

	cachePath := filepath.Join(basePath, CacheFileName)

	cache := &Cache{
		data: CacheData{
			Clusters: make(map[string]*CacheEntry),
			Bastions: make(map[string]*CacheEntry),
		},
		path: cachePath,
		ttl:  ttl,
	}

	// Try to load existing cache
	if err := cache.Load(); err != nil {
		// Not an error if file doesn't exist
		if !os.IsNotExist(err) {
			log.Warn().Err(err).Msg("Failed to load cache, starting fresh")
		}
	}

	return cache, nil
}

// GetCluster retrieves a cached cluster entry by name.
// Returns nil if entry doesn't exist or is expired.
func (c *Cache) GetCluster(name string) *CacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.data.Clusters[name]
	if !ok {
		return nil
	}

	// Check if expired
	if c.isExpired(entry) {
		log.Debug().Msgf("Cache entry for cluster '%s' is expired", name)
		return nil
	}

	return entry
}

// SetCluster stores a cluster entry in the cache.
func (c *Cache) SetCluster(name string, entry *CacheEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry.CachedAt = time.Now()
	c.data.Clusters[name] = entry

	return c.saveLocked()
}

// GetBastion retrieves a cached bastion entry for a cluster.
// Returns nil if entry doesn't exist or is expired.
func (c *Cache) GetBastion(clusterName string) *CacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.data.Bastions[clusterName]
	if !ok {
		return nil
	}

	// Check if expired
	if c.isExpired(entry) {
		log.Debug().Msgf("Cache entry for bastion (cluster '%s') is expired", clusterName)
		return nil
	}

	return entry
}

// SetBastion stores a bastion entry for a cluster in the cache.
func (c *Cache) SetBastion(clusterName string, entry *CacheEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry.CachedAt = time.Now()
	c.data.Bastions[clusterName] = entry

	return c.saveLocked()
}

// Invalidate removes a cluster and its associated bastion from the cache.
func (c *Cache) Invalidate(clusterName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.data.Clusters, clusterName)
	delete(c.data.Bastions, clusterName)

	return c.saveLocked()
}

// InvalidateAll clears the entire cache.
func (c *Cache) InvalidateAll() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.data.Clusters = make(map[string]*CacheEntry)
	c.data.Bastions = make(map[string]*CacheEntry)

	return c.saveLocked()
}

// GetAllClusters returns all non-expired cluster entries.
func (c *Cache) GetAllClusters() map[string]*CacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make(map[string]*CacheEntry)
	for name, entry := range c.data.Clusters {
		if !c.isExpired(entry) {
			result[name] = entry
		}
	}
	return result
}

// GetClusterTTL returns the remaining TTL for a cluster entry.
// Returns 0 if the entry doesn't exist or is expired.
func (c *Cache) GetClusterTTL(name string) time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.data.Clusters[name]
	if !ok {
		return 0
	}

	remaining := c.ttl - time.Since(entry.CachedAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Load reads the cache from disk.
func (c *Cache) Load() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := os.ReadFile(c.path)
	if err != nil {
		return err
	}

	var cacheData CacheData
	if err := json.Unmarshal(data, &cacheData); err != nil {
		return fmt.Errorf("failed to parse cache: %w", err)
	}

	// Initialize maps if nil
	if cacheData.Clusters == nil {
		cacheData.Clusters = make(map[string]*CacheEntry)
	}
	if cacheData.Bastions == nil {
		cacheData.Bastions = make(map[string]*CacheEntry)
	}

	c.data = cacheData
	log.Debug().Msgf("Loaded cache with %d clusters and %d bastions",
		len(c.data.Clusters), len(c.data.Bastions))

	return nil
}

// Save persists the cache to disk.
func (c *Cache) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.saveLocked()
}

// saveLocked saves the cache to disk (must be called with lock held).
func (c *Cache) saveLocked() error {
	// Ensure directory exists
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	data, err := json.MarshalIndent(c.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	if err := os.WriteFile(c.path, data, 0600); err != nil {
		return fmt.Errorf("failed to write cache: %w", err)
	}

	return nil
}

// isExpired checks if a cache entry has expired.
func (c *Cache) isExpired(entry *CacheEntry) bool {
	return time.Since(entry.CachedAt) > c.ttl
}

// Path returns the cache file path.
func (c *Cache) Path() string {
	return c.path
}

// CleanExpired removes all expired entries from the cache.
func (c *Cache) CleanExpired() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	modified := false

	// Clean expired clusters
	for name, entry := range c.data.Clusters {
		if c.isExpired(entry) {
			delete(c.data.Clusters, name)
			modified = true
		}
	}

	// Clean expired bastions
	for name, entry := range c.data.Bastions {
		if c.isExpired(entry) {
			delete(c.data.Bastions, name)
			modified = true
		}
	}

	if modified {
		return c.saveLocked()
	}
	return nil
}
