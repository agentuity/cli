package project

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/Masterminds/semver"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	"github.com/spf13/cobra"
	yc "github.com/zijiren233/yaml-comment"
	"gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	initPath = "/cli/project"
)

var Version string

var (
	ErrProjectNotFound         = errors.New("project not found")
	ErrProjectMissingProjectId = errors.New("missing project_id value")
)

type initProjectResult struct {
	Success bool        `json:"success"`
	Data    ProjectData `json:"data"`
	Message string      `json:"message"`
}

type ProjectData struct {
	APIKey           string            `json:"api_key"`
	ProjectId        string            `json:"id"`
	OrgId            string            `json:"orgId"`
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

	client := util.NewAPIClient(logger, args.BaseURL, args.Token)

	var result initProjectResult
	if err := client.Do("POST", initPath, payload, &result); err != nil {
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
	Memory string `json:"memory,omitempty" yaml:"memory,omitempty" hc:"The memory requirements"`
	CPU    string `json:"cpu,omitempty" yaml:"cpu,omitempty" hc:"The CPU requirements"`
	Disk   string `json:"disk,omitempty" yaml:"disk,omitempty" hc:"The disk size requirements"`

	CPUQuantity    resource.Quantity `json:"-" yaml:"-"`
	MemoryQuantity resource.Quantity `json:"-" yaml:"-"`
	DiskQuantity   resource.Quantity `json:"-" yaml:"-"`
}

type Deployment struct {
	Command   string     `json:"command" yaml:"command"`
	Args      []string   `json:"args" yaml:"args"`
	Resources *Resources `json:"resources" yaml:"resources" hc:"You should tune the resources for the deployment"`
}

type Watch struct {
	Enabled bool     `json:"enabled" yaml:"enabled" hc:"Whether to watch for changes and automatically restart the server"`
	Files   []string `json:"files" yaml:"files" hc:"Rules for files to watch for changes"`
}

type Development struct {
	Port    int      `json:"port" yaml:"port" hc:"The port to run the development server on which can be overridden by setting the PORT environment variable"`
	Watch   Watch    `json:"watch" yaml:"watch"`
	Command string   `json:"command" yaml:"command" hc:"The command to run the development server"`
	Args    []string `json:"args" yaml:"args" hc:"The arguments to pass to the development server"`
}

type AgentConfig struct {
	ID          string `json:"id" yaml:"id" hc:"The ID of the Agent which is automatically generated"`
	Name        string `json:"name" yaml:"name" hc:"The name of the Agent which is editable"`
	Description string `json:"description,omitempty" yaml:"description,omitempty" hc:"The description of the Agent which is editable"`
}

type Project struct {
	Version     string        `json:"version" yaml:"version" hc:"The version semver range required to run this project"`
	ProjectId   string        `json:"project_id" yaml:"project_id" hc:"The ID of the project which is automatically generated"`
	Name        string        `json:"name" yaml:"name" hc:"The name of the project which is editable"`
	Description string        `json:"description" yaml:"description" hc:"The description of the project which is editable"`
	Development *Development  `json:"development,omitempty" yaml:"development,omitempty" hc:"The development configuration for the project"`
	Deployment  *Deployment   `json:"deployment,omitempty" yaml:"deployment,omitempty"`
	Bundler     *Bundler      `json:"bundler,omitempty" yaml:"bundler,omitempty" hc:"You should not need to change these value"`
	Agents      []AgentConfig `json:"agents" yaml:"agents" hc:"The agents that are part of this project"`
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
		return ErrProjectMissingProjectId
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
		return fmt.Errorf("missing bundler.Agents.dir value (or its empty), please run `agentuity new` to create a new project")
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
	of.WriteString("# ------------------------------------------------\n")
	of.WriteString("# This file is generated by Agentuity\n")
	of.WriteString("# You should check this file into version control\n")
	of.WriteString("# ------------------------------------------------\n")
	of.WriteString("\n")
	enc := yaml.NewEncoder(of)
	enc.SetIndent(2)
	yenc := yc.NewEncoder(enc)
	return yenc.Encode(p)
}

const (
	defaultMemory = "1Gi"
	defaultCPU    = "1000M"
	defaultDisk   = "100Mi"
)

// NewProject will create a new project that is empty.
func NewProject() *Project {
	var version string
	if Version == "" || Version == "dev" {
		version = ">=0.0.0" // should only happen in dev cli
	} else {
		version = ">=" + Version
	}
	return &Project{
		Version: version,
		Deployment: &Deployment{
			Resources: &Resources{
				Memory: defaultMemory,
				CPU:    defaultCPU,
				Disk:   defaultDisk,
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
		var apiErr *util.APIError
		if errors.As(err, &apiErr) {
			if apiErr.Status == 409 {
				return true, nil
			}
		}
		return false, fmt.Errorf("error validating project name: %w", err)
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
		return nil, fmt.Errorf("error listing projects: %w", err)
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
		return nil, fmt.Errorf("error deleting projects: %w", err)
	}
	if !resp.Success {
		return nil, errors.New(resp.Message)
	}
	return resp.Data, nil
}

func (p *Project) GetProject(logger logger.Logger, baseUrl string, token string) (*ProjectData, error) {
	if p.ProjectId == "" {
		return nil, ErrProjectNotFound
	}
	client := util.NewAPIClient(logger, baseUrl, token)

	var projectResponse ProjectResponse
	if err := client.Do("GET", fmt.Sprintf("/cli/project/%s", p.ProjectId), nil, &projectResponse); err != nil {
		var apiErr *util.APIError
		if errors.As(err, &apiErr) {
			if apiErr.Status == 404 {
				return nil, ErrProjectNotFound
			}
		}
		return nil, fmt.Errorf("error getting project env: %w", err)
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
		return nil, fmt.Errorf("error setting project env: %w", err)
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
		return fmt.Errorf("error deleting project env: %w", err)
	}
	if !projectResponse.Success {
		return errors.New(projectResponse.Message)
	}
	return nil
}

type ProjectImportRequest struct {
	Name              string        `json:"name"`
	Description       string        `json:"description"`
	Provider          string        `json:"provider"`
	OrgId             string        `json:"orgId"`
	Agents            []AgentConfig `json:"agents"`
	EnableWebhookAuth bool          `json:"enableWebhookAuth"`
}

type ProjectImportResponse struct {
	ID          string        `json:"id"`
	Agents      []AgentConfig `json:"agents"`
	APIKey      string        `json:"apiKey"`
	IOAuthToken string        `json:"ioAuthToken"`
}

func (p *Project) Import(logger logger.Logger, baseUrl string, token string, orgId string, enableWebhookAuth bool) (*ProjectImportResponse, error) {
	client := util.NewAPIClient(logger, baseUrl, token)

	var resp Response[ProjectImportResponse]
	var req ProjectImportRequest
	req.Name = p.Name
	req.Description = p.Description
	req.OrgId = orgId
	req.Agents = p.Agents
	req.Provider = p.Bundler.Identifier
	req.EnableWebhookAuth = enableWebhookAuth

	if err := client.Do("POST", "/cli/project/import", req, &resp); err != nil {
		return nil, fmt.Errorf("error importing project: %w", err)
	}

	p.ProjectId = resp.Data.ID
	p.Agents = resp.Data.Agents

	return &resp.Data, nil
}

type Bundler struct {
	Enabled     bool               `yaml:"enabled" json:"enabled"`
	Identifier  string             `yaml:"identifier" json:"identifier"`
	Language    string             `yaml:"language" json:"language"`
	Framework   string             `yaml:"framework,omitempty" json:"framework,omitempty"`
	Runtime     string             `yaml:"runtime,omitempty" json:"runtime,omitempty"`
	AgentConfig AgentBundlerConfig `yaml:"agents" json:"agents"`
	Ignore      []string           `yaml:"ignore,omitempty" json:"ignore,omitempty"`
	CLIVersion  string             `yaml:"-" json:"-"`
}

type AgentBundlerConfig struct {
	Dir string `yaml:"dir" json:"dir"`
}

type DeploymentConfig struct {
	Provider   string   `yaml:"provider" json:"provider"`
	Language   string   `yaml:"language" json:"language"`
	Runtime    string   `yaml:"runtime,omitempty" json:"runtime,omitempty"`
	MinVersion string   `yaml:"min_version,omitempty" json:"min_version,omitempty"` // FIXME
	WorkingDir string   `yaml:"working_dir,omitempty" json:"working_dir,omitempty"`
	Command    []string `yaml:"command,omitempty" json:"command,omitempty"`
	Env        []string `yaml:"env,omitempty" json:"env,omitempty"`
}

func NewDeploymentConfig() *DeploymentConfig {
	return &DeploymentConfig{}
}

func (c *DeploymentConfig) Write(logger logger.Logger, dir string) error {
	fn := filepath.Join(dir, ".agentuity", ".manifest.yaml")
	of, err := os.Create(fn)
	if err != nil {
		return err
	}
	defer of.Close()
	enc := yaml.NewEncoder(of)
	enc.SetIndent(2)
	err = enc.Encode(c)
	if err != nil {
		return err
	}
	logger.Debug("deployment config written to %s", fn)
	return nil
}

type ProjectContext struct {
	Logger     logger.Logger
	Project    *Project
	Dir        string
	APIURL     string
	APPURL     string
	Token      string
	NewProject bool
}

func LoadProject(logger logger.Logger, dir string, apiUrl string, appUrl string, token string) ProjectContext {
	theproject := NewProject()
	if err := theproject.Load(dir); err != nil {
		if err == ErrProjectMissingProjectId {
			return ProjectContext{
				Logger:     logger,
				Dir:        dir,
				Project:    theproject,
				APIURL:     apiUrl,
				APPURL:     appUrl,
				Token:      token,
				NewProject: true,
			}
		}
		errsystem.New(errsystem.ErrInvalidConfiguration, err,
			errsystem.WithContextMessage("Error loading project from disk")).ShowErrorAndExit()
	}
	return ProjectContext{
		Logger:  logger,
		Project: theproject,
		Dir:     dir,
		APIURL:  apiUrl,
		APPURL:  appUrl,
		Token:   token,
	}
}

func EnsureProject(cmd *cobra.Command) ProjectContext {
	logger := env.NewLogger(cmd)
	dir := ResolveProjectDir(logger, cmd)
	apiUrl, appUrl := util.GetURLs(logger)
	var token string
	// if the --api-key flag is used, we only need to verify the api key
	if cmd.Flags().Changed("api-key") {
		token = util.EnsureLoggedInWithOnlyAPIKey()
	} else {
		token, _ = util.EnsureLoggedIn()
	}
	p := LoadProject(logger, dir, apiUrl, appUrl, token)
	if !p.NewProject && Version != "" && Version != "dev" && p.Project.Version != "" {
		v := semver.MustParse(Version)
		c, err := semver.NewConstraint(p.Project.Version)
		if err != nil {
			errsystem.New(errsystem.ErrInvalidConfiguration, err,
				errsystem.WithContextMessage(fmt.Sprintf("Error parsing project version constraint: %s", p.Project.Version))).ShowErrorAndExit()
		}
		if !c.Check(v) {
			logger.Fatal("This project is not compatible with CLI version %s. Please upgrade your Agentuity CLI to version %s.", Version, p.Project.Version)
		}
	}
	return p
}

func ResolveProjectDir(logger logger.Logger, cmd *cobra.Command) string {
	cwd, err := os.Getwd()
	if err != nil {
		errsystem.New(errsystem.ErrEnvironmentVariablesNotSet, err,
			errsystem.WithUserMessage(fmt.Sprintf("Failed to get current directory: %s", err))).ShowErrorAndExit()
	}
	dir := cwd
	dirFlag, _ := cmd.Flags().GetString("dir")
	if dirFlag != "" {
		dir = dirFlag
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		errsystem.New(errsystem.ErrEnvironmentVariablesNotSet, err,
			errsystem.WithUserMessage(fmt.Sprintf("Failed to get absolute path: %s", err))).ShowErrorAndExit()
	}
	if !ProjectExists(abs) {
		logger.Fatal("Project file not found: %s", filepath.Join(abs, "agentuity.yaml"))
	}
	return abs
}
