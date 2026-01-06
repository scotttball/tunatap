package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/scotttball/tunatap/internal/state"
	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	commit    = "none"
	date      = "unknown"
	cfgFile   string
	debug     bool
	rawOutput bool
	homePath  string
)

// rootCmd represents the base command
var rootCmd = &cobra.Command{
	Use:   "tunatap",
	Short: "SSH tunnel manager for OCI bastion services",
	Long: `Tunatap is a CLI tool for managing SSH tunnels through OCI Bastion services.
It simplifies connecting to OKE clusters and other private resources via bastion hosts.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Set up logging
		if rawOutput {
			logPath := filepath.Join(homePath, "tunatap.log")
			logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
			if err != nil {
				return fmt.Errorf("failed to open log file: %w", err)
			}
			log.Logger = log.Output(zerolog.New(logFile))
		} else {
			log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
		}

		if debug {
			zerolog.SetGlobalLevel(zerolog.DebugLevel)
		} else {
			zerolog.SetGlobalLevel(zerolog.InfoLevel)
		}

		// Initialize global state
		globalState := state.GetInstance()
		globalState.SetHomePath(homePath)

		return nil
	},
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.tunatap/config.yaml)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
	rootCmd.PersistentFlags().BoolVar(&rawOutput, "raw", false, "output raw logs to file instead of console")
}

// SetVersionInfo sets the version information for the CLI
func SetVersionInfo(v, c, d string) {
	version = v
	commit = c
	date = d
}

// SetHomePath sets the home path for configuration
func SetHomePath(path string) {
	homePath = path
}

// GetConfigFile returns the config file path
func GetConfigFile() string {
	if cfgFile != "" {
		return cfgFile
	}
	return filepath.Join(homePath, "config.yaml")
}

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("tunatap %s\n", version)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built:  %s\n", date)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
