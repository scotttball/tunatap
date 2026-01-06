package cmd

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestSetVersionInfo(t *testing.T) {
	SetVersionInfo("1.0.0", "abc123", "2024-01-01")

	if version != "1.0.0" {
		t.Errorf("version = %q, want %q", version, "1.0.0")
	}

	if commit != "abc123" {
		t.Errorf("commit = %q, want %q", commit, "abc123")
	}

	if date != "2024-01-01" {
		t.Errorf("date = %q, want %q", date, "2024-01-01")
	}
}

func TestSetHomePath(t *testing.T) {
	testPath := "/test/home/path"
	SetHomePath(testPath)

	if homePath != testPath {
		t.Errorf("homePath = %q, want %q", homePath, testPath)
	}
}

func TestGetConfigFile(t *testing.T) {
	// Save original values
	origCfgFile := cfgFile
	origHomePath := homePath
	defer func() {
		cfgFile = origCfgFile
		homePath = origHomePath
	}()

	// Reset state
	cfgFile = ""
	homePath = "/test/home"

	// With no config file set, should use default
	path := GetConfigFile()
	expected := filepath.Join("/test/home", "config.yaml")
	if path != expected {
		t.Errorf("GetConfigFile() = %q, want %q", path, expected)
	}

	// With config file set
	cfgFile = "/custom/config.yaml"
	path = GetConfigFile()
	if path != "/custom/config.yaml" {
		t.Errorf("GetConfigFile() with custom = %q, want %q", path, "/custom/config.yaml")
	}
}

func TestRootCommand(t *testing.T) {
	// Test that root command exists and has expected properties
	if rootCmd == nil {
		t.Fatal("rootCmd is nil")
	}

	if rootCmd.Use != "tunatap" {
		t.Errorf("rootCmd.Use = %q, want %q", rootCmd.Use, "tunatap")
	}

	// Should have subcommands
	if len(rootCmd.Commands()) == 0 {
		t.Error("rootCmd should have subcommands")
	}
}

func TestVersionCommand(t *testing.T) {
	// Find version command
	var versionCommand *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "version" {
			versionCommand = cmd
			break
		}
	}

	if versionCommand == nil {
		t.Fatal("version command not found")
	}
}

func TestCommandFlags(t *testing.T) {
	// Test that expected flags exist
	flag := rootCmd.PersistentFlags().Lookup("config")
	if flag == nil {
		t.Error("--config flag not found")
	}

	flag = rootCmd.PersistentFlags().Lookup("debug")
	if flag == nil {
		t.Error("--debug flag not found")
	}

	flag = rootCmd.PersistentFlags().Lookup("raw")
	if flag == nil {
		t.Error("--raw flag not found")
	}
}

func TestSubcommands(t *testing.T) {
	expectedCommands := []string{"connect", "setup", "list", "doctor", "version"}

	for _, cmdName := range expectedCommands {
		found := false
		for _, cmd := range rootCmd.Commands() {
			if cmd.Use == cmdName || cmd.Name() == cmdName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected subcommand %q not found", cmdName)
		}
	}
}
