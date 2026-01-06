package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestSetupCommandExists(t *testing.T) {
	if setupCmd == nil {
		t.Fatal("setupCmd is nil")
	}

	if setupCmd.Use != "setup" {
		t.Errorf("setupCmd.Use = %q, want %q", setupCmd.Use, "setup")
	}
}

func TestSetupSubcommands(t *testing.T) {
	expectedSubcommands := []string{"init", "show", "add-cluster", "add-tenancy"}

	for _, cmdName := range expectedSubcommands {
		found := false
		for _, cmd := range setupCmd.Commands() {
			if cmd.Name() == cmdName {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected setup subcommand %q not found", cmdName)
		}
	}
}

func TestSetupInitCommand(t *testing.T) {
	var initCmd *cobra.Command
	for _, cmd := range setupCmd.Commands() {
		if cmd.Name() == "init" {
			initCmd = cmd
			break
		}
	}

	if initCmd == nil {
		t.Fatal("setup init command not found")
	}

	if initCmd.Short == "" {
		t.Error("setup init command should have a short description")
	}
}

func TestSetupShowCommand(t *testing.T) {
	var showCmd *cobra.Command
	for _, cmd := range setupCmd.Commands() {
		if cmd.Name() == "show" {
			showCmd = cmd
			break
		}
	}

	if showCmd == nil {
		t.Fatal("setup show command not found")
	}
}

func TestSetupAddTenancyCommand(t *testing.T) {
	var addTenancyCmd *cobra.Command
	for _, cmd := range setupCmd.Commands() {
		if cmd.Name() == "add-tenancy" {
			addTenancyCmd = cmd
			break
		}
	}

	if addTenancyCmd == nil {
		t.Fatal("setup add-tenancy command not found")
	}

	// Should require 2 arguments
	if addTenancyCmd.Args == nil {
		t.Error("setup add-tenancy should have Args validation")
	}
}
