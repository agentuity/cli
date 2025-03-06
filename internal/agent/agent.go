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

// ListAgents will list all the Agents in the project which are deployed
func ListAgents(logger logger.Logger, baseUrl string, token string, projectId string) ([]Agent, error) {
	client := util.NewAPIClient(logger, baseUrl, token)

	var resp ListResponse
	if err := client.Do("GET", fmt.Sprintf("/cli/agent/%s", url.PathEscape(projectId)), nil, &resp); err != nil {
		return nil, fmt.Errorf("error fetching list of Agents: %s", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("error fetching list of Agents: %s", resp.Message)
	}
	return resp.Data, nil
}

// CreateAgent will create a new agent in the project
func CreateAgent(logger logger.Logger, baseUrl string, token string, projectId string, name string, description string, authType string) (string, error) {
	client := util.NewAPIClient(logger, baseUrl, token)

	var resp Response[string]
	if err := client.Do("POST", fmt.Sprintf("/cli/agent/%s", url.PathEscape(projectId)), map[string]any{"name": name, "description": description, "auth_type": authType}, &resp); err != nil {
		return "", fmt.Errorf("error creating agent: %s", err)
	}
	if !resp.Success {
		return "", fmt.Errorf("error creating agent: %s", resp.Message)
	}
	return resp.Data, nil
}

// DeleteAgent will delete one or more Agents from the project
func DeleteAgents(logger logger.Logger, baseUrl string, token string, projectId string, agentIds []string) ([]string, error) {
	client := util.NewAPIClient(logger, baseUrl, token)

	var resp Response[[]string]
	if err := client.Do("DELETE", "/cli/agent", map[string]any{"ids": agentIds}, &resp); err != nil {
		return nil, fmt.Errorf("error deleting Agents: %s", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("error deleting Agents: %s", resp.Message)
	}
	return resp.Data, nil
}

type AgentAPIKey struct {
	ID     string         `json:"id"`
	Config map[string]any `json:"config"`
}

func GetApiKey(logger logger.Logger, baseUrl string, token string, agentId string) (string, error) {
	client := util.NewAPIClient(logger, baseUrl, token)

	var resp Response[*AgentAPIKey]
	if err := client.Do("GET", fmt.Sprintf("/cli/agent/%s/io/source/webhook", url.PathEscape(agentId)), nil, &resp); err != nil {
		return "", fmt.Errorf("error getting Agent API key: %s", err)
	}

	if !resp.Success {
		return "", fmt.Errorf("error getting Agent API key: %s", resp.Message)
	}

	if resp.Data == nil {
		return "", nil
	}

	if kv, ok := resp.Data.Config["authorization"].(map[string]any); ok {
		if token, ok := kv["token"].(string); ok {
			return token, nil
		}
	}

	return "", fmt.Errorf("no API key found for Agent %s", agentId)
}
