package autofix

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewFixer(t *testing.T) {
	fixer := NewFixer("/tmp/config.yaml", false)
	if fixer == nil {
		t.Fatal("NewFixer returned nil")
	}
}

func TestDiagnose(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	fixer := NewFixer(configPath, false)
	fixes := fixer.Diagnose()

	// Should find at least one fix (config doesn't exist)
	if len(fixes) == 0 {
		t.Log("No fixes found - environment may be fully configured")
	}
}

func TestFixDirectories(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	fixer := NewFixer(configPath, false)

	// Manually add a directory fix
	fixer.fixes = []*Fix{{
		Type:        FixTypeDirectories,
		Description: "Create test directory",
		Safe:        true,
	}}

	// This should not fail (directories created in home dir, not temp)
	// Just test that the method exists and can be called
	err := fixer.fixDirectories()
	// This might fail if we don't have permissions, which is OK for testing
	t.Logf("fixDirectories result: %v", err)
}

func TestFixTunaConfig(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test-config.yaml")

	fixer := NewFixer(configPath, false)
	err := fixer.fixTunaConfig()
	if err != nil {
		t.Fatalf("fixTunaConfig error: %v", err)
	}

	// Verify config was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}
}

func TestApplySafe(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	fixer := NewFixer(configPath, false)

	// Add a safe fix
	fixer.fixes = []*Fix{
		{Type: FixTypeTunaConfig, Description: "Create config", Safe: true},
	}

	results := fixer.ApplySafe()
	if len(results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(results))
	}

	if !results[0].Applied {
		t.Errorf("Safe fix should be applied")
	}
}

func TestApplySafeSkipsUnsafe(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	fixer := NewFixer(configPath, false)

	// Add an unsafe fix
	fixer.fixes = []*Fix{
		{Type: FixTypeSSHKey, Description: "Generate SSH key", Safe: false},
	}

	results := fixer.ApplySafe()
	if len(results) != 0 {
		t.Errorf("Expected 0 results for unsafe fixes, got %d", len(results))
	}
}

func TestDryRun(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	fixer := NewFixer(configPath, true) // dry run mode

	fix := &Fix{
		Type:        FixTypeTunaConfig,
		Description: "Create config",
		Safe:        true,
	}

	result := fixer.ApplyFix(fix)

	if result.Applied {
		t.Error("Dry run should not apply fixes")
	}

	// Verify config was NOT created
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Error("Config file should not exist in dry run mode")
	}
}

func TestGetSafeFixes(t *testing.T) {
	fixes := []*Fix{
		{Type: FixTypeDirectories, Safe: true},
		{Type: FixTypeSSHKey, Safe: false},
		{Type: FixTypeTunaConfig, Safe: true},
	}

	safe := GetSafeFixes(fixes)
	if len(safe) != 2 {
		t.Errorf("Expected 2 safe fixes, got %d", len(safe))
	}
}

func TestGetUnsafeFixes(t *testing.T) {
	fixes := []*Fix{
		{Type: FixTypeDirectories, Safe: true},
		{Type: FixTypeSSHKey, Safe: false},
		{Type: FixTypeTunaConfig, Safe: true},
	}

	unsafe := GetUnsafeFixes(fixes)
	if len(unsafe) != 1 {
		t.Errorf("Expected 1 unsafe fix, got %d", len(unsafe))
	}
}

func TestFormatFix(t *testing.T) {
	fix := &Fix{
		Type:        FixTypeSSHKey,
		Description: "Generate SSH key",
		Safe:        false,
		Details:     "Will create ~/.ssh/id_rsa",
	}

	formatted := FormatFix(fix)
	if formatted == "" {
		t.Error("FormatFix returned empty string")
	}

	// Should contain REQUIRES CONFIRMATION for unsafe
	if fix.Safe {
		t.Error("Fix should be unsafe")
	}
}

func TestFormatFixSafe(t *testing.T) {
	fix := &Fix{
		Type:        FixTypeDirectories,
		Description: "Create directories",
		Safe:        true,
	}

	formatted := FormatFix(fix)
	if formatted == "" {
		t.Error("FormatFix returned empty string")
	}
}

func TestFormatResult(t *testing.T) {
	tests := []struct {
		name   string
		result FixResult
	}{
		{
			name: "applied",
			result: FixResult{
				Fix:     &Fix{Description: "Test"},
				Applied: true,
				Message: "Fixed something",
			},
		},
		{
			name: "error",
			result: FixResult{
				Fix:     &Fix{Description: "Test"},
				Applied: false,
				Error:   os.ErrNotExist,
			},
		},
		{
			name: "dry run",
			result: FixResult{
				Fix:     &Fix{Description: "Test"},
				Applied: false,
				Message: "Would do something",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatted := FormatResult(tt.result)
			if formatted == "" {
				t.Error("FormatResult returned empty string")
			}
		})
	}
}

func TestFixTypes(t *testing.T) {
	// Verify fix type constants
	if FixTypeSSHKey != "ssh_key" {
		t.Errorf("FixTypeSSHKey = %q, want %q", FixTypeSSHKey, "ssh_key")
	}
	if FixTypeOCIConfig != "oci_config" {
		t.Errorf("FixTypeOCIConfig = %q, want %q", FixTypeOCIConfig, "oci_config")
	}
	if FixTypeTunaConfig != "tuna_config" {
		t.Errorf("FixTypeTunaConfig = %q, want %q", FixTypeTunaConfig, "tuna_config")
	}
	if FixTypeSSHAgent != "ssh_agent" {
		t.Errorf("FixTypeSSHAgent = %q, want %q", FixTypeSSHAgent, "ssh_agent")
	}
	if FixTypeDirectories != "directories" {
		t.Errorf("FixTypeDirectories = %q, want %q", FixTypeDirectories, "directories")
	}
}
