package discovery

import (
	"context"
	"errors"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/containerengine"
	"github.com/scotttball/tunatap/internal/client"
)

func TestDiscoverClusterByOCID_Success(t *testing.T) {
	mock := client.NewMockOCIClient()

	// Set up a test cluster
	clusterOCID := "ocid1.cluster.oc1.us-ashburn-1.aaaaaaaatest123"
	clusterName := "test-cluster"
	compartmentID := "ocid1.compartment.oc1..testcompartment"
	vcnID := "ocid1.vcn.oc1.us-ashburn-1.testvnc"
	subnetID := "ocid1.subnet.oc1.us-ashburn-1.testsubnet"
	privateEndpoint := "10.0.1.100:6443"

	mock.AddCluster(&containerengine.Cluster{
		Id:            &clusterOCID,
		Name:          &clusterName,
		CompartmentId: &compartmentID,
		VcnId:         &vcnID,
		EndpointConfig: &containerengine.ClusterEndpointConfig{
			SubnetId: &subnetID,
		},
		Endpoints: &containerengine.ClusterEndpoints{
			PrivateEndpoint: &privateEndpoint,
		},
	})

	discoverer := NewDiscoverer(mock, nil)

	cluster, err := discoverer.DiscoverClusterByOCID(context.Background(), clusterOCID)
	if err != nil {
		t.Fatalf("DiscoverClusterByOCID failed: %v", err)
	}

	if cluster.OCID != clusterOCID {
		t.Errorf("Expected OCID %s, got %s", clusterOCID, cluster.OCID)
	}

	if cluster.Name != clusterName {
		t.Errorf("Expected name %s, got %s", clusterName, cluster.Name)
	}

	if cluster.Region != "us-ashburn-1" {
		t.Errorf("Expected region us-ashburn-1, got %s", cluster.Region)
	}

	if cluster.CompartmentID != compartmentID {
		t.Errorf("Expected compartment ID %s, got %s", compartmentID, cluster.CompartmentID)
	}

	if cluster.EndpointIP != "10.0.1.100" {
		t.Errorf("Expected endpoint IP 10.0.1.100, got %s", cluster.EndpointIP)
	}

	if cluster.EndpointPort != 6443 {
		t.Errorf("Expected endpoint port 6443, got %d", cluster.EndpointPort)
	}

	// Verify the region was set on the client
	calls := mock.GetCalls()
	var setRegionCalled bool
	for _, call := range calls {
		if call.Method == "SetRegion" && len(call.Args) > 0 && call.Args[0] == "us-ashburn-1" {
			setRegionCalled = true
			break
		}
	}
	if !setRegionCalled {
		t.Error("Expected SetRegion to be called with us-ashburn-1")
	}
}

func TestDiscoverClusterByOCID_InvalidOCIDFormat(t *testing.T) {
	mock := client.NewMockOCIClient()
	discoverer := NewDiscoverer(mock, nil)

	tests := []struct {
		name    string
		ocid    string
		wantErr error
	}{
		{
			name:    "empty string",
			ocid:    "",
			wantErr: ErrInvalidOCID,
		},
		{
			name:    "not an OCID",
			ocid:    "my-cluster-name",
			wantErr: ErrInvalidOCID,
		},
		{
			name:    "wrong resource type",
			ocid:    "ocid1.bastion.oc1.us-ashburn-1.test123",
			wantErr: ErrInvalidOCID,
		},
		{
			name:    "partial OCID",
			ocid:    "ocid1.cluster",
			wantErr: ErrInvalidOCID,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := discoverer.DiscoverClusterByOCID(context.Background(), tc.ocid)
			if err == nil {
				t.Fatalf("Expected error for OCID %q, got nil", tc.ocid)
			}

			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Expected error %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestDiscoverClusterByOCID_NotFound(t *testing.T) {
	mock := client.NewMockOCIClient()
	// Don't add any clusters - simulates not found

	discoverer := NewDiscoverer(mock, nil)

	clusterOCID := "ocid1.cluster.oc1.us-ashburn-1.doesnotexist"
	_, err := discoverer.DiscoverClusterByOCID(context.Background(), clusterOCID)

	if err == nil {
		t.Fatal("Expected error for non-existent cluster, got nil")
	}

	// The error should indicate the cluster was not found
	errStr := err.Error()
	if !containsSubstring(errStr, "not found") && !containsSubstring(errStr, "not accessible") {
		t.Errorf("Error should mention not found or not accessible: %s", errStr)
	}
}

func TestDiscoverClusterByOCID_AuthError(t *testing.T) {
	mock := client.NewMockOCIClient()

	// Simulate a 401 error
	mock.ClusterError = &mockServiceError{
		statusCode: 401,
		code:       "NotAuthenticated",
		message:    "Authentication failed",
	}

	discoverer := NewDiscoverer(mock, nil)

	clusterOCID := "ocid1.cluster.oc1.us-ashburn-1.test123"
	_, err := discoverer.DiscoverClusterByOCID(context.Background(), clusterOCID)

	if err == nil {
		t.Fatal("Expected error for auth failure, got nil")
	}

	errStr := err.Error()
	if !containsSubstring(errStr, "authentication") {
		t.Errorf("Error should mention authentication: %s", errStr)
	}
}

func TestDiscoverClusterByOCID_NotAuthorizedOrNotFound(t *testing.T) {
	mock := client.NewMockOCIClient()

	// Simulate the common NotAuthorizedOrNotFound error
	mock.ClusterError = &mockServiceError{
		statusCode: 404,
		code:       "NotAuthorizedOrNotFound",
		message:    "Authorization failed or requested resource not found",
	}

	discoverer := NewDiscoverer(mock, nil)

	clusterOCID := "ocid1.cluster.oc1.us-ashburn-1.test123"
	_, err := discoverer.DiscoverClusterByOCID(context.Background(), clusterOCID)

	if err == nil {
		t.Fatal("Expected error for NotAuthorizedOrNotFound, got nil")
	}

	if !errors.Is(err, ErrClusterAccessDenied) {
		t.Errorf("Expected ErrClusterAccessDenied, got: %v", err)
	}

	errStr := err.Error()
	// Should contain helpful suggestions
	if !containsSubstring(errStr, "IAM") || !containsSubstring(errStr, "policy") {
		t.Errorf("Error should contain IAM policy suggestions: %s", errStr)
	}
}

func TestDiscoverClusterByOCID_Forbidden(t *testing.T) {
	mock := client.NewMockOCIClient()

	// Simulate a 403 error
	mock.ClusterError = &mockServiceError{
		statusCode: 403,
		code:       "NotAuthorized",
		message:    "You don't have permission to access this resource",
	}

	discoverer := NewDiscoverer(mock, nil)

	clusterOCID := "ocid1.cluster.oc1.us-ashburn-1.test123"
	_, err := discoverer.DiscoverClusterByOCID(context.Background(), clusterOCID)

	if err == nil {
		t.Fatal("Expected error for forbidden, got nil")
	}

	if !errors.Is(err, ErrClusterAccessDenied) {
		t.Errorf("Expected ErrClusterAccessDenied, got: %v", err)
	}
}

func TestIsClusterOCID(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"ocid1.cluster.oc1.us-ashburn-1.test123", true},
		{"ocid1.cluster.oc1.eu-frankfurt-1.xyz", true},
		{"ocid1.bastion.oc1.us-ashburn-1.test123", false},
		{"my-cluster-name", false},
		{"", false},
		{"ocid1.cluster", false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := IsClusterOCID(tc.input)
			if result != tc.expected {
				t.Errorf("IsClusterOCID(%q) = %v, expected %v", tc.input, result, tc.expected)
			}
		})
	}
}

func TestParseEndpoint(t *testing.T) {
	tests := []struct {
		input        string
		expectedIP   string
		expectedPort int
	}{
		{"10.0.1.100:6443", "10.0.1.100", 6443},
		{"192.168.1.1:8443", "192.168.1.1", 8443},
		{"10.0.0.1", "10.0.0.1", 6443},  // No port, default to 6443
		{"10.0.0.1:", "10.0.0.1", 6443}, // Empty port, default to 6443
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			ip, port := parseEndpoint(tc.input)
			if ip != tc.expectedIP {
				t.Errorf("parseEndpoint(%q) IP = %s, expected %s", tc.input, ip, tc.expectedIP)
			}
			if port != tc.expectedPort {
				t.Errorf("parseEndpoint(%q) port = %d, expected %d", tc.input, port, tc.expectedPort)
			}
		})
	}
}

func TestDiscoverClusterWithHints_AuthFailure(t *testing.T) {
	mock := client.NewMockOCIClient()
	mock.ShouldFailAuth = true

	discoverer := NewDiscoverer(mock, nil)

	_, err := discoverer.DiscoverClusterWithHints(context.Background(), "test-cluster", nil)
	if err == nil {
		t.Fatal("Expected error for auth failure")
	}

	errStr := err.Error()
	if !containsSubstring(errStr, "tenancy") || !containsSubstring(errStr, "OCID") {
		t.Errorf("Error should mention tenancy OCID: %s", errStr)
	}
}

func TestDiscoverClusterWithHints_ClusterNotFound(t *testing.T) {
	mock := client.NewMockOCIClient()
	mock.AddSubscribedRegion("us-ashburn-1", true)

	discoverer := NewDiscoverer(mock, nil)

	_, err := discoverer.DiscoverClusterWithHints(context.Background(), "nonexistent-cluster", nil)
	if err == nil {
		t.Fatal("Expected error for cluster not found")
	}

	if !errors.Is(err, ErrClusterNotFound) {
		t.Errorf("Expected ErrClusterNotFound, got: %v", err)
	}
}

// mockServiceError implements common.ServiceError for testing.
type mockServiceError struct {
	statusCode int
	code       string
	message    string
}

func (e *mockServiceError) GetHTTPStatusCode() int  { return e.statusCode }
func (e *mockServiceError) GetMessage() string      { return e.message }
func (e *mockServiceError) GetCode() string         { return e.code }
func (e *mockServiceError) GetOpcRequestID() string { return "test-request-id" }
func (e *mockServiceError) Error() string           { return e.message }

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
