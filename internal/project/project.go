package project

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/agentuity/cli/internal/util"
	"github.com/shopmonkeyus/go-common/logger"
	"gopkg.in/yaml.v3"
)

const (
	initPath        = "/project/init"
	initWaitMessage = "Waiting for init to complete in the browser..."
)

type InitProjectResult struct {
	APIKey    string
	ProjectId string
	Provider  string
}

// InitProject will open a browser and wait for the user to finish initializing the project.
// It will return the API key and project ID if the project is initialized successfully.
func InitProject(logger logger.Logger, baseUrl string, provider string, name string, description string) (*InitProjectResult, error) {
	var result InitProjectResult
	callback := func(query url.Values) error {
		apikey := query.Get("apikey")
		projectId := query.Get("project_id")
		provider := query.Get("provider")
		if apikey == "" {
			return fmt.Errorf("no apikey found")
		}
		if projectId == "" {
			return fmt.Errorf("no project_id found")
		}
		if provider == "" {
			return fmt.Errorf("no provider found")
		}
		result.APIKey = apikey
		result.ProjectId = projectId
		result.Provider = provider
		return nil
	}
	query := map[string]string{"provider": provider}
	if name != "" {
		query["name"] = name
	}
	if description != "" {
		query["description"] = description
	}
	if err := util.BrowserFlow(util.BrowserFlowOptions{
		Logger:      logger,
		BaseUrl:     baseUrl,
		StartPath:   initPath,
		WaitMessage: initWaitMessage,
		Callback:    callback,
		Query:       query,
	}); err != nil {
		return nil, err
	}
	return &result, nil
}

func getFilename(dir string) string {
	return filepath.Join(dir, "agentuity.yaml")
}

func ProjectExists(dir string) bool {
	fn := getFilename(dir)
	_, err := os.Stat(fn)
	return err == nil
}

type Project struct {
	ProjectId string `json:"project_id" yaml:"project_id"`
	Provider  string `json:"provider" yaml:"provider"`
}

// Load will load the project from a file in the given directory.
func (p *Project) Load(dir string) error {
	fn := getFilename(dir)
	if _, err := os.Stat(fn); os.IsNotExist(err) {
		return nil
	}
	of, err := os.Open(fn)
	if err != nil {
		return err
	}
	defer of.Close()
	return yaml.NewDecoder(of).Decode(p)
}

// Save will save the project to a file in the given directory.
func (p *Project) Save(dir string) error {
	fn := getFilename(dir)
	of, err := os.Create(fn)
	if err != nil {
		return err
	}
	defer of.Close()
	enc := yaml.NewEncoder(of)
	enc.SetIndent(2)
	return enc.Encode(p)
}

// NewProject will create a new project that is empty.
func NewProject() *Project {
	return &Project{}
}
