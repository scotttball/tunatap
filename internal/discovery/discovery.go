package discovery

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/scotttball/tunatap/internal/client"
	"github.com/scotttball/tunatap/internal/config"
	"github.com/scotttball/tunatap/pkg/utils"
)

var (
	// ErrClusterNotFound is returned when no cluster matches the search criteria.
	ErrClusterNotFound = errors.New("cluster not found in any accessible compartment")

	// ErrMultipleClustersFound is returned when multiple clusters match the search criteria.
	ErrMultipleClustersFound = errors.New("multiple clusters found with same name")

	// ErrNoBastionFound is returned when no bastion is found for the cluster.
	ErrNoBastionFound = errors.New("no bastion found that can reach cluster")

	// ErrDiscoveryTimeout is returned when discovery times out.
	ErrDiscoveryTimeout = errors.New("discovery timed out")

	// ErrClusterAccessDenied is returned when the user lacks permission to access the cluster.
	ErrClusterAccessDenied = errors.New("cluster access denied")

	// ErrInvalidOCID is returned when an OCID format is invalid.
	ErrInvalidOCID = errors.New("invalid OCID format")
)

// DiscoveredCluster contains information about a discovered cluster.
type DiscoveredCluster struct {
	OCID            string
	Name            string
	CompartmentID   string
	CompartmentPath string
	Region          string
	VcnID           string
	SubnetID        string
	EndpointIP      string
	EndpointPort    int
}

// DiscoveredBastion contains information about a discovered bastion.
type DiscoveredBastion struct {
	OCID          string
	Name          string
	Type          string
	CompartmentID string
}

// DiscoveryHints provides optional hints to speed up discovery.
type DiscoveryHints struct {
	Region          string
	CompartmentPath string
	TenancyOCID     string
}

// Discoverer handles cluster and bastion discovery.
type Discoverer struct {
	ociClient client.OCIClientInterface
	cache     *Cache
}

// NewDiscoverer creates a new discovery service.
func NewDiscoverer(ociClient client.OCIClientInterface, cache *Cache) *Discoverer {
	return &Discoverer{
		ociClient: ociClient,
		cache:     cache,
	}
}

// DiscoverCluster finds a cluster by name across all compartments and regions.
func (d *Discoverer) DiscoverCluster(ctx context.Context, clusterName string) (*DiscoveredCluster, error) {
	return d.DiscoverClusterWithHints(ctx, clusterName, nil)
}

// DiscoverClusterByOCID looks up a cluster directly by its OCID.
// This is more efficient than name-based discovery and should be used when the OCID is known.
func (d *Discoverer) DiscoverClusterByOCID(ctx context.Context, clusterOCID string) (*DiscoveredCluster, error) {
	// Validate OCID format
	ocidParts := utils.ParseOCID(clusterOCID)
	if ocidParts == nil {
		return nil, fmt.Errorf("%w: '%s' is not a valid OCID", ErrInvalidOCID, clusterOCID)
	}

	if ocidParts.ResourceType != "cluster" {
		return nil, fmt.Errorf("%w: expected cluster OCID but got '%s' OCID", ErrInvalidOCID, ocidParts.ResourceType)
	}

	// Extract region from OCID
	region := ocidParts.Region
	if region == "" {
		return nil, fmt.Errorf("%w: could not extract region from OCID", ErrInvalidOCID)
	}

	log.Info().Msgf("Looking up cluster by OCID in region %s...", region)

	// Set region and fetch cluster directly
	d.ociClient.SetRegion(region)

	fullCluster, err := d.ociClient.GetCluster(ctx, clusterOCID)
	if err != nil {
		// Classify the error to provide better messaging
		ociErr := client.ClassifyOCIError(err, "get cluster by OCID")

		switch ociErr.Type {
		case client.ErrorTypeNotAuthorizedOrNotFound:
			return nil, fmt.Errorf("%w: cluster OCID '%s' not accessible\n\n%s",
				ErrClusterAccessDenied, clusterOCID, ociErr.Suggestion)
		case client.ErrorTypeNotAuthenticated:
			return nil, fmt.Errorf("authentication failed when accessing cluster\n\n%s", ociErr.Suggestion)
		case client.ErrorTypeNotAuthorized:
			return nil, fmt.Errorf("%w: insufficient permissions to access cluster '%s'\n\n%s",
				ErrClusterAccessDenied, clusterOCID, ociErr.Suggestion)
		case client.ErrorTypeNotFound:
			return nil, fmt.Errorf("%w: cluster '%s' does not exist in region %s\n\n%s",
				ErrClusterNotFound, clusterOCID, region, ociErr.Suggestion)
		default:
			return nil, fmt.Errorf("failed to get cluster: %w", err)
		}
	}

	// Build discovered cluster from response
	cluster := &DiscoveredCluster{
		OCID:   clusterOCID,
		Region: region,
	}

	if fullCluster.Name != nil {
		cluster.Name = *fullCluster.Name
	}

	if fullCluster.CompartmentId != nil {
		cluster.CompartmentID = *fullCluster.CompartmentId
	}

	if fullCluster.VcnId != nil {
		cluster.VcnID = *fullCluster.VcnId
	}

	if fullCluster.EndpointConfig != nil && fullCluster.EndpointConfig.SubnetId != nil {
		cluster.SubnetID = *fullCluster.EndpointConfig.SubnetId
	}

	if fullCluster.Endpoints != nil && fullCluster.Endpoints.PrivateEndpoint != nil {
		cluster.EndpointIP, cluster.EndpointPort = parseEndpoint(*fullCluster.Endpoints.PrivateEndpoint)
	}

	// Cache the result using the cluster name as key
	if d.cache != nil && cluster.Name != "" {
		if err := d.cache.SetCluster(cluster.Name, &CacheEntry{
			OCID:            cluster.OCID,
			Region:          cluster.Region,
			CompartmentOCID: cluster.CompartmentID,
			VcnID:           cluster.VcnID,
			SubnetID:        cluster.SubnetID,
			EndpointIP:      cluster.EndpointIP,
			EndpointPort:    cluster.EndpointPort,
		}); err != nil {
			log.Warn().Err(err).Msg("Failed to cache cluster info")
		}
	}

	log.Info().Msgf("Found cluster '%s' in region %s", cluster.Name, cluster.Region)

	return cluster, nil
}

// IsClusterOCID returns true if the input looks like a cluster OCID.
func IsClusterOCID(input string) bool {
	return utils.IsClusterOCID(input)
}

// DiscoverClusterWithHints finds a cluster using optional hints to speed up discovery.
func (d *Discoverer) DiscoverClusterWithHints(ctx context.Context, clusterName string, hints *DiscoveryHints) (*DiscoveredCluster, error) {
	// Check cache first
	if d.cache != nil {
		if cached := d.cache.GetCluster(clusterName); cached != nil {
			log.Info().Msgf("Using cached cluster info for '%s' (expires in %s)",
				clusterName, d.cache.GetClusterTTL(clusterName).Round(time.Minute))
			return &DiscoveredCluster{
				OCID:          cached.OCID,
				Name:          clusterName,
				CompartmentID: cached.CompartmentOCID,
				Region:        cached.Region,
				VcnID:         cached.VcnID,
				SubnetID:      cached.SubnetID,
				EndpointIP:    cached.EndpointIP,
				EndpointPort:  cached.EndpointPort,
			}, nil
		}
	}

	log.Info().Msgf("Discovering cluster '%s'...", clusterName)

	// Get tenancy OCID
	tenancyOCID, err := d.ociClient.GetTenancyOCID()
	if err != nil {
		return nil, fmt.Errorf("failed to get tenancy OCID: %w", err)
	}

	// Get regions to search
	regions, err := d.getRegionsToSearch(ctx, tenancyOCID, hints)
	if err != nil {
		return nil, fmt.Errorf("failed to get regions: %w", err)
	}

	log.Debug().Msgf("Searching %d regions: %v", len(regions), regions)

	// Search each region
	var allMatches []*DiscoveredCluster
	var mu sync.Mutex

	for _, region := range regions {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		log.Debug().Msgf("Searching region: %s", region)
		d.ociClient.SetRegion(region)

		matches, err := d.searchClusterInRegion(ctx, tenancyOCID, clusterName, region, hints)
		if err != nil {
			log.Warn().Err(err).Msgf("Error searching region %s", region)
			continue
		}

		mu.Lock()
		allMatches = append(allMatches, matches...)
		mu.Unlock()

		// If we found exactly one and no hints specified, we can return early
		if len(allMatches) == 1 && (hints == nil || hints.Region == "") {
			break
		}
	}

	if len(allMatches) == 0 {
		return nil, fmt.Errorf("%w: '%s' not found in any accessible compartment across %d regions",
			ErrClusterNotFound, clusterName, len(regions))
	}

	if len(allMatches) > 1 {
		// Return error with details about all matches
		var details []string
		for _, m := range allMatches {
			details = append(details, fmt.Sprintf("  - %s (region: %s, compartment: %s)",
				m.OCID, m.Region, m.CompartmentPath))
		}
		return nil, fmt.Errorf("%w: '%s' found in multiple locations:\n%s\n\nUse --region to specify which one to use",
			ErrMultipleClustersFound, clusterName, strings.Join(details, "\n"))
	}

	cluster := allMatches[0]

	// Get full cluster details
	d.ociClient.SetRegion(cluster.Region)
	fullCluster, err := d.ociClient.GetCluster(ctx, cluster.OCID)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster details: %w", err)
	}

	// Extract endpoint info
	if fullCluster.Endpoints != nil && fullCluster.Endpoints.PrivateEndpoint != nil {
		cluster.EndpointIP, cluster.EndpointPort = parseEndpoint(*fullCluster.Endpoints.PrivateEndpoint)
	}

	// Extract VCN and subnet
	if fullCluster.VcnId != nil {
		cluster.VcnID = *fullCluster.VcnId
	}
	if fullCluster.EndpointConfig != nil && fullCluster.EndpointConfig.SubnetId != nil {
		cluster.SubnetID = *fullCluster.EndpointConfig.SubnetId
	}

	// Cache the result
	if d.cache != nil {
		if err := d.cache.SetCluster(clusterName, &CacheEntry{
			OCID:            cluster.OCID,
			Region:          cluster.Region,
			CompartmentOCID: cluster.CompartmentID,
			VcnID:           cluster.VcnID,
			SubnetID:        cluster.SubnetID,
			EndpointIP:      cluster.EndpointIP,
			EndpointPort:    cluster.EndpointPort,
		}); err != nil {
			log.Warn().Err(err).Msg("Failed to cache cluster info")
		}
	}

	log.Info().Msgf("Discovered cluster '%s' in region %s (compartment: %s)",
		clusterName, cluster.Region, cluster.CompartmentPath)

	return cluster, nil
}

// getRegionsToSearch determines which regions to search.
func (d *Discoverer) getRegionsToSearch(ctx context.Context, tenancyOCID string, hints *DiscoveryHints) ([]string, error) {
	// If hint specifies a region, use only that
	if hints != nil && hints.Region != "" {
		return []string{hints.Region}, nil
	}

	// Get subscribed regions
	subscriptions, err := d.ociClient.GetSubscribedRegions(ctx, tenancyOCID)
	if err != nil {
		return nil, err
	}

	// Put home region first
	var regions []string
	var homeRegion string

	for _, sub := range subscriptions {
		if sub.RegionName == nil {
			continue
		}
		if sub.IsHomeRegion != nil && *sub.IsHomeRegion {
			homeRegion = *sub.RegionName
		} else {
			regions = append(regions, *sub.RegionName)
		}
	}

	if homeRegion != "" {
		regions = append([]string{homeRegion}, regions...)
	}

	return regions, nil
}

// searchClusterInRegion searches for a cluster in a specific region.
func (d *Discoverer) searchClusterInRegion(ctx context.Context, tenancyOCID, clusterName, region string, _ *DiscoveryHints) ([]*DiscoveredCluster, error) {
	// Build compartment tree
	tree, err := BuildCompartmentTree(ctx, d.ociClient, tenancyOCID)
	if err != nil {
		return nil, err
	}

	var matches []*DiscoveredCluster
	var mu sync.Mutex

	// Search each compartment
	err = tree.ForEachParallel(ctx, 5, func(ctx context.Context, node *CompartmentNode) error {
		clusters, err := d.ociClient.ListClustersInCompartment(ctx, node.ID)
		if err != nil {
			// Log but don't fail - user may not have access to all compartments
			log.Debug().Err(err).Msgf("Failed to list clusters in compartment %s", node.Path)
			return nil
		}

		for _, c := range clusters {
			if c.Name != nil && strings.EqualFold(*c.Name, clusterName) {
				match := &DiscoveredCluster{
					OCID:            *c.Id,
					Name:            *c.Name,
					CompartmentID:   node.ID,
					CompartmentPath: node.Path,
					Region:          region,
				}

				mu.Lock()
				matches = append(matches, match)
				mu.Unlock()
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return matches, nil
}

// DiscoverBastion finds a bastion that can reach the cluster's private endpoint.
func (d *Discoverer) DiscoverBastion(ctx context.Context, cluster *DiscoveredCluster) (*DiscoveredBastion, error) {
	// Check cache first
	if d.cache != nil {
		if cached := d.cache.GetBastion(cluster.Name); cached != nil {
			log.Info().Msgf("Using cached bastion info for cluster '%s'", cluster.Name)
			return &DiscoveredBastion{
				OCID:          cached.OCID,
				CompartmentID: cached.CompartmentOCID,
			}, nil
		}
	}

	log.Info().Msgf("Discovering bastion for cluster '%s'...", cluster.Name)

	// Set region
	d.ociClient.SetRegion(cluster.Region)

	// List bastions in the cluster's compartment
	bastions, err := d.ociClient.ListBastions(ctx, cluster.CompartmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to list bastions: %w", err)
	}

	if len(bastions) == 0 {
		return nil, fmt.Errorf("%w: no bastions found in compartment %s", ErrNoBastionFound, cluster.CompartmentPath)
	}

	// Find the first active bastion
	// TODO: Could be smarter about matching bastion to cluster's subnet
	for _, b := range bastions {
		if b.LifecycleState == "ACTIVE" && b.Id != nil {
			// Get full bastion details
			fullBastion, err := d.ociClient.GetBastion(ctx, *b.Id)
			if err != nil {
				continue
			}

			bastion := &DiscoveredBastion{
				OCID:          *b.Id,
				CompartmentID: cluster.CompartmentID,
			}

			if b.Name != nil {
				bastion.Name = *b.Name
			}

			if fullBastion.BastionType != nil {
				bastion.Type = *fullBastion.BastionType
			} else {
				bastion.Type = "STANDARD"
			}

			// Cache the result
			if d.cache != nil {
				if err := d.cache.SetBastion(cluster.Name, &CacheEntry{
					OCID:            bastion.OCID,
					CompartmentOCID: bastion.CompartmentID,
					Region:          cluster.Region,
				}); err != nil {
					log.Warn().Err(err).Msg("Failed to cache bastion info")
				}
			}

			log.Info().Msgf("Discovered bastion '%s' (%s)", bastion.Name, bastion.Type)
			return bastion, nil
		}
	}

	return nil, fmt.Errorf("%w: no active bastions found", ErrNoBastionFound)
}

// ResolveToConfig converts discovered resources to config.Cluster format.
func (d *Discoverer) ResolveToConfig(discovered *DiscoveredCluster, bastion *DiscoveredBastion) (*config.Cluster, error) {
	cluster := &config.Cluster{
		ClusterName:     discovered.Name,
		Region:          discovered.Region,
		Ocid:            &discovered.OCID,
		CompartmentOcid: &discovered.CompartmentID,
	}

	if bastion != nil {
		cluster.BastionId = &bastion.OCID
		cluster.BastionType = &bastion.Type
	}

	if discovered.EndpointIP != "" {
		cluster.Endpoints = []*config.ClusterEndpoint{
			{
				Name: "private",
				Ip:   discovered.EndpointIP,
				Port: discovered.EndpointPort,
			},
		}
	}

	return cluster, nil
}

// parseEndpoint parses an endpoint string like "10.0.1.100:6443" into IP and port.
func parseEndpoint(endpoint string) (string, int) {
	parts := strings.Split(endpoint, ":")
	if len(parts) != 2 {
		return endpoint, 6443
	}

	ip := parts[0]
	port := 6443

	if _, err := fmt.Sscanf(parts[1], "%d", &port); err != nil {
		port = 6443
	}

	return ip, port
}

// GetMultipleClusterChoices returns formatted choices for interactive selection.
func GetMultipleClusterChoices(clusters []*DiscoveredCluster) []string {
	var choices []string
	for _, c := range clusters {
		choices = append(choices, fmt.Sprintf("%s (%s, %s)", c.Name, c.Region, c.CompartmentPath))
	}
	return choices
}
