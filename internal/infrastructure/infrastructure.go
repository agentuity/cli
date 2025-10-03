package infrastructure

import (
	"context"
	"fmt"
	"time"

	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
)

// Response represents the standard API response format
type Response[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

// Cluster represents a cluster in the infrastructure
type Cluster struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Provider        string    `json:"provider"`
	Type            string    `json:"type"` // backend uses "type" instead of "size"
	Region          string    `json:"region"`
	OrgID           *string   `json:"orgId"`     // nullable in response from list
	OrgName         *string   `json:"orgName"`   // joined from org table in list
	CreatedAt       string    `json:"createdAt"` // from baseProperties
	UpdatedAt       *string   `json:"updatedAt"` // only in detail view
	Token           string    `json:"token"`
	TokenExpiration time.Time `json:"tokenExpiration"`
}

// Machine represents a machine in the infrastructure
type Machine struct {
	ID          string                 `json:"id"`
	ClusterID   string                 `json:"clusterId"`  // backend uses camelCase
	InstanceID  string                 `json:"instanceId"` // provider specific instance id
	Status      string                 `json:"status"`     // enum: provisioned, running, stopping, stopped, paused, resuming, error
	Provider    string                 `json:"provider"`
	Region      string                 `json:"region"`
	Metadata    map[string]interface{} `json:"metadata"`    // provider specific metadata (only in detail view)
	StartedAt   *string                `json:"startedAt"`   // nullable timestamp
	StoppedAt   *string                `json:"stoppedAt"`   // nullable timestamp
	PausedAt    *string                `json:"pausedAt"`    // nullable timestamp
	ErroredAt   *string                `json:"erroredAt"`   // nullable timestamp
	Error       *string                `json:"error"`       // error details if status is error
	ClusterName *string                `json:"clusterName"` // joined from cluster table
	OrgID       *string                `json:"orgId"`       // from machine table
	OrgName     *string                `json:"orgName"`     // joined from org table
	CreatedAt   string                 `json:"createdAt"`   // from baseProperties
	UpdatedAt   *string                `json:"updatedAt"`   // only in detail view
}

// CreateClusterArgs represents the arguments for creating a cluster
type CreateClusterArgs struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Type     string `json:"type"` // backend expects "type" instead of "size"
	Region   string `json:"region"`
	OrgID    string `json:"orgId"` // backend expects camelCase orgId
}

// CreateCluster creates a new infrastructure cluster
func CreateCluster(ctx context.Context, logger logger.Logger, baseURL string, token string, args CreateClusterArgs) (*Cluster, error) {
	client := util.NewAPIClient(ctx, logger, baseURL, token)

	payload := map[string]any{
		"name":     args.Name,
		"provider": args.Provider,
		"type":     args.Type, // backend expects "type" instead of "size"
		"region":   args.Region,
		"orgId":    args.OrgID, // backend expects camelCase orgId
	}

	var resp Response[Cluster]
	if err := client.Do("POST", "/cli/cluster", payload, &resp); err != nil {
		return nil, fmt.Errorf("error creating cluster: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("cluster creation failed: %s", resp.Message)
	}

	return &resp.Data, nil
}

// ListClusters retrieves all clusters for the organization
func ListClusters(ctx context.Context, logger logger.Logger, baseURL string, token string) ([]Cluster, error) {
	client := util.NewAPIClient(ctx, logger, baseURL, token)

	var resp Response[[]Cluster]
	if err := client.Do("GET", "/cli/cluster", nil, &resp); err != nil {
		return nil, fmt.Errorf("error listing clusters: %w", err)
	}

	return resp.Data, nil
}

// GetCluster retrieves a specific cluster by ID
func GetCluster(ctx context.Context, logger logger.Logger, baseURL string, token string, clusterID string) (*Cluster, error) {
	client := util.NewAPIClient(ctx, logger, baseURL, token)

	var resp Response[Cluster]
	if err := client.Do("GET", fmt.Sprintf("/cli/cluster/%s", clusterID), nil, &resp); err != nil {
		return nil, fmt.Errorf("error getting cluster: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("cluster not found: %s", resp.Message)
	}

	return &resp.Data, nil
}

// DeleteCluster removes a cluster by ID
func DeleteCluster(ctx context.Context, logger logger.Logger, baseURL string, token string, clusterID string) error {
	client := util.NewAPIClient(ctx, logger, baseURL, token)

	var resp Response[any]
	if err := client.Do("DELETE", fmt.Sprintf("/cli/cluster/%s", clusterID), nil, &resp); err != nil {
		return fmt.Errorf("error deleting cluster: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("cluster deletion failed: %s", resp.Message)
	}

	return nil
}

// ListMachines retrieves all machines, optionally filtered by cluster
func ListMachines(ctx context.Context, logger logger.Logger, baseURL string, token string, clusterFilter string) ([]Machine, error) {
	client := util.NewAPIClient(ctx, logger, baseURL, token)

	path := "/cli/machine"
	if clusterFilter != "" {
		path = fmt.Sprintf("%s?clusterId=%s", path, clusterFilter)
	}

	var resp Response[[]Machine]
	if err := client.Do("GET", path, nil, &resp); err != nil {
		return nil, fmt.Errorf("error listing machines: %w", err)
	}

	return resp.Data, nil
}

// GetMachine retrieves a specific machine by ID
func GetMachine(ctx context.Context, logger logger.Logger, baseURL string, token string, machineID string) (*Machine, error) {
	client := util.NewAPIClient(ctx, logger, baseURL, token)

	var resp Response[Machine]
	if err := client.Do("GET", fmt.Sprintf("/cli/machine/%s", machineID), nil, &resp); err != nil {
		return nil, fmt.Errorf("error getting machine: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("machine not found: %s", resp.Message)
	}

	return &resp.Data, nil
}

// DeleteMachine removes a machine by ID
func DeleteMachine(ctx context.Context, logger logger.Logger, baseURL string, token string, machineID string) error {
	client := util.NewAPIClient(ctx, logger, baseURL, token)

	var resp Response[any]
	if err := client.Do("DELETE", fmt.Sprintf("/cli/machine/%s", machineID), nil, &resp); err != nil {
		return fmt.Errorf("error deleting machine: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("machine deletion failed: %s", resp.Message)
	}

	return nil
}

type CreateMachineResponse struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

// CreateMachine creates a new machine in the provisioning state
func CreateMachine(ctx context.Context, logger logger.Logger, baseURL string, token string, clusterID string, orgID string, provider string, region string) (*CreateMachineResponse, error) {
	client := util.NewAPIClient(ctx, logger, baseURL, token)

	var resp Response[CreateMachineResponse]
	var data = map[string]string{
		"clusterId": clusterID,
		"orgId":     orgID,
		"provider":  provider,
		"region":    region,
	}
	if err := client.Do("POST", "/cli/machine", data, &resp); err != nil {
		return nil, fmt.Errorf("error deleting machine: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("machine creation failed: %s", resp.Message)
	}

	if setup, ok := setups[provider]; ok {
		if err := setup.CreateMachine(ctx, logger, region, resp.Data.Token, clusterID); err != nil {
			client.Do("DELETE", "/cli/machine", map[string]string{"id": resp.Data.ID}, &resp)
			return nil, fmt.Errorf("error creating machine: %w", err)
		}
	}

	return &resp.Data, nil
}
