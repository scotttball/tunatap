package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/oracle/oci-go-sdk/v65/bastion"
	"github.com/oracle/oci-go-sdk/v65/containerengine"
	"github.com/oracle/oci-go-sdk/v65/identity"
)

// MockOCIClient is a mock implementation of OCIClientInterface for testing.
type MockOCIClient struct {
	mu sync.RWMutex

	// Configuration
	Region      string
	AuthType    AuthType
	TenancyOCID string

	// Mock data stores
	Compartments          map[string]string                           // path -> OCID
	CompartmentsByID      map[string][]identity.Compartment           // parent OCID -> child compartments
	Clusters              map[string]*containerengine.Cluster         // OCID -> Cluster
	ClustersByCompartment map[string][]containerengine.ClusterSummary // compartment OCID -> clusters
	Bastions              map[string]*bastion.Bastion                 // OCID -> Bastion
	Sessions              map[string]*bastion.Session                 // OCID -> Session
	Objects               map[string][]byte                           // "namespace/bucket/object" -> content
	Namespace             string
	SubscribedRegions     []identity.RegionSubscription

	// Behavior configuration
	CreateSessionDelay time.Duration
	SessionActiveDelay time.Duration
	ShouldFailAuth     bool
	ShouldFailSession  bool
	ShouldFailCluster  bool

	// Specific error simulation for testing
	ClusterError          error // Custom error to return from cluster operations
	BastionError          error // Custom error to return from bastion operations
	CompartmentError      error // Custom error to return from compartment operations
	RegionError           error // Custom error to return from region operations

	// Call tracking for assertions
	Calls []MockCall
}

// MockCall records a method call for test assertions.
type MockCall struct {
	Method string
	Args   []interface{}
	Time   time.Time
}

// NewMockOCIClient creates a new mock client with default configuration.
func NewMockOCIClient() *MockOCIClient {
	return &MockOCIClient{
		AuthType:              AuthTypeConfigFile,
		TenancyOCID:           "ocid1.tenancy.oc1..mock",
		Compartments:          make(map[string]string),
		CompartmentsByID:      make(map[string][]identity.Compartment),
		Clusters:              make(map[string]*containerengine.Cluster),
		ClustersByCompartment: make(map[string][]containerengine.ClusterSummary),
		Bastions:              make(map[string]*bastion.Bastion),
		Sessions:              make(map[string]*bastion.Session),
		Objects:               make(map[string][]byte),
		Namespace:             "test-namespace",
		SubscribedRegions:     []identity.RegionSubscription{},
		Calls:                 make([]MockCall, 0),
	}
}

func (m *MockOCIClient) recordCall(method string, args ...interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls = append(m.Calls, MockCall{
		Method: method,
		Args:   args,
		Time:   time.Now(),
	})
}

// GetCalls returns all recorded calls (thread-safe).
func (m *MockOCIClient) GetCalls() []MockCall {
	m.mu.RLock()
	defer m.mu.RUnlock()
	calls := make([]MockCall, len(m.Calls))
	copy(calls, m.Calls)
	return calls
}

// ResetCalls clears recorded calls.
func (m *MockOCIClient) ResetCalls() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls = make([]MockCall, 0)
}

// SetRegion sets the region for the mock client.
func (m *MockOCIClient) SetRegion(region string) {
	m.recordCall("SetRegion", region)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Region = region
}

// GetAuthType returns the authentication type.
func (m *MockOCIClient) GetAuthType() AuthType {
	m.recordCall("GetAuthType")
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.AuthType
}

// GetNamespace returns the mock namespace.
func (m *MockOCIClient) GetNamespace(ctx context.Context, tenancyOcid string) (string, error) {
	m.recordCall("GetNamespace", tenancyOcid)
	if m.ShouldFailAuth {
		return "", fmt.Errorf("mock auth failure")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Namespace, nil
}

// GetObject retrieves a mock object.
func (m *MockOCIClient) GetObject(ctx context.Context, namespace, bucket, object string) ([]byte, error) {
	m.recordCall("GetObject", namespace, bucket, object)
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := fmt.Sprintf("%s/%s/%s", namespace, bucket, object)
	if data, ok := m.Objects[key]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("object not found: %s", key)
}

// GetCompartmentIDByPath returns a mock compartment OCID.
func (m *MockOCIClient) GetCompartmentIDByPath(ctx context.Context, tenancyOcid, path string) (*string, error) {
	m.recordCall("GetCompartmentIDByPath", tenancyOcid, path)
	m.mu.RLock()
	defer m.mu.RUnlock()

	if ocid, ok := m.Compartments[path]; ok {
		return &ocid, nil
	}
	return nil, fmt.Errorf("compartment not found: %s", path)
}

// FetchClusterID finds a cluster OCID by name.
func (m *MockOCIClient) FetchClusterID(ctx context.Context, compartmentID, clusterName string) (*string, error) {
	m.recordCall("FetchClusterID", compartmentID, clusterName)
	if m.ShouldFailCluster {
		return nil, fmt.Errorf("mock cluster lookup failure")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	for ocid, cluster := range m.Clusters {
		if cluster.Name != nil && *cluster.Name == clusterName {
			return &ocid, nil
		}
	}
	return nil, fmt.Errorf("cluster not found: %s", clusterName)
}

// GetCluster retrieves cluster details.
func (m *MockOCIClient) GetCluster(ctx context.Context, clusterID string) (*containerengine.Cluster, error) {
	m.recordCall("GetCluster", clusterID)
	if m.ClusterError != nil {
		return nil, m.ClusterError
	}
	if m.ShouldFailCluster {
		return nil, fmt.Errorf("mock cluster retrieval failure")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	if cluster, ok := m.Clusters[clusterID]; ok {
		return cluster, nil
	}
	return nil, fmt.Errorf("cluster not found: %s", clusterID)
}

// ListBastions lists mock bastions.
func (m *MockOCIClient) ListBastions(ctx context.Context, compartmentID string) ([]bastion.BastionSummary, error) {
	m.recordCall("ListBastions", compartmentID)
	if m.BastionError != nil {
		return nil, m.BastionError
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	var summaries []bastion.BastionSummary
	for _, b := range m.Bastions {
		summaries = append(summaries, bastion.BastionSummary{
			Id:             b.Id,
			Name:           b.Name,
			BastionType:    b.BastionType,
			LifecycleState: bastion.BastionLifecycleStateActive,
		})
	}
	return summaries, nil
}

// GetBastion retrieves bastion details.
func (m *MockOCIClient) GetBastion(ctx context.Context, bastionID string) (*bastion.Bastion, error) {
	m.recordCall("GetBastion", bastionID)
	m.mu.RLock()
	defer m.mu.RUnlock()

	if b, ok := m.Bastions[bastionID]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("bastion not found: %s", bastionID)
}

// CreateSession creates a mock session.
func (m *MockOCIClient) CreateSession(ctx context.Context, bastionID string, sessionDetails bastion.CreateSessionDetails) (*bastion.Session, error) {
	m.recordCall("CreateSession", bastionID, sessionDetails)
	if m.ShouldFailSession {
		return nil, fmt.Errorf("mock session creation failure")
	}

	if m.CreateSessionDelay > 0 {
		time.Sleep(m.CreateSessionDelay)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	sessionID := fmt.Sprintf("ocid1.session.oc1..mock%d", len(m.Sessions)+1)
	session := &bastion.Session{
		Id:             &sessionID,
		BastionId:      &bastionID,
		DisplayName:    sessionDetails.DisplayName,
		LifecycleState: bastion.SessionLifecycleStateCreating,
	}

	m.Sessions[sessionID] = session
	return session, nil
}

// GetSession retrieves session details.
func (m *MockOCIClient) GetSession(ctx context.Context, bastionID, sessionID string) (*bastion.Session, error) {
	m.recordCall("GetSession", bastionID, sessionID)
	m.mu.RLock()
	defer m.mu.RUnlock()

	if session, ok := m.Sessions[sessionID]; ok {
		return session, nil
	}
	return nil, fmt.Errorf("session not found: %s", sessionID)
}

// ListSessions lists mock sessions.
func (m *MockOCIClient) ListSessions(ctx context.Context, bastionID string) ([]bastion.SessionSummary, error) {
	m.recordCall("ListSessions", bastionID)
	m.mu.RLock()
	defer m.mu.RUnlock()

	var summaries []bastion.SessionSummary
	for _, s := range m.Sessions {
		if s.BastionId != nil && *s.BastionId == bastionID {
			summaries = append(summaries, bastion.SessionSummary{
				Id:             s.Id,
				BastionId:      s.BastionId,
				DisplayName:    s.DisplayName,
				LifecycleState: s.LifecycleState,
			})
		}
	}
	return summaries, nil
}

// DeleteSession deletes a mock session.
func (m *MockOCIClient) DeleteSession(ctx context.Context, bastionID, sessionID string) error {
	m.recordCall("DeleteSession", bastionID, sessionID)
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.Sessions[sessionID]; ok {
		delete(m.Sessions, sessionID)
		return nil
	}
	return fmt.Errorf("session not found: %s", sessionID)
}

// WaitForSessionActive waits for a session to become active.
func (m *MockOCIClient) WaitForSessionActive(ctx context.Context, bastionID, sessionID string) (*bastion.Session, error) {
	m.recordCall("WaitForSessionActive", bastionID, sessionID)

	if m.SessionActiveDelay > 0 {
		select {
		case <-time.After(m.SessionActiveDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if session, ok := m.Sessions[sessionID]; ok {
		// Transition to active
		session.LifecycleState = bastion.SessionLifecycleStateActive
		return session, nil
	}
	return nil, fmt.Errorf("session not found: %s", sessionID)
}

// Helper methods for test setup

// AddCompartment adds a compartment mapping for tests.
func (m *MockOCIClient) AddCompartment(path, ocid string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Compartments[path] = ocid
}

// AddCluster adds a cluster for tests.
func (m *MockOCIClient) AddCluster(cluster *containerengine.Cluster) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cluster.Id != nil {
		m.Clusters[*cluster.Id] = cluster
	}
}

// AddBastion adds a bastion for tests.
func (m *MockOCIClient) AddBastion(b *bastion.Bastion) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b.Id != nil {
		m.Bastions[*b.Id] = b
	}
}

// AddObject adds an object for tests.
func (m *MockOCIClient) AddObject(namespace, bucket, object string, content []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s/%s/%s", namespace, bucket, object)
	m.Objects[key] = content
}

// Discovery operations (zero-touch support)

// GetTenancyOCID returns the mock tenancy OCID.
func (m *MockOCIClient) GetTenancyOCID() (string, error) {
	m.recordCall("GetTenancyOCID")
	if m.ShouldFailAuth {
		return "", fmt.Errorf("mock auth failure")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.TenancyOCID, nil
}

// ListCompartments returns mock compartments for a parent.
func (m *MockOCIClient) ListCompartments(ctx context.Context, parentID string) ([]identity.Compartment, error) {
	m.recordCall("ListCompartments", parentID)
	m.mu.RLock()
	defer m.mu.RUnlock()

	if compartments, ok := m.CompartmentsByID[parentID]; ok {
		return compartments, nil
	}
	return []identity.Compartment{}, nil
}

// ListClustersInCompartment returns mock clusters in a compartment.
func (m *MockOCIClient) ListClustersInCompartment(ctx context.Context, compartmentID string) ([]containerengine.ClusterSummary, error) {
	m.recordCall("ListClustersInCompartment", compartmentID)
	if m.ShouldFailCluster {
		return nil, fmt.Errorf("mock cluster listing failure")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	if clusters, ok := m.ClustersByCompartment[compartmentID]; ok {
		return clusters, nil
	}
	return []containerengine.ClusterSummary{}, nil
}

// GetSubscribedRegions returns mock subscribed regions.
func (m *MockOCIClient) GetSubscribedRegions(ctx context.Context, tenancyID string) ([]identity.RegionSubscription, error) {
	m.recordCall("GetSubscribedRegions", tenancyID)
	if m.RegionError != nil {
		return nil, m.RegionError
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.SubscribedRegions, nil
}

// Helper methods for discovery test setup

// AddCompartmentByID adds a compartment to a parent for tests.
func (m *MockOCIClient) AddCompartmentByID(parentID string, compartment identity.Compartment) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CompartmentsByID[parentID] = append(m.CompartmentsByID[parentID], compartment)
}

// AddClusterToCompartment adds a cluster summary to a compartment for tests.
func (m *MockOCIClient) AddClusterToCompartment(compartmentID string, cluster containerengine.ClusterSummary) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ClustersByCompartment[compartmentID] = append(m.ClustersByCompartment[compartmentID], cluster)
}

// AddSubscribedRegion adds a subscribed region for tests.
func (m *MockOCIClient) AddSubscribedRegion(regionName string, isHomeRegion bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SubscribedRegions = append(m.SubscribedRegions, identity.RegionSubscription{
		RegionName:   &regionName,
		IsHomeRegion: &isHomeRegion,
	})
}
