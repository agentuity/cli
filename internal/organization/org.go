package organization

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/agentuity/go-common/logger"
)

const (
	listPath = "/cli/organization"
)

type Organization struct {
	OrgId string `json:"id"`
	Name  string `json:"name"`
}

type listOrganizationsResult struct {
	Success bool           `json:"success"`
	Data    []Organization `json:"data"`
	Message string         `json:"message"`
}

func ListOrganizations(logger logger.Logger, apiUrl string, token string) ([]Organization, error) {
	var result listOrganizationsResult

	req, err := http.NewRequest("GET", apiUrl+listPath, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if !result.Success {
		return nil, fmt.Errorf("failed to list organizations: %s", result.Message)
	}

	return result.Data, nil
}
