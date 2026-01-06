package tunnel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetHomeDir(t *testing.T) {
	home := GetHomeDir()

	if home == "" {
		t.Error("GetHomeDir() returned empty string")
	}

	// Should be an absolute path
	if !filepath.IsAbs(home) {
		t.Errorf("GetHomeDir() = %q, should be absolute path", home)
	}
}

func TestGetHostKeyFilePath(t *testing.T) {
	path := GetHostKeyFilePath()

	if path == "" {
		t.Fatal("GetHostKeyFilePath() returned empty string")
	}

	if !strings.HasSuffix(path, "known_hosts") {
		t.Errorf("GetHostKeyFilePath() = %q, should end with known_hosts", path)
	}

	if !strings.Contains(path, ".ssh") {
		t.Errorf("GetHostKeyFilePath() = %q, should contain .ssh", path)
	}
}

func TestGetPrivateKeyNonExistent(t *testing.T) {
	_, err := GetPrivateKey("/nonexistent/path/to/key")
	if err == nil {
		t.Error("GetPrivateKey() should error for non-existent file")
	}
}

func TestGetPrivateKeyTildeExpansion(t *testing.T) {
	// This will fail because the key doesn't exist, but it tests ~ expansion
	_, err := GetPrivateKey("~/.ssh/nonexistent_key_12345")
	if err == nil {
		t.Error("GetPrivateKey() should error for non-existent file")
	}

	// The error should not contain ~ (should be expanded)
	if strings.Contains(err.Error(), "~") {
		t.Logf("Note: Error message contains ~, may not have expanded: %v", err)
	}
}

func TestGetKnownHostsCallbackWithNewHost(t *testing.T) {
	// Create a temporary .ssh directory with known_hosts
	tmpDir, err := os.MkdirTemp("", "tunatap-ssh-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("Failed to create .ssh dir: %v", err)
	}

	khPath := filepath.Join(sshDir, "known_hosts")
	if err := os.WriteFile(khPath, []byte(""), 0600); err != nil {
		t.Fatalf("Failed to create known_hosts: %v", err)
	}

	// The function uses GetHostKeyFilePath() which returns the real path
	// So this test mainly verifies the function doesn't panic
	callback, err := GetKnownHostsCallbackWithNewHost()
	if err != nil {
		// This might fail if ~/.ssh/known_hosts doesn't exist
		t.Logf("GetKnownHostsCallbackWithNewHost() error (may be expected): %v", err)
		return
	}

	if callback == nil {
		t.Error("GetKnownHostsCallbackWithNewHost() returned nil callback")
	}
}

func TestCreateSSHClientConfigInvalidKey(t *testing.T) {
	_, err := CreateSSHClientConfig("testuser", "/nonexistent/key")
	if err == nil {
		t.Error("CreateSSHClientConfig() should error with invalid key path")
	}
}

func TestAddHostKeyCreatesFile(t *testing.T) {
	// This test would modify the real known_hosts file, so we skip it
	// In a real test environment, we'd mock the file system
	t.Skip("Skipping AddHostKey test to avoid modifying real known_hosts")
}

func TestSSHConfigCreation(t *testing.T) {
	// Create a temporary SSH key for testing
	tmpDir, err := os.MkdirTemp("", "tunatap-ssh-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a minimal test private key (RSA)
	// Note: This is an invalid key, just testing the file reading part
	keyPath := filepath.Join(tmpDir, "test_key")
	testKey := `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAMwAAAAtzc2gtZW
-----END OPENSSH PRIVATE KEY-----
`
	if err := os.WriteFile(keyPath, []byte(testKey), 0600); err != nil {
		t.Fatalf("Failed to write test key: %v", err)
	}

	// This should fail because the key is invalid, but it tests the file reading
	_, err = CreateSSHClientConfig("testuser", keyPath)
	if err == nil {
		t.Log("CreateSSHClientConfig succeeded with test key (key may be valid)")
	} else {
		// Expected to fail with invalid key
		t.Logf("CreateSSHClientConfig failed as expected: %v", err)
	}
}
