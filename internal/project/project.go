package project

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/resource"
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

type Resources struct {
	Memory string `json:"memory,omitempty" yaml:"memory,omitempty"`
	CPU    string `json:"cpu,omitempty" yaml:"cpu,omitempty"`

	CPUQuantity    resource.Quantity `json:"-" yaml:"-"`
	MemoryQuantity resource.Quantity `json:"-" yaml:"-"`
}

type Deployment struct {
	Resources *Resources `json:"resources,omitempty" yaml:"resources,omitempty"`
}

type Project struct {
	ProjectId  string      `json:"project_id" yaml:"project_id"`
	Provider   string      `json:"provider" yaml:"provider"`
	Deployment *Deployment `json:"deploy,omitempty" yaml:"deploy,omitempty"`
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
	if err := yaml.NewDecoder(of).Decode(p); err != nil {
		return err
	}
	if p.ProjectId == "" {
		return fmt.Errorf("missing project_id value")
	}
	if p.Provider == "" {
		return fmt.Errorf("missing provider value")
	}
	if p.Deployment != nil {
		if p.Deployment.Resources != nil {
			val, err := resource.ParseQuantity(p.Deployment.Resources.CPU)
			if err != nil {
				return fmt.Errorf("error validating deploy cpu value '%s'. %w", p.Deployment.Resources.CPU, err)
			}
			p.Deployment.Resources.CPUQuantity = val
			val, err = resource.ParseQuantity(p.Deployment.Resources.Memory)
			if err != nil {
				return fmt.Errorf("error validating deploy memory value '%s'. %w", p.Deployment.Resources.Memory, err)
			}
			p.Deployment.Resources.MemoryQuantity = val
		}
	}
	return nil
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
