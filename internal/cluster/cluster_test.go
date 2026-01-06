package cluster

import (
	"testing"

	"github.com/scotttball/tunatap/internal/config"
	"github.com/scotttball/tunatap/pkg/utils"
)

func TestFindAvailablePort(t *testing.T) {
	// Find an available port starting from a high number
	port, err := FindAvailablePort(50000)
	if err != nil {
		t.Fatalf("FindAvailablePort() error = %v", err)
	}

	if port < 50000 {
		t.Errorf("FindAvailablePort(50000) = %d, should be >= 50000", port)
	}

	if port > 65535 {
		t.Errorf("FindAvailablePort(50000) = %d, should be <= 65535", port)
	}
}

func TestFindAvailablePortInvalidRange(t *testing.T) {
	// Test with a port that's too high
	_, err := FindAvailablePort(65536)
	if err == nil {
		t.Error("FindAvailablePort(65536) should error")
	}
}

func TestSetClusterLocalPort(t *testing.T) {
	cluster := &config.Cluster{}

	SetClusterLocalPort(cluster, 6443)

	if cluster.LocalPort == nil {
		t.Fatal("LocalPort should be set")
	}

	// Port should be >= 6443 (might be higher if 6443 is in use)
	if *cluster.LocalPort < 6443 {
		t.Errorf("LocalPort = %d, should be >= 6443", *cluster.LocalPort)
	}
}

func TestSetClusterLocalPortExisting(t *testing.T) {
	existingPort := 8080
	cluster := &config.Cluster{
		LocalPort: &existingPort,
	}

	// When localPort is 0 (auto), should use existing cluster config port
	SetClusterLocalPort(cluster, 0)

	if cluster.LocalPort == nil {
		t.Fatal("LocalPort should be set")
	}

	// Should try to use the existing port (8080)
	if *cluster.LocalPort < 8080 {
		t.Errorf("LocalPort = %d, should start from existing port 8080", *cluster.LocalPort)
	}
}

func TestSetClusterLocalPortExplicitOverride(t *testing.T) {
	existingPort := 8080
	cluster := &config.Cluster{
		LocalPort: &existingPort,
	}

	// When localPort is explicitly specified, it should override cluster config
	SetClusterLocalPort(cluster, 9090)

	if cluster.LocalPort == nil {
		t.Fatal("LocalPort should be set")
	}

	// Should try to use the explicit port (9090), not the cluster config (8080)
	if *cluster.LocalPort < 9090 {
		t.Errorf("LocalPort = %d, should start from explicit port 9090", *cluster.LocalPort)
	}
}

func TestSetClusterLocalPortAuto(t *testing.T) {
	cluster := &config.Cluster{}

	// When localPort is 0 (auto) and no cluster config, should find ephemeral port
	SetClusterLocalPort(cluster, 0)

	if cluster.LocalPort == nil {
		t.Fatal("LocalPort should be set")
	}

	// Ephemeral port should be in valid range
	if *cluster.LocalPort < 1024 || *cluster.LocalPort > 65535 {
		t.Errorf("LocalPort = %d, should be a valid ephemeral port", *cluster.LocalPort)
	}
}

func TestSetClusterURL(t *testing.T) {
	ocid := "ocid1.cluster.oc1.iad.test123"
	cluster := &config.Cluster{
		Region: "us-ashburn-1",
		Ocid:   &ocid,
	}

	SetClusterURL(cluster)

	if cluster.URL == nil {
		t.Fatal("URL should be set")
	}

	expected := "https://cloud.oracle.com/containers/clusters/ocid1.cluster.oc1.iad.test123?region=us-ashburn-1"
	if *cluster.URL != expected {
		t.Errorf("URL = %q, want %q", *cluster.URL, expected)
	}
}

func TestSetClusterURLExisting(t *testing.T) {
	existingURL := "https://custom.url.com"
	ocid := "ocid1.cluster.oc1.iad.test123"
	cluster := &config.Cluster{
		Region: "us-ashburn-1",
		Ocid:   &ocid,
		URL:    &existingURL,
	}

	SetClusterURL(cluster)

	// Should not overwrite existing URL
	if *cluster.URL != existingURL {
		t.Errorf("URL = %q, should not overwrite existing %q", *cluster.URL, existingURL)
	}
}

func TestListClusters(t *testing.T) {
	cfg := &config.Config{
		Clusters: []*config.Cluster{
			{ClusterName: "cluster1"},
			{ClusterName: "cluster2"},
		},
	}

	clusters := ListClusters(cfg)

	if len(clusters) != 2 {
		t.Errorf("ListClusters() returned %d clusters, want 2", len(clusters))
	}
}

func TestListClustersEmpty(t *testing.T) {
	cfg := &config.Config{
		Clusters: []*config.Cluster{},
	}

	clusters := ListClusters(cfg)

	if len(clusters) != 0 {
		t.Errorf("ListClusters() returned %d clusters, want 0", len(clusters))
	}
}

func TestGetClusterInfo(t *testing.T) {
	ocid := "ocid1.cluster.oc1.iad.test"
	tenant := "my-tenant"
	compartment := "my-compartment"
	bastionId := "ocid1.bastion.oc1.iad.test"
	bastionType := "STANDARD"
	localPort := 6443
	url := "https://example.com"

	cluster := &config.Cluster{
		ClusterName:     "test-cluster",
		Region:          "us-ashburn-1",
		Ocid:            &ocid,
		Tenant:          &tenant,
		Compartment:     &compartment,
		BastionId:       &bastionId,
		BastionType:     &bastionType,
		LocalPort:       &localPort,
		URL:             &url,
		Endpoints: []*config.ClusterEndpoint{
			{Name: "private", Ip: "10.0.0.1", Port: 6443},
		},
	}

	info := GetClusterInfo(cluster)

	if info["name"] != "test-cluster" {
		t.Errorf("info[name] = %v, want %q", info["name"], "test-cluster")
	}

	if info["region"] != "us-ashburn-1" {
		t.Errorf("info[region] = %v, want %q", info["region"], "us-ashburn-1")
	}

	if info["ocid"] != ocid {
		t.Errorf("info[ocid] = %v, want %q", info["ocid"], ocid)
	}

	if info["tenant"] != tenant {
		t.Errorf("info[tenant] = %v, want %q", info["tenant"], tenant)
	}

	endpoints, ok := info["endpoints"].([]map[string]interface{})
	if !ok || len(endpoints) != 1 {
		t.Error("info[endpoints] should have 1 endpoint")
	}
}

func TestGetClusterInfoMinimal(t *testing.T) {
	cluster := &config.Cluster{
		ClusterName: "minimal-cluster",
		Region:      "eu-frankfurt-1",
	}

	info := GetClusterInfo(cluster)

	if info["name"] != "minimal-cluster" {
		t.Errorf("info[name] = %v, want %q", info["name"], "minimal-cluster")
	}

	// Optional fields should not be present
	if _, ok := info["ocid"]; ok {
		t.Error("info should not have ocid for minimal cluster")
	}
}

func TestSetClusterTenancyNotFound(t *testing.T) {
	tenant := "nonexistent-tenant"
	cluster := &config.Cluster{
		Tenant: &tenant,
	}

	err := SetClusterTenancy(nil, nil, cluster)
	if err == nil {
		t.Error("SetClusterTenancy should error for non-existent tenant")
	}
}

func TestSetClusterTenancyNoTenant(t *testing.T) {
	cluster := &config.Cluster{}

	err := SetClusterTenancy(nil, nil, cluster)
	if err != nil {
		t.Errorf("SetClusterTenancy should not error when tenant is nil: %v", err)
	}
}

func TestSetClusterOcidMissingFields(t *testing.T) {
	cluster := &config.Cluster{}

	err := SetClusterOcid(nil, nil, cluster)
	if err == nil {
		t.Error("SetClusterOcid should error when required fields are missing")
	}
}

func TestSetClusterOcidWithOcid(t *testing.T) {
	ocid := "ocid1.cluster.oc1.iad.test"
	compartmentOcid := "ocid1.compartment.oc1..test"
	cluster := &config.Cluster{
		Ocid:            &ocid,
		CompartmentOcid: &compartmentOcid,
	}

	// Should not error when OCID and CompartmentOcid are already set
	err := SetClusterOcid(nil, nil, cluster)
	if err != nil {
		t.Errorf("SetClusterOcid should not error when OCID is set: %v", err)
	}
}

func TestGetClusterBastionNilCompartment(t *testing.T) {
	cluster := &config.Cluster{}

	_, err := GetClusterBastion(nil, nil, cluster)
	if err == nil {
		t.Error("GetClusterBastion should error when CompartmentOcid is nil")
	}
}

func TestAssignBastionByName(t *testing.T) {
	// This test requires OCI SDK types, which are harder to mock
	// Just verify the function exists and handles empty input
	t.Log("assignBastionByName test - requires OCI SDK mocking")
}

func TestValidateAndUpdateClusterMinimal(t *testing.T) {
	// This test requires an OCI client, so we can't fully test it
	// Just verify it handles nil inputs gracefully
	cluster := &config.Cluster{
		ClusterName: "test",
		Region:      "us-ashburn-1",
	}

	// This will fail without an OCI client, but shouldn't panic
	err := ValidateAndUpdateCluster(nil, nil, cluster, false, 6443)
	if err == nil {
		// If it doesn't error, that's fine too
		t.Log("ValidateAndUpdateCluster succeeded with nil client")
	} else {
		t.Logf("ValidateAndUpdateCluster failed as expected: %v", err)
	}

	// LocalPort should still be set
	if cluster.LocalPort == nil {
		t.Error("LocalPort should be set even if validation fails")
	}
}

func TestUtilsIntegration(t *testing.T) {
	// Verify utils package works with cluster package
	ptr := utils.StringPtr("test")
	if *ptr != "test" {
		t.Error("utils.StringPtr integration failed")
	}
}
