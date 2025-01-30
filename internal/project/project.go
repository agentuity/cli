package project

import (
	"fmt"
	"net/url"

	"github.com/agentuity/cli/internal/util"
	"github.com/shopmonkeyus/go-common/logger"
)

const (
	initPath        = "/project/init"
	successInitPath = "/project/init/success"
	initWaitMessage = "Waiting for init to complete in the browser..."
)

type InitProjectResult struct {
	APIKey    string
	ProjectId string
}

// InitProject will open a browser and wait for the user to finish initializing the project.
// It will return the API key and project ID if the project is initialized successfully.
func InitProject(logger logger.Logger, baseUrl string, projectType string) (*InitProjectResult, error) {
	var result InitProjectResult
	callback := func(query url.Values) error {
		apikey := query.Get("apikey")
		projectId := query.Get("project_id")
		if apikey == "" {
			return fmt.Errorf("no apikey found")
		}
		if projectId == "" {
			return fmt.Errorf("no project_id found")
		}
		result.APIKey = apikey
		result.ProjectId = projectId
		return nil
	}
	var query map[string]string
	if projectType != "" {
		query = map[string]string{"project_type": projectType}
	}
	if err := util.BrowserFlow(util.BrowserFlowOptions{
		Logger:      logger,
		BaseUrl:     baseUrl,
		StartPath:   initPath,
		SuccessPath: successInitPath,
		WaitMessage: initWaitMessage,
		Callback:    callback,
		Query:       query,
	}); err != nil {
		return nil, err
	}
	return &result, nil
}
