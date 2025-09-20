package project

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/project"
	"github.com/agentuity/go-common/slice"
	cstr "github.com/agentuity/go-common/string"
	"github.com/agentuity/go-common/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

const (
	initPath = "/cli/project"
)

var Version string

type initProjectResult struct {
	Success bool        `json:"success"`
	Data    ProjectData `json:"data"`
	Message string      `json:"message"`
}

type ProjectData struct {
	APIKey           string                `json:"api_key"`
	ProjectKey       string                `json:"projectKey"`
	ProjectId        string                `json:"id"`
	OrgId            string                `json:"orgId"`
	Env              map[string]string     `json:"env"`
	Secrets          map[string]string     `json:"secrets"`
	WebhookAuthToken string                `json:"webhookAuthToken,omitempty"`
	Agents           []project.AgentConfig `json:"agents"`
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
	Agents            []project.AgentConfig
	AuthType          string
	Framework         string
}

// InitProject will create a new project in the organization.
// It will return the API key and project ID if the project is initialized successfully.
func InitProject(ctx context.Context, logger logger.Logger, args InitProjectArgs) (*ProjectData, error) {

	agents := make([]map[string]any, 0)
	for _, agent := range args.Agents {
		agents = append(agents, map[string]any{
			"name":        agent.Name,
			"description": agent.Description,
		})
	}
	payload := map[string]any{
		"organization_id":   args.OrgId,
		"provider":          args.Provider,
		"name":              args.Name,
		"description":       args.Description,
		"enableWebhookAuth": args.EnableWebhookAuth,
		"agents":            agents,
		"authType":          args.AuthType,
		"framework":         args.Framework,
	}
	logger.Trace("sending new project payload: %s", cstr.JSONStringify(payload))

	client := util.NewAPIClient(ctx, logger, args.BaseURL, args.Token)

	var result initProjectResult
	if err := client.Do("POST", initPath, payload, &result); err != nil {
		return nil, err
	}
	return &result.Data, nil
}

const (
	defaultMemory = "1Gi"
	defaultCPU    = "1000M"
	defaultDisk   = "100Mi"
)

// NewProject will create a new project that is empty.
func NewProject() *project.Project {
	var version string
	if Version == "" || Version == "dev" {
		version = ">=0.0.0" // should only happen in dev cli
	} else {
		version = ">=" + Version
	}
	return &project.Project{
		Version: version,
		Deployment: &project.Deployment{
			Resources: &project.Resources{
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

func ProjectWithNameExists(ctx context.Context, logger logger.Logger, baseUrl string, token string, orgId string, name string) (bool, error) {
	client := util.NewAPIClient(ctx, logger, baseUrl, token)

	var resp Response[bool]
	if err := client.Do("GET", fmt.Sprintf("/cli/project/exists/%s?orgId=%s", url.PathEscape(name), url.PathEscape(orgId)), nil, &resp); err != nil {
		var apiErr *util.APIError
		if errors.As(err, &apiErr) {
			if apiErr.Status == http.StatusConflict {
				return true, nil
			}
			if apiErr.Status == http.StatusUnprocessableEntity {
				return false, apiErr
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
	OrgId       string `json:"orgId"`
	OrgName     string `json:"orgName"`
}

func ListProjects(ctx context.Context, logger logger.Logger, baseUrl string, token string) ([]ProjectListData, error) {
	client := util.NewAPIClient(ctx, logger, baseUrl, token)

	var resp Response[[]ProjectListData]
	if err := client.Do("GET", "/cli/project", nil, &resp); err != nil {
		return nil, fmt.Errorf("error listing projects: %w", err)
	}
	return resp.Data, nil
}

type DeploymentListData struct {
	ID        string   `json:"id"`
	Message   string   `json:"message"`
	Tags      []string `json:"tags"`
	Active    bool     `json:"active"`
	CreatedAt string   `json:"createdAt"`
}

func ListDeployments(ctx context.Context, logger logger.Logger, baseUrl string, token string, projectId string) ([]DeploymentListData, error) {
	client := util.NewAPIClient(ctx, logger, baseUrl, token)

	var resp Response[[]DeploymentListData]
	if err := client.Do("GET", fmt.Sprintf("/cli/project/%s/deployments", projectId), nil, &resp); err != nil {
		return nil, fmt.Errorf("error listing deployments: %w", err)
	}
	return resp.Data, nil
}

func DeleteDeployment(ctx context.Context, logger logger.Logger, baseUrl string, token string, projectId string, deploymentId string) error {
	client := util.NewAPIClient(ctx, logger, baseUrl, token)

	var resp Response[string]
	if err := client.Do("DELETE", fmt.Sprintf("/cli/project/%s/deployments/%s", projectId, deploymentId), nil, &resp); err != nil {
		return fmt.Errorf("error deleting deployment: %w", err)
	}
	if !resp.Success {
		return errors.New(resp.Message)
	}
	return nil
}

func RollbackDeployment(ctx context.Context, logger logger.Logger, baseUrl string, token string, projectId string, deploymentId string) error {
	client := util.NewAPIClient(ctx, logger, baseUrl, token)

	var resp Response[string]
	if err := client.Do("POST", fmt.Sprintf("/cli/project/%s/deployments/%s/rollback", projectId, deploymentId), nil, &resp); err != nil {
		return fmt.Errorf("error rolling back deployment: %w", err)
	}
	if !resp.Success {
		return errors.New(resp.Message)
	}
	return nil
}

func DeleteProjects(ctx context.Context, logger logger.Logger, baseUrl string, token string, ids []string) ([]string, error) {
	client := util.NewAPIClient(ctx, logger, baseUrl, token)

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

func GetProject(ctx context.Context, logger logger.Logger, baseUrl string, token string, projectId string, shouldMask bool, includeProjectKeys bool) (*ProjectData, error) {
	if projectId == "" {
		return nil, project.ErrProjectNotFound
	}
	client := util.NewAPIClient(ctx, logger, baseUrl, token)

	var projectResponse ProjectResponse
	if err := client.Do("GET", fmt.Sprintf("/cli/project/%s?mask=%t&includeProjectKeys=%t", projectId, shouldMask, includeProjectKeys), nil, &projectResponse); err != nil {
		var apiErr *util.APIError
		if errors.As(err, &apiErr) {
			if apiErr.Status == 404 {
				return nil, project.ErrProjectNotFound
			}
		}
		return nil, fmt.Errorf("error getting project env: %w", err)
	}
	if !projectResponse.Success {
		return nil, errors.New(projectResponse.Message)
	}
	return &projectResponse.Data, nil
}

func SetProjectEnv(ctx context.Context, logger logger.Logger, baseUrl string, token string, projectId string, env map[string]string, secrets map[string]string) (*ProjectData, error) {
	client := util.NewAPIClient(ctx, logger, baseUrl, token)
	var projectResponse ProjectResponse
	_env := make(map[string]string)
	for k, v := range env {
		if !strings.HasPrefix(k, "AGENTUITY_") {
			_env[k] = v
		}
	}
	_secrets := make(map[string]string)
	for k, v := range secrets {
		if !strings.HasPrefix(k, "AGENTUITY_") {
			_secrets[k] = v
		}
	}
	if err := client.Do("PUT", fmt.Sprintf("/cli/project/%s/env", projectId), map[string]any{
		"env":     _env,
		"secrets": _secrets,
	}, &projectResponse); err != nil {
		return nil, fmt.Errorf("error setting project env: %w", err)
	}
	if !projectResponse.Success {
		return nil, errors.New(projectResponse.Message)
	}
	return &projectResponse.Data, nil
}

func DeleteProjectEnv(ctx context.Context, logger logger.Logger, baseUrl string, token string, projectId string, env []string, secrets []string) error {
	client := util.NewAPIClient(ctx, logger, baseUrl, token)
	var projectResponse ProjectResponse
	if err := client.Do("DELETE", fmt.Sprintf("/cli/project/%s/env", projectId), map[string]any{
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
	Name                string                `json:"name"`
	Description         string                `json:"description"`
	Provider            string                `json:"provider"`
	OrgId               string                `json:"orgId"`
	Agents              []project.AgentConfig `json:"agents"`
	EnableWebhookAuth   bool                  `json:"enableWebhookAuth"`
	CopiedFromProjectId string                `json:"copiedFromProjectId"`
}

type ProjectImportResponse struct {
	ID          string                `json:"id"`
	Agents      []project.AgentConfig `json:"agents"`
	APIKey      string                `json:"apiKey"`
	ProjectKey  string                `json:"projectKey"`
	IOAuthToken string                `json:"ioAuthToken"`
}

func ProjectImport(ctx context.Context, logger logger.Logger, baseUrl string, token string, orgId string, p *project.Project, enableWebhookAuth bool) (*ProjectImportResponse, error) {
	client := util.NewAPIClient(ctx, logger, baseUrl, token)

	var resp Response[ProjectImportResponse]
	var req ProjectImportRequest
	req.Name = p.Name
	req.Description = p.Description
	req.OrgId = orgId
	req.Agents = p.Agents
	req.Provider = p.Bundler.Identifier
	req.EnableWebhookAuth = enableWebhookAuth
	req.CopiedFromProjectId = p.ProjectId

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
	Logger       logger.Logger
	Project      *project.Project
	Dir          string
	APIURL       string
	APPURL       string
	TransportURL string
	Token        string
	NewProject   bool
}

func LoadProject(logger logger.Logger, dir string, apiUrl string, appUrl string, transportUrl, token string) ProjectContext {
	theproject := NewProject()
	if err := theproject.Load(dir); err != nil {
		if err == project.ErrProjectMissingProjectId {
			return ProjectContext{
				Logger:       logger,
				Dir:          dir,
				Project:      theproject,
				APIURL:       apiUrl,
				APPURL:       appUrl,
				TransportURL: transportUrl,
				Token:        token,
				NewProject:   true,
			}
		}
		errsystem.New(errsystem.ErrInvalidConfiguration, err,
			errsystem.WithContextMessage("Error loading project from disk")).ShowErrorAndExit()
	}
	return ProjectContext{
		Logger:       logger,
		Project:      theproject,
		Dir:          dir,
		APIURL:       apiUrl,
		APPURL:       appUrl,
		TransportURL: transportUrl,
		Token:        token,
	}
}

func isVersionCheckRequired(ver string) bool {
	if ver != "" && ver != "dev" && !strings.Contains(ver, "-next") {
		return true
	}
	return false
}

func EnsureProject(ctx context.Context, cmd *cobra.Command) ProjectContext {
	logger := env.NewLogger(cmd)
	dir := ResolveProjectDir(logger, cmd, true)
	urls := util.GetURLs(logger)
	apiUrl := urls.API
	appUrl := urls.App
	transportUrl := urls.Transport
	var token string
	// if the --api-key flag is used, we only need to verify the api key
	if cmd.Flags().Changed("api-key") {
		token = util.EnsureLoggedInWithOnlyAPIKey(ctx, logger, cmd)
	} else {
		token, _ = util.EnsureLoggedIn(ctx, logger, cmd)
	}
	p := LoadProject(logger, dir, apiUrl, appUrl, transportUrl, token)
	if !p.NewProject && isVersionCheckRequired(Version) && p.Project.Version != "" {
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

func TryProject(ctx context.Context, cmd *cobra.Command) ProjectContext {
	logger := env.NewLogger(cmd)
	dir := ResolveProjectDir(logger, cmd, false)
	urls := util.GetURLs(logger)
	apiUrl := urls.API
	appUrl := urls.App
	transportUrl := urls.Transport
	var token string
	// if the --api-key flag is used, we only need to verify the api key
	if cmd.Flags().Changed("api-key") {
		token = util.EnsureLoggedInWithOnlyAPIKey(ctx, logger, cmd)
	} else {
		apikey, _, ok := util.TryLoggedIn()
		if ok {
			token = apikey
		}
	}
	if token == "" || dir == "" || !util.Exists(project.GetProjectFilename(dir)) {
		return ProjectContext{
			Logger:       logger,
			Dir:          dir,
			NewProject:   true,
			APIURL:       apiUrl,
			APPURL:       appUrl,
			TransportURL: transportUrl,
			Token:        token,
		}
	}
	p := LoadProject(logger, dir, apiUrl, appUrl, transportUrl, token)
	if !p.NewProject && isVersionCheckRequired(Version) && p.Project.Version != "" {
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

func ResolveProjectDir(logger logger.Logger, cmd *cobra.Command, required bool) string {
	cwd, err := os.Getwd()
	if err != nil {
		errsystem.New(errsystem.ErrEnvironmentVariablesNotSet, err,
			errsystem.WithUserMessage("Failed to get current working directory: %s", err)).ShowErrorAndExit()
	}
	dir := cwd
	dirFlag, _ := cmd.Flags().GetString("dir")
	if dirFlag != "" {
		dir = dirFlag
	} else {
		if val, ok := os.LookupEnv("VSCODE_CWD"); ok {
			dir = val
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		errsystem.New(errsystem.ErrEnvironmentVariablesNotSet, err,
			errsystem.WithUserMessage("Failed to get absolute path to %s: %s", dir, err)).ShowErrorAndExit()
	}
	if !project.ProjectExists(abs) && required {
		dir = viper.GetString("preferences.project_dir")
		if project.ProjectExists(dir) {
			tui.ShowWarning("Using your last used project directory (%s). You should change into the correct directory or use the --dir flag.", dir)
			os.Chdir(dir)
			return dir
		}
		explanation := "No Agentuity project file found in the directory " + abs + "\n\nMake sure you are in an Agentuity project directory or use the --dir flag to specify a project directory."
		if tui.HasTTY {
			tui.ShowBanner("Agentuity Project Not Found", explanation, false)
		} else {
			logger.Error(explanation)
		}
		os.Exit(1)
	}
	if project.ProjectExists(abs) {
		// if we are successful, set the project dir in the config
		viper.Set("preferences.project_dir", abs)
		viper.WriteConfig()
	}
	return abs
}

func SaveEnvValue(ctx context.Context, logger logger.Logger, dir string, keyvalues map[string]string) error {
	filename := filepath.Join(dir, ".env")
	envs, err := env.ParseEnvFile(filename)
	if err != nil {
		return fmt.Errorf("error parsing env file: %w", err)
	}
	for k, v := range keyvalues {
		found := false
		for i, env := range envs {
			if env.Key == k {
				envs[i].Val = v
				found = true
				break
			}
		}
		if !found {
			envs = append(envs, env.EnvLine{Key: k, Val: v})
		}
	}
	return env.WriteEnvFile(filename, envs)
}

func RemoveEnvValues(ctx context.Context, logger logger.Logger, dir string, keys ...string) error {
	filename := filepath.Join(dir, ".env")
	envs, err := env.ParseEnvFile(filename)
	if err != nil {
		return fmt.Errorf("error parsing env file: %w", err)
	}
	newenvs := make([]env.EnvLine, 0)
	for _, env := range envs {
		found := slice.Contains(keys, env.Key)
		if !found {
			newenvs = append(newenvs, env)
		}
	}
	if len(newenvs) != len(envs) {
		return env.WriteEnvFile(filename, newenvs)
	}
	return nil
}
