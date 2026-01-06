package cluster

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strings"

	"github.com/koki-develop/go-fzf"
	"github.com/oracle/oci-go-sdk/v65/bastion"
	"github.com/rs/zerolog/log"
	"github.com/scotttball/tunatap/internal/client"
	"github.com/scotttball/tunatap/internal/config"
	"github.com/scotttball/tunatap/internal/state"
	"github.com/scotttball/tunatap/pkg/utils"
)

// ValidateAndUpdateCluster validates and populates cluster configuration.
func ValidateAndUpdateCluster(ctx context.Context, ociClient *client.OCIClient, cluster *config.Cluster, useBastion bool, localPort int) error {
	SetClusterLocalPort(cluster, localPort)

	if err := SetClusterTenancy(ctx, ociClient, cluster); err != nil {
		return err
	}

	if err := SetClusterOcid(ctx, ociClient, cluster); err != nil {
		return err
	}

	SetClusterURL(cluster)

	if useBastion && cluster.BastionId == nil {
		bastionId, err := GetClusterBastion(ctx, ociClient, cluster)
		if err != nil {
			return err
		}
		cluster.BastionId = bastionId

		if cluster.BastionId != nil {
			b, err := ociClient.GetBastion(ctx, *cluster.BastionId)
			if err != nil {
				return fmt.Errorf("failed to fetch bastion: %w", err)
			}

			log.Info().Msgf("Bastion type: %s", *b.BastionType)
			cluster.BastionType = b.BastionType
		}
	}

	return nil
}

// FindAvailablePort checks if the given port is available, finds next if not.
func FindAvailablePort(startPort int) (int, error) {
	port := startPort
	for {
		ln, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", port))
		if err == nil {
			_ = ln.Close()
			return port, nil
		}

		log.Warn().Msgf("Port %d is not available, trying next port...", port)
		port++

		if port > 65535 {
			return 0, fmt.Errorf("no available ports found")
		}
	}
}

// SetClusterLocalPort sets or finds an available local port for the cluster.
// If localPort is 0 or negative, it will find any available port (ephemeral allocation).
func SetClusterLocalPort(cluster *config.Cluster, localPort int) {
	// Use cluster config port if command-line port not specified (default 6443)
	if localPort <= 0 && cluster.LocalPort != nil && *cluster.LocalPort > 0 {
		localPort = *cluster.LocalPort
	}

	// If still 0 or negative, use ephemeral port allocation
	if localPort <= 0 {
		port, err := FindEphemeralPort()
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to find available ephemeral port")
		}
		cluster.LocalPort = &port
		log.Info().Msgf("Using ephemeral port: %d", port)
		return
	}

	// Find available port starting from the specified port
	port, err := FindAvailablePort(localPort)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to find available port")
	}

	cluster.LocalPort = &port
}

// FindEphemeralPort finds any available TCP port on localhost.
func FindEphemeralPort() (int, error) {
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()

	addr := ln.Addr().(*net.TCPAddr)
	return addr.Port, nil
}

// SetClusterTenancy sets the tenancy information for the cluster.
func SetClusterTenancy(ctx context.Context, ociClient *client.OCIClient, cluster *config.Cluster) error {
	globalState := state.GetInstance()

	if cluster.Tenant != nil {
		if tenancyId, ok := globalState.GetTenancyByName(*cluster.Tenant); ok {
			cluster.TenantOcid = tenancyId
		} else {
			return fmt.Errorf("tenant '%s' not found in tenancies list", *cluster.Tenant)
		}
	}

	return nil
}

// SetClusterOcid sets the OCID and compartment information for the cluster.
func SetClusterOcid(ctx context.Context, ociClient *client.OCIClient, cluster *config.Cluster) error {
	if cluster.Ocid == nil {
		if (cluster.Compartment == nil && cluster.CompartmentOcid == nil) || cluster.Tenant == nil {
			return fmt.Errorf("compartment and tenant must be set for the cluster if the cluster id is not set")
		}

		if cluster.CompartmentOcid == nil {
			compartmentId, err := ociClient.GetCompartmentIdByPath(ctx, *cluster.TenantOcid, *cluster.Compartment)
			if err != nil {
				return fmt.Errorf("failed to get compartment id: %w", err)
			}
			cluster.CompartmentOcid = compartmentId
		}

		clusterId, err := ociClient.FetchClusterId(ctx, *cluster.CompartmentOcid, cluster.ClusterName)
		if err != nil {
			return fmt.Errorf("failed to fetch cluster id: %w", err)
		}
		cluster.Ocid = clusterId
	} else {
		if cluster.CompartmentOcid == nil {
			resp, err := ociClient.GetCluster(ctx, *cluster.Ocid)
			if err != nil {
				return fmt.Errorf("failed to fetch cluster: %w", err)
			}
			cluster.CompartmentOcid = resp.CompartmentId
		}
	}

	return nil
}

// SetClusterURL sets the OCI console URL for the cluster.
func SetClusterURL(cluster *config.Cluster) {
	if cluster.URL == nil {
		cluster.URL = utils.StringPtr(fmt.Sprintf("%s/%s?region=%s",
			"https://cloud.oracle.com/containers/clusters",
			*cluster.Ocid,
			cluster.Region))
	}
}

// GetClusterBastion finds and returns a bastion ID for the cluster.
func GetClusterBastion(ctx context.Context, ociClient *client.OCIClient, cluster *config.Cluster) (*string, error) {
	if cluster.CompartmentOcid == nil {
		return nil, fmt.Errorf("compartment OCID not set")
	}

	bastions, err := ociClient.ListBastions(ctx, *cluster.CompartmentOcid)
	if err != nil {
		return nil, fmt.Errorf("failed to list bastions: %w", err)
	}

	if len(bastions) == 0 {
		return nil, fmt.Errorf("no bastions found in compartment")
	}

	if len(bastions) == 1 {
		log.Info().Msgf("Found only one bastion. Using it, OCID: %s", *bastions[0].Id)
		return bastions[0].Id, nil
	}

	if cluster.Bastion != nil {
		return assignBastionByName(cluster, bastions)
	}

	return allowUserToSelectBastion(cluster, bastions)
}

// assignBastionByName finds a bastion by name.
func assignBastionByName(cluster *config.Cluster, bastions []bastion.BastionSummary) (*string, error) {
	idx := slices.IndexFunc(bastions, func(c bastion.BastionSummary) bool {
		return strings.EqualFold(*c.Name, *cluster.Bastion)
	})

	if idx != -1 {
		log.Info().Msgf("Found bastion '%s' with OCID: %s", *cluster.Bastion, *bastions[idx].Id)
		return bastions[idx].Id, nil
	}

	return nil, fmt.Errorf("bastion '%s' not found", *cluster.Bastion)
}

// allowUserToSelectBastion prompts the user to select a bastion interactively.
func allowUserToSelectBastion(cluster *config.Cluster, bastions []bastion.BastionSummary) (*string, error) {
	f, err := fzf.New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize fuzzy finder: %w", err)
	}

	idxs, err := f.Find(bastions, func(i int) string { return *bastions[i].Name })
	if err != nil || len(idxs) == 0 {
		return nil, fmt.Errorf("no bastion selected")
	}

	return bastions[idxs[0]].Id, nil
}

// ListClusters returns all clusters from config.
func ListClusters(cfg *config.Config) []*config.Cluster {
	return cfg.Clusters
}

// GetClusterInfo returns detailed info about a cluster.
func GetClusterInfo(cluster *config.Cluster) map[string]interface{} {
	info := map[string]interface{}{
		"name":   cluster.ClusterName,
		"region": cluster.Region,
	}

	if cluster.Ocid != nil {
		info["ocid"] = *cluster.Ocid
	}
	if cluster.Tenant != nil {
		info["tenant"] = *cluster.Tenant
	}
	if cluster.Compartment != nil {
		info["compartment"] = *cluster.Compartment
	}
	if cluster.BastionId != nil {
		info["bastion_id"] = *cluster.BastionId
	}
	if cluster.BastionType != nil {
		info["bastion_type"] = *cluster.BastionType
	}
	if cluster.LocalPort != nil {
		info["local_port"] = *cluster.LocalPort
	}
	if cluster.URL != nil {
		info["url"] = *cluster.URL
	}
	if len(cluster.Endpoints) > 0 {
		endpoints := make([]map[string]interface{}, len(cluster.Endpoints))
		for i, ep := range cluster.Endpoints {
			endpoints[i] = map[string]interface{}{
				"name": ep.Name,
				"ip":   ep.Ip,
				"port": ep.Port,
			}
		}
		info["endpoints"] = endpoints
	}

	return info
}
