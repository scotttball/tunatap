package utils

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// HomeDir returns the user's home directory.
func HomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback for older systems
		if runtime.GOOS == "windows" {
			home = os.Getenv("USERPROFILE")
			if home == "" {
				home = os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
			}
		} else {
			home = os.Getenv("HOME")
		}
		if home == "" {
			return "", err
		}
	}
	return home, nil
}

// ExpandPath expands ~ to home directory and normalizes path separators.
func ExpandPath(path string) string {
	if path == "" {
		return path
	}

	// Expand ~
	if strings.HasPrefix(path, "~") {
		home, err := HomeDir()
		if err == nil {
			path = filepath.Join(home, path[1:])
		}
	}

	// Normalize path separators for the current OS
	return filepath.Clean(path)
}

// DefaultSSHDir returns the default SSH directory for the current OS.
func DefaultSSHDir() string {
	home, err := HomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh")
}

// DefaultSSHPrivateKey returns the default SSH private key path.
func DefaultSSHPrivateKey() string {
	return filepath.Join(DefaultSSHDir(), "id_rsa")
}

// DefaultSSHKnownHosts returns the default known_hosts file path.
func DefaultSSHKnownHosts() string {
	return filepath.Join(DefaultSSHDir(), "known_hosts")
}

// DefaultOCIConfigDir returns the default OCI config directory.
func DefaultOCIConfigDir() string {
	home, err := HomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".oci")
}

// DefaultOCIConfigPath returns the default OCI config file path.
func DefaultOCIConfigPath() string {
	return filepath.Join(DefaultOCIConfigDir(), "config")
}

// DefaultTunatapDir returns the default tunatap config directory.
func DefaultTunatapDir() string {
	home, err := HomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".tunatap")
}

// DefaultTunatapConfigPath returns the default tunatap config file path.
func DefaultTunatapConfigPath() string {
	return filepath.Join(DefaultTunatapDir(), "config.yaml")
}

// JoinPath joins path elements using the correct separator for the current OS.
func JoinPath(elem ...string) string {
	return filepath.Join(elem...)
}

// IsWindows returns true if running on Windows.
func IsWindows() bool {
	return runtime.GOOS == "windows"
}

// IsMac returns true if running on macOS.
func IsMac() bool {
	return runtime.GOOS == "darwin"
}

// IsLinux returns true if running on Linux.
func IsLinux() bool {
	return runtime.GOOS == "linux"
}
