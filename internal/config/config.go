package config

// Config represents the main application configuration.
type Config struct {
	// Tenancies maps tenancy names to their OCIDs (legacy format).
	Tenancies map[string]*string `yaml:"tenancies,omitempty"`

	// TenancyList is a list of tenancy configurations (new format).
	TenancyList []*TenantInfo `yaml:"tenancy_list,omitempty"`

	// Clusters is a list of cluster configurations.
	Clusters []*Cluster `yaml:"clusters,omitempty"`

	// CatalogSources is a list of remote catalog sources for team config sharing.
	CatalogSources []*CatalogSource `yaml:"catalog_sources,omitempty"`

	// RemoteConfig specifies where to fetch remote configuration from OCI Object Storage.
	RemoteConfig *RemoteConfig `yaml:"remote_config,omitempty"`

	// SshPrivateKeyFile is the path to the SSH private key for bastion connections.
	SshPrivateKeyFile string `yaml:"ssh_private_key_file,omitempty"`

	// SshSocksProxy is an optional SOCKS proxy address for SSH connections.
	SshSocksProxy string `yaml:"ssh_socks_proxy,omitempty"`

	// SshConnectionPoolSize is the maximum number of SSH connections in the pool.
	SshConnectionPoolSize *int `yaml:"ssh_connection_pool_size,omitempty"`

	// SshConnectionWarmupCount is the number of connections to pre-establish.
	SshConnectionWarmupCount *int `yaml:"ssh_connection_warmup_count,omitempty"`

	// SshConnectionMaxConcurrentUse is the max concurrent uses per SSH connection.
	SshConnectionMaxConcurrentUse *int `yaml:"ssh_connection_max_concurrent_use,omitempty"`

	// OCIAuthType specifies the OCI authentication type.
	// Options: "auto", "config", "instance_principal", "security_token", "resource_principal"
	OCIAuthType string `yaml:"oci_auth_type,omitempty"`

	// OCIConfigPath is the path to the OCI config file.
	OCIConfigPath string `yaml:"oci_config_path,omitempty"`

	// OCIProfile is the profile to use from the OCI config file.
	OCIProfile string `yaml:"oci_profile,omitempty"`

	// Zero-Touch settings

	// UseEphemeralKeys enables ephemeral in-memory SSH keys (never written to disk).
	// Default: true when SshPrivateKeyFile is not set.
	UseEphemeralKeys bool `yaml:"use_ephemeral_keys,omitempty"`

	// CacheTTLHours is the cache TTL in hours for discovered cluster mappings.
	// Default: 24 hours.
	CacheTTLHours *int `yaml:"cache_ttl_hours,omitempty"`

	// SkipDiscovery disables auto-discovery of clusters not in config.
	SkipDiscovery bool `yaml:"skip_discovery,omitempty"`

	// DiscoveryRegions specifies which regions to search during discovery.
	// If empty, all subscribed regions are searched.
	DiscoveryRegions []string `yaml:"discovery_regions,omitempty"`

	// Monitoring settings

	// HealthEndpoint is the address for the health HTTP server (e.g., "localhost:9090").
	// If set, enables health/metrics endpoints.
	HealthEndpoint string `yaml:"health_endpoint,omitempty"`

	// AuditLogging enables audit logging of tunnel connect/disconnect events.
	// Default: true
	AuditLogging *bool `yaml:"audit_logging,omitempty"`
}

// TenantInfo represents a tenancy configuration.
type TenantInfo struct {
	// Name is the display name for the tenancy.
	Name string `yaml:"name"`

	// ID is the tenancy OCID.
	ID string `yaml:"id"`

	// Namespace is the Object Storage namespace.
	Namespace string `yaml:"namespace,omitempty"`
}

// CatalogSource represents a source for shared cluster catalogs.
type CatalogSource struct {
	// Name is the display name for the catalog source.
	Name string `yaml:"name"`

	// URL is the catalog URL (https://, oci://, or file://).
	URL string `yaml:"url"`

	// Type is the source type ("https", "oci", "file").
	Type string `yaml:"type,omitempty"`

	// OCIBucket is the bucket name for OCI Object Storage sources.
	OCIBucket string `yaml:"oci_bucket,omitempty"`

	// OCIObject is the object name for OCI Object Storage sources.
	OCIObject string `yaml:"oci_object,omitempty"`

	// OCIRegion is the region for OCI Object Storage sources.
	OCIRegion string `yaml:"oci_region,omitempty"`

	// Enabled indicates if this source should be used.
	Enabled bool `yaml:"enabled"`

	// Priority determines merge order (higher wins).
	Priority int `yaml:"priority,omitempty"`
}

// RemoteConfig specifies the OCI Object Storage location for remote configuration.
type RemoteConfig struct {
	Region      string `yaml:"region"`
	TenancyOcid string `yaml:"tenancy_ocid"`
	Bucket      string `yaml:"bucket"`
	Object      string `yaml:"object"`
}

// Cluster represents a Kubernetes cluster configuration.
type Cluster struct {
	// ClusterName is the display name of the cluster.
	ClusterName string `yaml:"cluster_name"`

	// Region is the OCI region where the cluster is located.
	Region string `yaml:"region"`

	// Ocid is the cluster's OCID (optional if tenant/compartment provided).
	Ocid *string `yaml:"ocid,omitempty"`

	// Tenant is the tenancy name (from tenancies map).
	Tenant *string `yaml:"tenant,omitempty"`

	// TenantOcid is the resolved tenancy OCID.
	TenantOcid *string `yaml:"tenant_ocid,omitempty"`

	// Compartment is the compartment path (e.g., "parent/child").
	Compartment *string `yaml:"compartment,omitempty"`

	// CompartmentOcid is the resolved compartment OCID.
	CompartmentOcid *string `yaml:"compartment_ocid,omitempty"`

	// BastionId is the bastion service OCID.
	BastionId *string `yaml:"bastion_id,omitempty"`

	// BastionType is the type of bastion ("STANDARD" or "INTERNAL").
	BastionType *string `yaml:"bastion_type,omitempty"`

	// Bastion is the bastion name (for lookup).
	Bastion *string `yaml:"bastion,omitempty"`

	// JumpBoxIP is the jump box IP for internal bastions.
	JumpBoxIP *string `yaml:"jumpbox_ip,omitempty"`

	// LocalPort is the local port for the tunnel.
	LocalPort *int `yaml:"local_port,omitempty"`

	// URL is the OCI console URL for the cluster.
	URL *string `yaml:"url,omitempty"`

	// Endpoints contains the cluster API endpoints.
	Endpoints []*ClusterEndpoint `yaml:"endpoints,omitempty"`
}

// ClusterEndpoint represents a cluster API endpoint.
type ClusterEndpoint struct {
	// Name is the endpoint name (e.g., "private", "public").
	Name string `yaml:"name,omitempty"`

	// Ip is the endpoint IP address.
	Ip string `yaml:"ip"`

	// Port is the endpoint port.
	Port int `yaml:"port"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	poolSize := 5
	warmupCount := 2
	maxConcurrent := 10

	return &Config{
		Tenancies:                     make(map[string]*string),
		Clusters:                      []*Cluster{},
		SshConnectionPoolSize:         &poolSize,
		SshConnectionWarmupCount:      &warmupCount,
		SshConnectionMaxConcurrentUse: &maxConcurrent,
	}
}

// GetPoolSize returns the connection pool size with default fallback.
func (c *Config) GetPoolSize() int {
	if c.SshConnectionPoolSize != nil {
		return *c.SshConnectionPoolSize
	}
	return 5
}

// GetWarmupCount returns the warmup count with default fallback.
func (c *Config) GetWarmupCount() int {
	if c.SshConnectionWarmupCount != nil {
		return *c.SshConnectionWarmupCount
	}
	return 2
}

// GetMaxConcurrent returns the max concurrent use with default fallback.
func (c *Config) GetMaxConcurrent() int {
	if c.SshConnectionMaxConcurrentUse != nil {
		return *c.SshConnectionMaxConcurrentUse
	}
	return 10
}

// GetCacheTTLHours returns the cache TTL in hours with default fallback.
func (c *Config) GetCacheTTLHours() int {
	if c.CacheTTLHours != nil {
		return *c.CacheTTLHours
	}
	return 24 // Default 24 hours
}

// IsAuditLoggingEnabled returns whether audit logging is enabled (default: true).
func (c *Config) IsAuditLoggingEnabled() bool {
	if c.AuditLogging != nil {
		return *c.AuditLogging
	}
	return true // Enabled by default
}
