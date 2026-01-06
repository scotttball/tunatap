package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	if cfg.Tenancies == nil {
		t.Error("DefaultConfig().Tenancies should not be nil")
	}

	if cfg.Clusters == nil {
		t.Error("DefaultConfig().Clusters should not be nil")
	}

	// Check defaults
	if cfg.GetPoolSize() != 5 {
		t.Errorf("DefaultConfig().GetPoolSize() = %d, want 5", cfg.GetPoolSize())
	}

	if cfg.GetWarmupCount() != 2 {
		t.Errorf("DefaultConfig().GetWarmupCount() = %d, want 2", cfg.GetWarmupCount())
	}

	if cfg.GetMaxConcurrent() != 10 {
		t.Errorf("DefaultConfig().GetMaxConcurrent() = %d, want 10", cfg.GetMaxConcurrent())
	}
}

func TestConfigGetters(t *testing.T) {
	cfg := &Config{}

	// Test with nil values (should return defaults)
	if cfg.GetPoolSize() != 5 {
		t.Errorf("GetPoolSize() with nil = %d, want 5", cfg.GetPoolSize())
	}

	if cfg.GetWarmupCount() != 2 {
		t.Errorf("GetWarmupCount() with nil = %d, want 2", cfg.GetWarmupCount())
	}

	if cfg.GetMaxConcurrent() != 10 {
		t.Errorf("GetMaxConcurrent() with nil = %d, want 10", cfg.GetMaxConcurrent())
	}

	// Test with custom values
	poolSize := 10
	warmupCount := 5
	maxConcurrent := 20

	cfg.SshConnectionPoolSize = &poolSize
	cfg.SshConnectionWarmupCount = &warmupCount
	cfg.SshConnectionMaxConcurrentUse = &maxConcurrent

	if cfg.GetPoolSize() != poolSize {
		t.Errorf("GetPoolSize() = %d, want %d", cfg.GetPoolSize(), poolSize)
	}

	if cfg.GetWarmupCount() != warmupCount {
		t.Errorf("GetWarmupCount() = %d, want %d", cfg.GetWarmupCount(), warmupCount)
	}

	if cfg.GetMaxConcurrent() != maxConcurrent {
		t.Errorf("GetMaxConcurrent() = %d, want %d", cfg.GetMaxConcurrent(), maxConcurrent)
	}
}

func TestClusterStruct(t *testing.T) {
	cluster := &Cluster{
		ClusterName: "test-cluster",
		Region:      "us-ashburn-1",
	}

	if cluster.ClusterName != "test-cluster" {
		t.Errorf("ClusterName = %q, want %q", cluster.ClusterName, "test-cluster")
	}

	if cluster.Region != "us-ashburn-1" {
		t.Errorf("Region = %q, want %q", cluster.Region, "us-ashburn-1")
	}
}

func TestClusterEndpointStruct(t *testing.T) {
	endpoint := &ClusterEndpoint{
		Name: "private",
		Ip:   "10.0.0.1",
		Port: 6443,
	}

	if endpoint.Name != "private" {
		t.Errorf("Name = %q, want %q", endpoint.Name, "private")
	}

	if endpoint.Ip != "10.0.0.1" {
		t.Errorf("Ip = %q, want %q", endpoint.Ip, "10.0.0.1")
	}

	if endpoint.Port != 6443 {
		t.Errorf("Port = %d, want %d", endpoint.Port, 6443)
	}
}

func TestRemoteConfigStruct(t *testing.T) {
	rc := &RemoteConfig{
		Region:      "us-ashburn-1",
		TenancyOcid: "ocid1.tenancy.oc1..test",
		Bucket:      "config-bucket",
		Object:      "config.yaml",
	}

	if rc.Region != "us-ashburn-1" {
		t.Errorf("Region = %q, want %q", rc.Region, "us-ashburn-1")
	}

	if rc.Bucket != "config-bucket" {
		t.Errorf("Bucket = %q, want %q", rc.Bucket, "config-bucket")
	}
}

func TestReadConfigNonExistent(t *testing.T) {
	// Reading non-existent file should return default config
	cfg, err := ReadConfig("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("ReadConfig() error = %v, want nil for non-existent file", err)
	}

	if cfg == nil {
		t.Fatal("ReadConfig() returned nil for non-existent file")
	}

	// Should have defaults
	if cfg.GetPoolSize() != 5 {
		t.Errorf("Default pool size = %d, want 5", cfg.GetPoolSize())
	}
}

func TestReadConfigEmpty(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "tunatap-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create empty config file
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create empty config: %v", err)
	}

	cfg, err := ReadConfig(cfgPath)
	if err != nil {
		t.Fatalf("ReadConfig() error = %v, want nil for empty file", err)
	}

	if cfg == nil {
		t.Fatal("ReadConfig() returned nil for empty file")
	}
}

func TestReadConfigValid(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "tunatap-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create valid config file
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `
ssh_private_key_file: ~/.ssh/id_rsa
ssh_connection_pool_size: 8
clusters:
  - cluster_name: test-cluster
    region: us-ashburn-1
    endpoints:
      - ip: 10.0.0.1
        port: 6443
`
	if err := os.WriteFile(cfgPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	cfg, err := ReadConfig(cfgPath)
	if err != nil {
		t.Fatalf("ReadConfig() error = %v", err)
	}

	if cfg.SshPrivateKeyFile != "~/.ssh/id_rsa" {
		t.Errorf("SshPrivateKeyFile = %q, want %q", cfg.SshPrivateKeyFile, "~/.ssh/id_rsa")
	}

	if cfg.GetPoolSize() != 8 {
		t.Errorf("GetPoolSize() = %d, want 8", cfg.GetPoolSize())
	}

	if len(cfg.Clusters) != 1 {
		t.Fatalf("len(Clusters) = %d, want 1", len(cfg.Clusters))
	}

	if cfg.Clusters[0].ClusterName != "test-cluster" {
		t.Errorf("ClusterName = %q, want %q", cfg.Clusters[0].ClusterName, "test-cluster")
	}
}

func TestSaveConfig(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "tunatap-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "subdir", "config.yaml")

	cfg := DefaultConfig()
	cfg.SshPrivateKeyFile = "~/.ssh/test_key"
	cfg.Clusters = append(cfg.Clusters, &Cluster{
		ClusterName: "saved-cluster",
		Region:      "eu-frankfurt-1",
	})

	err = SaveConfig(cfgPath, cfg)
	if err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Error("SaveConfig() did not create the file")
	}

	// Read it back
	loadedCfg, err := ReadConfig(cfgPath)
	if err != nil {
		t.Fatalf("ReadConfig() after save error = %v", err)
	}

	if loadedCfg.SshPrivateKeyFile != cfg.SshPrivateKeyFile {
		t.Errorf("Loaded SshPrivateKeyFile = %q, want %q", loadedCfg.SshPrivateKeyFile, cfg.SshPrivateKeyFile)
	}

	if len(loadedCfg.Clusters) != 1 {
		t.Fatalf("Loaded len(Clusters) = %d, want 1", len(loadedCfg.Clusters))
	}

	if loadedCfg.Clusters[0].ClusterName != "saved-cluster" {
		t.Errorf("Loaded ClusterName = %q, want %q", loadedCfg.Clusters[0].ClusterName, "saved-cluster")
	}
}

func TestFindClusterByName(t *testing.T) {
	cfg := &Config{
		Clusters: []*Cluster{
			{ClusterName: "cluster-1", Region: "us-ashburn-1"},
			{ClusterName: "Cluster-2", Region: "eu-frankfurt-1"},
			{ClusterName: "CLUSTER-3", Region: "ap-tokyo-1"},
		},
	}

	tests := []struct {
		name      string
		search    string
		wantFound bool
		wantName  string
	}{
		{"exact match", "cluster-1", true, "cluster-1"},
		{"case insensitive", "CLUSTER-1", true, "cluster-1"},
		{"mixed case", "cLuStEr-2", true, "Cluster-2"},
		{"not found", "cluster-4", false, ""},
		{"empty search", "", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FindClusterByName(cfg, tt.search)

			if tt.wantFound {
				if got == nil {
					t.Errorf("FindClusterByName(%q) returned nil, want cluster", tt.search)
				} else if got.ClusterName != tt.wantName {
					t.Errorf("FindClusterByName(%q).ClusterName = %q, want %q", tt.search, got.ClusterName, tt.wantName)
				}
			} else {
				if got != nil {
					t.Errorf("FindClusterByName(%q) = %v, want nil", tt.search, got)
				}
			}
		})
	}
}

func TestGetClusterEndpoint(t *testing.T) {
	cluster := &Cluster{
		ClusterName: "test",
		Endpoints: []*ClusterEndpoint{
			{Name: "private", Ip: "10.0.0.1", Port: 6443},
			{Name: "public", Ip: "203.0.113.1", Port: 6443},
		},
	}

	tests := []struct {
		name     string
		epName   string
		wantIp   string
		wantNil  bool
	}{
		{"first endpoint by default", "", "10.0.0.1", false},
		{"by name private", "private", "10.0.0.1", false},
		{"by name public", "public", "203.0.113.1", false},
		{"case insensitive", "PRIVATE", "10.0.0.1", false},
		{"not found returns first", "nonexistent", "10.0.0.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetClusterEndpoint(cluster, tt.epName)

			if tt.wantNil {
				if got != nil {
					t.Errorf("GetClusterEndpoint(%q) = %v, want nil", tt.epName, got)
				}
			} else {
				if got == nil {
					t.Errorf("GetClusterEndpoint(%q) returned nil", tt.epName)
				} else if got.Ip != tt.wantIp {
					t.Errorf("GetClusterEndpoint(%q).Ip = %q, want %q", tt.epName, got.Ip, tt.wantIp)
				}
			}
		})
	}
}

func TestGetClusterEndpointNoEndpoints(t *testing.T) {
	cluster := &Cluster{
		ClusterName: "test",
		Endpoints:   []*ClusterEndpoint{},
	}

	got := GetClusterEndpoint(cluster, "")
	if got != nil {
		t.Errorf("GetClusterEndpoint with no endpoints = %v, want nil", got)
	}
}

func TestGetDefaultConfigPath(t *testing.T) {
	path, err := GetDefaultConfigPath()
	if err != nil {
		t.Fatalf("GetDefaultConfigPath() error = %v", err)
	}

	if path == "" {
		t.Error("GetDefaultConfigPath() returned empty string")
	}

	// Should contain .tunatap
	if !filepath.IsAbs(path) {
		t.Errorf("GetDefaultConfigPath() = %q, should be absolute path", path)
	}
}
