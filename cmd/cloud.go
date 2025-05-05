package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/agentuity/cli/internal/agent"
	"github.com/agentuity/cli/internal/deployer"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/ignore"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/crypto"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/api/resource"
)

var cloudCmd = &cobra.Command{
	Use:   "cloud",
	Short: "Cloud related commands",
	Long: `Cloud related commands for deploying and managing your projects in the Agentuity Cloud.

Use the subcommands to deploy and manage your cloud resources.`,
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
		OrgSecret    *string `json:"orgSecret,omitempty"`
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
	Agents    []startAgent       `json:"agents"`
	Resources *Resources         `json:"resources,omitempty"`
	Metadata  *deployer.Metadata `json:"metadata,omitempty"`
}

func ShowNewProjectImport(ctx context.Context, logger logger.Logger, cmd *cobra.Command, apiUrl, apikey, projectId string, project *project.Project, dir string, isImport bool) {
	title := "Import Project"
	var message string
	if isImport {
		message = "Importing this project will update the project and agent identifiers in the project and add the project to your organization."
	} else {
		if projectId == "" {
			title = "Import Project from Template"
			message = "This project appears to be a new project from a template. By continuing, this project will be added to your organization."
		} else {
			message = fmt.Sprintf("A project with the id %s was not found in your organization. By continuing, this project will be added to your organization.", projectId)
		}
	}
	tui.ShowBanner(title, message, false)
	tui.WaitForAnyKey()
	tui.ClearScreen()
	orgId := promptForOrganization(ctx, logger, cmd, apiUrl, apikey)
	name, description := promptForProjectDetail(ctx, logger, apiUrl, apikey, project.Name, project.Description, orgId)
	project.Name = name
	project.Description = description
	var createWebhookAuth bool
	auth := getAgentAuthType(logger, "")
	if auth == "bearer" {
		createWebhookAuth = true
	}
	tui.ClearScreen()
	tui.ShowSpinner("Importing project ...", func() {
		result, err := project.Import(ctx, logger, apiUrl, apikey, orgId, createWebhookAuth)
		if err != nil {
			if isCancelled(ctx) {
				os.Exit(1)
			}
			errsystem.New(errsystem.ErrImportingProject, err,
				errsystem.WithContextMessage("Error importing project")).ShowErrorAndExit()
		}
		if err := project.Save(dir); err != nil {
			errsystem.New(errsystem.ErrSaveProject, err,
				errsystem.WithContextMessage("Error saving project after import")).ShowErrorAndExit()
		}
		saveEnv(dir, result.APIKey)
	})
	tui.ShowSuccess("Project imported successfully")
}

var cloudDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy project to the cloud",
	Long: `Deploy your project to the Agentuity Cloud.

This command packages your project, uploads it to the Agentuity Cloud,
and starts the deployment process. It will reconcile any differences
between local and remote agents.

Flags:
  --dir    The directory containing the project to deploy

Examples:
  agentuity cloud deploy
  agentuity deploy
  agentuity cloud deploy --dir /path/to/project`,
	Run: func(cmd *cobra.Command, args []string) {
		parentCtx := context.Background()
		ctx, cancel := signal.NotifyContext(parentCtx, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		logger := env.NewLogger(cmd)
		context := project.EnsureProject(ctx, cmd)
		theproject := context.Project
		dir := context.Dir
		apiUrl := context.APIURL
		appUrl := context.APPURL
		transportUrl := context.TransportURL
		token := context.Token
		ci, _ := cmd.Flags().GetBool("ci")
		ciRemoteUrl, _ := cmd.Flags().GetString("ci-remote-url")
		ciBranch, _ := cmd.Flags().GetString("ci-branch")
		ciCommit, _ := cmd.Flags().GetString("ci-commit")
		ciMessage, _ := cmd.Flags().GetString("ci-message")
		ciGitProvider, _ := cmd.Flags().GetString("ci-git-provider")

		deploymentConfig := project.NewDeploymentConfig()
		client := util.NewAPIClient(ctx, logger, apiUrl, token)
		var envFile *deployer.EnvFile
		var projectData *project.ProjectData
		var state map[string]agentListState

		if !ci {
			checkForUpgrade(ctx, logger, true)

			loadTemplates(ctx, cmd)

			var keys []string

			if !context.NewProject {
				keys, state = reconcileAgentList(logger, cmd, apiUrl, token, context)

				if len(keys) == 0 {
					tui.ShowWarning("no Agents found")
					tui.ShowBanner("Create a new Agent", tui.Text("Use the ")+tui.Command("agent new")+tui.Text(" command to create a new Agent"), false)
					os.Exit(1)
				}
			}

			var err error
			var le []env.EnvLine
			var projectExists bool
			var action func()

			if !context.NewProject {
				action = func() {
					var err error
					projectData, err = theproject.GetProject(ctx, logger, apiUrl, token)
					if err != nil {
						if err == project.ErrProjectNotFound {
							return
						}
						if isCancelled(ctx) {
							os.Exit(1)
						}
						errsystem.New(errsystem.ErrApiRequest, err,
							errsystem.WithContextMessage("Error listing project environment")).ShowErrorAndExit()
					}
					projectExists = true
				}
				tui.ShowSpinner("", action)
			}

			if !projectExists {
				var projectId string
				if theproject != nil {
					projectId = theproject.ProjectId
				}
				ShowNewProjectImport(ctx, logger, cmd, apiUrl, token, projectId, theproject, dir, false)
			}

			// check to see if we have any env vars that are not in the project
			envfilename := filepath.Join(dir, ".env")
			if tui.HasTTY && util.Exists(envfilename) {

				le, err = env.ParseEnvFile(envfilename)
				if err != nil {
					errsystem.New(errsystem.ErrParseEnvironmentFile, err,
						errsystem.WithContextMessage("Error parsing .env file")).ShowErrorAndExit()
				}
				envFile = &deployer.EnvFile{Filepath: envfilename, Env: le}

				var foundkeys []string
				for _, ev := range le {
					if isAgentuityEnv.MatchString(ev.Key) {
						continue
					}
					if projectData != nil && projectData.Env != nil && projectData.Env[ev.Key] == ev.Val {
						continue
					}
					if projectData != nil && projectData.Secrets != nil && projectData.Secrets[ev.Key] == ev.Val {
						continue
					}
					foundkeys = append(foundkeys, ev.Key)
				}
				if len(foundkeys) > 0 {
					var title string
					switch {
					case len(foundkeys) < 3 && len(foundkeys) > 1:
						title = fmt.Sprintf("The environment variables %s from .env are not been set in the project. Would you like to add it?", strings.Join(foundkeys, ", "))
					case len(foundkeys) == 1:
						title = fmt.Sprintf("The environment variable %s from .env has not been set in the project. Would you like to add it?", foundkeys[0])
					default:
						title = fmt.Sprintf("There are %d environment variables from .env that are not set in the project. Would you like to add them?", len(foundkeys))
					}
					if !tui.Ask(logger, title, true) {
						tui.ShowWarning("cancelled")
						return
					}
					envs, secrets := loadEnvFile(le, false)
					pd, err := theproject.SetProjectEnv(ctx, logger, apiUrl, token, envs, secrets)
					if err != nil {
						if isCancelled(ctx) {
							os.Exit(1)
						}
						errsystem.New(errsystem.ErrEnvironmentVariablesNotSet, err,
							errsystem.WithContextMessage("Failed to set project environment variables")).ShowErrorAndExit()
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

			if tui.HasTTY {
				_, localIssues, remoteIssues, err := buildAgentTree(keys, state, context)
				if err != nil {
					errsystem.New(errsystem.ErrInvalidConfiguration, err,
						errsystem.WithContextMessage("Failed to build agent tree")).ShowErrorAndExit()
				}

				showAgentWarnings(remoteIssues, localIssues, true)
			}
		}

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
			errsystem.New(errsystem.ErrDeployProject, err).ShowErrorAndExit()
		}

		if err := deploymentConfig.Write(logger, dir); err != nil {
			errsystem.New(errsystem.ErrWriteConfigurationFile, err,
				errsystem.WithContextMessage("Error writing deployment config to disk")).ShowErrorAndExit()
		}

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

		if !ci {
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
		}

		// check for a deploymentId flag and if so we can append it to the deployment url
		deploymentId, _ := cmd.Flags().GetString("deploymentId")
		if deploymentId != "" {
			logger.Debug("deploymentId flag provided: %s", deploymentId)
			deploymentId = "/" + deploymentId
		}

		var gitInfo deployer.GitInfo
		var originType string
		isOverwritingGitInfo := ciRemoteUrl != "" || ciBranch != "" || ciCommit != "" || ciMessage != "" || ciGitProvider != ""
		if ci && isOverwritingGitInfo {
			originType = "ci"
			gitInfo = deployer.GitInfo{
				RemoteURL:     &ciRemoteUrl,
				Branch:        &ciBranch,
				Commit:        &ciCommit,
				CommitMessage: &ciMessage,
				GitProvider:   &ciGitProvider,
				IsRepo:        true,
			}
		} else {
			info, err := deployer.GetGitInfoRecursive(logger, dir)
			if err != nil {
				logger.Debug("Failed to get git info: %v", err)
			}
			gitInfo = *info
			originType = "cli"
		}

		startRequest.Metadata = &deployer.Metadata{
			Origin: deployer.MetadataOrigin{
				Type: originType,
				Data: map[string]interface{}{
					"machine": deployer.GetMachineInfo(),
					"git":     gitInfo,
				},
			},
		}

		// Start deployment
		if err := client.Do("PUT", fmt.Sprintf("/cli/deploy/start/%s%s", theproject.ProjectId, deploymentId), startRequest, &startResponse); err != nil {
			errsystem.New(errsystem.ErrDeployProject, err,
				errsystem.WithContextMessage("Error starting deployment")).ShowErrorAndExit()
		}

		if !startResponse.Success {
			if startResponse.Message != nil {
				errsystem.New(errsystem.ErrDeployProject, fmt.Errorf("%s", *startResponse.Message),
					errsystem.WithContextMessage("Error starting deployment")).ShowErrorAndExit()
			}
			errsystem.New(errsystem.ErrDeployProject, fmt.Errorf("unknown error"),
				errsystem.WithContextMessage("Unknown API error starting deployment")).ShowErrorAndExit()
		}

		var orgSecret string

		if startResponse.Data.OrgSecret == nil {
			orgSecret = os.Getenv("AGENTUITY_ORG_SECRET")
			if orgSecret == "" {
				orgSecret = viper.GetString("org.secret")
				if orgSecret == "" {
					errsystem.New(errsystem.ErrDeployProject, fmt.Errorf("no org secret found"),
						errsystem.WithContextMessage("No org secret found")).ShowErrorAndExit()
				}
			}
		} else {
			orgSecret = *startResponse.Data.OrgSecret
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
				errsystem.New(errsystem.ErrSaveProject, err,
					errsystem.WithContextMessage("Error saving project with new Agents")).ShowErrorAndExit()
			}
			logger.Debug("saved project with updated Agents")
		}

		// load up any gitignore files
		gitignore := filepath.Join(dir, ignore.Ignore)
		rules := ignore.Empty()
		if util.Exists(gitignore) {
			r, err := ignore.ParseFile(gitignore)
			if err != nil {
				errsystem.New(errsystem.ErrInvalidConfiguration, err,
					errsystem.WithContextMessage("Error parsing .gitignore file")).ShowErrorAndExit()
			}
			rules = r
		}
		rules.AddDefaults()

		// add any provider specific ignore rules
		for _, rule := range theproject.Bundler.Ignore {
			if err := rules.Add(rule); err != nil {
				errsystem.New(errsystem.ErrInvalidConfiguration, err,
					errsystem.WithContextMessage(fmt.Sprintf("Error adding project ignore rule: %s. %s", rule, err))).ShowErrorAndExit()
			}
		}

		// create a temp file we're going to use for zip and upload
		tmpfile, err := os.CreateTemp("", "agentuity-deploy-*.zip")
		if err != nil {
			errsystem.New(errsystem.ErrCreateTemporaryFile, err,
				errsystem.WithContextMessage("Error creating temp file")).ShowErrorAndExit()
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
				errsystem.New(errsystem.ErrCreateZipFile, err,
					errsystem.WithContextMessage("Error zipping project")).ShowErrorAndExit()
			}
			logger.Debug("zip file created in %v", time.Since(started))
		}

		tui.ShowSpinner("Packaging ...", zipaction)

		dof, err := os.Open(tmpfile.Name())
		if err != nil {
			errsystem.New(errsystem.ErrOpenFile, err,
				errsystem.WithContextMessage("Error opening deployment zip file")).ShowErrorAndExit()
		}
		defer dof.Close()

		ef, err := os.CreateTemp("", "agentuity-deploy-*.zip")
		if err != nil {
			errsystem.New(errsystem.ErrCreateTemporaryFile, err,
				errsystem.WithContextMessage("Error creating temp file")).ShowErrorAndExit()
		}
		defer os.Remove(ef.Name())
		defer ef.Close()

		if err := crypto.EncryptStream(dof, ef, orgSecret); err != nil {
			errsystem.New(errsystem.ErrEncryptingDeploymentZipFile, err,
				errsystem.WithContextMessage("Error encrypting deployment zip file")).ShowErrorAndExit()
		}
		dof.Close()
		os.Remove(tmpfile.Name()) // remove the unencrypted zip file
		ef, err = os.Open(ef.Name())
		if err != nil {
			errsystem.New(errsystem.ErrOpenFile, err,
				errsystem.WithContextMessage("Error opening encrypted deployment zip file")).ShowErrorAndExit()
		}
		defer ef.Close()

		fi, err := ef.Stat()
		if err != nil {
			errsystem.New(errsystem.ErrEncryptingDeploymentZipFile, err,
				errsystem.WithContextMessage("Error getting file stats after encryption")).ShowErrorAndExit()
		}
		started := time.Now()
		var webhookToken string

		action := func() {
			url := util.TransformUrl(startResponse.Data.Url)
			// send the zip file to the upload endpoint provided
			logger.Trace("uploading to %s", url)
			// NOTE: we don't use the apiclient here because we're not going to our api
			req, err := http.NewRequest("PUT", url, ef)
			if err != nil {
				errsystem.New(errsystem.ErrUploadProject, err,
					errsystem.WithContextMessage("Error creating PUT request")).ShowErrorAndExit()
			}
			req.ContentLength = fi.Size()
			// NOTE: this is a one-time signed url so we don't need to add authorization header
			req.Header.Set("Content-Type", "application/zip")
			req.Header.Set("Content-Length", strconv.FormatInt(fi.Size(), 10))

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				if err := updateDeploymentStatus(logger, apiUrl, token, startResponse.Data.DeploymentId, "failed"); err != nil {
					errsystem.New(errsystem.ErrApiRequest, err,
						errsystem.WithContextMessage("Error updating deployment status to failed")).ShowErrorAndExit()
				}
				errsystem.New(errsystem.ErrUploadProject, err,
					errsystem.WithContextMessage("Error deploying project")).ShowErrorAndExit()
			}
			if resp.StatusCode > 299 {
				buf, _ := io.ReadAll(resp.Body)
				if err := updateDeploymentStatus(logger, apiUrl, token, startResponse.Data.DeploymentId, "failed"); err != nil {
					errsystem.New(errsystem.ErrApiRequest, err,
						errsystem.WithContextMessage("Error updating deployment status to failed")).ShowErrorAndExit()
				}
				errsystem.New(errsystem.ErrUploadProject, nil,
					errsystem.WithContextMessage(fmt.Sprintf("Unexpected response (status %d): %s", resp.StatusCode, string(buf))),
					errsystem.WithUserMessage("Unexpected response from API for deployment")).ShowErrorAndExit()
			}
			resp.Body.Close()
			logger.Debug("deployment uploaded %d bytes in %v", fi.Size(), time.Since(started))

			// tell the api that we've completed the upload for the deployment
			if err := updateDeploymentStatusCompleted(logger, apiUrl, token, startResponse.Data.DeploymentId); err != nil {
				errsystem.New(errsystem.ErrApiRequest, err,
					errsystem.WithContextMessage("Error updating deployment status to completed")).ShowErrorAndExit()
			}
			if len(theproject.Agents) == 1 {
				webhookToken, err = agent.GetApiKey(ctx, logger, apiUrl, token, theproject.Agents[0].ID)
				if err != nil {
					errsystem.New(errsystem.ErrApiRequest, err,
						errsystem.WithContextMessage("Error getting Agent API key")).ShowErrorAndExit()
				}
			}
		}

		tui.ShowSpinner("Deploying ...", action)

		format, _ := cmd.Flags().GetString("format")
		if format == "json" {
			buf, _ := json.Marshal(theproject)
			kv := map[string]any{}
			json.Unmarshal(buf, &kv)
			kv["deployment_id"] = startResponse.Data.DeploymentId
			kv["deployment_url"] = fmt.Sprintf("%s/projects/%s/deployments", appUrl, theproject.ProjectId)
			kv["project_url"] = fmt.Sprintf("%s/projects/%s", appUrl, theproject.ProjectId)
			json.NewEncoder(os.Stdout).Encode(kv)
		} else {
			if tui.HasTTY {
				if tui.HasTTY {
					body := tui.Body("· Track your project at\n  " + tui.Link("%s/projects/%s", appUrl, theproject.ProjectId))
					var body2 string

					if len(theproject.Agents) == 1 {
						body2 = "\n\n"
						if webhookToken != "" {
							body2 += tui.Body("· Run ") + tui.Command("agent apikey "+theproject.Agents[0].ID) + tui.Body("\n  to fetch the Webhook API key for this webhook")
							body2 += "\n\n"
						}
						body2 += tui.Body(fmt.Sprintf("· Send %s webhook POST request to\n  ", theproject.Agents[0].Name) + tui.Link("%s/webhook/%s", transportUrl, strings.Replace(theproject.Agents[0].ID, "agent_", "", 1)))
					}

					tui.ShowBanner("Your project was deployed successfully!", body+body2, true)
				}
			}
		}
	},
}

func updateDeploymentStatus(logger logger.Logger, apiUrl, token, deploymentId, status string) error {
	client := util.NewAPIClient(context.Background(), logger, apiUrl, token)
	payload := map[string]string{"state": status}
	return client.Do("PUT", fmt.Sprintf("/cli/deploy/upload/%s", deploymentId), payload, nil)
}

func updateDeploymentStatusCompleted(logger logger.Logger, apiUrl, token, deploymentId string) error {
	client := util.NewAPIClient(context.Background(), logger, apiUrl, token)
	payload := map[string]any{"state": "completed"}
	return client.Do("PUT", fmt.Sprintf("/cli/deploy/upload/%s", deploymentId), payload, nil)
}

func init() {
	rootCmd.AddCommand(cloudCmd)
	rootCmd.AddCommand(cloudDeployCmd)
	cloudCmd.AddCommand(cloudDeployCmd)
	cloudDeployCmd.Flags().StringP("dir", "d", ".", "The directory to the project to deploy")
	cloudDeployCmd.Flags().String("deploymentId", "", "Used to track a specific deployment")
	cloudDeployCmd.Flags().Bool("ci", false, "Used to track a specific CI job")
	cloudDeployCmd.Flags().String("ci-remote-url", "", "Used to set the remote repository URL for your deployment metadata")
	cloudDeployCmd.Flags().String("ci-branch", "", "Used to set the branch name for your deployment metadata")
	cloudDeployCmd.Flags().String("ci-commit", "", "Used to set the commit hash for your deployment metadata")
	cloudDeployCmd.Flags().String("ci-message", "", "Used to set the commit message for your deployment metadata")
	cloudDeployCmd.Flags().String("ci-git-provider", "", "Used to set the git provider for your deployment metadata")

	cloudDeployCmd.Flags().MarkHidden("deploymentId")
	cloudDeployCmd.Flags().MarkHidden("ci")
	cloudDeployCmd.Flags().MarkHidden("ci-remote-url")
	cloudDeployCmd.Flags().MarkHidden("ci-branch")
	cloudDeployCmd.Flags().MarkHidden("ci-commit")
	cloudDeployCmd.Flags().MarkHidden("ci-messsage")
	cloudDeployCmd.Flags().MarkHidden("ci-git-provider")

	cloudDeployCmd.Flags().String("format", "text", "The output format to use for results which can be either 'text' or 'json'")
	cloudDeployCmd.Flags().String("org-id", "", "The organization to create the project in")
	cloudDeployCmd.Flags().String("templates-dir", "", "The directory to load the templates. Defaults to loading them from the github.com/agentuity/templates repository")
}
