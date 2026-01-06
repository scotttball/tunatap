package client

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/bastion"
	"github.com/oracle/oci-go-sdk/v65/containerengine"
	"github.com/oracle/oci-go-sdk/v65/identity"
)

// OCIClientInterface defines the interface for OCI operations.
// This interface enables mocking for tests without requiring actual OCI credentials.
//
//go:generate mockgen -destination=mock_client.go -package=client github.com/scotttball/tunatap/internal/client OCIClientInterface
type OCIClientInterface interface {
	// Region management
	SetRegion(region string)
	GetAuthType() AuthType

	// Object Storage operations
	GetNamespace(ctx context.Context, tenancyOcid string) (string, error)
	GetObject(ctx context.Context, namespace, bucket, object string) ([]byte, error)

	// Identity operations
	GetCompartmentIDByPath(ctx context.Context, tenancyOcid, path string) (*string, error)

	// Container Engine operations
	FetchClusterID(ctx context.Context, compartmentID, clusterName string) (*string, error)
	GetCluster(ctx context.Context, clusterID string) (*containerengine.Cluster, error)

	// Bastion operations
	ListBastions(ctx context.Context, compartmentID string) ([]bastion.BastionSummary, error)
	GetBastion(ctx context.Context, bastionID string) (*bastion.Bastion, error)

	// Session operations
	CreateSession(ctx context.Context, bastionID string, sessionDetails bastion.CreateSessionDetails) (*bastion.Session, error)
	GetSession(ctx context.Context, bastionID, sessionID string) (*bastion.Session, error)
	ListSessions(ctx context.Context, bastionID string) ([]bastion.SessionSummary, error)
	DeleteSession(ctx context.Context, bastionID, sessionID string) error
	WaitForSessionActive(ctx context.Context, bastionID, sessionID string) (*bastion.Session, error)

	// Discovery operations (zero-touch support)
	GetTenancyOCID() (string, error)
	ListCompartments(ctx context.Context, parentID string) ([]identity.Compartment, error)
	ListClustersInCompartment(ctx context.Context, compartmentID string) ([]containerengine.ClusterSummary, error)
	GetSubscribedRegions(ctx context.Context, tenancyID string) ([]identity.RegionSubscription, error)
}

// Ensure OCIClient implements OCIClientInterface
var _ OCIClientInterface = (*OCIClient)(nil)
