package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/scotttball/tunatap/internal/config"
	"github.com/scotttball/tunatap/pkg/utils"
	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Set up tunatap configuration",
	Long: `Interactive setup wizard for tunatap configuration.

This command helps you create or update your tunatap configuration file
with cluster definitions, tenancy information, and SSH settings.`,
	RunE: runSetup,
}

func init() {
	rootCmd.AddCommand(setupCmd)
}

func runSetup(cmd *cobra.Command, args []string) error {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Welcome to tunatap setup!")
	fmt.Println("This wizard will help you configure tunatap.")
	fmt.Println()

	// Load existing config or create new one
	cfgPath := GetConfigFile()
	cfg, err := config.ReadConfig(cfgPath)
	if err != nil {
		log.Warn().Err(err).Msg("Could not read existing config, creating new one")
		cfg = config.DefaultConfig()
	}

	// SSH Private Key
	fmt.Print("SSH private key file path [~/.ssh/id_rsa]: ")
	sshKey, _ := reader.ReadString('\n')
	sshKey = strings.TrimSpace(sshKey)
	if sshKey == "" {
		sshKey = "~/.ssh/id_rsa"
	}
	cfg.SshPrivateKeyFile = sshKey

	// SOCKS Proxy (optional)
	fmt.Print("SOCKS proxy address (leave empty if none): ")
	socksProxy, _ := reader.ReadString('\n')
	socksProxy = strings.TrimSpace(socksProxy)
	cfg.SshSocksProxy = socksProxy

	// Add clusters
	fmt.Print("\nWould you like to add a cluster? [y/N]: ")
	addCluster, _ := reader.ReadString('\n')
	addCluster = strings.TrimSpace(strings.ToLower(addCluster))

	for addCluster == "y" || addCluster == "yes" {
		cluster, err := promptForCluster(reader)
		if err != nil {
			log.Error().Err(err).Msg("Failed to add cluster")
		} else {
			cfg.Clusters = append(cfg.Clusters, cluster)
			fmt.Println("Cluster added successfully!")
		}

		fmt.Print("\nWould you like to add another cluster? [y/N]: ")
		addCluster, _ = reader.ReadString('\n')
		addCluster = strings.TrimSpace(strings.ToLower(addCluster))
	}

	// Save configuration
	if err := config.SaveConfig(cfgPath, cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("\nConfiguration saved to: %s\n", cfgPath)
	fmt.Println("You can now use 'tunatap connect' to connect to your clusters.")

	return nil
}

func promptForCluster(reader *bufio.Reader) (*config.Cluster, error) {
	cluster := &config.Cluster{}

	fmt.Print("\nCluster name: ")
	name, _ := reader.ReadString('\n')
	cluster.ClusterName = strings.TrimSpace(name)
	if cluster.ClusterName == "" {
		return nil, fmt.Errorf("cluster name is required")
	}

	fmt.Print("OCI Region (e.g., us-ashburn-1): ")
	region, _ := reader.ReadString('\n')
	cluster.Region = strings.TrimSpace(region)
	if cluster.Region == "" {
		return nil, fmt.Errorf("region is required")
	}

	fmt.Print("Cluster OCID (leave empty to lookup by name): ")
	ocid, _ := reader.ReadString('\n')
	ocid = strings.TrimSpace(ocid)
	if ocid != "" {
		cluster.Ocid = &ocid
	}

	if cluster.Ocid == nil {
		fmt.Print("Tenancy name: ")
		tenant, _ := reader.ReadString('\n')
		tenant = strings.TrimSpace(tenant)
		if tenant != "" {
			cluster.Tenant = &tenant
		}

		fmt.Print("Compartment path (e.g., parent/child): ")
		compartment, _ := reader.ReadString('\n')
		compartment = strings.TrimSpace(compartment)
		if compartment != "" {
			cluster.Compartment = &compartment
		}
	}

	fmt.Print("Bastion name (leave empty to auto-detect): ")
	bastionName, _ := reader.ReadString('\n')
	bastionName = strings.TrimSpace(bastionName)
	if bastionName != "" {
		cluster.Bastion = &bastionName
	}

	fmt.Print("Local port [6443]: ")
	portStr, _ := reader.ReadString('\n')
	portStr = strings.TrimSpace(portStr)
	port := 6443
	if portStr != "" {
		fmt.Sscanf(portStr, "%d", &port)
	}
	cluster.LocalPort = &port

	// Add at least one endpoint
	fmt.Print("\nAdd cluster endpoint IP: ")
	ip, _ := reader.ReadString('\n')
	ip = strings.TrimSpace(ip)
	if ip != "" {
		fmt.Print("Endpoint port [6443]: ")
		epPortStr, _ := reader.ReadString('\n')
		epPortStr = strings.TrimSpace(epPortStr)
		epPort := 6443
		if epPortStr != "" {
			fmt.Sscanf(epPortStr, "%d", &epPort)
		}

		cluster.Endpoints = append(cluster.Endpoints, &config.ClusterEndpoint{
			Name: "default",
			Ip:   ip,
			Port: epPort,
		})
	}

	return cluster, nil
}

// setupInitCmd initializes a new configuration
var setupInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgPath := GetConfigFile()

		// Check if config already exists
		if _, err := os.Stat(cfgPath); err == nil {
			return fmt.Errorf("config file already exists at %s", cfgPath)
		}

		// Create default config
		cfg := config.DefaultConfig()
		cfg.SshPrivateKeyFile = "~/.ssh/id_rsa"

		// Ensure directory exists
		if err := os.MkdirAll(filepath.Dir(cfgPath), 0o750); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}

		if err := config.SaveConfig(cfgPath, cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("Created new configuration at: %s\n", cfgPath)
		fmt.Println("Edit this file to add your clusters, or run 'tunatap setup' for interactive configuration.")
		return nil
	},
}

// setupShowCmd shows the current configuration
var setupShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.ReadConfig(GetConfigFile())
		if err != nil {
			return fmt.Errorf("failed to read config: %w", err)
		}

		fmt.Printf("Configuration file: %s\n\n", GetConfigFile())

		fmt.Println("SSH Settings:")
		fmt.Printf("  Private Key: %s\n", cfg.SshPrivateKeyFile)
		if cfg.SshSocksProxy != "" {
			fmt.Printf("  SOCKS Proxy: %s\n", cfg.SshSocksProxy)
		}
		fmt.Printf("  Pool Size: %d\n", cfg.GetPoolSize())
		fmt.Printf("  Warmup Count: %d\n", cfg.GetWarmupCount())
		fmt.Printf("  Max Concurrent: %d\n", cfg.GetMaxConcurrent())

		fmt.Printf("\nClusters: %d\n", len(cfg.Clusters))
		for _, c := range cfg.Clusters {
			fmt.Printf("  - %s (%s)\n", c.ClusterName, c.Region)
			if c.Ocid != nil {
				fmt.Printf("    OCID: %s\n", *c.Ocid)
			}
			if len(c.Endpoints) > 0 {
				fmt.Printf("    Endpoints: %d\n", len(c.Endpoints))
			}
		}

		fmt.Printf("\nTenancies: %d\n", len(cfg.Tenancies))
		for name := range cfg.Tenancies {
			fmt.Printf("  - %s\n", name)
		}

		return nil
	},
}

// setupAddClusterCmd adds a cluster to configuration
var setupAddClusterCmd = &cobra.Command{
	Use:   "add-cluster",
	Short: "Add a cluster to configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.ReadConfig(GetConfigFile())
		if err != nil {
			return fmt.Errorf("failed to read config: %w", err)
		}

		reader := bufio.NewReader(os.Stdin)
		cluster, err := promptForCluster(reader)
		if err != nil {
			return err
		}

		cfg.Clusters = append(cfg.Clusters, cluster)

		if err := config.SaveConfig(GetConfigFile(), cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Println("Cluster added successfully!")
		return nil
	},
}

// setupAddTenancyCmd adds a tenancy to configuration
var setupAddTenancyCmd = &cobra.Command{
	Use:   "add-tenancy [name] [ocid]",
	Short: "Add a tenancy to configuration",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		ocid := args[1]

		cfg, err := config.ReadConfig(GetConfigFile())
		if err != nil {
			return fmt.Errorf("failed to read config: %w", err)
		}

		if cfg.Tenancies == nil {
			cfg.Tenancies = make(map[string]*string)
		}

		cfg.Tenancies[name] = utils.StringPtr(ocid)

		if err := config.SaveConfig(GetConfigFile(), cfg); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("Tenancy '%s' added successfully!\n", name)
		return nil
	},
}

func init() {
	setupCmd.AddCommand(setupInitCmd)
	setupCmd.AddCommand(setupShowCmd)
	setupCmd.AddCommand(setupAddClusterCmd)
	setupCmd.AddCommand(setupAddTenancyCmd)
}
