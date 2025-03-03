package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/agentuity/cli/internal/deployer"
	"github.com/agentuity/cli/internal/ignore"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/tui"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/api/resource"
)

var cloudCmd = &cobra.Command{
	Use:   "cloud",
	Short: "Cloud related commands",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

type Agent struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}
type startResponse struct {
	Success bool `json:"success"`
	Data    struct {
		DeploymentId string  `json:"deploymentId"`
		Url          string  `json:"url"`
		Created      []Agent `json:"created,omitempty"`
	}
	Message *string `json:"message,omitempty"`
}

type Resources struct {
	Memory int64 `json:"memory,omitempty"`
	CPU    int64 `json:"cpu,omitempty"`
	Disk   int64 `json:"disk,omitempty"`
}

type startAgent struct {
	Agent
	Remove bool `json:"remove,omitempty"`
}

type startRequest struct {
	Agents    []startAgent `json:"agents"`
	Resources *Resources   `json:"resources,omitempty"`
}

type projectContext struct {
	Logger  logger.Logger
	Project *project.Project
	Dir     string
	APIURL  string
	APPURL  string
	Token   string
}

func ensureProject(cmd *cobra.Command) projectContext {
	logger := env.NewLogger(cmd)
	dir := resolveProjectDir(logger, cmd)
	apiUrl, appUrl := getURLs(logger)
	token := viper.GetString("auth.api_key")

	// validate our project
	theproject := project.NewProject()
	if err := theproject.Load(dir); err != nil {
		logger.Fatal("error loading project: %s", err)
	}

	return projectContext{
		Logger:  logger,
		Project: theproject,
		Dir:     dir,
		APIURL:  apiUrl,
		APPURL:  appUrl,
		Token:   token,
	}
}

var cloudDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy project to the cloud",
	Run: func(cmd *cobra.Command, args []string) {
		ctx := context.Background()
		context := ensureProject(cmd)
		logger := context.Logger
		theproject := context.Project
		dir := context.Dir
		apiUrl := context.APIURL
		appUrl := context.APPURL
		token := context.Token

		keys, state := reconcileAgentList(logger, apiUrl, token, context)

		if len(keys) == 0 {
			tui.ShowWarning("no Agents found")
			tui.ShowBanner("Create a new Agent", tui.Text("Use the ")+tui.Command("agent new")+tui.Text(" command to create a new Agent"), false)
			return
		}

		deploymentConfig := project.NewDeploymentConfig()

		client := util.NewAPIClient(logger, apiUrl, token)
		var err error
		var le []env.EnvLine
		var envFile *deployer.EnvFile
		var projectData *project.ProjectData

		action := func() {
			var err error
			projectData, err = theproject.ListProjectEnv(logger, apiUrl, token)
			if err != nil {
				logger.Fatal("error listing project env: %s", err)
			}
		}
		tui.ShowSpinner(logger, "", action)

		// check to see if we have any env vars that are not in the project
		envfilename := filepath.Join(dir, ".env")
		if util.Exists(envfilename) {

			le, err = env.ParseEnvFile(envfilename)
			if err != nil {
				logger.Fatal("error parsing env file: %s. %s", envfilename, err)
			}
			envFile = &deployer.EnvFile{Filepath: envfilename, Env: le}

			var foundkeys []string
			for _, ev := range le {
				if isAgentuityEnv.MatchString(ev.Key) {
					continue
				}
				if projectData.Env != nil && projectData.Env[ev.Key] == ev.Val {
					continue
				}
				if projectData.Secrets != nil && projectData.Secrets[ev.Key] == ev.Val {
					continue
				}
				foundkeys = append(foundkeys, ev.Key)
			}
			if len(foundkeys) > 0 {
				var title string
				switch {
				case len(foundkeys) < 3 && len(foundkeys) > 1:
					title = fmt.Sprintf("The environment variables %s from .env are not in the project. Would you like to add it?", strings.Join(foundkeys, ", "))
				case len(foundkeys) == 1:
					title = fmt.Sprintf("The environment variable %s from .env is not in the project. Would you like to add it?", foundkeys[0])
				default:
					title = fmt.Sprintf("There are %d environment variables from .envthat are not in the project. Would you like to add them?", len(foundkeys))
				}
				if !tui.Ask(logger, title, true) {
					tui.ShowWarning("cancelled")
					return
				}
				envs, secrets := loadEnvFile(le, false)
				pd, err := theproject.SetProjectEnv(logger, apiUrl, token, envs, secrets)
				if err != nil {
					logger.Fatal("failed to set project env: %s", err)
				}
				projectData = pd // overwrite with the new version
				switch {
				case len(envs) > 0 && len(secrets) > 0:
					tui.ShowSuccess("Environment variables and secrets added")
				case len(envs) == 1:
					tui.ShowSuccess("Environment variable added")
				case len(envs) > 1:
					tui.ShowSuccess("Environment variables added")
				case len(secrets) == 1:
					tui.ShowSuccess("Secret added")
				case len(secrets) > 1:
					tui.ShowSuccess("Secrets added")
				}
			}
		}

		_, localIssues, remoteIssues, err := buildAgentTree(keys, state, context)
		if err != nil {
			logger.Fatal("%s", err)
		}

		showAgentWarnings(remoteIssues, localIssues, true)

		deploymentConfig.Provider = theproject.Bundler.Identifier
		deploymentConfig.Language = theproject.Bundler.Language
		deploymentConfig.Runtime = theproject.Bundler.Runtime
		deploymentConfig.Command = append([]string{theproject.Deployment.Command}, theproject.Deployment.Args...)

		if err := deployer.PreflightCheck(ctx, logger, deployer.DeployPreflightCheckData{
			Dir:           dir,
			APIClient:     client,
			APIURL:        apiUrl,
			APIKey:        token,
			Envfile:       envFile,
			Project:       theproject,
			ProjectData:   projectData,
			Config:        deploymentConfig,
			OSEnvironment: loadOSEnv(),
			PromptHelpers: createPromptHelper(),
		}); err != nil {
			logger.Fatal("%s", err)
		}

		cleanup, err := deploymentConfig.Write(logger, dir)
		if err != nil {
			logger.Fatal("error writing deployment config: %s", err)
		}
		defer cleanup()

		var startResponse startResponse
		var startRequest startRequest

		if theproject.Deployment.Resources != nil {
			startRequest.Resources = &Resources{
				Memory: theproject.Deployment.Resources.MemoryQuantity.ScaledValue(resource.Mega),
				CPU:    theproject.Deployment.Resources.CPUQuantity.ScaledValue(resource.Mega),
				Disk:   theproject.Deployment.Resources.DiskQuantity.ScaledValue(resource.Mega),
			}
		}
		for _, agent := range theproject.Agents {
			startRequest.Agents = append(startRequest.Agents, startAgent{
				Agent: Agent{
					ID:          agent.ID,
					Name:        agent.Name,
					Description: agent.Description,
				},
			})
		}
		hasLocalDeletes := make(map[string]bool)

		for _, agent := range state {
			if agent.FoundLocal && !agent.FoundRemote {
				startRequest.Agents = append(startRequest.Agents, startAgent{
					Agent: Agent{
						ID:          "",
						Name:        agent.Agent.Name,
						Description: agent.Agent.Description,
					},
				})
			} else if agent.FoundRemote && !agent.FoundLocal {
				hasLocalDeletes[agent.Agent.ID] = true
				startRequest.Agents = append(startRequest.Agents, startAgent{
					Agent: Agent{
						ID:          agent.Agent.ID,
						Name:        agent.Agent.Name,
						Description: agent.Agent.Description,
					},
					Remove: true,
				})
			}
		}

		// Start deployment
		if err := client.Do("PUT", fmt.Sprintf("/cli/deploy/start/%s", theproject.ProjectId), startRequest, &startResponse); err != nil {
			logger.Fatal("error starting deployment: %s", err)
		}

		if !startResponse.Success {
			if startResponse.Message != nil {
				logger.Fatal("error starting deployment: %s", *startResponse.Message)
			}
			logger.Fatal("unknown error starting deployment")
		}

		var saveProject bool

		// remove any agents that were deleted from the project
		if len(hasLocalDeletes) > 0 {
			var newagents []project.AgentConfig
			for _, agent := range state {
				if _, ok := hasLocalDeletes[agent.Agent.ID]; ok {
					continue
				}
				if agent.Agent.ID == "" {
					continue
				}
				newagents = append(newagents, project.AgentConfig(*agent.Agent))
			}
			theproject.Agents = newagents
			saveProject = true
		}

		// save any new agents to the project that we're created as part of the deployment
		if len(startResponse.Data.Created) > 0 {
			for _, agent := range startResponse.Data.Created {
				theproject.Agents = append(theproject.Agents, project.AgentConfig{
					ID:          agent.ID,
					Name:        agent.Name,
					Description: agent.Description,
				})
			}
			saveProject = true
		}

		if saveProject {
			if err := theproject.Save(dir); err != nil {
				logger.Fatal("error saving project with new Agents: %s", err)
			}
			logger.Debug("saved project with updated Agents")
		}

		// load up any gitignore files
		gitignore := filepath.Join(dir, ignore.Ignore)
		rules := ignore.Empty()
		if util.Exists(gitignore) {
			r, err := ignore.ParseFile(gitignore)
			if err != nil {
				logger.Fatal("error parsing gitignore: %s", err)
			}
			rules = r
		}
		rules.AddDefaults()

		// add any provider specific ignore rules
		for _, rule := range theproject.Bundler.Ignore {
			if err := rules.Add(rule); err != nil {
				logger.Fatal("error adding project ignore rule: %s. %s", rule, err)
			}
		}

		// create a temp file we're going to use for zip and upload
		tmpfile, err := os.CreateTemp("", "agentuity-deploy-*.zip")
		if err != nil {
			logger.Fatal("error creating temp file: %s", err)
		}
		defer os.Remove(tmpfile.Name())
		tmpfile.Close()

		zipaction := func() {
			// zip up our directory
			started := time.Now()
			logger.Debug("creating a zip file of %s into %s", dir, tmpfile.Name())
			if err := util.ZipDir(dir, tmpfile.Name(), func(fn string, fi os.FileInfo) bool {
				notok := rules.Ignore(fn, fi)
				if notok {
					logger.Trace("❌ %s", fn)
				} else {
					logger.Trace("❎ %s", fn)
				}
				return !notok
			}); err != nil {
				logger.Fatal("error zipping project: %s", err)
			}
			logger.Debug("zip file created in %v", time.Since(started))
		}

		tui.ShowSpinner(logger, "Packaging ...", zipaction)

		of, err := os.Open(tmpfile.Name())
		if err != nil {
			logger.Fatal("error opening deloyment zip file: %s", err)
		}
		defer of.Close()

		fi, _ := os.Stat(tmpfile.Name())
		started := time.Now()
		// var webhookToken string

		action = func() {
			// send the zip file to the upload endpoint provided
			req, err := http.NewRequest("PUT", startResponse.Data.Url, of)
			if err != nil {
				logger.Fatal("error creating PUT request", err)
			}
			req.ContentLength = fi.Size()
			// NOTE: this is a one-time signed url so we don't need to add authorization header
			req.Header.Set("Content-Type", "application/zip")
			req.Header.Set("Content-Length", strconv.FormatInt(fi.Size(), 10))

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				if err := updateDeploymentStatus(logger, apiUrl, token, startResponse.Data.DeploymentId, "failed"); err != nil {
					logger.Fatal("%s", err)
				}
				logger.Fatal("error uploading deployment: %s", err)
			}
			if resp.StatusCode != http.StatusOK {
				buf, _ := io.ReadAll(resp.Body)
				if err := updateDeploymentStatus(logger, apiUrl, token, startResponse.Data.DeploymentId, "failed"); err != nil {
					logger.Fatal("%s", err)
				}
				logger.Fatal("error uploading deployment (%s) %s", resp.Status, string(buf))
			}
			resp.Body.Close()
			logger.Debug("deployment uploaded %d bytes in %v", fi.Size(), time.Since(started))

			// tell the api that we've completed the upload for the deployment
			if err := updateDeploymentStatusCompleted(logger, apiUrl, token, startResponse.Data.DeploymentId); err != nil {
				logger.Fatal("%s", err)
			}
		}

		tui.ShowSpinner(logger, "Deploying ...", action)

		body := tui.Body("· Track Agent deployment at " + tui.Link("%s/projects/%s?deploymentId=%s", appUrl, theproject.ProjectId, startResponse.Data.DeploymentId))
		body2 := tui.Body(fmt.Sprintf("· Send %s webhook request to ", theproject.Agents[0].Name) + tui.Link("%s/run/%s", apiUrl, theproject.Agents[0].ID))

		tui.ShowBanner("Your project was deployed successfully!", body+"\n\n"+body2, true)
	},
}

func updateDeploymentStatus(logger logger.Logger, apiUrl, token, deploymentId, status string) error {
	client := util.NewAPIClient(logger, apiUrl, token)
	payload := map[string]string{"state": status}
	return client.Do("PUT", fmt.Sprintf("/cli/deploy/upload/%s", deploymentId), payload, nil)
}

func updateDeploymentStatusCompleted(logger logger.Logger, apiUrl, token, deploymentId string) error {
	client := util.NewAPIClient(logger, apiUrl, token)
	payload := map[string]any{"state": "completed"}
	return client.Do("PUT", fmt.Sprintf("/cli/deploy/upload/%s", deploymentId), payload, nil)
}

func init() {
	rootCmd.AddCommand(cloudCmd)
	rootCmd.AddCommand(cloudDeployCmd)
	cloudCmd.AddCommand(cloudDeployCmd)
	cloudDeployCmd.Flags().StringP("dir", "d", ".", "The directory to the project to deploy")
}
