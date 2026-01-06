package preflight

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/v65/bastion"
	"github.com/rs/zerolog/log"
	"github.com/scotttball/tunatap/internal/client"
	"github.com/scotttball/tunatap/internal/config"
)

// CheckResult represents the result of a preflight check.
type CheckResult struct {
	Name        string
	Status      CheckStatus
	Message     string
	Details     string
	Suggestion  string
	AutoFixable bool
}

// CheckStatus represents the status of a check.
type CheckStatus string

const (
	StatusOK      CheckStatus = "ok"
	StatusWarning CheckStatus = "warning"
	StatusError   CheckStatus = "error"
	StatusSkipped CheckStatus = "skipped"
)

// CheckFunc is a function that performs a preflight check.
type CheckFunc func(ctx context.Context, opts *CheckOptions) CheckResult

// CheckOptions contains options for preflight checks.
type CheckOptions struct {
	Config      *config.Config
	Cluster     *config.Cluster
	OCIClient   *client.OCIClient
	Verbose     bool
	Timeout     time.Duration
	SkipNetwork bool
}

// Checker performs preflight checks.
type Checker struct {
	opts   *CheckOptions
	checks []CheckFunc
}

// NewChecker creates a new preflight checker.
func NewChecker(opts *CheckOptions) *Checker {
	c := &Checker{opts: opts}
	c.registerChecks()
	return c
}

// registerChecks registers all preflight checks.
func (c *Checker) registerChecks() {
	c.checks = []CheckFunc{
		CheckOCIAuthentication,
		CheckOCICLIInstalled,
		CheckBastionServiceHealth,
		CheckBastionIAMPermissions,
		CheckClusterAccess,
		CheckSSHAgentAvailable,
		CheckBastionEndpointReachable,
	}
}

// RunAll runs all preflight checks.
func (c *Checker) RunAll(ctx context.Context) []CheckResult {
	results := make([]CheckResult, 0, len(c.checks))
	for _, check := range c.checks {
		result := check(ctx, c.opts)
		results = append(results, result)
	}
	return results
}

// RunForCluster runs cluster-specific preflight checks.
func (c *Checker) RunForCluster(ctx context.Context) []CheckResult {
	if c.opts.Cluster == nil {
		return []CheckResult{{
			Name:    "Cluster Selection",
			Status:  StatusError,
			Message: "No cluster specified",
		}}
	}

	results := make([]CheckResult, 0)
	results = append(results, CheckOCIAuthentication(ctx, c.opts))
	results = append(results, CheckBastionServiceHealth(ctx, c.opts))
	results = append(results, CheckClusterAccess(ctx, c.opts))

	if !c.opts.SkipNetwork {
		results = append(results, CheckBastionEndpointReachable(ctx, c.opts))
	}

	return results
}

// CheckOCIAuthentication verifies OCI authentication is working.
func CheckOCIAuthentication(ctx context.Context, opts *CheckOptions) CheckResult {
	result := CheckResult{
		Name:        "OCI Authentication",
		AutoFixable: false,
	}

	if opts.OCIClient == nil {
		result.Status = StatusSkipped
		result.Message = "OCI client not available"
		return result
	}

	// Try to get the tenancy ID as a simple auth check
	authType := opts.OCIClient.GetAuthType()

	result.Status = StatusOK
	result.Message = fmt.Sprintf("Authenticated using %s", authType)
	return result
}

// CheckOCICLIInstalled verifies the OCI CLI is installed and configured.
func CheckOCICLIInstalled(ctx context.Context, opts *CheckOptions) CheckResult {
	result := CheckResult{
		Name:        "OCI CLI",
		AutoFixable: true,
	}

	// Check if OCI CLI is in PATH
	ociPath, err := exec.LookPath("oci")
	if err != nil {
		result.Status = StatusWarning
		result.Message = "OCI CLI not found in PATH"
		result.Suggestion = "Install OCI CLI: https://docs.oracle.com/en-us/iaas/Content/API/SDKDocs/cliinstall.htm"
		result.Details = "The OCI CLI is required for exec-auth kubeconfig generation"
		return result
	}

	// Check OCI CLI version
	cmd := exec.CommandContext(ctx, ociPath, "--version")
	output, err := cmd.Output()
	if err != nil {
		result.Status = StatusWarning
		result.Message = "OCI CLI installed but version check failed"
		result.Details = err.Error()
		return result
	}

	version := strings.TrimSpace(string(output))
	result.Status = StatusOK
	result.Message = fmt.Sprintf("Installed (%s)", version)
	return result
}

// CheckBastionServiceHealth verifies the bastion service is healthy.
func CheckBastionServiceHealth(ctx context.Context, opts *CheckOptions) CheckResult {
	result := CheckResult{
		Name:        "Bastion Service",
		AutoFixable: false,
	}

	if opts.OCIClient == nil {
		result.Status = StatusSkipped
		result.Message = "OCI client not available"
		return result
	}

	if opts.Cluster == nil || opts.Cluster.BastionId == nil {
		result.Status = StatusSkipped
		result.Message = "No bastion configured for cluster"
		return result
	}

	// Get bastion details
	bastionInfo, err := opts.OCIClient.GetBastion(ctx, *opts.Cluster.BastionId)
	if err != nil {
		result.Status = StatusError
		result.Message = "Failed to get bastion details"
		result.Details = err.Error()
		result.Suggestion = "Check that the bastion ID is correct and you have permission to access it"
		return result
	}

	// Check bastion lifecycle state
	switch bastionInfo.LifecycleState {
	case bastion.BastionLifecycleStateActive:
		result.Status = StatusOK
		result.Message = fmt.Sprintf("Bastion '%s' is active", *bastionInfo.Name)
	case bastion.BastionLifecycleStateCreating:
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("Bastion '%s' is still being created", *bastionInfo.Name)
		result.Suggestion = "Wait for bastion creation to complete"
	case bastion.BastionLifecycleStateDeleted, bastion.BastionLifecycleStateDeleting:
		result.Status = StatusError
		result.Message = fmt.Sprintf("Bastion '%s' is deleted/deleting", *bastionInfo.Name)
		result.Suggestion = "Create a new bastion or update cluster configuration"
	case bastion.BastionLifecycleStateFailed:
		result.Status = StatusError
		result.Message = fmt.Sprintf("Bastion '%s' is in failed state", *bastionInfo.Name)
		result.Suggestion = "Check the bastion service in OCI Console for errors"
	default:
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("Bastion '%s' is in state: %s", *bastionInfo.Name, bastionInfo.LifecycleState)
	}

	return result
}

// CheckBastionIAMPermissions verifies IAM permissions for bastion operations.
func CheckBastionIAMPermissions(ctx context.Context, opts *CheckOptions) CheckResult {
	result := CheckResult{
		Name:        "Bastion IAM Permissions",
		AutoFixable: false,
	}

	if opts.OCIClient == nil {
		result.Status = StatusSkipped
		result.Message = "OCI client not available"
		return result
	}

	if opts.Cluster == nil || opts.Cluster.CompartmentOcid == nil {
		result.Status = StatusSkipped
		result.Message = "No compartment configured for cluster"
		return result
	}

	// Try to list bastions in the compartment as a permission check
	bastions, err := opts.OCIClient.ListBastions(ctx, *opts.Cluster.CompartmentOcid)
	if err != nil {
		if strings.Contains(err.Error(), "NotAuthorizedOrNotFound") ||
			strings.Contains(err.Error(), "Authorization failed") {
			result.Status = StatusError
			result.Message = "Missing IAM permissions for bastion operations"
			result.Details = err.Error()
			result.Suggestion = "Ensure your user/group has policies like:\n" +
				"  Allow group <group> to manage bastion-session in compartment <compartment>\n" +
				"  Allow group <group> to read bastion in compartment <compartment>"
			return result
		}

		result.Status = StatusWarning
		result.Message = "Could not verify IAM permissions"
		result.Details = err.Error()
		return result
	}

	result.Status = StatusOK
	result.Message = fmt.Sprintf("Can list bastions (found %d in compartment)", len(bastions))
	return result
}

// CheckClusterAccess verifies access to the OKE cluster.
func CheckClusterAccess(ctx context.Context, opts *CheckOptions) CheckResult {
	result := CheckResult{
		Name:        "Cluster Access",
		AutoFixable: false,
	}

	if opts.OCIClient == nil {
		result.Status = StatusSkipped
		result.Message = "OCI client not available"
		return result
	}

	if opts.Cluster == nil || opts.Cluster.Ocid == nil {
		result.Status = StatusSkipped
		result.Message = "No cluster OCID configured"
		return result
	}

	// Try to get cluster details
	cluster, err := opts.OCIClient.GetCluster(ctx, *opts.Cluster.Ocid)
	if err != nil {
		if strings.Contains(err.Error(), "NotAuthorizedOrNotFound") ||
			strings.Contains(err.Error(), "Authorization failed") {
			result.Status = StatusError
			result.Message = "Cannot access cluster - permission denied"
			result.Details = err.Error()
			result.Suggestion = "Ensure your user/group has policies like:\n" +
				"  Allow group <group> to use clusters in compartment <compartment>"
			return result
		}

		result.Status = StatusError
		result.Message = "Failed to access cluster"
		result.Details = err.Error()
		return result
	}

	result.Status = StatusOK
	result.Message = fmt.Sprintf("Cluster '%s' accessible (state: %s)", *cluster.Name, cluster.LifecycleState)
	return result
}

// CheckSSHAgentAvailable checks if SSH agent is running and has keys.
func CheckSSHAgentAvailable(ctx context.Context, opts *CheckOptions) CheckResult {
	result := CheckResult{
		Name:        "SSH Agent",
		AutoFixable: true,
	}

	// Check SSH_AUTH_SOCK environment variable
	authSock := os.Getenv("SSH_AUTH_SOCK")
	if authSock == "" {
		result.Status = StatusWarning
		result.Message = "SSH agent not detected (SSH_AUTH_SOCK not set)"
		result.Suggestion = "Start SSH agent with: eval $(ssh-agent -s) && ssh-add"
		result.Details = "SSH agent is recommended for bastion authentication"
		return result
	}

	// Check if the socket exists
	if _, err := os.Stat(authSock); err != nil {
		result.Status = StatusWarning
		result.Message = "SSH agent socket not accessible"
		result.Details = fmt.Sprintf("SSH_AUTH_SOCK=%s: %v", authSock, err)
		result.Suggestion = "Restart SSH agent: eval $(ssh-agent -s) && ssh-add"
		return result
	}

	// Try to list keys in agent
	cmd := exec.CommandContext(ctx, "ssh-add", "-l")
	output, err := cmd.Output()
	if err != nil {
		result.Status = StatusWarning
		result.Message = "SSH agent running but no keys loaded"
		result.Suggestion = "Add keys with: ssh-add ~/.ssh/id_rsa (or your key path)"
		return result
	}

	keyCount := len(strings.Split(strings.TrimSpace(string(output)), "\n"))
	result.Status = StatusOK
	result.Message = fmt.Sprintf("SSH agent running with %d key(s)", keyCount)
	return result
}

// CheckBastionEndpointReachable checks network connectivity to the bastion.
func CheckBastionEndpointReachable(ctx context.Context, opts *CheckOptions) CheckResult {
	result := CheckResult{
		Name:        "Bastion Network",
		AutoFixable: false,
	}

	if opts.Cluster == nil || opts.Cluster.BastionId == nil {
		result.Status = StatusSkipped
		result.Message = "No bastion configured"
		return result
	}

	// Construct bastion host address
	bastionHost := fmt.Sprintf("host.bastion.%s.oci.oraclecloud.com", opts.Cluster.Region)

	// Try to resolve DNS
	_, err := net.LookupHost(bastionHost)
	if err != nil {
		result.Status = StatusWarning
		result.Message = fmt.Sprintf("Cannot resolve bastion host: %s", bastionHost)
		result.Details = err.Error()
		result.Suggestion = "Check your DNS configuration and network connectivity"
		return result
	}

	// Try TCP connection to SSH port with timeout
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	address := fmt.Sprintf("%s:22", bastionHost)
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		result.Status = StatusWarning
		result.Message = "Cannot reach bastion SSH endpoint"
		result.Details = fmt.Sprintf("Connection to %s failed: %v", address, err)
		result.Suggestion = "Check firewall rules and network connectivity"
		return result
	}
	conn.Close()

	result.Status = StatusOK
	result.Message = fmt.Sprintf("Bastion endpoint reachable at %s", bastionHost)
	return result
}

// CheckClusterEndpointReachable checks if the cluster's private endpoint is reachable.
// This is typically only reachable through the bastion tunnel.
func CheckClusterEndpointReachable(ctx context.Context, opts *CheckOptions) CheckResult {
	result := CheckResult{
		Name:        "Cluster Endpoint",
		AutoFixable: false,
	}

	if opts.Cluster == nil || len(opts.Cluster.Endpoints) == 0 {
		result.Status = StatusSkipped
		result.Message = "No cluster endpoints configured"
		return result
	}

	endpoint := opts.Cluster.Endpoints[0]
	address := fmt.Sprintf("%s:%d", endpoint.Ip, endpoint.Port)

	// Note: This will typically fail since the cluster endpoint is private
	// This check is mainly informational
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 3 * time.Second
	}

	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		// Expected for private endpoints
		result.Status = StatusOK
		result.Message = fmt.Sprintf("Cluster endpoint %s is private (requires tunnel)", address)
		result.Details = "This is expected - use tunatap to create a tunnel"
		return result
	}
	conn.Close()

	// If we can connect directly, it might be a public endpoint
	result.Status = StatusOK
	result.Message = fmt.Sprintf("Cluster endpoint %s is directly reachable", address)
	result.Details = "Note: Direct connectivity may indicate a public endpoint"
	return result
}

// PrintResults prints check results in a formatted way.
func PrintResults(results []CheckResult, verbose bool) {
	fmt.Println()
	for _, r := range results {
		icon := getStatusIcon(r.Status)
		fmt.Printf("%s %s: %s\n", icon, r.Name, r.Message)

		if verbose || r.Status == StatusError {
			if r.Details != "" {
				fmt.Printf("    Details: %s\n", r.Details)
			}
			if r.Suggestion != "" {
				fmt.Printf("    Suggestion: %s\n", r.Suggestion)
			}
		}
	}
	fmt.Println()
}

// getStatusIcon returns an icon for the status.
func getStatusIcon(status CheckStatus) string {
	switch status {
	case StatusOK:
		return "✓"
	case StatusWarning:
		return "⚠"
	case StatusError:
		return "✗"
	case StatusSkipped:
		return "○"
	default:
		return "?"
	}
}

// HasErrors returns true if any results have error status.
func HasErrors(results []CheckResult) bool {
	for _, r := range results {
		if r.Status == StatusError {
			return true
		}
	}
	return false
}

// HasWarnings returns true if any results have warning status.
func HasWarnings(results []CheckResult) bool {
	for _, r := range results {
		if r.Status == StatusWarning {
			return true
		}
	}
	return false
}

// GetAutoFixable returns results that can be auto-fixed.
func GetAutoFixable(results []CheckResult) []CheckResult {
	fixable := make([]CheckResult, 0)
	for _, r := range results {
		if r.AutoFixable && (r.Status == StatusWarning || r.Status == StatusError) {
			fixable = append(fixable, r)
		}
	}
	return fixable
}

// RunQuickCheck runs a quick connectivity check for a cluster.
func RunQuickCheck(ctx context.Context, ociClient *client.OCIClient, cluster *config.Cluster) error {
	opts := &CheckOptions{
		OCIClient:   ociClient,
		Cluster:     cluster,
		SkipNetwork: false,
		Timeout:     5 * time.Second,
	}

	results := []CheckResult{
		CheckOCIAuthentication(ctx, opts),
		CheckBastionServiceHealth(ctx, opts),
	}

	for _, r := range results {
		if r.Status == StatusError {
			log.Error().Str("check", r.Name).Msg(r.Message)
			if r.Suggestion != "" {
				log.Info().Msg(r.Suggestion)
			}
			return fmt.Errorf("%s: %s", r.Name, r.Message)
		}
		if r.Status == StatusWarning {
			log.Warn().Str("check", r.Name).Msg(r.Message)
		}
	}

	return nil
}
