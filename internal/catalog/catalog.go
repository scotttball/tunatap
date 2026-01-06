package catalog

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/scotttball/tunatap/internal/client"
	"github.com/scotttball/tunatap/internal/config"
	"gopkg.in/yaml.v3"
)

// SharedCatalog represents a shared cluster catalog.
type SharedCatalog struct {
	Version     string               `yaml:"version"`
	Name        string               `yaml:"name"`
	Description string               `yaml:"description,omitempty"`
	Maintainer  string               `yaml:"maintainer,omitempty"`
	Updated     string               `yaml:"updated,omitempty"`
	Clusters    []*config.Cluster    `yaml:"clusters"`
	Tenancies   []*config.TenantInfo `yaml:"tenancies,omitempty"`
	Defaults    *CatalogDefaults     `yaml:"defaults,omitempty"`
}

// CatalogDefaults contains default values applied to catalog entries.
type CatalogDefaults struct {
	Region      string `yaml:"region,omitempty"`
	BastionType string `yaml:"bastion_type,omitempty"`
	LocalPort   int    `yaml:"local_port,omitempty"`
}

// CatalogManager handles fetching and merging catalogs.
type CatalogManager struct {
	sources    []*config.CatalogSource
	cacheDir   string
	cacheTTL   time.Duration
	ociClient  *client.OCIClient
	httpClient *http.Client
}

// NewCatalogManager creates a new catalog manager.
func NewCatalogManager(sources []*config.CatalogSource, cacheDir string) *CatalogManager {
	return &CatalogManager{
		sources:  sources,
		cacheDir: cacheDir,
		cacheTTL: 1 * time.Hour,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetOCIClient sets the OCI client for fetching from Object Storage.
func (m *CatalogManager) SetOCIClient(client *client.OCIClient) {
	m.ociClient = client
}

// SetCacheTTL sets the cache time-to-live.
func (m *CatalogManager) SetCacheTTL(ttl time.Duration) {
	m.cacheTTL = ttl
}

// FetchAll fetches all enabled catalog sources.
func (m *CatalogManager) FetchAll(ctx context.Context) ([]*SharedCatalog, error) {
	catalogs := make([]*SharedCatalog, 0)

	for _, source := range m.sources {
		if !source.Enabled {
			continue
		}

		log.Debug().Str("source", source.Name).Msg("Fetching catalog")

		catalog, err := m.FetchSource(ctx, source)
		if err != nil {
			log.Warn().Err(err).Str("source", source.Name).Msg("Failed to fetch catalog")
			continue
		}

		catalogs = append(catalogs, catalog)
	}

	return catalogs, nil
}

// FetchSource fetches a single catalog source.
func (m *CatalogManager) FetchSource(ctx context.Context, source *config.CatalogSource) (*SharedCatalog, error) {
	// Check cache first
	cached, err := m.loadFromCache(source)
	if err == nil && cached != nil {
		return cached, nil
	}

	// Fetch based on source type
	var data []byte
	sourceType := source.Type
	if sourceType == "" {
		sourceType = m.detectSourceType(source)
	}

	switch sourceType {
	case "https", "http":
		data, err = m.fetchHTTPS(ctx, source.URL)
	case "oci":
		data, err = m.fetchOCI(ctx, source)
	case "file":
		data, err = m.fetchFile(source.URL)
	default:
		return nil, fmt.Errorf("unknown source type: %s", sourceType)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to fetch catalog: %w", err)
	}

	// Parse catalog
	var catalog SharedCatalog
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("failed to parse catalog: %w", err)
	}

	// Apply defaults
	if catalog.Defaults != nil {
		applyDefaults(&catalog)
	}

	// Cache result
	m.saveToCache(source, data)

	return &catalog, nil
}

// detectSourceType determines the source type from the URL.
func (m *CatalogManager) detectSourceType(source *config.CatalogSource) string {
	if source.OCIBucket != "" {
		return "oci"
	}

	u, err := url.Parse(source.URL)
	if err != nil {
		return "file"
	}

	switch u.Scheme {
	case "https", "http":
		return "https"
	case "file", "":
		return "file"
	case "oci":
		return "oci"
	default:
		return "file"
	}
}

// fetchHTTPS fetches a catalog from an HTTPS URL.
func (m *CatalogManager) fetchHTTPS(ctx context.Context, urlStr string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/yaml, application/x-yaml, text/yaml, text/plain")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	return io.ReadAll(resp.Body)
}

// fetchOCI fetches a catalog from OCI Object Storage.
func (m *CatalogManager) fetchOCI(ctx context.Context, source *config.CatalogSource) ([]byte, error) {
	if m.ociClient == nil {
		return nil, fmt.Errorf("OCI client not configured")
	}

	// Get namespace
	namespace := ""
	if source.OCIRegion != "" {
		m.ociClient.SetRegion(source.OCIRegion)
	}

	// Parse OCI URL if provided
	bucket := source.OCIBucket
	object := source.OCIObject

	if source.URL != "" && strings.HasPrefix(source.URL, "oci://") {
		// Parse oci://namespace/bucket/object format
		parts := strings.SplitN(strings.TrimPrefix(source.URL, "oci://"), "/", 3)
		if len(parts) >= 3 {
			namespace = parts[0]
			bucket = parts[1]
			object = parts[2]
		}
	}

	if bucket == "" || object == "" {
		return nil, fmt.Errorf("OCI bucket and object are required")
	}

	return m.ociClient.GetObject(ctx, namespace, bucket, object)
}

// fetchFile fetches a catalog from a local file.
func (m *CatalogManager) fetchFile(path string) ([]byte, error) {
	// Handle file:// URLs
	path = strings.TrimPrefix(path, "file://")

	// Expand home directory
	if strings.HasPrefix(path, "~") {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, path[1:])
	}

	return os.ReadFile(path)
}

// loadFromCache loads a catalog from the cache.
func (m *CatalogManager) loadFromCache(source *config.CatalogSource) (*SharedCatalog, error) {
	if m.cacheDir == "" {
		return nil, fmt.Errorf("cache not configured")
	}

	cachePath := m.cachePath(source)
	info, err := os.Stat(cachePath)
	if err != nil {
		return nil, err
	}

	// Check if cache is expired
	if time.Since(info.ModTime()) > m.cacheTTL {
		return nil, fmt.Errorf("cache expired")
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, err
	}

	var catalog SharedCatalog
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return nil, err
	}

	log.Debug().Str("source", source.Name).Msg("Loaded catalog from cache")
	return &catalog, nil
}

// saveToCache saves a catalog to the cache.
func (m *CatalogManager) saveToCache(source *config.CatalogSource, data []byte) error {
	if m.cacheDir == "" {
		return nil
	}

	// Ensure cache directory exists
	if err := os.MkdirAll(m.cacheDir, 0o750); err != nil {
		return err
	}

	cachePath := m.cachePath(source)
	return os.WriteFile(cachePath, data, 0o600)
}

// cachePath returns the cache file path for a source.
func (m *CatalogManager) cachePath(source *config.CatalogSource) string {
	safeName := strings.ReplaceAll(source.Name, "/", "_")
	return filepath.Join(m.cacheDir, fmt.Sprintf("catalog-%s.yaml", safeName))
}

// applyDefaults applies catalog defaults to all clusters.
func applyDefaults(catalog *SharedCatalog) {
	for _, cluster := range catalog.Clusters {
		if cluster.Region == "" && catalog.Defaults.Region != "" {
			cluster.Region = catalog.Defaults.Region
		}
		if cluster.BastionType == nil && catalog.Defaults.BastionType != "" {
			cluster.BastionType = &catalog.Defaults.BastionType
		}
		if cluster.LocalPort == nil && catalog.Defaults.LocalPort > 0 {
			port := catalog.Defaults.LocalPort
			cluster.LocalPort = &port
		}
	}
}

// MergeCatalogs merges multiple catalogs with local config.
// Local config takes precedence over catalog entries with the same cluster name.
func MergeCatalogs(local *config.Config, catalogs []*SharedCatalog) *config.Config {
	if local == nil {
		local = config.DefaultConfig()
	}

	// Create a map of local clusters by name for quick lookup
	localClusters := make(map[string]*config.Cluster)
	for _, c := range local.Clusters {
		localClusters[c.ClusterName] = c
	}

	// Create a map of local tenancies by name
	localTenancies := make(map[string]*config.TenantInfo)
	for _, t := range local.TenancyList {
		localTenancies[t.Name] = t
	}

	// Sort catalogs by priority (higher priority = processed later = takes precedence)
	// For now, we process in order and local config always wins

	for _, catalog := range catalogs {
		// Add clusters that don't exist locally
		for _, cluster := range catalog.Clusters {
			if _, exists := localClusters[cluster.ClusterName]; !exists {
				local.Clusters = append(local.Clusters, cluster)
				localClusters[cluster.ClusterName] = cluster
				log.Debug().Str("cluster", cluster.ClusterName).Str("catalog", catalog.Name).Msg("Added cluster from catalog")
			}
		}

		// Add tenancies that don't exist locally
		for _, tenancy := range catalog.Tenancies {
			if _, exists := localTenancies[tenancy.Name]; !exists {
				local.TenancyList = append(local.TenancyList, tenancy)
				localTenancies[tenancy.Name] = tenancy
				log.Debug().Str("tenancy", tenancy.Name).Str("catalog", catalog.Name).Msg("Added tenancy from catalog")
			}
		}
	}

	return local
}

// RefreshCatalogs refreshes all catalog caches.
func (m *CatalogManager) RefreshCatalogs(ctx context.Context) error {
	// Clear cache
	if m.cacheDir != "" {
		files, _ := filepath.Glob(filepath.Join(m.cacheDir, "catalog-*.yaml"))
		for _, f := range files {
			os.Remove(f)
		}
	}

	// Fetch all
	_, err := m.FetchAll(ctx)
	return err
}

// ValidateCatalog validates a catalog file.
func ValidateCatalog(data []byte) (*SharedCatalog, error) {
	var catalog SharedCatalog
	if err := yaml.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	if catalog.Version == "" {
		return nil, fmt.Errorf("catalog version is required")
	}

	if catalog.Name == "" {
		return nil, fmt.Errorf("catalog name is required")
	}

	for i, cluster := range catalog.Clusters {
		if cluster.ClusterName == "" {
			return nil, fmt.Errorf("cluster %d: cluster_name is required", i)
		}
		if cluster.Region == "" && (catalog.Defaults == nil || catalog.Defaults.Region == "") {
			return nil, fmt.Errorf("cluster %s: region is required", cluster.ClusterName)
		}
	}

	return &catalog, nil
}

// GenerateSampleCatalog generates a sample catalog YAML.
func GenerateSampleCatalog() string {
	return `# Tunatap Shared Cluster Catalog
# Share this file with your team to provide a curated list of clusters

version: "1.0"
name: "team-catalog"
description: "Shared cluster catalog for the platform team"
maintainer: "platform-team@example.com"
updated: "2024-01-15T10:00:00Z"

# Default values applied to all clusters (can be overridden per-cluster)
defaults:
  region: "us-ashburn-1"
  bastion_type: "STANDARD"

# Shared tenancy configurations (optional)
tenancies:
  - name: "production"
    id: "ocid1.tenancy.oc1..example"
    namespace: "prod-namespace"

# Cluster definitions
clusters:
  - cluster_name: "prod-cluster"
    region: "us-ashburn-1"
    ocid: "ocid1.cluster.oc1.iad.example"
    tenant: "production"
    compartment: "platform/kubernetes"
    bastion_type: "STANDARD"
    endpoints:
      - name: "private"
        ip: "10.0.0.100"
        port: 6443
    url: "https://cloud.oracle.com/containers/clusters/ocid1.cluster.oc1.iad.example?region=us-ashburn-1"

  - cluster_name: "staging-cluster"
    region: "us-phoenix-1"
    ocid: "ocid1.cluster.oc1.phx.example"
    tenant: "production"
    compartment: "platform/kubernetes-staging"
    endpoints:
      - name: "private"
        ip: "10.1.0.100"
        port: 6443
`
}
