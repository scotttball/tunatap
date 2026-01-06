package kubeconfig

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Kubeconfig represents a Kubernetes configuration file.
type Kubeconfig struct {
	APIVersion     string         `yaml:"apiVersion"`
	Kind           string         `yaml:"kind"`
	CurrentContext string         `yaml:"current-context,omitempty"`
	Clusters       []ClusterEntry `yaml:"clusters"`
	Contexts       []ContextEntry `yaml:"contexts"`
	Users          []UserEntry    `yaml:"users,omitempty"`
	Preferences    map[string]any `yaml:"preferences,omitempty"`
}

// ClusterEntry represents a cluster configuration.
type ClusterEntry struct {
	Name    string        `yaml:"name"`
	Cluster ClusterConfig `yaml:"cluster"`
}

// ClusterConfig contains cluster connection details.
type ClusterConfig struct {
	Server                   string `yaml:"server"`
	InsecureSkipTLSVerify    bool   `yaml:"insecure-skip-tls-verify,omitempty"`
	CertificateAuthorityData string `yaml:"certificate-authority-data,omitempty"`
}

// ContextEntry represents a context configuration.
type ContextEntry struct {
	Name    string        `yaml:"name"`
	Context ContextConfig `yaml:"context"`
}

// ContextConfig contains context details.
type ContextConfig struct {
	Cluster   string `yaml:"cluster"`
	User      string `yaml:"user,omitempty"`
	Namespace string `yaml:"namespace,omitempty"`
}

// UserEntry represents a user configuration.
type UserEntry struct {
	Name string     `yaml:"name"`
	User UserConfig `yaml:"user"`
}

// UserConfig contains user authentication details.
type UserConfig struct {
	Token                 string      `yaml:"token,omitempty"`
	ClientCertificateData string      `yaml:"client-certificate-data,omitempty"`
	ClientKeyData         string      `yaml:"client-key-data,omitempty"`
	Exec                  *ExecConfig `yaml:"exec,omitempty"`
}

// ExecConfig represents an exec-based authentication configuration.
type ExecConfig struct {
	APIVersion         string       `yaml:"apiVersion"`
	Command            string       `yaml:"command"`
	Args               []string     `yaml:"args,omitempty"`
	Env                []ExecEnvVar `yaml:"env,omitempty"`
	InteractiveMode    string       `yaml:"interactiveMode,omitempty"`
	ProvideClusterInfo bool         `yaml:"provideClusterInfo,omitempty"`
}

// ExecEnvVar represents an environment variable for exec config.
type ExecEnvVar struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

// NewKubeconfig creates a new empty Kubeconfig.
func NewKubeconfig() *Kubeconfig {
	return &Kubeconfig{
		APIVersion:  "v1",
		Kind:        "Config",
		Clusters:    []ClusterEntry{},
		Contexts:    []ContextEntry{},
		Users:       []UserEntry{},
		Preferences: make(map[string]any),
	}
}

// AddCluster adds a cluster to the kubeconfig.
func (k *Kubeconfig) AddCluster(name, server string, insecureSkipTLS bool) {
	k.Clusters = append(k.Clusters, ClusterEntry{
		Name: name,
		Cluster: ClusterConfig{
			Server:                server,
			InsecureSkipTLSVerify: insecureSkipTLS,
		},
	})
}

// AddClusterWithCA adds a cluster with certificate authority data.
func (k *Kubeconfig) AddClusterWithCA(name, server, caData string) {
	k.Clusters = append(k.Clusters, ClusterEntry{
		Name: name,
		Cluster: ClusterConfig{
			Server:                   server,
			CertificateAuthorityData: caData,
		},
	})
}

// AddContext adds a context to the kubeconfig.
func (k *Kubeconfig) AddContext(name, clusterName, userName string) {
	k.Contexts = append(k.Contexts, ContextEntry{
		Name: name,
		Context: ContextConfig{
			Cluster: clusterName,
			User:    userName,
		},
	})
}

// AddContextWithNamespace adds a context with a default namespace.
func (k *Kubeconfig) AddContextWithNamespace(name, clusterName, userName, namespace string) {
	k.Contexts = append(k.Contexts, ContextEntry{
		Name: name,
		Context: ContextConfig{
			Cluster:   clusterName,
			User:      userName,
			Namespace: namespace,
		},
	})
}

// AddUserWithToken adds a user with token authentication.
func (k *Kubeconfig) AddUserWithToken(name, token string) {
	k.Users = append(k.Users, UserEntry{
		Name: name,
		User: UserConfig{
			Token: token,
		},
	})
}

// AddUserWithExec adds a user with exec-based authentication (e.g., for OCI).
func (k *Kubeconfig) AddUserWithExec(name, command string, args []string) {
	k.Users = append(k.Users, UserEntry{
		Name: name,
		User: UserConfig{
			Exec: &ExecConfig{
				APIVersion: "client.authentication.k8s.io/v1beta1",
				Command:    command,
				Args:       args,
			},
		},
	})
}

// AddOCIUser adds a user configured for OCI OKE authentication.
func (k *Kubeconfig) AddOCIUser(name, clusterID, region string) {
	k.AddOCIUserWithProfile(name, clusterID, region, "")
}

// AddOCIUserWithProfile adds a user configured for OCI OKE authentication with a specific profile.
func (k *Kubeconfig) AddOCIUserWithProfile(name, clusterID, region, profile string) {
	args := []string{
		"ce", "cluster", "generate-token",
		"--cluster-id", clusterID,
		"--region", region,
	}

	if profile != "" {
		args = append(args, "--profile", profile)
	}

	k.Users = append(k.Users, UserEntry{
		Name: name,
		User: UserConfig{
			Exec: &ExecConfig{
				APIVersion: "client.authentication.k8s.io/v1beta1",
				Command:    "oci",
				Args:       args,
			},
		},
	})
}

// OCIKubeconfigOptions contains options for generating an OCI OKE kubeconfig.
type OCIKubeconfigOptions struct {
	ClusterName string
	ClusterID   string
	Region      string
	Endpoint    string // The API server endpoint (e.g., https://localhost:6443)
	Profile     string // OCI config profile
	Namespace   string // Default namespace
	CAData      string // Certificate authority data (base64 encoded)
}

// NewOCIKubeconfig creates a kubeconfig for an OCI OKE cluster using exec-auth.
// This generates a kubeconfig similar to `oci ce cluster create-kubeconfig`.
func NewOCIKubeconfig(opts OCIKubeconfigOptions) *Kubeconfig {
	k := NewKubeconfig()

	contextName := fmt.Sprintf("tuna-%s", opts.ClusterName)
	clusterName := contextName
	userName := contextName

	// Add cluster
	if opts.CAData != "" {
		k.AddClusterWithCA(clusterName, opts.Endpoint, opts.CAData)
	} else {
		k.AddCluster(clusterName, opts.Endpoint, true)
	}

	// Add user with OCI exec-auth
	k.AddOCIUserWithProfile(userName, opts.ClusterID, opts.Region, opts.Profile)

	// Add context
	if opts.Namespace != "" {
		k.AddContextWithNamespace(contextName, clusterName, userName, opts.Namespace)
	} else {
		k.AddContext(contextName, clusterName, userName)
	}

	// Set current context
	k.SetCurrentContext(contextName)

	return k
}

// NewOCIKubeconfigForTunnel creates a kubeconfig for tunneled access to an OCI OKE cluster.
// Uses localhost endpoint with the tunnel port and OCI exec-auth for token generation.
func NewOCIKubeconfigForTunnel(clusterName, clusterID, region string, port int, profile string) *Kubeconfig {
	return NewOCIKubeconfig(OCIKubeconfigOptions{
		ClusterName: clusterName,
		ClusterID:   clusterID,
		Region:      region,
		Endpoint:    fmt.Sprintf("https://localhost:%d", port),
		Profile:     profile,
	})
}

// NewInsecureKubeconfig creates a simple kubeconfig without OCI auth (for testing/development).
func NewInsecureKubeconfig(clusterName string, port int) *Kubeconfig {
	k := NewKubeconfig()

	contextName := fmt.Sprintf("tuna-%s", clusterName)
	k.AddCluster(contextName, fmt.Sprintf("https://localhost:%d", port), true)
	k.AddContext(contextName, contextName, "")
	k.SetCurrentContext(contextName)

	return k
}

// SetCurrentContext sets the current context.
func (k *Kubeconfig) SetCurrentContext(name string) {
	k.CurrentContext = name
}

// WriteToFile writes the kubeconfig to a file.
func (k *Kubeconfig) WriteToFile(path string) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := yaml.Marshal(k)
	if err != nil {
		return fmt.Errorf("failed to marshal kubeconfig: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	return nil
}

// ToYAML returns the kubeconfig as YAML string.
func (k *Kubeconfig) ToYAML() (string, error) {
	data, err := yaml.Marshal(k)
	if err != nil {
		return "", fmt.Errorf("failed to marshal kubeconfig: %w", err)
	}
	return string(data), nil
}

// LoadFromFile loads a kubeconfig from a file.
func LoadFromFile(path string) (*Kubeconfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read kubeconfig: %w", err)
	}

	var k Kubeconfig
	if err := yaml.Unmarshal(data, &k); err != nil {
		return nil, fmt.Errorf("failed to parse kubeconfig: %w", err)
	}

	return &k, nil
}

// MergeKubeconfigs merges multiple kubeconfigs into one.
func MergeKubeconfigs(configs ...*Kubeconfig) *Kubeconfig {
	merged := NewKubeconfig()

	for _, cfg := range configs {
		merged.Clusters = append(merged.Clusters, cfg.Clusters...)
		merged.Contexts = append(merged.Contexts, cfg.Contexts...)
		merged.Users = append(merged.Users, cfg.Users...)

		// Use first non-empty current context
		if merged.CurrentContext == "" && cfg.CurrentContext != "" {
			merged.CurrentContext = cfg.CurrentContext
		}
	}

	return merged
}
