package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/rs/zerolog/log"
	"github.com/scotttball/tunatap/internal/client"
	"github.com/scotttball/tunatap/internal/config"
	"github.com/scotttball/tunatap/pkg/utils"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List resources",
	Long:  `List configured clusters, bastions, and other resources.`,
}

var listClustersCmd = &cobra.Command{
	Use:   "clusters",
	Short: "List configured clusters",
	RunE:  runListClusters,
}

var listBastionsCmd = &cobra.Command{
	Use:   "bastions",
	Short: "List bastions in a compartment",
	RunE:  runListBastions,
}

var listTenanciesCmd = &cobra.Command{
	Use:   "tenancies",
	Short: "List configured tenancies",
	RunE:  runListTenancies,
}

var (
	compartmentOcid string
	region          string
)

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.AddCommand(listClustersCmd)
	listCmd.AddCommand(listBastionsCmd)
	listCmd.AddCommand(listTenanciesCmd)

	listBastionsCmd.Flags().StringVarP(&compartmentOcid, "compartment", "c", "", "compartment OCID")
	listBastionsCmd.Flags().StringVarP(&region, "region", "r", "", "OCI region")
}

func runListClusters(cmd *cobra.Command, args []string) error {
	cfg, err := config.ReadConfig(GetConfigFile())
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	if len(cfg.Clusters) == 0 {
		fmt.Println("No clusters configured.")
		fmt.Println("Run 'tunatap setup' to add clusters.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tREGION\tENDPOINTS\tBASION")

	for _, c := range cfg.Clusters {
		endpointCount := len(c.Endpoints)
		bastionInfo := "-"
		if c.Bastion != nil {
			bastionInfo = *c.Bastion
		} else if c.BastionId != nil {
			// Truncate OCID for display
			id := *c.BastionId
			if len(id) > 20 {
				bastionInfo = id[:20] + "..."
			} else {
				bastionInfo = id
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%d\t%s\n",
			c.ClusterName,
			c.Region,
			endpointCount,
			bastionInfo,
		)
	}

	w.Flush()
	return nil
}

func runListBastions(cmd *cobra.Command, args []string) error {
	if compartmentOcid == "" {
		return fmt.Errorf("--compartment flag is required")
	}
	if region == "" {
		return fmt.Errorf("--region flag is required")
	}

	// Create OCI client
	configPath := utils.DefaultOCIConfigPath()
	ociClient, err := client.NewOCIClientWithProfile(configPath, "DEFAULT")
	if err != nil {
		return fmt.Errorf("failed to create OCI client: %w", err)
	}

	ociClient.SetRegion(region)

	log.Info().Msgf("Listing bastions in compartment %s...", compartmentOcid)

	ctx := context.Background()
	bastions, err := ociClient.ListBastions(ctx, compartmentOcid)
	if err != nil {
		return fmt.Errorf("failed to list bastions: %w", err)
	}

	if len(bastions) == 0 {
		fmt.Println("No bastions found in the specified compartment.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSTATE\tTYPE\tOCID")

	for _, b := range bastions {
		bastionType := "-"
		if b.BastionType != nil {
			bastionType = *b.BastionType
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			*b.Name,
			b.LifecycleState,
			bastionType,
			*b.Id,
		)
	}

	w.Flush()
	return nil
}

func runListTenancies(cmd *cobra.Command, args []string) error {
	cfg, err := config.ReadConfig(GetConfigFile())
	if err != nil {
		return fmt.Errorf("failed to read config: %w", err)
	}

	if len(cfg.Tenancies) == 0 {
		fmt.Println("No tenancies configured.")
		fmt.Println("Run 'tunatap setup add-tenancy <name> <ocid>' to add tenancies.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tOCID")

	for name, ocid := range cfg.Tenancies {
		ocidStr := "-"
		if ocid != nil {
			ocidStr = *ocid
			// Truncate for display
			if len(ocidStr) > 50 {
				ocidStr = ocidStr[:50] + "..."
			}
		}
		fmt.Fprintf(w, "%s\t%s\n", name, ocidStr)
	}

	w.Flush()
	return nil
}
