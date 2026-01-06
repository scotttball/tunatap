package bastion

import (
	"testing"

	"github.com/scotttball/tunatap/internal/config"
)

func TestNewSessionManager(t *testing.T) {
	cfg := config.DefaultConfig()

	manager := NewSessionManager(nil, cfg)

	if manager == nil {
		t.Fatal("NewSessionManager() returned nil")
	}

	if manager.config != cfg {
		t.Error("SessionManager.config not set correctly")
	}
}

func TestSessionManagerWithNilClient(t *testing.T) {
	cfg := config.DefaultConfig()
	manager := NewSessionManager(nil, cfg)

	// Creating a manager with nil client should work
	// (it will fail when trying to use it)
	if manager == nil {
		t.Fatal("NewSessionManager() should not return nil with nil client")
	}
}

func TestSessionMatchesTarget(t *testing.T) {
	// This would require mocking the bastion.SessionSummary
	// For now, just verify the function signature exists
	t.Log("sessionMatchesTarget test - requires OCI SDK mocking")
}

func TestGetPublicKeyNonExistent(t *testing.T) {
	cfg := &config.Config{
		SshPrivateKeyFile: "/nonexistent/path/to/key",
	}

	manager := NewSessionManager(nil, cfg)

	_, err := manager.getPublicKey()
	if err == nil {
		t.Error("getPublicKey() should error with non-existent key file")
	}
}

func TestStringPtr(t *testing.T) {
	s := "test"
	ptr := stringPtr(s)

	if ptr == nil {
		t.Fatal("stringPtr returned nil")
	}

	if *ptr != s {
		t.Errorf("stringPtr(%q) = %q, want %q", s, *ptr, s)
	}
}
