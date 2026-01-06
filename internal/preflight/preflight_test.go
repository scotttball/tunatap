package preflight

import (
	"context"
	"testing"
	"time"
)

func TestCheckStatusConstants(t *testing.T) {
	// Verify status constants are defined
	if StatusOK != "ok" {
		t.Errorf("StatusOK = %q, want %q", StatusOK, "ok")
	}
	if StatusWarning != "warning" {
		t.Errorf("StatusWarning = %q, want %q", StatusWarning, "warning")
	}
	if StatusError != "error" {
		t.Errorf("StatusError = %q, want %q", StatusError, "error")
	}
	if StatusSkipped != "skipped" {
		t.Errorf("StatusSkipped = %q, want %q", StatusSkipped, "skipped")
	}
}

func TestNewChecker(t *testing.T) {
	opts := &CheckOptions{
		Timeout: 5 * time.Second,
	}

	checker := NewChecker(opts)
	if checker == nil {
		t.Fatal("NewChecker returned nil")
	}

	if len(checker.checks) == 0 {
		t.Error("Checker should have registered checks")
	}
}

func TestCheckOCIAuthenticationNoClient(t *testing.T) {
	ctx := context.Background()
	opts := &CheckOptions{
		OCIClient: nil,
	}

	result := CheckOCIAuthentication(ctx, opts)

	if result.Status != StatusSkipped {
		t.Errorf("Status = %q, want %q", result.Status, StatusSkipped)
	}

	if result.Name != "OCI Authentication" {
		t.Errorf("Name = %q, want %q", result.Name, "OCI Authentication")
	}
}

func TestCheckOCICLIInstalled(t *testing.T) {
	ctx := context.Background()
	opts := &CheckOptions{}

	result := CheckOCICLIInstalled(ctx, opts)

	// Result depends on whether OCI CLI is installed
	if result.Name != "OCI CLI" {
		t.Errorf("Name = %q, want %q", result.Name, "OCI CLI")
	}

	// Either OK or Warning is acceptable
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("Status = %q, want OK or Warning", result.Status)
	}
}

func TestCheckBastionServiceHealthNoClient(t *testing.T) {
	ctx := context.Background()
	opts := &CheckOptions{
		OCIClient: nil,
	}

	result := CheckBastionServiceHealth(ctx, opts)

	if result.Status != StatusSkipped {
		t.Errorf("Status = %q, want %q", result.Status, StatusSkipped)
	}
}

func TestCheckBastionIAMPermissionsNoClient(t *testing.T) {
	ctx := context.Background()
	opts := &CheckOptions{
		OCIClient: nil,
	}

	result := CheckBastionIAMPermissions(ctx, opts)

	if result.Status != StatusSkipped {
		t.Errorf("Status = %q, want %q", result.Status, StatusSkipped)
	}
}

func TestCheckClusterAccessNoClient(t *testing.T) {
	ctx := context.Background()
	opts := &CheckOptions{
		OCIClient: nil,
	}

	result := CheckClusterAccess(ctx, opts)

	if result.Status != StatusSkipped {
		t.Errorf("Status = %q, want %q", result.Status, StatusSkipped)
	}
}

func TestCheckSSHAgentAvailable(t *testing.T) {
	ctx := context.Background()
	opts := &CheckOptions{}

	result := CheckSSHAgentAvailable(ctx, opts)

	if result.Name != "SSH Agent" {
		t.Errorf("Name = %q, want %q", result.Name, "SSH Agent")
	}

	// Result depends on SSH_AUTH_SOCK environment
	if result.Status != StatusOK && result.Status != StatusWarning {
		t.Errorf("Status = %q, want OK or Warning", result.Status)
	}
}

func TestCheckBastionEndpointReachableNoCluster(t *testing.T) {
	ctx := context.Background()
	opts := &CheckOptions{
		Cluster: nil,
	}

	result := CheckBastionEndpointReachable(ctx, opts)

	if result.Status != StatusSkipped {
		t.Errorf("Status = %q, want %q", result.Status, StatusSkipped)
	}
}

func TestGetStatusIcon(t *testing.T) {
	tests := []struct {
		status CheckStatus
		want   string
	}{
		{StatusOK, "✓"},
		{StatusWarning, "⚠"},
		{StatusError, "✗"},
		{StatusSkipped, "○"},
		{"unknown", "?"},
	}

	for _, tt := range tests {
		got := getStatusIcon(tt.status)
		if got != tt.want {
			t.Errorf("getStatusIcon(%q) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestHasErrors(t *testing.T) {
	tests := []struct {
		name    string
		results []CheckResult
		want    bool
	}{
		{
			name:    "no errors",
			results: []CheckResult{{Status: StatusOK}, {Status: StatusWarning}},
			want:    false,
		},
		{
			name:    "has error",
			results: []CheckResult{{Status: StatusOK}, {Status: StatusError}},
			want:    true,
		},
		{
			name:    "empty",
			results: []CheckResult{},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasErrors(tt.results)
			if got != tt.want {
				t.Errorf("HasErrors() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasWarnings(t *testing.T) {
	tests := []struct {
		name    string
		results []CheckResult
		want    bool
	}{
		{
			name:    "no warnings",
			results: []CheckResult{{Status: StatusOK}, {Status: StatusError}},
			want:    false,
		},
		{
			name:    "has warning",
			results: []CheckResult{{Status: StatusOK}, {Status: StatusWarning}},
			want:    true,
		},
		{
			name:    "empty",
			results: []CheckResult{},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasWarnings(tt.results)
			if got != tt.want {
				t.Errorf("HasWarnings() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetAutoFixable(t *testing.T) {
	results := []CheckResult{
		{Name: "check1", Status: StatusOK, AutoFixable: true},
		{Name: "check2", Status: StatusError, AutoFixable: true},
		{Name: "check3", Status: StatusWarning, AutoFixable: false},
		{Name: "check4", Status: StatusWarning, AutoFixable: true},
	}

	fixable := GetAutoFixable(results)

	if len(fixable) != 2 {
		t.Errorf("GetAutoFixable() returned %d results, want 2", len(fixable))
	}

	// Should include check2 and check4
	names := make(map[string]bool)
	for _, r := range fixable {
		names[r.Name] = true
	}

	if !names["check2"] {
		t.Error("Expected check2 to be fixable")
	}
	if !names["check4"] {
		t.Error("Expected check4 to be fixable")
	}
}

func TestRunAllChecks(t *testing.T) {
	ctx := context.Background()
	opts := &CheckOptions{
		Timeout: 1 * time.Second,
	}

	checker := NewChecker(opts)
	results := checker.RunAll(ctx)

	// Should have results from all registered checks
	if len(results) == 0 {
		t.Error("RunAll should return results")
	}

	// Verify all results have names
	for _, r := range results {
		if r.Name == "" {
			t.Error("Check result should have a name")
		}
	}
}

func TestRunForClusterNoCluster(t *testing.T) {
	ctx := context.Background()
	opts := &CheckOptions{
		Cluster: nil,
	}

	checker := NewChecker(opts)
	results := checker.RunForCluster(ctx)

	if len(results) != 1 {
		t.Errorf("RunForCluster with no cluster should return 1 error result, got %d", len(results))
	}

	if results[0].Status != StatusError {
		t.Errorf("Status = %q, want %q", results[0].Status, StatusError)
	}
}
