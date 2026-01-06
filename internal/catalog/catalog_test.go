package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/scotttball/tunatap/internal/config"
)

func TestNewCatalogManager(t *testing.T) {
	sources := []*config.CatalogSource{
		{Name: "test", URL: "https://example.com/catalog.yaml", Enabled: true},
	}

	manager := NewCatalogManager(sources, "/tmp/cache")

	if manager == nil {
		t.Fatal("NewCatalogManager returned nil")
	}

	if len(manager.sources) != 1 {
		t.Errorf("Expected 1 source, got %d", len(manager.sources))
	}
}

func TestFetchHTTPS(t *testing.T) {
	// Create test server
	catalogYAML := `
version: "1.0"
name: "test-catalog"
clusters:
  - cluster_name: "test-cluster"
    region: "us-ashburn-1"
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.Write([]byte(catalogYAML))
	}))
	defer server.Close()

	manager := NewCatalogManager(nil, "")

	ctx := context.Background()
	data, err := manager.fetchHTTPS(ctx, server.URL)
	if err != nil {
		t.Fatalf("fetchHTTPS error: %v", err)
	}

	if len(data) == 0 {
		t.Error("fetchHTTPS returned empty data")
	}
}

func TestFetchFile(t *testing.T) {
	// Create temp file
	tempDir := t.TempDir()
	catalogPath := filepath.Join(tempDir, "catalog.yaml")
	catalogYAML := `
version: "1.0"
name: "test-catalog"
`
	if err := os.WriteFile(catalogPath, []byte(catalogYAML), 0o600); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	manager := NewCatalogManager(nil, "")

	data, err := manager.fetchFile(catalogPath)
	if err != nil {
		t.Fatalf("fetchFile error: %v", err)
	}

	if len(data) == 0 {
		t.Error("fetchFile returned empty data")
	}
}

func TestDetectSourceType(t *testing.T) {
	manager := NewCatalogManager(nil, "")

	tests := []struct {
		source *config.CatalogSource
		want   string
	}{
		{
			source: &config.CatalogSource{URL: "https://example.com/catalog.yaml"},
			want:   "https",
		},
		{
			source: &config.CatalogSource{URL: "http://example.com/catalog.yaml"},
			want:   "https",
		},
		{
			source: &config.CatalogSource{URL: "file:///path/to/catalog.yaml"},
			want:   "file",
		},
		{
			source: &config.CatalogSource{URL: "/path/to/catalog.yaml"},
			want:   "file",
		},
		{
			source: &config.CatalogSource{OCIBucket: "my-bucket", OCIObject: "catalog.yaml"},
			want:   "oci",
		},
	}

	for _, tt := range tests {
		got := manager.detectSourceType(tt.source)
		if got != tt.want {
			t.Errorf("detectSourceType(%v) = %q, want %q", tt.source, got, tt.want)
		}
	}
}

func TestCachePath(t *testing.T) {
	manager := NewCatalogManager(nil, "/tmp/cache")

	source := &config.CatalogSource{Name: "test-catalog"}
	path := manager.cachePath(source)

	expected := "/tmp/cache/catalog-test-catalog.yaml"
	if path != expected {
		t.Errorf("cachePath = %q, want %q", path, expected)
	}
}

func TestCachePathWithSlash(t *testing.T) {
	manager := NewCatalogManager(nil, "/tmp/cache")

	source := &config.CatalogSource{Name: "team/test-catalog"}
	path := manager.cachePath(source)

	expected := "/tmp/cache/catalog-team_test-catalog.yaml"
	if path != expected {
		t.Errorf("cachePath = %q, want %q", path, expected)
	}
}

func TestSaveAndLoadCache(t *testing.T) {
	tempDir := t.TempDir()
	manager := NewCatalogManager(nil, tempDir)

	source := &config.CatalogSource{Name: "test"}
	data := []byte(`version: "1.0"
name: "test-catalog"
clusters: []
`)

	// Save to cache
	if err := manager.saveToCache(source, data); err != nil {
		t.Fatalf("saveToCache error: %v", err)
	}

	// Verify file exists
	cachePath := manager.cachePath(source)
	if _, err := os.Stat(cachePath); err != nil {
		t.Errorf("Cache file not created: %v", err)
	}

	// Load from cache
	catalog, err := manager.loadFromCache(source)
	if err != nil {
		t.Fatalf("loadFromCache error: %v", err)
	}

	if catalog.Name != "test-catalog" {
		t.Errorf("catalog.Name = %q, want %q", catalog.Name, "test-catalog")
	}
}

func TestCacheExpiry(t *testing.T) {
	tempDir := t.TempDir()
	manager := NewCatalogManager(nil, tempDir)
	manager.SetCacheTTL(1 * time.Millisecond) // Very short TTL

	source := &config.CatalogSource{Name: "test"}
	data := []byte(`version: "1.0"
name: "test-catalog"
clusters: []
`)

	// Save to cache
	if err := manager.saveToCache(source, data); err != nil {
		t.Fatalf("saveToCache error: %v", err)
	}

	// Wait for cache to expire
	time.Sleep(10 * time.Millisecond)

	// Load should fail due to expiry
	_, err := manager.loadFromCache(source)
	if err == nil {
		t.Error("Expected cache to be expired")
	}
}

func TestMergeCatalogs(t *testing.T) {
	local := &config.Config{
		Clusters: []*config.Cluster{
			{ClusterName: "local-cluster", Region: "us-ashburn-1"},
		},
	}

	catalogs := []*SharedCatalog{
		{
			Name: "test-catalog",
			Clusters: []*config.Cluster{
				{ClusterName: "remote-cluster", Region: "us-phoenix-1"},
				{ClusterName: "local-cluster", Region: "eu-frankfurt-1"}, // Should not override
			},
		},
	}

	merged := MergeCatalogs(local, catalogs)

	if len(merged.Clusters) != 2 {
		t.Errorf("Expected 2 clusters, got %d", len(merged.Clusters))
	}

	// Check that local cluster was not overwritten
	for _, c := range merged.Clusters {
		if c.ClusterName == "local-cluster" {
			if c.Region != "us-ashburn-1" {
				t.Errorf("Local cluster should not be overwritten, got region %q", c.Region)
			}
		}
	}
}

func TestMergeCatalogsNilLocal(t *testing.T) {
	catalogs := []*SharedCatalog{
		{
			Name: "test-catalog",
			Clusters: []*config.Cluster{
				{ClusterName: "remote-cluster", Region: "us-phoenix-1"},
			},
		},
	}

	merged := MergeCatalogs(nil, catalogs)

	if merged == nil {
		t.Fatal("MergeCatalogs returned nil")
	}

	if len(merged.Clusters) != 1 {
		t.Errorf("Expected 1 cluster, got %d", len(merged.Clusters))
	}
}

func TestApplyDefaults(t *testing.T) {
	catalog := &SharedCatalog{
		Defaults: &CatalogDefaults{
			Region:      "us-ashburn-1",
			BastionType: "STANDARD",
			LocalPort:   6443,
		},
		Clusters: []*config.Cluster{
			{ClusterName: "cluster1"},                         // Should get defaults
			{ClusterName: "cluster2", Region: "us-phoenix-1"}, // Should keep its region
		},
	}

	applyDefaults(catalog)

	if catalog.Clusters[0].Region != "us-ashburn-1" {
		t.Errorf("cluster1.Region = %q, want %q", catalog.Clusters[0].Region, "us-ashburn-1")
	}

	if catalog.Clusters[1].Region != "us-phoenix-1" {
		t.Errorf("cluster2.Region = %q, want %q", catalog.Clusters[1].Region, "us-phoenix-1")
	}

	if catalog.Clusters[0].BastionType == nil || *catalog.Clusters[0].BastionType != "STANDARD" {
		t.Error("cluster1.BastionType should be STANDARD")
	}

	if catalog.Clusters[0].LocalPort == nil || *catalog.Clusters[0].LocalPort != 6443 {
		t.Error("cluster1.LocalPort should be 6443")
	}
}

func TestValidateCatalog(t *testing.T) {
	validCatalog := []byte(`
version: "1.0"
name: "test-catalog"
clusters:
  - cluster_name: "test-cluster"
    region: "us-ashburn-1"
`)

	catalog, err := ValidateCatalog(validCatalog)
	if err != nil {
		t.Fatalf("ValidateCatalog error: %v", err)
	}

	if catalog.Name != "test-catalog" {
		t.Errorf("catalog.Name = %q, want %q", catalog.Name, "test-catalog")
	}
}

func TestValidateCatalogInvalid(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{
			name:    "missing version",
			data:    []byte(`name: "test"`),
			wantErr: "version is required",
		},
		{
			name:    "missing name",
			data:    []byte(`version: "1.0"`),
			wantErr: "name is required",
		},
		{
			name: "cluster missing name",
			data: []byte(`
version: "1.0"
name: "test"
clusters:
  - region: "us-ashburn-1"
`),
			wantErr: "cluster_name is required",
		},
		{
			name: "cluster missing region",
			data: []byte(`
version: "1.0"
name: "test"
clusters:
  - cluster_name: "test"
`),
			wantErr: "region is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateCatalog(tt.data)
			if err == nil {
				t.Error("Expected error")
			}
		})
	}
}

func TestGenerateSampleCatalog(t *testing.T) {
	sample := GenerateSampleCatalog()

	if sample == "" {
		t.Error("GenerateSampleCatalog returned empty string")
	}

	// Validate the sample
	_, err := ValidateCatalog([]byte(sample))
	if err != nil {
		t.Errorf("Sample catalog is invalid: %v", err)
	}
}
