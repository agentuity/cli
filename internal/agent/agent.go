package agent

import (
	"fmt"
	"net/url"

	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
)

type Agent struct {
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

type Response[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type ListResponse = Response[[]Agent]

// ListAgents will list all the agents in the project which are deployed
func ListAgents(logger logger.Logger, baseUrl string, token string, projectId string) ([]Agent, error) {
	client := util.NewAPIClient(logger, baseUrl, token)

	var resp ListResponse
	if err := client.Do("GET", fmt.Sprintf("/cli/agent/%s", url.PathEscape(projectId)), nil, &resp); err != nil {
		return nil, fmt.Errorf("error fetching list of agents: %s", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("error fetching list of agents: %s", resp.Message)
	}
	return resp.Data, nil
}

// CreateAgent will create a new agent in the project
func CreateAgent(logger logger.Logger, baseUrl string, token string, projectId string, name string, description string) (string, error) {
	client := util.NewAPIClient(logger, baseUrl, token)

	var resp Response[string]
	if err := client.Do("POST", fmt.Sprintf("/cli/agent/%s", url.PathEscape(projectId)), map[string]any{"name": name, "description": description}, &resp); err != nil {
		return "", fmt.Errorf("error creating agent: %s", err)
	}
	if !resp.Success {
		return "", fmt.Errorf("error creating agent: %s", resp.Message)
	}
	return resp.Data, nil
}

// DeleteAgent will delete one or more agents from the project
func DeleteAgents(logger logger.Logger, baseUrl string, token string, projectId string, agentIds []string) ([]string, error) {
	client := util.NewAPIClient(logger, baseUrl, token)

	var resp Response[[]string]
	if err := client.Do("DELETE", "/cli/agent", map[string]any{"ids": agentIds}, &resp); err != nil {
		return nil, fmt.Errorf("error deleting agents: %s", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("error deleting agents: %s", resp.Message)
	}
	return resp.Data, nil
}
