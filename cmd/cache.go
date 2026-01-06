package cmd

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/scotttball/tunatap/internal/config"
	"github.com/scotttball/tunatap/internal/discovery"
	"github.com/scotttball/tunatap/pkg/utils"
	"github.com/spf13/cobra"
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage the discovery cache",
	Long:  `Commands for managing the cluster discovery cache.`,
}

var cacheShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show cached entries",
	Long:  `Display all cached cluster and bastion discovery entries.`,
	RunE:  runCacheShow,
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear [cluster]",
	Short: "Clear cache entries",
	Long: `Clear cached discovery entries.

If a cluster name is provided, only that cluster's cache is cleared.
Otherwise, the entire cache is cleared.`,
	RunE: runCacheClear,
}

func init() {
	rootCmd.AddCommand(cacheCmd)
	cacheCmd.AddCommand(cacheShowCmd)
	cacheCmd.AddCommand(cacheClearCmd)
}

func runCacheShow(cmd *cobra.Command, args []string) error {
	// Load config to get TTL setting
	cfg, _ := config.ReadConfig(GetConfigFile())
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	ttl := time.Duration(cfg.GetCacheTTLHours()) * time.Hour
	cache, err := discovery.NewCache(utils.DefaultTunatapDir(), ttl)
	if err != nil {
		return fmt.Errorf("failed to load cache: %w", err)
	}

	clusters := cache.GetAllClusters()

	if len(clusters) == 0 {
		fmt.Println("Cache is empty.")
		fmt.Printf("Cache file: %s\n", cache.Path())
		return nil
	}

	fmt.Printf("Discovery Cache (%s)\n", cache.Path())
	fmt.Println("═════════════════════════════════════════════════════════════")
	fmt.Println()

	fmt.Println("Clusters:")
	fmt.Println("─────────────────────────────────────────────────────────────")
	for name, entry := range clusters {
		ttlRemaining := cache.GetClusterTTL(name)
		fmt.Printf("  %s\n", name)
		fmt.Printf("    OCID:        %s\n", entry.OCID)
		fmt.Printf("    Region:      %s\n", entry.Region)
		fmt.Printf("    Compartment: %s\n", entry.CompartmentOCID)
		if entry.EndpointIP != "" {
			fmt.Printf("    Endpoint:    %s:%d\n", entry.EndpointIP, entry.EndpointPort)
		}
		fmt.Printf("    Cached:      %s\n", entry.CachedAt.Format(time.RFC3339))
		fmt.Printf("    Expires in:  %s\n", ttlRemaining.Round(time.Minute))
		fmt.Println()
	}

	// Show bastion entries too
	bastionCount := 0
	for name := range clusters {
		if bastion := cache.GetBastion(name); bastion != nil {
			if bastionCount == 0 {
				fmt.Println("Bastions:")
				fmt.Println("─────────────────────────────────────────────────────────────")
			}
			bastionCount++
			fmt.Printf("  %s (for cluster: %s)\n", bastion.OCID, name)
			fmt.Printf("    Region:      %s\n", bastion.Region)
			fmt.Printf("    Cached:      %s\n", bastion.CachedAt.Format(time.RFC3339))
			fmt.Println()
		}
	}

	return nil
}

func runCacheClear(cmd *cobra.Command, args []string) error {
	// Load config to get TTL setting
	cfg, _ := config.ReadConfig(GetConfigFile())
	if cfg == nil {
		cfg = config.DefaultConfig()
	}

	ttl := time.Duration(cfg.GetCacheTTLHours()) * time.Hour
	cache, err := discovery.NewCache(utils.DefaultTunatapDir(), ttl)
	if err != nil {
		return fmt.Errorf("failed to load cache: %w", err)
	}

	if len(args) > 0 {
		// Clear specific cluster
		clusterName := args[0]
		if cache.GetCluster(clusterName) == nil {
			log.Warn().Msgf("Cluster '%s' not found in cache", clusterName)
			return nil
		}

		if err := cache.Invalidate(clusterName); err != nil {
			return fmt.Errorf("failed to clear cache for '%s': %w", clusterName, err)
		}
		fmt.Printf("Cleared cache for cluster: %s\n", clusterName)
	} else {
		// Clear all
		if err := cache.InvalidateAll(); err != nil {
			return fmt.Errorf("failed to clear cache: %w", err)
		}
		fmt.Println("Cache cleared.")
	}

	return nil
}
