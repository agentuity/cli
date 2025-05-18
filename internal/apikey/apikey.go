package apikey

import (
	"context"
	"fmt"

	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
)

type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type User struct {
	ID        string `json:"id"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	PhotoUrl  string `json:"photoUrl"`
}

type Org struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type APIKey struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	OwnerId    string  `json:"ownerId"`
	OrgId      string  `json:"orgId"`
	ProjectId  string  `json:"projectId"`
	ExpiresAt  string  `json:"expiresAt"`
	LastUsedAt string  `json:"lastUsedAt"`
	Value      string  `json:"value"`
	Project    Project `json:"project"`
	User       User    `json:"user"`
	Org        Org     `json:"org"`
}

type Response[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type ListResponse = Response[[]APIKey]
type CreateResponse = Response[APIKey]
type DeleteResponse = Response[int]
type GetResponse = Response[*APIKey]

func List(ctx context.Context, logger logger.Logger, baseUrl string, token string, orgId string, projectId string) ([]APIKey, error) {
	client := util.NewAPIClient(ctx, logger, baseUrl, token)

	var resp ListResponse
	if err := client.Do("GET", fmt.Sprintf("/cli/apikey?orgId=%s&projectId=%s", orgId, projectId), nil, &resp); err != nil {
		return nil, fmt.Errorf("error fetching list of API keys: %s", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("error fetching list of API keys: %s", resp.Message)
	}
	return resp.Data, nil
}

func Create(ctx context.Context, logger logger.Logger, baseUrl string, token string, orgId string, projectId string, name string, expiresAt string) (APIKey, error) {
	client := util.NewAPIClient(ctx, logger, baseUrl, token)

	var resp CreateResponse
	if err := client.Do("POST", "/cli/apikey", map[string]any{"name": name, "expiresAt": expiresAt, "orgId": orgId, "projectId": projectId}, &resp); err != nil {
		return APIKey{}, fmt.Errorf("error creating API key: %s", err)
	}
	if !resp.Success {
		return APIKey{}, fmt.Errorf("error creating API key: %s", resp.Message)
	}
	return resp.Data, nil
}

func Delete(ctx context.Context, logger logger.Logger, baseUrl string, token string, id string) error {
	client := util.NewAPIClient(ctx, logger, baseUrl, token)

	var resp DeleteResponse
	if err := client.Do("DELETE", fmt.Sprintf("/cli/apikey/%s", id), nil, &resp); err != nil {
		return fmt.Errorf("error deleting API key: %s", err)
	}
	if !resp.Success {
		return fmt.Errorf("error deleting API key: %s", resp.Message)
	}
	return nil
}

func Get(ctx context.Context, logger logger.Logger, baseUrl string, token string, id string) (*APIKey, error) {
	client := util.NewAPIClient(ctx, logger, baseUrl, token)

	var resp GetResponse
	if err := client.Do("GET", fmt.Sprintf("/cli/apikey/%s", id), nil, &resp); err != nil {
		return nil, fmt.Errorf("error fetching API key: %s", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("error fetching API key: %s", resp.Message)
	}
	return resp.Data, nil
}
