package autofix

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/scotttball/tunatap/internal/config"
	"github.com/scotttball/tunatap/pkg/utils"
)

// FixType represents the type of fix.
type FixType string

const (
	FixTypeSSHKey      FixType = "ssh_key"
	FixTypeOCIConfig   FixType = "oci_config"
	FixTypeTunaConfig  FixType = "tuna_config"
	FixTypeSSHAgent    FixType = "ssh_agent"
	FixTypeDirectories FixType = "directories"
)

// Fix represents an auto-fix action.
type Fix struct {
	Type        FixType
	Description string
	Safe        bool // If true, can be applied without confirmation
	DryRun      bool // If true, only describe what would be done
	Applied     bool
	Error       error
	Details     string
}

// FixResult represents the result of applying a fix.
type FixResult struct {
	Fix     *Fix
	Applied bool
	Error   error
	Message string
}

// Fixer handles auto-fix operations.
type Fixer struct {
	configPath string
	dryRun     bool
	fixes      []*Fix
}

// NewFixer creates a new fixer.
func NewFixer(configPath string, dryRun bool) *Fixer {
	return &Fixer{
		configPath: configPath,
		dryRun:     dryRun,
		fixes:      make([]*Fix, 0),
	}
}

// Diagnose identifies issues that can be auto-fixed.
func (f *Fixer) Diagnose() []*Fix {
	f.fixes = make([]*Fix, 0)

	// Check tunatap config directory
	f.checkConfigDirectory()

	// Check SSH key
	f.checkSSHKey()

	// Check SSH agent
	f.checkSSHAgent()

	// Check OCI config
	f.checkOCIConfig()

	// Check tunatap config
	f.checkTunaConfig()

	return f.fixes
}

// ApplyAll applies all fixes.
func (f *Fixer) ApplyAll() []FixResult {
	results := make([]FixResult, 0)

	for _, fix := range f.fixes {
		result := f.ApplyFix(fix)
		results = append(results, result)
	}

	return results
}

// ApplySafe applies only safe fixes.
func (f *Fixer) ApplySafe() []FixResult {
	results := make([]FixResult, 0)

	for _, fix := range f.fixes {
		if fix.Safe {
			result := f.ApplyFix(fix)
			results = append(results, result)
		}
	}

	return results
}

// ApplyFix applies a single fix.
func (f *Fixer) ApplyFix(fix *Fix) FixResult {
	if f.dryRun {
		return FixResult{
			Fix:     fix,
			Applied: false,
			Message: fmt.Sprintf("[DRY RUN] Would apply: %s", fix.Description),
		}
	}

	var err error
	var message string

	switch fix.Type {
	case FixTypeDirectories:
		err = f.fixDirectories()
		message = "Created required directories"
	case FixTypeSSHKey:
		err = f.fixSSHKey()
		message = "Generated SSH key pair"
	case FixTypeSSHAgent:
		err = f.fixSSHAgent()
		message = "SSH agent instructions provided"
	case FixTypeOCIConfig:
		err = f.fixOCIConfig()
		message = "OCI config instructions provided"
	case FixTypeTunaConfig:
		err = f.fixTunaConfig()
		message = "Created default tunatap config"
	default:
		err = fmt.Errorf("unknown fix type: %s", fix.Type)
	}

	return FixResult{
		Fix:     fix,
		Applied: err == nil,
		Error:   err,
		Message: message,
	}
}

// checkConfigDirectory checks if required directories exist.
func (f *Fixer) checkConfigDirectory() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	dirs := []string{
		filepath.Join(home, ".tunatap"),
		filepath.Join(home, ".tunatap", "cache"),
		filepath.Join(home, ".tunatap", "audit"),
	}

	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			f.fixes = append(f.fixes, &Fix{
				Type:        FixTypeDirectories,
				Description: fmt.Sprintf("Create directory: %s", dir),
				Safe:        true,
				Details:     dir,
			})
		}
	}
}

// checkSSHKey checks for SSH key issues.
func (f *Fixer) checkSSHKey() {
	keyPath := utils.DefaultSSHPrivateKey()

	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		f.fixes = append(f.fixes, &Fix{
			Type:        FixTypeSSHKey,
			Description: fmt.Sprintf("Generate SSH key at %s", keyPath),
			Safe:        false, // Requires user confirmation
			Details:     keyPath,
		})
	}
}

// checkSSHAgent checks SSH agent status.
func (f *Fixer) checkSSHAgent() {
	authSock := os.Getenv("SSH_AUTH_SOCK")
	if authSock == "" {
		f.fixes = append(f.fixes, &Fix{
			Type:        FixTypeSSHAgent,
			Description: "SSH agent not running",
			Safe:        false, // Can't automatically start agent in user's shell
			Details:     "Run: eval $(ssh-agent -s) && ssh-add",
		})
		return
	}

	// Check if agent has keys
	cmd := exec.Command("ssh-add", "-l")
	output, err := cmd.Output()
	if err != nil || strings.Contains(string(output), "no identities") {
		f.fixes = append(f.fixes, &Fix{
			Type:        FixTypeSSHAgent,
			Description: "No keys loaded in SSH agent",
			Safe:        false,
			Details:     "Run: ssh-add ~/.ssh/id_rsa (or your key path)",
		})
	}
}

// checkOCIConfig checks OCI configuration.
func (f *Fixer) checkOCIConfig() {
	ociConfigPath := utils.DefaultOCIConfigPath()

	if _, err := os.Stat(ociConfigPath); os.IsNotExist(err) {
		f.fixes = append(f.fixes, &Fix{
			Type:        FixTypeOCIConfig,
			Description: "OCI config not found",
			Safe:        false,
			Details:     "Run: oci setup config",
		})
	}
}

// checkTunaConfig checks tunatap configuration.
func (f *Fixer) checkTunaConfig() {
	if _, err := os.Stat(f.configPath); os.IsNotExist(err) {
		f.fixes = append(f.fixes, &Fix{
			Type:        FixTypeTunaConfig,
			Description: fmt.Sprintf("Create default config at %s", f.configPath),
			Safe:        true,
			Details:     f.configPath,
		})
	}
}

// fixDirectories creates required directories.
func (f *Fixer) fixDirectories() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	dirs := []string{
		filepath.Join(home, ".tunatap"),
		filepath.Join(home, ".tunatap", "cache"),
		filepath.Join(home, ".tunatap", "audit"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
		log.Info().Str("directory", dir).Msg("Created directory")
	}

	return nil
}

// fixSSHKey generates an SSH key pair.
func (f *Fixer) fixSSHKey() error {
	keyPath := utils.DefaultSSHPrivateKey()

	// Ensure .ssh directory exists
	sshDir := filepath.Dir(keyPath)
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("failed to create .ssh directory: %w", err)
	}

	// Generate key using ssh-keygen
	cmd := exec.Command("ssh-keygen",
		"-t", "rsa",
		"-b", "4096",
		"-f", keyPath,
		"-N", "", // Empty passphrase
		"-C", "tunatap-generated-key",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ssh-keygen failed: %w\n%s", err, output)
	}

	log.Info().Str("key", keyPath).Msg("Generated SSH key")
	return nil
}

// fixSSHAgent provides instructions for SSH agent setup.
func (f *Fixer) fixSSHAgent() error {
	// We can't automatically start the SSH agent in the user's shell
	// Just return instructions
	return fmt.Errorf("manual action required: eval $(ssh-agent -s) && ssh-add")
}

// fixOCIConfig provides instructions for OCI config setup.
func (f *Fixer) fixOCIConfig() error {
	// We can't automatically run oci setup config as it's interactive
	return fmt.Errorf("manual action required: oci setup config")
}

// fixTunaConfig creates a default tunatap config.
func (f *Fixer) fixTunaConfig() error {
	cfg := config.DefaultConfig()

	// Set default SSH key path
	cfg.SshPrivateKeyFile = utils.DefaultSSHPrivateKey()

	if err := config.SaveConfig(f.configPath, cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	log.Info().Str("path", f.configPath).Msg("Created default config")
	return nil
}

// GetSafeFixes returns fixes that can be safely applied.
func GetSafeFixes(fixes []*Fix) []*Fix {
	safe := make([]*Fix, 0)
	for _, fix := range fixes {
		if fix.Safe {
			safe = append(safe, fix)
		}
	}
	return safe
}

// GetUnsafeFixes returns fixes that require confirmation.
func GetUnsafeFixes(fixes []*Fix) []*Fix {
	unsafe := make([]*Fix, 0)
	for _, fix := range fixes {
		if !fix.Safe {
			unsafe = append(unsafe, fix)
		}
	}
	return unsafe
}

// FormatFix formats a fix for display.
func FormatFix(fix *Fix) string {
	safeLabel := "[SAFE]"
	if !fix.Safe {
		safeLabel = "[REQUIRES CONFIRMATION]"
	}

	result := fmt.Sprintf("%s %s", safeLabel, fix.Description)
	if fix.Details != "" {
		result += fmt.Sprintf("\n    Details: %s", fix.Details)
	}
	return result
}

// FormatResult formats a fix result for display.
func FormatResult(result FixResult) string {
	if result.Applied {
		return fmt.Sprintf("✓ %s", result.Message)
	}
	if result.Error != nil {
		return fmt.Sprintf("✗ %s: %v", result.Fix.Description, result.Error)
	}
	return fmt.Sprintf("○ %s", result.Message)
}
