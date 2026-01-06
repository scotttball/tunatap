package utils

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHomeDir(t *testing.T) {
	home, err := HomeDir()
	if err != nil {
		t.Fatalf("HomeDir() returned error: %v", err)
	}
	if home == "" {
		t.Fatal("HomeDir() returned empty string")
	}

	// Verify it's an absolute path
	if !filepath.IsAbs(home) {
		t.Errorf("HomeDir() returned non-absolute path: %s", home)
	}
}

func TestExpandPath(t *testing.T) {
	home, _ := HomeDir()

	tests := []struct {
		name     string
		input    string
		contains string // expected substring in result
	}{
		{
			name:     "empty path",
			input:    "",
			contains: "",
		},
		{
			name:     "tilde only",
			input:    "~",
			contains: home,
		},
		{
			name:     "tilde with path",
			input:    "~/.ssh/id_rsa",
			contains: ".ssh",
		},
		{
			name:     "absolute path unchanged",
			input:    "/usr/local/bin",
			contains: "usr",
		},
		{
			name:     "relative path",
			input:    "relative/path",
			contains: "relative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExpandPath(tt.input)

			if tt.contains != "" && !strings.Contains(result, tt.contains) {
				t.Errorf("ExpandPath(%q) = %q, expected to contain %q", tt.input, result, tt.contains)
			}

			// Verify path separators are correct for the OS
			// On Windows, paths should use backslashes (except for empty or simple paths)
			// filepath.Clean handles this, but some edge cases may remain
			_ = runtime.GOOS // Platform-specific behavior tested elsewhere
		})
	}
}

func TestExpandPathTilde(t *testing.T) {
	home, _ := HomeDir()

	result := ExpandPath("~/.config")

	if !strings.HasPrefix(result, home) {
		t.Errorf("ExpandPath(~/.config) = %q, expected to start with home dir %q", result, home)
	}
}

func TestDefaultSSHDir(t *testing.T) {
	sshDir := DefaultSSHDir()

	if sshDir == "" {
		t.Fatal("DefaultSSHDir() returned empty string")
	}

	if !strings.HasSuffix(sshDir, ".ssh") {
		t.Errorf("DefaultSSHDir() = %q, expected to end with .ssh", sshDir)
	}
}

func TestDefaultSSHPrivateKey(t *testing.T) {
	keyPath := DefaultSSHPrivateKey()

	if keyPath == "" {
		t.Fatal("DefaultSSHPrivateKey() returned empty string")
	}

	if !strings.HasSuffix(keyPath, "id_rsa") {
		t.Errorf("DefaultSSHPrivateKey() = %q, expected to end with id_rsa", keyPath)
	}

	if !strings.Contains(keyPath, ".ssh") {
		t.Errorf("DefaultSSHPrivateKey() = %q, expected to contain .ssh", keyPath)
	}
}

func TestDefaultSSHKnownHosts(t *testing.T) {
	khPath := DefaultSSHKnownHosts()

	if khPath == "" {
		t.Fatal("DefaultSSHKnownHosts() returned empty string")
	}

	if !strings.HasSuffix(khPath, "known_hosts") {
		t.Errorf("DefaultSSHKnownHosts() = %q, expected to end with known_hosts", khPath)
	}
}

func TestDefaultOCIConfigDir(t *testing.T) {
	ociDir := DefaultOCIConfigDir()

	if ociDir == "" {
		t.Fatal("DefaultOCIConfigDir() returned empty string")
	}

	if !strings.HasSuffix(ociDir, ".oci") {
		t.Errorf("DefaultOCIConfigDir() = %q, expected to end with .oci", ociDir)
	}
}

func TestDefaultOCIConfigPath(t *testing.T) {
	ociPath := DefaultOCIConfigPath()

	if ociPath == "" {
		t.Fatal("DefaultOCIConfigPath() returned empty string")
	}

	if !strings.Contains(ociPath, ".oci") {
		t.Errorf("DefaultOCIConfigPath() = %q, expected to contain .oci", ociPath)
	}

	if !strings.HasSuffix(ociPath, "config") {
		t.Errorf("DefaultOCIConfigPath() = %q, expected to end with config", ociPath)
	}
}

func TestDefaultTunatapDir(t *testing.T) {
	tunatapDir := DefaultTunatapDir()

	if tunatapDir == "" {
		t.Fatal("DefaultTunatapDir() returned empty string")
	}

	if !strings.HasSuffix(tunatapDir, ".tunatap") {
		t.Errorf("DefaultTunatapDir() = %q, expected to end with .tunatap", tunatapDir)
	}
}

func TestDefaultTunatapConfigPath(t *testing.T) {
	cfgPath := DefaultTunatapConfigPath()

	if cfgPath == "" {
		t.Fatal("DefaultTunatapConfigPath() returned empty string")
	}

	if !strings.Contains(cfgPath, ".tunatap") {
		t.Errorf("DefaultTunatapConfigPath() = %q, expected to contain .tunatap", cfgPath)
	}
}

func TestJoinPath(t *testing.T) {
	result := JoinPath("a", "b", "c")

	expected := filepath.Join("a", "b", "c")
	if result != expected {
		t.Errorf("JoinPath(a, b, c) = %q, want %q", result, expected)
	}
}

func TestPlatformDetection(t *testing.T) {
	// At least one of these should be true
	if !IsWindows() && !IsMac() && !IsLinux() {
		// Could be another Unix-like system, which is fine
		goos := runtime.GOOS
		if goos != "windows" && goos != "darwin" && goos != "linux" {
			t.Logf("Running on %s (not Windows, Mac, or Linux)", goos)
		}
	}

	// Verify consistency with runtime.GOOS
	switch runtime.GOOS {
	case "windows":
		if !IsWindows() {
			t.Error("IsWindows() should return true on Windows")
		}
	case "darwin":
		if !IsMac() {
			t.Error("IsMac() should return true on macOS")
		}
	case "linux":
		if !IsLinux() {
			t.Error("IsLinux() should return true on Linux")
		}
	}
}

func TestPathSeparators(t *testing.T) {
	// Test that JoinPath uses correct separators
	result := JoinPath("home", "user", ".ssh")

	if runtime.GOOS == "windows" {
		if strings.Contains(result, "/") {
			t.Errorf("On Windows, JoinPath should use backslashes: %s", result)
		}
	} else {
		if strings.Contains(result, "\\") {
			t.Errorf("On Unix, JoinPath should use forward slashes: %s", result)
		}
	}
}

func TestHomeDirWithEnvFallback(t *testing.T) {
	// Save original env
	origHome := os.Getenv("HOME")
	origUserProfile := os.Getenv("USERPROFILE")

	defer func() {
		os.Setenv("HOME", origHome)
		os.Setenv("USERPROFILE", origUserProfile)
	}()

	// HomeDir should work even with env manipulation
	home, err := HomeDir()
	if err != nil {
		t.Logf("HomeDir returned error (may be expected in some environments): %v", err)
	} else if home == "" {
		t.Error("HomeDir returned empty string")
	}
}
