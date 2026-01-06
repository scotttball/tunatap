package client

import (
	"context"
	"testing"
	"time"

	"github.com/oracle/oci-go-sdk/v65/bastion"
	"github.com/oracle/oci-go-sdk/v65/containerengine"
)

func TestMockOCIClient_BasicOperations(t *testing.T) {
	mock := NewMockOCIClient()

	// Test region
	mock.SetRegion("us-ashburn-1")
	if mock.Region != "us-ashburn-1" {
		t.Errorf("Expected region us-ashburn-1, got %s", mock.Region)
	}

	// Verify call was recorded
	calls := mock.GetCalls()
	if len(calls) != 1 || calls[0].Method != "SetRegion" {
		t.Errorf("Expected SetRegion call to be recorded, got %v", calls)
	}
}

func TestMockOCIClient_CompartmentLookup(t *testing.T) {
	mock := NewMockOCIClient()
	ctx := context.Background()

	// Add test compartment
	mock.AddCompartment("infrastructure/kubernetes", "ocid1.compartment.oc1..test123")

	// Test successful lookup
	ocid, err := mock.GetCompartmentIdByPath(ctx, "ocid1.tenancy.oc1..root", "infrastructure/kubernetes")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if *ocid != "ocid1.compartment.oc1..test123" {
		t.Errorf("Expected ocid1.compartment.oc1..test123, got %s", *ocid)
	}

	// Test not found
	_, err = mock.GetCompartmentIdByPath(ctx, "ocid1.tenancy.oc1..root", "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent compartment")
	}
}

func TestMockOCIClient_ClusterOperations(t *testing.T) {
	mock := NewMockOCIClient()
	ctx := context.Background()

	// Add test cluster
	clusterName := "prod-cluster"
	clusterId := "ocid1.cluster.oc1..testcluster"
	mock.AddCluster(&containerengine.Cluster{
		Id:   &clusterId,
		Name: &clusterName,
	})

	// Test FetchClusterId
	foundId, err := mock.FetchClusterId(ctx, "ocid1.compartment.oc1..test", "prod-cluster")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if *foundId != clusterId {
		t.Errorf("Expected %s, got %s", clusterId, *foundId)
	}

	// Test GetCluster
	cluster, err := mock.GetCluster(ctx, clusterId)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if *cluster.Name != clusterName {
		t.Errorf("Expected cluster name %s, got %s", clusterName, *cluster.Name)
	}

	// Test failure mode
	mock.ShouldFailCluster = true
	_, err = mock.FetchClusterId(ctx, "ocid1.compartment.oc1..test", "prod-cluster")
	if err == nil {
		t.Error("Expected error when ShouldFailCluster is true")
	}
}

func TestMockOCIClient_BastionOperations(t *testing.T) {
	mock := NewMockOCIClient()
	ctx := context.Background()

	// Add test bastion
	bastionId := "ocid1.bastion.oc1..testbastion"
	bastionName := "prod-bastion"
	bastionType := "STANDARD"
	mock.AddBastion(&bastion.Bastion{
		Id:          &bastionId,
		Name:        &bastionName,
		BastionType: &bastionType,
	})

	// Test ListBastions
	bastions, err := mock.ListBastions(ctx, "ocid1.compartment.oc1..test")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(bastions) != 1 {
		t.Errorf("Expected 1 bastion, got %d", len(bastions))
	}

	// Test GetBastion
	b, err := mock.GetBastion(ctx, bastionId)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if *b.Name != bastionName {
		t.Errorf("Expected bastion name %s, got %s", bastionName, *b.Name)
	}
}

func TestMockOCIClient_SessionLifecycle(t *testing.T) {
	mock := NewMockOCIClient()
	ctx := context.Background()

	bastionId := "ocid1.bastion.oc1..testbastion"

	// Create session
	sessionDetails := bastion.CreateSessionDetails{
		BastionId:   &bastionId,
		DisplayName: stringPtr("test-session"),
	}

	session, err := mock.CreateSession(ctx, bastionId, sessionDetails)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}
	if session.LifecycleState != bastion.SessionLifecycleStateCreating {
		t.Errorf("Expected CREATING state, got %s", session.LifecycleState)
	}

	sessionId := *session.Id

	// Wait for active
	activeSession, err := mock.WaitForSessionActive(ctx, bastionId, sessionId)
	if err != nil {
		t.Fatalf("Failed to wait for session: %v", err)
	}
	if activeSession.LifecycleState != bastion.SessionLifecycleStateActive {
		t.Errorf("Expected ACTIVE state, got %s", activeSession.LifecycleState)
	}

	// List sessions
	sessions, err := mock.ListSessions(ctx, bastionId)
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(sessions))
	}

	// Delete session
	err = mock.DeleteSession(ctx, bastionId, sessionId)
	if err != nil {
		t.Fatalf("Failed to delete session: %v", err)
	}

	// Verify deleted
	sessions, _ = mock.ListSessions(ctx, bastionId)
	if len(sessions) != 0 {
		t.Errorf("Expected 0 sessions after delete, got %d", len(sessions))
	}
}

func TestMockOCIClient_SessionWithDelay(t *testing.T) {
	mock := NewMockOCIClient()
	mock.CreateSessionDelay = 50 * time.Millisecond
	mock.SessionActiveDelay = 50 * time.Millisecond

	ctx := context.Background()
	bastionId := "ocid1.bastion.oc1..testbastion"

	start := time.Now()

	sessionDetails := bastion.CreateSessionDetails{
		BastionId:   &bastionId,
		DisplayName: stringPtr("delayed-session"),
	}

	session, err := mock.CreateSession(ctx, bastionId, sessionDetails)
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	_, err = mock.WaitForSessionActive(ctx, bastionId, *session.Id)
	if err != nil {
		t.Fatalf("Failed to wait for session: %v", err)
	}

	elapsed := time.Since(start)
	if elapsed < 100*time.Millisecond {
		t.Errorf("Expected delays to be honored, but elapsed time was only %v", elapsed)
	}
}

func TestMockOCIClient_SessionFailure(t *testing.T) {
	mock := NewMockOCIClient()
	mock.ShouldFailSession = true

	ctx := context.Background()
	bastionId := "ocid1.bastion.oc1..testbastion"

	sessionDetails := bastion.CreateSessionDetails{
		BastionId:   &bastionId,
		DisplayName: stringPtr("failing-session"),
	}

	_, err := mock.CreateSession(ctx, bastionId, sessionDetails)
	if err == nil {
		t.Error("Expected session creation to fail")
	}
}

func TestMockOCIClient_ObjectStorage(t *testing.T) {
	mock := NewMockOCIClient()
	ctx := context.Background()

	// Add test object
	mock.AddObject("test-ns", "test-bucket", "config.yaml", []byte("key: value"))

	// Test GetNamespace
	ns, err := mock.GetNamespace(ctx, "ocid1.tenancy.oc1..test")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if ns != "test-namespace" {
		t.Errorf("Expected test-namespace, got %s", ns)
	}

	// Test GetObject
	data, err := mock.GetObject(ctx, "test-ns", "test-bucket", "config.yaml")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if string(data) != "key: value" {
		t.Errorf("Expected 'key: value', got %s", string(data))
	}

	// Test object not found
	_, err = mock.GetObject(ctx, "test-ns", "test-bucket", "nonexistent.yaml")
	if err == nil {
		t.Error("Expected error for nonexistent object")
	}
}

func TestMockOCIClient_CallTracking(t *testing.T) {
	mock := NewMockOCIClient()
	ctx := context.Background()

	mock.SetRegion("us-ashburn-1")
	mock.GetAuthType()
	mock.GetNamespace(ctx, "test-tenancy")

	calls := mock.GetCalls()
	if len(calls) != 3 {
		t.Errorf("Expected 3 calls recorded, got %d", len(calls))
	}

	expectedMethods := []string{"SetRegion", "GetAuthType", "GetNamespace"}
	for i, expected := range expectedMethods {
		if calls[i].Method != expected {
			t.Errorf("Call %d: expected %s, got %s", i, expected, calls[i].Method)
		}
	}

	// Test reset
	mock.ResetCalls()
	calls = mock.GetCalls()
	if len(calls) != 0 {
		t.Errorf("Expected 0 calls after reset, got %d", len(calls))
	}
}

func stringPtr(s string) *string {
	return &s
}
