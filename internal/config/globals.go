package config

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/rs/zerolog/log"
	"github.com/scotttball/tunatap/internal/state"
	"gopkg.in/yaml.v3"
)

const (
	defaultTunaConfigFileName = "config"
	defaultOutputConfigFile   = "tunatap-config.yaml"
)

// ConfigureGlobals sets up global state from the configuration.
func ConfigureGlobals(config *Config) error {
	globalState := state.GetInstance()

	// Set tenancies in global state
	if config.Tenancies != nil {
		globalState.SetTenancies(config.Tenancies)
	}

	log.Debug().Msg("Global state configured from config")
	return nil
}

// SessionValidate validates the OCI session/token for the given profile.
func SessionValidate(configPath *string, authType, profileName, region string) error {
	// Check if the OCI config file exists
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	ociConfigPath := filepath.Join(home, ".oci", "config")
	if configPath != nil && *configPath != "" {
		ociConfigPath = *configPath
	}

	if _, err := os.Stat(ociConfigPath); os.IsNotExist(err) {
		return fmt.Errorf("OCI config file not found at %s", ociConfigPath)
	}

	// For session auth, check if token needs refresh
	if authType == "bmc_operator_access" || authType == "security_token" {
		return validateSecurityToken(ociConfigPath, profileName, region)
	}

	return nil
}

// validateSecurityToken checks if the security token is valid and refreshes if needed.
func validateSecurityToken(configPath, profileName, region string) error {
	log.Debug().Msgf("Validating security token for profile: %s", profileName)

	// Try to create a config provider to test token validity
	configProvider := common.CustomProfileConfigProvider(configPath, profileName)

	// Test the config by getting the tenancy OCID
	_, err := configProvider.TenancyOCID()
	if err != nil {
		log.Warn().Err(err).Msg("Token validation failed, attempting refresh")
		return refreshSecurityToken(profileName, region)
	}

	return nil
}

// refreshSecurityToken attempts to refresh the security token using OCI CLI.
func refreshSecurityToken(profileName, region string) error {
	log.Info().Msgf("Refreshing security token for profile: %s", profileName)

	// Try using OCI CLI to refresh the session
	cmd := exec.Command("oci", "session", "refresh", "--profile", profileName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// If refresh fails, try authenticate
		log.Warn().Msgf("Session refresh failed: %s", string(output))
		log.Info().Msg("Attempting to authenticate...")

		authCmd := exec.Command("oci", "session", "authenticate", "--profile", profileName, "--region", region)
		authCmd.Stdin = os.Stdin
		authCmd.Stdout = os.Stdout
		authCmd.Stderr = os.Stderr

		if authErr := authCmd.Run(); authErr != nil {
			return fmt.Errorf("failed to authenticate: %w", authErr)
		}
	}

	log.Info().Msg("Security token refreshed successfully")
	return nil
}

// LoadRemoteConfig loads configuration from OCI Object Storage.
func LoadRemoteConfig(ctx context.Context, config *Config, ociClient interface {
	GetNamespace(ctx context.Context, tenancyOcid string) (string, error)
	GetObject(ctx context.Context, namespace, bucket, object string) ([]byte, error)
}) (string, error) {
	if config.RemoteConfig == nil {
		return "", fmt.Errorf("remote config not specified")
	}

	ociProfileName := fmt.Sprintf("tunatap_remote_cfg_%s", config.RemoteConfig.Region)
	log.Info().Msgf("Using OCI profile: %s", ociProfileName)

	globalState := state.GetInstance()
	remoteConfig := config.RemoteConfig

	homePath := globalState.GetHomePath()
	remoteConfigPath := filepath.Join(homePath, defaultOutputConfigFile)
	configPath := filepath.Join(homePath, defaultTunaConfigFileName)
	log.Info().Msgf("Using config file: %s", configPath)

	err := SessionValidate(&configPath, "bmc_operator_access", ociProfileName, remoteConfig.Region)
	if err != nil {
		return remoteConfigPath, err
	}

	namespace, err := ociClient.GetNamespace(ctx, remoteConfig.TenancyOcid)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get object storage namespace")
		return remoteConfigPath, err
	}

	resp, err := ociClient.GetObject(ctx, namespace, remoteConfig.Bucket, remoteConfig.Object)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get remote config")
		return remoteConfigPath, err
	}

	log.Debug().Msgf("Remote config loaded: %d bytes", len(resp))

	// Save the response to the output YAML file
	err = os.WriteFile(remoteConfigPath, resp, 0644)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to save remote config to file: %s", remoteConfigPath)
		return remoteConfigPath, err
	}

	remoteCfg := Config{}
	err = yaml.Unmarshal(resp, &remoteCfg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to unmarshal remote config")
		return remoteConfigPath, err
	}

	// Merge remote config into main config
	config.Tenancies = remoteCfg.Tenancies
	config.Clusters = remoteCfg.Clusters

	return remoteConfigPath, nil
}

// FindClusterByName finds a cluster by name in the config.
func FindClusterByName(config *Config, name string) *Cluster {
	name = strings.ToLower(name)
	for _, cluster := range config.Clusters {
		if strings.ToLower(cluster.ClusterName) == name {
			return cluster
		}
	}
	return nil
}

// GetClusterEndpoint returns the first endpoint or a specific named endpoint.
func GetClusterEndpoint(cluster *Cluster, name string) *ClusterEndpoint {
	if len(cluster.Endpoints) == 0 {
		return nil
	}

	if name == "" {
		return cluster.Endpoints[0]
	}

	name = strings.ToLower(name)
	for _, ep := range cluster.Endpoints {
		if strings.ToLower(ep.Name) == name {
			return ep
		}
	}

	return cluster.Endpoints[0]
}
