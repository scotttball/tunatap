package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"
	"github.com/scotttball/tunatap/internal/catalog"
	"github.com/scotttball/tunatap/internal/config"
	"github.com/spf13/cobra"
)

var catalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "Manage shared cluster catalogs",
	Long: `Manage shared cluster catalogs for team configuration sharing.

Catalogs allow teams to share curated lists of clusters that can be
fetched from HTTPS URLs, OCI Object Storage, or local files.`,
}

var catalogListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured catalog sources",
	RunE:  runCatalogList,
}

var catalogFetchCmd = &cobra.Command{
	Use:   "fetch",
	Short: "Fetch and merge all enabled catalogs",
	RunE:  runCatalogFetch,
}

var catalogRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Refresh catalog cache",
	RunE:  runCatalogRefresh,
}

var catalogAddCmd = &cobra.Command{
	Use:   "add <name> <url>",
	Short: "Add a catalog source",
	Long: `Add a new catalog source.

Examples:
  # Add HTTPS catalog
  tunatap catalog add team-catalog https://example.com/clusters.yaml

  # Add OCI Object Storage catalog
  tunatap catalog add team-catalog oci://namespace/bucket/catalog.yaml --region us-ashburn-1

  # Add local file catalog
  tunatap catalog add local-catalog file:///path/to/catalog.yaml`,
	Args: cobra.ExactArgs(2),
	RunE: runCatalogAdd,
}

var catalogRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a catalog source",
	Args:  cobra.ExactArgs(1),
	RunE:  runCatalogRemove,
}

var catalogShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show contents of a catalog",
	Args:  cobra.ExactArgs(1),
	RunE:  runCatalogShow,
}

var catalogSampleCmd = &cobra.Command{
	Use:   "sample",
	Short: "Generate a sample catalog file",
	RunE:  runCatalogSample,
}

var (
	catalogRegion string
)

func init() {
	rootCmd.AddCommand(catalogCmd)

	catalogCmd.AddCommand(catalogListCmd)
	catalogCmd.AddCommand(catalogFetchCmd)
	catalogCmd.AddCommand(catalogRefreshCmd)
	catalogCmd.AddCommand(catalogAddCmd)
	catalogCmd.AddCommand(catalogRemoveCmd)
	catalogCmd.AddCommand(catalogShowCmd)
	catalogCmd.AddCommand(catalogSampleCmd)

	catalogAddCmd.Flags().StringVar(&catalogRegion, "region", "", "OCI region for Object Storage catalogs")
}

func runCatalogList(cmd *cobra.Command, args []string) error {
	cfg, err := config.ReadConfig(GetConfigFile())
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	if len(cfg.CatalogSources) == 0 {
		fmt.Println("No catalog sources configured.")
		fmt.Println("Add one with: tunatap catalog add <name> <url>")
		return nil
	}

	fmt.Println("Configured catalog sources:")
	fmt.Println()
	for _, source := range cfg.CatalogSources {
		status := "disabled"
		if source.Enabled {
			status = "enabled"
		}
		fmt.Printf("  %s (%s)\n", source.Name, status)
		fmt.Printf("    URL: %s\n", source.URL)
		if source.Type != "" {
			fmt.Printf("    Type: %s\n", source.Type)
		}
		if source.Priority > 0 {
			fmt.Printf("    Priority: %d\n", source.Priority)
		}
		fmt.Println()
	}

	return nil
}

func runCatalogFetch(cmd *cobra.Command, args []string) error {
	cfg, err := config.ReadConfig(GetConfigFile())
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	if len(cfg.CatalogSources) == 0 {
		fmt.Println("No catalog sources configured.")
		return nil
	}

	// Create catalog manager
	cacheDir := getCatalogCacheDir()
	manager := catalog.NewCatalogManager(cfg.CatalogSources, cacheDir)

	// Fetch all catalogs
	fmt.Println("Fetching catalogs...")
	catalogs, err := manager.FetchAll(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to fetch catalogs: %w", err)
	}

	fmt.Printf("Fetched %d catalog(s)\n", len(catalogs))

	// Merge with local config
	mergedCfg := catalog.MergeCatalogs(cfg, catalogs)

	// Show summary
	fmt.Printf("\nTotal clusters available: %d\n", len(mergedCfg.Clusters))

	return nil
}

func runCatalogRefresh(cmd *cobra.Command, args []string) error {
	cfg, err := config.ReadConfig(GetConfigFile())
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	cacheDir := getCatalogCacheDir()
	manager := catalog.NewCatalogManager(cfg.CatalogSources, cacheDir)

	fmt.Println("Refreshing catalog cache...")
	if err := manager.RefreshCatalogs(cmd.Context()); err != nil {
		return fmt.Errorf("failed to refresh catalogs: %w", err)
	}

	fmt.Println("Cache refreshed successfully")
	return nil
}

func runCatalogAdd(cmd *cobra.Command, args []string) error {
	name := args[0]
	urlStr := args[1]

	cfg, err := config.ReadConfig(GetConfigFile())
	if err != nil {
		cfg = config.DefaultConfig()
	}

	// Check if source already exists
	for _, source := range cfg.CatalogSources {
		if source.Name == name {
			return fmt.Errorf("catalog source '%s' already exists", name)
		}
	}

	// Create new source
	source := &config.CatalogSource{
		Name:      name,
		URL:       urlStr,
		Enabled:   true,
		OCIRegion: catalogRegion,
	}

	// Add to config
	cfg.CatalogSources = append(cfg.CatalogSources, source)

	// Save config
	if err := config.SaveConfig(GetConfigFile(), cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Added catalog source: %s\n", name)
	return nil
}

func runCatalogRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, err := config.ReadConfig(GetConfigFile())
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	// Find and remove source
	found := false
	newSources := make([]*config.CatalogSource, 0)
	for _, source := range cfg.CatalogSources {
		if source.Name == name {
			found = true
			continue
		}
		newSources = append(newSources, source)
	}

	if !found {
		return fmt.Errorf("catalog source '%s' not found", name)
	}

	cfg.CatalogSources = newSources

	// Save config
	if err := config.SaveConfig(GetConfigFile(), cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Removed catalog source: %s\n", name)
	return nil
}

func runCatalogShow(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, err := config.ReadConfig(GetConfigFile())
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	// Find source
	var source *config.CatalogSource
	for _, s := range cfg.CatalogSources {
		if s.Name == name {
			source = s
			break
		}
	}

	if source == nil {
		return fmt.Errorf("catalog source '%s' not found", name)
	}

	// Fetch catalog
	cacheDir := getCatalogCacheDir()
	manager := catalog.NewCatalogManager(cfg.CatalogSources, cacheDir)

	catalogData, err := manager.FetchSource(cmd.Context(), source)
	if err != nil {
		return fmt.Errorf("failed to fetch catalog: %w", err)
	}

	// Display catalog info
	fmt.Printf("Catalog: %s\n", catalogData.Name)
	fmt.Printf("Version: %s\n", catalogData.Version)
	if catalogData.Description != "" {
		fmt.Printf("Description: %s\n", catalogData.Description)
	}
	if catalogData.Maintainer != "" {
		fmt.Printf("Maintainer: %s\n", catalogData.Maintainer)
	}
	if catalogData.Updated != "" {
		fmt.Printf("Updated: %s\n", catalogData.Updated)
	}
	fmt.Println()

	fmt.Printf("Clusters (%d):\n", len(catalogData.Clusters))
	for _, cluster := range catalogData.Clusters {
		fmt.Printf("  - %s (%s)\n", cluster.ClusterName, cluster.Region)
	}

	if len(catalogData.Tenancies) > 0 {
		fmt.Printf("\nTenancies (%d):\n", len(catalogData.Tenancies))
		for _, tenancy := range catalogData.Tenancies {
			fmt.Printf("  - %s\n", tenancy.Name)
		}
	}

	return nil
}

func runCatalogSample(cmd *cobra.Command, args []string) error {
	sample := catalog.GenerateSampleCatalog()
	fmt.Println(sample)
	return nil
}

func getCatalogCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Warn().Err(err).Msg("Could not get home directory")
		return ""
	}
	return filepath.Join(home, ".tunatap", "cache", "catalogs")
}
