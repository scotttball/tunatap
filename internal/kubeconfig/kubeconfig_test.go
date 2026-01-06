package kubeconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewKubeconfig(t *testing.T) {
	k := NewKubeconfig()

	if k.APIVersion != "v1" {
		t.Errorf("APIVersion = %q, want %q", k.APIVersion, "v1")
	}

	if k.Kind != "Config" {
		t.Errorf("Kind = %q, want %q", k.Kind, "Config")
	}

	if len(k.Clusters) != 0 {
		t.Errorf("Clusters should be empty, got %d", len(k.Clusters))
	}
}

func TestAddCluster(t *testing.T) {
	k := NewKubeconfig()
	k.AddCluster("test-cluster", "https://localhost:6443", true)

	if len(k.Clusters) != 1 {
		t.Fatalf("Expected 1 cluster, got %d", len(k.Clusters))
	}

	cluster := k.Clusters[0]
	if cluster.Name != "test-cluster" {
		t.Errorf("Name = %q, want %q", cluster.Name, "test-cluster")
	}

	if cluster.Cluster.Server != "https://localhost:6443" {
		t.Errorf("Server = %q, want %q", cluster.Cluster.Server, "https://localhost:6443")
	}

	if !cluster.Cluster.InsecureSkipTLSVerify {
		t.Error("InsecureSkipTLSVerify should be true")
	}
}

func TestAddClusterWithCA(t *testing.T) {
	k := NewKubeconfig()
	k.AddClusterWithCA("test-cluster", "https://localhost:6443", "base64-ca-data")

	if len(k.Clusters) != 1 {
		t.Fatalf("Expected 1 cluster, got %d", len(k.Clusters))
	}

	cluster := k.Clusters[0]
	if cluster.Cluster.CertificateAuthorityData != "base64-ca-data" {
		t.Errorf("CertificateAuthorityData = %q, want %q", cluster.Cluster.CertificateAuthorityData, "base64-ca-data")
	}

	if cluster.Cluster.InsecureSkipTLSVerify {
		t.Error("InsecureSkipTLSVerify should be false when CA is provided")
	}
}

func TestAddContext(t *testing.T) {
	k := NewKubeconfig()
	k.AddContext("test-context", "test-cluster", "test-user")

	if len(k.Contexts) != 1 {
		t.Fatalf("Expected 1 context, got %d", len(k.Contexts))
	}

	ctx := k.Contexts[0]
	if ctx.Name != "test-context" {
		t.Errorf("Name = %q, want %q", ctx.Name, "test-context")
	}

	if ctx.Context.Cluster != "test-cluster" {
		t.Errorf("Cluster = %q, want %q", ctx.Context.Cluster, "test-cluster")
	}

	if ctx.Context.User != "test-user" {
		t.Errorf("User = %q, want %q", ctx.Context.User, "test-user")
	}
}

func TestAddContextWithNamespace(t *testing.T) {
	k := NewKubeconfig()
	k.AddContextWithNamespace("test-context", "test-cluster", "test-user", "default")

	if len(k.Contexts) != 1 {
		t.Fatalf("Expected 1 context, got %d", len(k.Contexts))
	}

	ctx := k.Contexts[0]
	if ctx.Context.Namespace != "default" {
		t.Errorf("Namespace = %q, want %q", ctx.Context.Namespace, "default")
	}
}

func TestAddUserWithToken(t *testing.T) {
	k := NewKubeconfig()
	k.AddUserWithToken("test-user", "my-token")

	if len(k.Users) != 1 {
		t.Fatalf("Expected 1 user, got %d", len(k.Users))
	}

	user := k.Users[0]
	if user.Name != "test-user" {
		t.Errorf("Name = %q, want %q", user.Name, "test-user")
	}

	if user.User.Token != "my-token" {
		t.Errorf("Token = %q, want %q", user.User.Token, "my-token")
	}
}

func TestAddUserWithExec(t *testing.T) {
	k := NewKubeconfig()
	k.AddUserWithExec("test-user", "my-command", []string{"arg1", "arg2"})

	if len(k.Users) != 1 {
		t.Fatalf("Expected 1 user, got %d", len(k.Users))
	}

	user := k.Users[0]
	if user.User.Exec == nil {
		t.Fatal("Exec should not be nil")
	}

	if user.User.Exec.Command != "my-command" {
		t.Errorf("Command = %q, want %q", user.User.Exec.Command, "my-command")
	}

	if len(user.User.Exec.Args) != 2 {
		t.Fatalf("Expected 2 args, got %d", len(user.User.Exec.Args))
	}
}

func TestAddOCIUser(t *testing.T) {
	k := NewKubeconfig()
	k.AddOCIUser("test-user", "ocid1.cluster.oc1.iad.test", "us-ashburn-1")

	if len(k.Users) != 1 {
		t.Fatalf("Expected 1 user, got %d", len(k.Users))
	}

	user := k.Users[0]
	if user.User.Exec == nil {
		t.Fatal("Exec should not be nil")
	}

	if user.User.Exec.Command != "oci" {
		t.Errorf("Command = %q, want %q", user.User.Exec.Command, "oci")
	}

	// Check args contain expected values
	argsStr := strings.Join(user.User.Exec.Args, " ")
	if !strings.Contains(argsStr, "ce cluster generate-token") {
		t.Errorf("Args should contain 'ce cluster generate-token', got %q", argsStr)
	}

	if !strings.Contains(argsStr, "--cluster-id ocid1.cluster.oc1.iad.test") {
		t.Errorf("Args should contain cluster-id, got %q", argsStr)
	}

	if !strings.Contains(argsStr, "--region us-ashburn-1") {
		t.Errorf("Args should contain region, got %q", argsStr)
	}
}

func TestAddOCIUserWithProfile(t *testing.T) {
	k := NewKubeconfig()
	k.AddOCIUserWithProfile("test-user", "ocid1.cluster.oc1.iad.test", "us-ashburn-1", "my-profile")

	user := k.Users[0]
	argsStr := strings.Join(user.User.Exec.Args, " ")
	if !strings.Contains(argsStr, "--profile my-profile") {
		t.Errorf("Args should contain profile, got %q", argsStr)
	}
}

func TestNewOCIKubeconfig(t *testing.T) {
	k := NewOCIKubeconfig(OCIKubeconfigOptions{
		ClusterName: "my-cluster",
		ClusterID:   "ocid1.cluster.oc1.iad.test",
		Region:      "us-ashburn-1",
		Endpoint:    "https://localhost:6443",
		Profile:     "test-profile",
	})

	// Check cluster
	if len(k.Clusters) != 1 {
		t.Fatalf("Expected 1 cluster, got %d", len(k.Clusters))
	}

	if k.Clusters[0].Name != "tuna-my-cluster" {
		t.Errorf("Cluster name = %q, want %q", k.Clusters[0].Name, "tuna-my-cluster")
	}

	// Check context
	if len(k.Contexts) != 1 {
		t.Fatalf("Expected 1 context, got %d", len(k.Contexts))
	}

	if k.Contexts[0].Name != "tuna-my-cluster" {
		t.Errorf("Context name = %q, want %q", k.Contexts[0].Name, "tuna-my-cluster")
	}

	// Check user has exec auth
	if len(k.Users) != 1 {
		t.Fatalf("Expected 1 user, got %d", len(k.Users))
	}

	if k.Users[0].User.Exec == nil {
		t.Fatal("User should have exec config")
	}

	if k.Users[0].User.Exec.Command != "oci" {
		t.Errorf("Exec command = %q, want %q", k.Users[0].User.Exec.Command, "oci")
	}

	// Check current context
	if k.CurrentContext != "tuna-my-cluster" {
		t.Errorf("CurrentContext = %q, want %q", k.CurrentContext, "tuna-my-cluster")
	}
}

func TestNewOCIKubeconfigWithCA(t *testing.T) {
	k := NewOCIKubeconfig(OCIKubeconfigOptions{
		ClusterName: "my-cluster",
		ClusterID:   "ocid1.cluster.oc1.iad.test",
		Region:      "us-ashburn-1",
		Endpoint:    "https://localhost:6443",
		CAData:      "base64-ca-data",
	})

	if k.Clusters[0].Cluster.CertificateAuthorityData != "base64-ca-data" {
		t.Errorf("CertificateAuthorityData = %q, want %q", k.Clusters[0].Cluster.CertificateAuthorityData, "base64-ca-data")
	}

	if k.Clusters[0].Cluster.InsecureSkipTLSVerify {
		t.Error("InsecureSkipTLSVerify should be false when CA is provided")
	}
}

func TestNewOCIKubeconfigForTunnel(t *testing.T) {
	k := NewOCIKubeconfigForTunnel("my-cluster", "ocid1.cluster.oc1.iad.test", "us-ashburn-1", 8443, "")

	if k.Clusters[0].Cluster.Server != "https://localhost:8443" {
		t.Errorf("Server = %q, want %q", k.Clusters[0].Cluster.Server, "https://localhost:8443")
	}
}

func TestNewInsecureKubeconfig(t *testing.T) {
	k := NewInsecureKubeconfig("my-cluster", 6443)

	if len(k.Clusters) != 1 {
		t.Fatalf("Expected 1 cluster, got %d", len(k.Clusters))
	}

	if k.Clusters[0].Name != "tuna-my-cluster" {
		t.Errorf("Cluster name = %q, want %q", k.Clusters[0].Name, "tuna-my-cluster")
	}

	if !k.Clusters[0].Cluster.InsecureSkipTLSVerify {
		t.Error("InsecureSkipTLSVerify should be true")
	}

	// Should have no users
	if len(k.Users) != 0 {
		t.Errorf("Expected 0 users, got %d", len(k.Users))
	}
}

func TestToYAML(t *testing.T) {
	k := NewKubeconfig()
	k.AddCluster("test", "https://localhost:6443", true)
	k.AddContext("test", "test", "")
	k.SetCurrentContext("test")

	yaml, err := k.ToYAML()
	if err != nil {
		t.Fatalf("ToYAML() error = %v", err)
	}

	if !strings.Contains(yaml, "apiVersion: v1") {
		t.Error("YAML should contain apiVersion")
	}

	if !strings.Contains(yaml, "kind: Config") {
		t.Error("YAML should contain kind")
	}

	if !strings.Contains(yaml, "current-context: test") {
		t.Error("YAML should contain current-context")
	}
}

func TestWriteAndLoadFromFile(t *testing.T) {
	k := NewKubeconfig()
	k.AddCluster("test", "https://localhost:6443", true)
	k.AddContext("test", "test", "test-user")
	k.AddUserWithToken("test-user", "token123")
	k.SetCurrentContext("test")

	// Create temp file
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "kubeconfig.yaml")

	// Write
	if err := k.WriteToFile(path); err != nil {
		t.Fatalf("WriteToFile() error = %v", err)
	}

	// Verify file exists with correct permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	if info.Mode().Perm() != 0600 {
		t.Errorf("File permissions = %v, want %v", info.Mode().Perm(), 0600)
	}

	// Load
	loaded, err := LoadFromFile(path)
	if err != nil {
		t.Fatalf("LoadFromFile() error = %v", err)
	}

	if loaded.CurrentContext != k.CurrentContext {
		t.Errorf("CurrentContext = %q, want %q", loaded.CurrentContext, k.CurrentContext)
	}

	if len(loaded.Clusters) != len(k.Clusters) {
		t.Errorf("Clusters count = %d, want %d", len(loaded.Clusters), len(k.Clusters))
	}

	if len(loaded.Users) != len(k.Users) {
		t.Errorf("Users count = %d, want %d", len(loaded.Users), len(k.Users))
	}
}

func TestLoadFromFileNotFound(t *testing.T) {
	_, err := LoadFromFile("/nonexistent/path/kubeconfig.yaml")
	if err == nil {
		t.Error("LoadFromFile() should error for nonexistent file")
	}
}

func TestMergeKubeconfigs(t *testing.T) {
	k1 := NewKubeconfig()
	k1.AddCluster("cluster1", "https://server1:6443", true)
	k1.AddContext("ctx1", "cluster1", "user1")
	k1.AddUserWithToken("user1", "token1")
	k1.SetCurrentContext("ctx1")

	k2 := NewKubeconfig()
	k2.AddCluster("cluster2", "https://server2:6443", true)
	k2.AddContext("ctx2", "cluster2", "user2")
	k2.AddUserWithToken("user2", "token2")
	k2.SetCurrentContext("ctx2")

	merged := MergeKubeconfigs(k1, k2)

	if len(merged.Clusters) != 2 {
		t.Errorf("Merged clusters = %d, want 2", len(merged.Clusters))
	}

	if len(merged.Contexts) != 2 {
		t.Errorf("Merged contexts = %d, want 2", len(merged.Contexts))
	}

	if len(merged.Users) != 2 {
		t.Errorf("Merged users = %d, want 2", len(merged.Users))
	}

	// First non-empty current context is used
	if merged.CurrentContext != "ctx1" {
		t.Errorf("CurrentContext = %q, want %q", merged.CurrentContext, "ctx1")
	}
}

func TestMergeKubeconfigsEmpty(t *testing.T) {
	k1 := NewKubeconfig()
	k2 := NewKubeconfig()
	k2.SetCurrentContext("ctx2")

	merged := MergeKubeconfigs(k1, k2)

	// Should use ctx2 since k1 has no current context
	if merged.CurrentContext != "ctx2" {
		t.Errorf("CurrentContext = %q, want %q", merged.CurrentContext, "ctx2")
	}
}
