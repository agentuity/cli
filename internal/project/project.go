package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
	APIKey           string            `json:"api_key"`
	ProjectId        string            `json:"id"`
	Env              map[string]string `json:"env"`
	Secrets          map[string]string `json:"secrets"`
	WebhookAuthToken string            `json:"webhookAuthToken,omitempty"`
	AgentID          string            `json:"agentId"`
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
	AgentName         string
	AgentDescription  string
	AgentID           string
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
		"agent":             map[string]string{"name": args.AgentName, "description": args.AgentDescription},
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

type Development struct {
	Port  int  `json:"port,omitempty" yaml:"port,omitempty"`
	Watch bool `json:"watch,omitempty" yaml:"watch,omitempty"`
}

type AgentConfig struct {
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

type Project struct {
	ProjectId   string        `json:"project_id" yaml:"project_id"`
	Name        string        `json:"name" yaml:"name"`
	Description string        `json:"description" yaml:"description"`
	Development *Development  `json:"development,omitempty" yaml:"development,omitempty"`
	Deployment  *Deployment   `json:"deployment,omitempty" yaml:"deployment,omitempty"`
	Bundler     *Bundler      `json:"bundler,omitempty" yaml:"bundler,omitempty"`
	Agents      []AgentConfig `json:"agents" yaml:"agents"`
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
	if p.Bundler == nil {
		return fmt.Errorf("missing bundler value, please run `agentuity new` to create a new project")
	}
	if p.Bundler.Language == "" {
		return fmt.Errorf("missing bundler.language value, please run `agentuity new` to create a new project")
	}
	switch p.Bundler.Language {
	case "js", "javascript", "typescript":
		if p.Bundler.Runtime != "bunjs" && p.Bundler.Runtime != "nodejs" && p.Bundler.Runtime != "deno" {
			return fmt.Errorf("invalid bundler.runtime value: %s. only bunjs, nodejs, and deno are supported", p.Bundler.Runtime)
		}
	case "py", "python":
		if p.Bundler.Runtime != "uv" && p.Bundler.Runtime != "python" && p.Bundler.Runtime != "" {
			return fmt.Errorf("invalid bundler.runtime value: %s. only uv or python is supported", p.Bundler.Runtime)
		}
	default:
		return fmt.Errorf("invalid bundler.language value: %s. only js or py are supported", p.Bundler.Language)
	}
	if p.Bundler.AgentConfig.Dir == "" {
		return fmt.Errorf("missing bundler.agents.dir value (or its empty), please run `agentuity new` to create a new project")
	}
	if len(p.Agents) == 0 {
		return fmt.Errorf("missing agents, please run `agentuity new` to create a new project or `agentuity agent new` to create a new agent")
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
		Development: &Development{
			Port:  3500,
			Watch: true,
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

func ProjectWithNameExists(logger logger.Logger, baseUrl string, token string, name string) (bool, error) {
	client := util.NewAPIClient(logger, baseUrl, token)

	var resp Response[bool]
	if err := client.Do("GET", fmt.Sprintf("/cli/project/exists/%s", url.PathEscape(name)), nil, &resp); err != nil {
		return false, fmt.Errorf("error validating project name: %s", err)
	}
	return resp.Data, nil
}

type ProjectListData struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func ListProjects(logger logger.Logger, baseUrl string, token string) ([]ProjectListData, error) {
	client := util.NewAPIClient(logger, baseUrl, token)

	var resp Response[[]ProjectListData]
	if err := client.Do("GET", "/cli/project", nil, &resp); err != nil {
		return nil, fmt.Errorf("error listing projects: %s", err)
	}
	return resp.Data, nil
}

func DeleteProjects(logger logger.Logger, baseUrl string, token string, ids []string) ([]string, error) {
	client := util.NewAPIClient(logger, baseUrl, token)

	var resp Response[[]string]
	var payload = map[string]any{
		"ids": ids,
	}
	if err := client.Do("DELETE", "/cli/project", payload, &resp); err != nil {
		return nil, fmt.Errorf("error deleting projects: %s", err)
	}
	if !resp.Success {
		return nil, errors.New(resp.Message)
	}
	return resp.Data, nil
}

func (p *Project) ListProjectEnv(logger logger.Logger, baseUrl string, token string) (*ProjectData, error) {
	client := util.NewAPIClient(logger, baseUrl, token)

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
	client := util.NewAPIClient(logger, baseUrl, token)
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
	client := util.NewAPIClient(logger, baseUrl, token)
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

type Bundler struct {
	Language    string             `yaml:"language" json:"language"`
	Framework   string             `yaml:"framework,omitempty" json:"framework,omitempty"`
	Runtime     string             `yaml:"runtime,omitempty" json:"runtime,omitempty"`
	AgentConfig AgentBundlerConfig `yaml:"agents" json:"agents"`
	CLIVersion  string             `yaml:"-" json:"-"`
}

type AgentBundlerConfig struct {
	Dir string `yaml:"dir" json:"dir"`
}

type DeploymentConfig struct {
	Language   string      `yaml:"language" json:"language"`
	Runtime    string      `yaml:"runtime,omitempty" json:"runtime,omitempty"`
	MinVersion string      `yaml:"min_version,omitempty" json:"min_version,omitempty"`
	WorkingDir string      `yaml:"working_dir,omitempty" json:"working_dir,omitempty"`
	Command    []string    `yaml:"command,omitempty" json:"command,omitempty"`
	Env        []string    `yaml:"env,omitempty" json:"env,omitempty"`
	Deployment *Deployment `yaml:"deployment,omitempty" json:"deployment,omitempty"`
	Bundler    *Bundler    `yaml:"bundler,omitempty" json:"bundler,omitempty"`
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
