package client

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/oracle/oci-go-sdk/v65/bastion"
	"github.com/oracle/oci-go-sdk/v65/containerengine"
)

// MockOCIClient is a mock implementation of OCIClientInterface for testing.
type MockOCIClient struct {
	mu sync.RWMutex

	// Configuration
	Region   string
	AuthType AuthType

	// Mock data stores
	Compartments map[string]string            // path -> OCID
	Clusters     map[string]*containerengine.Cluster // OCID -> Cluster
	Bastions     map[string]*bastion.Bastion  // OCID -> Bastion
	Sessions     map[string]*bastion.Session  // OCID -> Session
	Objects      map[string][]byte            // "namespace/bucket/object" -> content
	Namespace    string

	// Behavior configuration
	CreateSessionDelay   time.Duration
	SessionActiveDelay   time.Duration
	ShouldFailAuth       bool
	ShouldFailSession    bool
	ShouldFailCluster    bool

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
		AuthType:     AuthTypeConfigFile,
		Compartments: make(map[string]string),
		Clusters:     make(map[string]*containerengine.Cluster),
		Bastions:     make(map[string]*bastion.Bastion),
		Sessions:     make(map[string]*bastion.Session),
		Objects:      make(map[string][]byte),
		Namespace:    "test-namespace",
		Calls:        make([]MockCall, 0),
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

// GetCompartmentIdByPath returns a mock compartment OCID.
func (m *MockOCIClient) GetCompartmentIdByPath(ctx context.Context, tenancyOcid, path string) (*string, error) {
	m.recordCall("GetCompartmentIdByPath", tenancyOcid, path)
	m.mu.RLock()
	defer m.mu.RUnlock()

	if ocid, ok := m.Compartments[path]; ok {
		return &ocid, nil
	}
	return nil, fmt.Errorf("compartment not found: %s", path)
}

// FetchClusterId finds a cluster OCID by name.
func (m *MockOCIClient) FetchClusterId(ctx context.Context, compartmentId, clusterName string) (*string, error) {
	m.recordCall("FetchClusterId", compartmentId, clusterName)
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
func (m *MockOCIClient) GetCluster(ctx context.Context, clusterId string) (*containerengine.Cluster, error) {
	m.recordCall("GetCluster", clusterId)
	if m.ShouldFailCluster {
		return nil, fmt.Errorf("mock cluster retrieval failure")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	if cluster, ok := m.Clusters[clusterId]; ok {
		return cluster, nil
	}
	return nil, fmt.Errorf("cluster not found: %s", clusterId)
}

// ListBastions lists mock bastions.
func (m *MockOCIClient) ListBastions(ctx context.Context, compartmentId string) ([]bastion.BastionSummary, error) {
	m.recordCall("ListBastions", compartmentId)
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
func (m *MockOCIClient) GetBastion(ctx context.Context, bastionId string) (*bastion.Bastion, error) {
	m.recordCall("GetBastion", bastionId)
	m.mu.RLock()
	defer m.mu.RUnlock()

	if b, ok := m.Bastions[bastionId]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("bastion not found: %s", bastionId)
}

// CreateSession creates a mock session.
func (m *MockOCIClient) CreateSession(ctx context.Context, bastionId string, sessionDetails bastion.CreateSessionDetails) (*bastion.Session, error) {
	m.recordCall("CreateSession", bastionId, sessionDetails)
	if m.ShouldFailSession {
		return nil, fmt.Errorf("mock session creation failure")
	}

	if m.CreateSessionDelay > 0 {
		time.Sleep(m.CreateSessionDelay)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	sessionId := fmt.Sprintf("ocid1.session.oc1..mock%d", len(m.Sessions)+1)
	session := &bastion.Session{
		Id:             &sessionId,
		BastionId:      &bastionId,
		DisplayName:    sessionDetails.DisplayName,
		LifecycleState: bastion.SessionLifecycleStateCreating,
	}

	m.Sessions[sessionId] = session
	return session, nil
}

// GetSession retrieves session details.
func (m *MockOCIClient) GetSession(ctx context.Context, bastionId, sessionId string) (*bastion.Session, error) {
	m.recordCall("GetSession", bastionId, sessionId)
	m.mu.RLock()
	defer m.mu.RUnlock()

	if session, ok := m.Sessions[sessionId]; ok {
		return session, nil
	}
	return nil, fmt.Errorf("session not found: %s", sessionId)
}

// ListSessions lists mock sessions.
func (m *MockOCIClient) ListSessions(ctx context.Context, bastionId string) ([]bastion.SessionSummary, error) {
	m.recordCall("ListSessions", bastionId)
	m.mu.RLock()
	defer m.mu.RUnlock()

	var summaries []bastion.SessionSummary
	for _, s := range m.Sessions {
		if s.BastionId != nil && *s.BastionId == bastionId {
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
func (m *MockOCIClient) DeleteSession(ctx context.Context, bastionId, sessionId string) error {
	m.recordCall("DeleteSession", bastionId, sessionId)
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.Sessions[sessionId]; ok {
		delete(m.Sessions, sessionId)
		return nil
	}
	return fmt.Errorf("session not found: %s", sessionId)
}

// WaitForSessionActive waits for a session to become active.
func (m *MockOCIClient) WaitForSessionActive(ctx context.Context, bastionId, sessionId string) (*bastion.Session, error) {
	m.recordCall("WaitForSessionActive", bastionId, sessionId)

	if m.SessionActiveDelay > 0 {
		select {
		case <-time.After(m.SessionActiveDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if session, ok := m.Sessions[sessionId]; ok {
		// Transition to active
		session.LifecycleState = bastion.SessionLifecycleStateActive
		return session, nil
	}
	return nil, fmt.Errorf("session not found: %s", sessionId)
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
