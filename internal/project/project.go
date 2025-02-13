package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"

	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	initPath = "/cli/project"
)

type initProjectResult struct {
	Success bool        `json:"success"`
	Data    ProjectData `json:"data"`
	Message string      `json:"message"`
}

type ProjectData struct {
	APIKey      string            `json:"api_key"`
	ProjectId   string            `json:"id"`
	Env         map[string]string `json:"env"`
	Secrets     map[string]string `json:"secrets"`
	IOId        string            `json:"ioId"`
	IOAuthToken string            `json:"ioAuthToken,omitempty"`
}

type InitProjectArgs struct {
	BaseURL           string
	Dir               string
	Token             string
	OrgId             string
	Provider          string
	Name              string
	Description       string
	EnableWebhookAuth bool
}

// InitProject will create a new project in the organization.
// It will return the API key and project ID if the project is initialized successfully.
func InitProject(logger logger.Logger, args InitProjectArgs) (*ProjectData, error) {

	payload := map[string]any{
		"organization_id":   args.OrgId,
		"provider":          args.Provider,
		"name":              args.Name,
		"description":       args.Description,
		"enableWebhookAuth": args.EnableWebhookAuth,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", args.BaseURL+initPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+args.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to initialize project: %s", resp.Status)
	}

	var result initProjectResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result.Data, nil
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
	Disk   string `json:"disk,omitempty" yaml:"disk,omitempty"`

	CPUQuantity    resource.Quantity `json:"-" yaml:"-"`
	MemoryQuantity resource.Quantity `json:"-" yaml:"-"`
	DiskQuantity   resource.Quantity `json:"-" yaml:"-"`
}

type Deployment struct {
	Resources *Resources `json:"resources,omitempty" yaml:"resources,omitempty"`
}

type IO struct {
	Type      string         `json:"type" yaml:"type"`
	ID        string         `json:"id,omitempty" yaml:"id,omitempty"`
	Direction string         `json:"direction" yaml:"-"`
	Config    map[string]any `json:"config,omitempty" yaml:"config,omitempty"`
}

type Project struct {
	ProjectId  string      `json:"project_id" yaml:"project_id"`
	Provider   string      `json:"provider" yaml:"provider"`
	Deployment *Deployment `json:"deployment,omitempty" yaml:"deployment,omitempty"`
	Inputs     []IO        `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Outputs    []IO        `json:"outputs,omitempty" yaml:"outputs,omitempty"`
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

const (
	defaultMemory = "1Gi"
	defaultCPU    = "1000M"
)

// NewProject will create a new project that is empty.
func NewProject() *Project {
	return &Project{
		Inputs: []IO{
			{
				Type: "webhook",
			},
		},
		Deployment: &Deployment{
			Resources: &Resources{
				Memory: defaultMemory,
				CPU:    defaultCPU,
			},
		},
	}
}

type Response[T any] struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    T      `json:"data"`
}

type ProjectResponse = Response[ProjectData]

func (p *Project) ListProjectEnv(logger logger.Logger, baseUrl string, token string) (*ProjectData, error) {
	client := util.NewAPIClient(baseUrl, token)

	var projectResponse ProjectResponse
	if err := client.Do("GET", fmt.Sprintf("/cli/project/%s", p.ProjectId), nil, &projectResponse); err != nil {
		logger.Fatal("error getting project env: %s", err)
	}
	if !projectResponse.Success {
		return nil, errors.New(projectResponse.Message)
	}
	return &projectResponse.Data, nil
}

func (p *Project) SetProjectEnv(logger logger.Logger, baseUrl string, token string, env map[string]string, secrets map[string]string) (*ProjectData, error) {
	client := util.NewAPIClient(baseUrl, token)
	var projectResponse ProjectResponse
	if err := client.Do("PUT", fmt.Sprintf("/cli/project/%s/env", p.ProjectId), map[string]any{
		"env":     env,
		"secrets": secrets,
	}, &projectResponse); err != nil {
		logger.Fatal("error setting project env: %s", err)
	}
	if !projectResponse.Success {
		return nil, errors.New(projectResponse.Message)
	}
	return &projectResponse.Data, nil
}

func (p *Project) DeleteProjectEnv(logger logger.Logger, baseUrl string, token string, env []string, secrets []string) error {
	client := util.NewAPIClient(baseUrl, token)
	var projectResponse ProjectResponse
	if err := client.Do("DELETE", fmt.Sprintf("/cli/project/%s/env", p.ProjectId), map[string]any{
		"env":     env,
		"secrets": secrets,
	}, &projectResponse); err != nil {
		logger.Fatal("error deleting project env: %s", err)
	}
	if !projectResponse.Success {
		return errors.New(projectResponse.Message)
	}
	return nil
}

type IOResponse = Response[IO]

func (p *Project) CreateIO(logger logger.Logger, baseUrl string, token string, direction string, io IO) (*IO, error) {
	client := util.NewAPIClient(baseUrl, token)
	var ioResponse IOResponse
	if err := client.Do("POST", fmt.Sprintf("/cli/project/%s/io", p.ProjectId), io, &ioResponse); err != nil {
		logger.Fatal("error creating io: %s", err)
	}
	if !ioResponse.Success {
		return nil, errors.New(ioResponse.Message)
	}
	return &ioResponse.Data, nil
}

func (p *Project) ListIO(logger logger.Logger, baseUrl string, token string, direction string) ([]IO, error) {
	client := util.NewAPIClient(baseUrl, token)
	var response Response[[]IO]
	if err := client.Do("GET", fmt.Sprintf("/cli/project/%s/io/%s", p.ProjectId, direction), nil, &response); err != nil {
		logger.Fatal("error creating io: %s", err)
	}
	if !response.Success {
		return nil, errors.New(response.Message)
	}
	return response.Data, nil
}

func (p *Project) DeleteIO(logger logger.Logger, baseUrl string, token string, id string) error {
	client := util.NewAPIClient(baseUrl, token)
	var response Response[any]
	if err := client.Do("DELETE", fmt.Sprintf("/cli/project/%s/io/%s", p.ProjectId, id), nil, &response); err != nil {
		logger.Fatal("error creating io: %s", err)
	}
	if !response.Success {
		return errors.New(response.Message)
	}
	return nil
}

type DeploymentConfig struct {
	Provider   string      `yaml:"provider" json:"provider"`
	Language   string      `yaml:"language" json:"language"`
	Runtime    string      `yaml:"runtime,omitempty" json:"runtime,omitempty"`
	MinVersion string      `yaml:"min_version,omitempty" json:"min_version,omitempty"`
	WorkingDir string      `yaml:"working_dir,omitempty" json:"working_dir,omitempty"`
	Command    []string    `yaml:"command,omitempty" json:"command,omitempty"`
	Env        []string    `yaml:"env,omitempty" json:"env,omitempty"`
	Deployment *Deployment `yaml:"deployment,omitempty" json:"deployment,omitempty"`
}

func NewDeploymentConfig() *DeploymentConfig {
	return &DeploymentConfig{}
}

type CleanupFunc func()

func (c *DeploymentConfig) Write(dir string) (CleanupFunc, error) {
	fn := filepath.Join(dir, "agentuity-deployment.yaml")
	cleanup := func() {
		os.Remove(fn)
	}
	of, err := os.Create(fn)
	if err != nil {
		return cleanup, err
	}
	defer of.Close()
	enc := yaml.NewEncoder(of)
	enc.SetIndent(2)
	err = enc.Encode(c)
	if err != nil {
		return cleanup, err
	}
	return cleanup, nil
}
