package discovery

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewCache(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewCache(tmpDir, DefaultCacheTTL)
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}

	if cache == nil {
		t.Fatal("NewCache() returned nil")
	}

	expectedPath := filepath.Join(tmpDir, CacheFileName)
	if cache.Path() != expectedPath {
		t.Errorf("Path() = %q, want %q", cache.Path(), expectedPath)
	}
}

func TestCache_ClusterOperations(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewCache(tmpDir, DefaultCacheTTL)
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}

	// Test GetCluster with non-existent entry
	entry := cache.GetCluster("test-cluster")
	if entry != nil {
		t.Error("GetCluster() should return nil for non-existent cluster")
	}

	// Test SetCluster
	testEntry := &CacheEntry{
		OCID:            "ocid1.cluster.test",
		Region:          "us-phoenix-1",
		CompartmentOCID: "ocid1.compartment.test",
		VcnID:           "ocid1.vcn.test",
		SubnetID:        "ocid1.subnet.test",
		EndpointIP:      "10.0.1.100",
		EndpointPort:    6443,
	}

	if err := cache.SetCluster("test-cluster", testEntry); err != nil {
		t.Fatalf("SetCluster() error = %v", err)
	}

	// Test GetCluster with existing entry
	retrieved := cache.GetCluster("test-cluster")
	if retrieved == nil {
		t.Fatal("GetCluster() returned nil for existing cluster")
	}

	if retrieved.OCID != testEntry.OCID {
		t.Errorf("OCID = %q, want %q", retrieved.OCID, testEntry.OCID)
	}

	if retrieved.Region != testEntry.Region {
		t.Errorf("Region = %q, want %q", retrieved.Region, testEntry.Region)
	}

	// Verify CachedAt was set
	if retrieved.CachedAt.IsZero() {
		t.Error("CachedAt should be set")
	}
}

func TestCache_BastionOperations(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewCache(tmpDir, DefaultCacheTTL)
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}

	// Test GetBastion with non-existent entry
	entry := cache.GetBastion("test-cluster")
	if entry != nil {
		t.Error("GetBastion() should return nil for non-existent entry")
	}

	// Test SetBastion
	testEntry := &CacheEntry{
		OCID:            "ocid1.bastion.test",
		Region:          "us-phoenix-1",
		CompartmentOCID: "ocid1.compartment.test",
	}

	if err := cache.SetBastion("test-cluster", testEntry); err != nil {
		t.Fatalf("SetBastion() error = %v", err)
	}

	// Test GetBastion with existing entry
	retrieved := cache.GetBastion("test-cluster")
	if retrieved == nil {
		t.Fatal("GetBastion() returned nil for existing entry")
	}

	if retrieved.OCID != testEntry.OCID {
		t.Errorf("OCID = %q, want %q", retrieved.OCID, testEntry.OCID)
	}
}

func TestCache_Invalidate(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewCache(tmpDir, DefaultCacheTTL)
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}

	// Add entries
	cache.SetCluster("test-cluster", &CacheEntry{OCID: "cluster-ocid"})
	cache.SetBastion("test-cluster", &CacheEntry{OCID: "bastion-ocid"})

	// Invalidate
	if err := cache.Invalidate("test-cluster"); err != nil {
		t.Fatalf("Invalidate() error = %v", err)
	}

	// Verify both are removed
	if cache.GetCluster("test-cluster") != nil {
		t.Error("Cluster should be removed after Invalidate()")
	}

	if cache.GetBastion("test-cluster") != nil {
		t.Error("Bastion should be removed after Invalidate()")
	}
}

func TestCache_InvalidateAll(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewCache(tmpDir, DefaultCacheTTL)
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}

	// Add multiple entries
	cache.SetCluster("cluster-1", &CacheEntry{OCID: "ocid-1"})
	cache.SetCluster("cluster-2", &CacheEntry{OCID: "ocid-2"})
	cache.SetBastion("cluster-1", &CacheEntry{OCID: "bastion-1"})

	// Invalidate all
	if err := cache.InvalidateAll(); err != nil {
		t.Fatalf("InvalidateAll() error = %v", err)
	}

	// Verify all are removed
	if len(cache.GetAllClusters()) != 0 {
		t.Error("All clusters should be removed after InvalidateAll()")
	}
}

func TestCache_Expiration(t *testing.T) {
	tmpDir := t.TempDir()

	// Use a very short TTL
	shortTTL := 50 * time.Millisecond

	cache, err := NewCache(tmpDir, shortTTL)
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}

	// Add entry
	cache.SetCluster("test-cluster", &CacheEntry{OCID: "test-ocid"})

	// Should exist immediately
	if cache.GetCluster("test-cluster") == nil {
		t.Error("Entry should exist immediately after creation")
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Should be expired now
	if cache.GetCluster("test-cluster") != nil {
		t.Error("Entry should be nil after expiration")
	}
}

func TestCache_Persistence(t *testing.T) {
	tmpDir := t.TempDir()

	// Create cache and add entry
	cache1, err := NewCache(tmpDir, DefaultCacheTTL)
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}

	testEntry := &CacheEntry{
		OCID:   "ocid1.cluster.test",
		Region: "us-phoenix-1",
	}
	cache1.SetCluster("test-cluster", testEntry)

	// Create new cache instance (should load from file)
	cache2, err := NewCache(tmpDir, DefaultCacheTTL)
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}

	// Verify entry persisted
	retrieved := cache2.GetCluster("test-cluster")
	if retrieved == nil {
		t.Fatal("Entry should be loaded from persisted cache")
	}

	if retrieved.OCID != testEntry.OCID {
		t.Errorf("OCID = %q, want %q", retrieved.OCID, testEntry.OCID)
	}
}

func TestCache_GetClusterTTL(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewCache(tmpDir, 1*time.Hour)
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}

	// Non-existent entry should return 0
	if ttl := cache.GetClusterTTL("non-existent"); ttl != 0 {
		t.Errorf("GetClusterTTL() = %v for non-existent, want 0", ttl)
	}

	// Add entry
	cache.SetCluster("test-cluster", &CacheEntry{OCID: "test-ocid"})

	// TTL should be close to 1 hour
	ttl := cache.GetClusterTTL("test-cluster")
	if ttl < 59*time.Minute || ttl > 1*time.Hour {
		t.Errorf("GetClusterTTL() = %v, want ~1 hour", ttl)
	}
}

func TestCache_CleanExpired(t *testing.T) {
	tmpDir := t.TempDir()

	// Use a very short TTL
	shortTTL := 50 * time.Millisecond

	cache, err := NewCache(tmpDir, shortTTL)
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}

	// Add entries
	cache.SetCluster("cluster-1", &CacheEntry{OCID: "ocid-1"})
	cache.SetBastion("cluster-1", &CacheEntry{OCID: "bastion-1"})

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Clean expired
	if err := cache.CleanExpired(); err != nil {
		t.Fatalf("CleanExpired() error = %v", err)
	}

	// Verify file was updated (load new cache to check)
	cache2, _ := NewCache(tmpDir, shortTTL)
	if len(cache2.data.Clusters) != 0 || len(cache2.data.Bastions) != 0 {
		t.Error("Expired entries should be removed from persisted cache")
	}
}

func TestCache_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()

	cache, err := NewCache(tmpDir, DefaultCacheTTL)
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}

	// Run concurrent operations
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				cache.SetCluster("cluster", &CacheEntry{OCID: "ocid"})
				cache.GetCluster("cluster")
				cache.SetBastion("cluster", &CacheEntry{OCID: "bastion"})
				cache.GetBastion("cluster")
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// If we got here without deadlock or panic, concurrency is handled
}

func TestNewCache_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "nested", "path")

	cache, err := NewCache(nestedPath, DefaultCacheTTL)
	if err != nil {
		t.Fatalf("NewCache() error = %v", err)
	}

	// Add entry to trigger save
	cache.SetCluster("test", &CacheEntry{OCID: "test"})

	// Verify directory was created
	if _, err := os.Stat(nestedPath); os.IsNotExist(err) {
		t.Error("NewCache should create nested directory on save")
	}
}
