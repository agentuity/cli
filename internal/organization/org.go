package organization

import (
	"fmt"

	"github.com/agentuity/cli/internal/util"
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

func ListOrganizations(logger logger.Logger, baseUrl string, token string) ([]Organization, error) {
	var result listOrganizationsResult
	client := util.NewAPIClient(logger, baseUrl, token)

	if err := client.Do("GET", listPath, nil, &result); err != nil {
		return nil, err
	}

	if !result.Success {
		return nil, fmt.Errorf("failed to list organizations: %s", result.Message)
	}

	return result.Data, nil
}
