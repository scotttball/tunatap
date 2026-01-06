package client

import (
	"context"

	"github.com/oracle/oci-go-sdk/v65/bastion"
	"github.com/oracle/oci-go-sdk/v65/containerengine"
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
	GetCompartmentIdByPath(ctx context.Context, tenancyOcid, path string) (*string, error)

	// Container Engine operations
	FetchClusterId(ctx context.Context, compartmentId, clusterName string) (*string, error)
	GetCluster(ctx context.Context, clusterId string) (*containerengine.Cluster, error)

	// Bastion operations
	ListBastions(ctx context.Context, compartmentId string) ([]bastion.BastionSummary, error)
	GetBastion(ctx context.Context, bastionId string) (*bastion.Bastion, error)

	// Session operations
	CreateSession(ctx context.Context, bastionId string, sessionDetails bastion.CreateSessionDetails) (*bastion.Session, error)
	GetSession(ctx context.Context, bastionId, sessionId string) (*bastion.Session, error)
	ListSessions(ctx context.Context, bastionId string) ([]bastion.SessionSummary, error)
	DeleteSession(ctx context.Context, bastionId, sessionId string) error
	WaitForSessionActive(ctx context.Context, bastionId, sessionId string) (*bastion.Session, error)
}

// Ensure OCIClient implements OCIClientInterface
var _ OCIClientInterface = (*OCIClient)(nil)
