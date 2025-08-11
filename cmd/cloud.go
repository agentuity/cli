package cmd

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/agentuity/cli/internal/agent"
	"github.com/agentuity/cli/internal/deployer"
	"github.com/agentuity/cli/internal/envutil"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/ignore"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/crypto"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/tui"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
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
		PublicKey    *string `json:"publicKey,omitempty"`
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
	Agents         []startAgent       `json:"agents"`
	Resources      *Resources         `json:"resources,omitempty"`
	Metadata       *deployer.Metadata `json:"metadata,omitempty"`
	Tags           []string           `json:"tags,omitempty"`
	TagDescription string             `json:"description,omitempty"`
	TagMessage     string             `json:"message,omitempty"`
	UsePrivateKey  bool               `json:"usePrivateKey,omitempty"`
}

func ShowNewProjectImport(ctx context.Context, logger logger.Logger, cmd *cobra.Command, apiUrl string, apikey string, projectId string, project *project.Project, dir string, isImport bool) {
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
		saveEnv(dir, result.APIKey, result.ProjectKey)
	})
	tui.ShowSuccess("Project imported successfully")
}

var envTemplateFileNames = []string{".env.example", ".env.template"}

var border = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1).BorderForeground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#999999"})
var redDiff = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#990000", Dark: "#EE0000"})

func createProjectIgnoreRules(dir string, theproject *project.Project, skipProjectIgnore bool) *ignore.Rules {
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

	if skipProjectIgnore {
		return rules
	}

	// add any provider specific ignore rules
	for _, rule := range theproject.Bundler.Ignore {
		if err := rules.Add(rule); err != nil {
			errsystem.New(errsystem.ErrInvalidConfiguration, err,
				errsystem.WithContextMessage(fmt.Sprintf("Error adding project ignore rule: %s. %s", rule, err))).ShowErrorAndExit()
		}
	}

	return rules
}

var cloudDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy project to the cloud",
	Long: `Deploy your project to the Agentuity Cloud.

This command packages your project, uploads it to the Agentuity Cloud,
and starts the deployment process. It will reconcile any differences
between local and remote agents.

Flags:
  --dir       The directory containing the project to deploy
  --dry-run   Save deployment zip file to specified directory instead of uploading

Examples:
  agentuity cloud deploy
  agentuity deploy
  agentuity cloud deploy --dir /path/to/project
  agentuity deploy --dry-run ./output`,
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
		ciLogsUrl, _ := cmd.Flags().GetString("ci-logs-url")
		tags, _ := cmd.Flags().GetStringArray("tag")
		description, _ := cmd.Flags().GetString("description")
		message, _ := cmd.Flags().GetString("message")
		dryRun, _ := cmd.Flags().GetString("dry-run")

		// remove duplicates and empty strings
		tags = util.RemoveDuplicates(tags)
		tags = util.RemoveEmpty(tags)

		var preview bool

		// If no tags are provided, default to ["latest"]
		if len(tags) == 0 {
			logger.Debug("no tags provided, setting to latest")
			tags = []string{"latest"}
		}

		if !slices.Contains(tags, "latest") {
			logger.Debug("latest tag not found in tags array, setting preview to true")
			preview = true
		}

		logger.Debug("preview: %v", preview)

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
			var projectExists bool
			var action func()

			if !context.NewProject {
				action = func() {
					projectData, err = theproject.GetProject(ctx, logger, apiUrl, token, false, false)
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

			force, _ := cmd.Flags().GetBool("force")
			if !tui.HasTTY {
				force = true
			}
			// check to see if we have any env vars that are not in the project
			envFile, projectData = envutil.ProcessEnvFiles(ctx, logger, dir, theproject, projectData, apiUrl, token, force, false)

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

		var zipMutator util.ZipDirCallbackMutator

		preflightAction := func() {
			zm, err := deployer.PreflightCheck(ctx, logger, deployer.DeployPreflightCheckData{
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
			})
			if err != nil {
				errsystem.New(errsystem.ErrDeployProject, err).ShowErrorAndExit()
			}

			if err := deploymentConfig.Write(logger, dir); err != nil {
				errsystem.New(errsystem.ErrWriteConfigurationFile, err,
					errsystem.WithContextMessage("Error writing deployment config to disk")).ShowErrorAndExit()
			}

			zipMutator = zm
		}

		tui.ShowSpinner("Bundling ...", preflightAction)

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
		var ciInfo deployer.CIInfo
		isOverwritingGitInfo := ciRemoteUrl != "" || ciBranch != "" || ciCommit != "" || ciMessage != "" || ciGitProvider != "" || ciLogsUrl != ""
		if ci && isOverwritingGitInfo {
			originType = "ci"
			ciInfo = deployer.CIInfo{
				LogsURL: ciLogsUrl,
			}
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

		data := map[string]interface{}{
			"machine": deployer.GetMachineInfo(),
			"git":     gitInfo,
		}
		if originType == "ci" && ciLogsUrl != "" {
			data["ci"] = ciInfo
		}
		startRequest.Metadata = &deployer.Metadata{
			Origin: deployer.MetadataOrigin{
				Type: originType,
				Data: data,
			},
		}

		startRequest.Tags = tags
		startRequest.TagDescription = description
		startRequest.TagMessage = message
		startRequest.UsePrivateKey = true

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
		var publicKey string

		if startResponse.Data.PublicKey != nil {
			publicKey = *startResponse.Data.PublicKey
		}

		if startResponse.Data.OrgSecret != nil {
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

		rules := createProjectIgnoreRules(dir, theproject, false)

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
			var seenGit, seenNodeModules, seenVenv bool
			logger.Debug("creating a zip file of %s into %s", dir, tmpfile.Name())
			if err := util.ZipDir(dir, tmpfile.Name(), util.WithMutator(zipMutator), util.WithMatcher(func(fn string, fi os.FileInfo) bool {
				notok := rules.Ignore(fn, fi)
				if notok {
					if strings.HasPrefix(fn, ".git") {
						if seenGit {
							return false
						}
						seenGit = true
					}
					if strings.HasPrefix(fn, "node_modules") {
						if seenNodeModules {
							return false
						}
						seenNodeModules = true
					}
					if strings.HasPrefix(fn, ".venv") {
						if seenVenv {
							return false
						}
						seenVenv = true
					}
					logger.Trace("❌ %s", fn)
				} else {
					logger.Trace("❎ %s", fn)
				}
				return !notok
			})); err != nil {
				errsystem.New(errsystem.ErrCreateZipFile, err,
					errsystem.WithContextMessage("Error zipping project")).ShowErrorAndExit()
			}
			logger.Debug("zip file created in %v", time.Since(started))
		}

		tui.ShowSpinner("Packaging ...", zipaction)

		if dryRun != "" {

			// Validate and create the dryRun directory if it doesn't exist
			if !util.Exists(dryRun) {
				if err := os.MkdirAll(dryRun, 0755); err != nil {
					errsystem.New(errsystem.ErrCreateZipFile, err,
						errsystem.WithContextMessage(fmt.Sprintf("Error creating dry run directory '%s': %v", dryRun, err))).ShowErrorAndExit()
				}
			}

			outputFile := filepath.Join(dryRun, fmt.Sprintf("agentuity-deploy-%s.zip", theproject.ProjectId))

			if _, err := util.CopyFile(tmpfile.Name(), outputFile); err != nil {
				errsystem.New(errsystem.ErrCreateZipFile, err,
					errsystem.WithContextMessage("Error copying deployment zip file")).ShowErrorAndExit()
			}

			format, _ := cmd.Flags().GetString("format")
			if format == "json" {
				result := map[string]interface{}{
					"dry_run":    true,
					"zip_file":   outputFile,
					"project_id": theproject.ProjectId,
				}
				json.NewEncoder(os.Stdout).Encode(result)
			} else {
				tui.ShowSuccess("Deployment zip saved to: %s", outputFile)
			}
			return
		}

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

		// check to see if the organization is configured to use a public key for encryption
		if publicKey != "" {
			block, _ := pem.Decode([]byte(publicKey))
			if block == nil {
				errsystem.New(errsystem.ErrEncryptingDeploymentZipFile, err,
					errsystem.WithContextMessage("Error decoding the PEM formatted public key for encrypting the deployment zip file")).ShowErrorAndExit()
			}
			pub, err := x509.ParsePKIXPublicKey(block.Bytes)
			if err != nil {
				errsystem.New(errsystem.ErrEncryptingDeploymentZipFile, err,
					errsystem.WithContextMessage("Error parsing the PEM formatted public key for encrypting the deployment zip file")).ShowErrorAndExit()
			}
			pubKey := pub.(ed25519.PublicKey)
			if _, err := crypto.EncryptHybridKEMDEMStream(pubKey, dof, ef); err != nil {
				errsystem.New(errsystem.ErrEncryptingDeploymentZipFile, err,
					errsystem.WithContextMessage("Error encrypting deployment zip file (public key)")).ShowErrorAndExit()
			}
		} else {
			if err := crypto.EncryptStream(dof, ef, orgSecret); err != nil {
				errsystem.New(errsystem.ErrEncryptingDeploymentZipFile, err,
					errsystem.WithContextMessage("Error encrypting deployment zip file")).ShowErrorAndExit()
			}
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

		uploadAction := func() {
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
		}

		tui.ShowSpinner("Uploading ...", uploadAction)

		deployAction := func() {
			// tell the api that we've completed the upload for the deployment
			if err := updateDeploymentStatusCompleted(logger, apiUrl, token, startResponse.Data.DeploymentId, preview); err != nil {
				errsystem.New(errsystem.ErrApiRequest, err,
					errsystem.WithContextMessage("Error updating deployment status to completed")).ShowErrorAndExit()
			}
			if len(theproject.Agents) == 1 {
				if len(theproject.Agents[0].Types) > 0 {
					webhookToken, err = agent.GetApiKey(ctx, logger, apiUrl, token, theproject.Agents[0].ID, theproject.Agents[0].Types[0])
					if err != nil {
						errsystem.New(errsystem.ErrApiRequest, err,
							errsystem.WithContextMessage("Error getting Agent API key")).ShowErrorAndExit()
					}
				}
			}
		}

		tui.ShowSpinner("Deploying ...", deployAction)

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

func updateDeploymentStatusCompleted(logger logger.Logger, apiUrl, token, deploymentId string, preview bool) error {
	client := util.NewAPIClient(context.Background(), logger, apiUrl, token)
	payload := map[string]any{"state": "completed", "preview": preview}
	return client.Do("PUT", fmt.Sprintf("/cli/deploy/upload/%s", deploymentId), payload, nil)
}

var cloudRollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Rollback (undeploy) or delete a deployment from the cloud",
	Long: `Rollback (undeploy) or delete a specific deployment for a project by selecting a project and deployment.

Examples:
  agentuity rollback
  agentuity cloud rollback
  agentuity rollback --tag name
  agentuity rollback --delete
`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		apikey, _ := util.EnsureLoggedIn(ctx, logger, cmd)
		apiUrl, _, _ := util.GetURLs(logger)
		deleteFlag, _ := cmd.Flags().GetBool("delete")
		dir, _ := cmd.Flags().GetString("dir")

		var selectedProject string
		if dir != "" {
			proj := project.EnsureProject(ctx, cmd)
			if proj.Project == nil {
				errsystem.New(errsystem.ErrApiRequest, fmt.Errorf("project not found")).ShowErrorAndExit()
			}
			selectedProject = proj.Project.ProjectId
		} else {
			projectId, _ := cmd.Flags().GetString("project")
			if projectId != "" {
				// look up the project by id
				projects, err := project.ListProjects(ctx, logger, apiUrl, apikey)
				if err != nil {
					errsystem.New(errsystem.ErrApiRequest, err).ShowErrorAndExit()
				}
				for _, p := range projects {
					if p.ID == projectId {
						selectedProject = p.ID
						break
					}
				}
				if selectedProject == "" {
					// this will never happen because we've already checked the project id
					errsystem.New(errsystem.ErrApiRequest, fmt.Errorf("project not found")).ShowErrorAndExit()
				}
			}
		}

		question := "Select a project to rollback a deployment"
		if deleteFlag {
			question = "Select a project to delete a deployment"
		}

		if selectedProject == "" {
			selectedProject = cloudSelectProject(ctx, logger, apiUrl, apikey, question)
		}

		if selectedProject == "" {
			return
		}

		// Try to get tag flag
		tag, _ := cmd.Flags().GetString("tag")
		var selectedDeployment string
		if tag != "" {
			// List deployments and match by tag
			var deployments []project.DeploymentListData
			action := func() {
				var err error
				deployments, err = project.ListDeployments(ctx, logger, apiUrl, apikey, selectedProject)
				if err != nil {
					errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to list deployments")).ShowErrorAndExit()
				}
			}
			tui.ShowSpinner("fetching deployments ...", action)
			for _, d := range deployments {
				for _, t := range d.Tags {
					if t == tag {
						selectedDeployment = d.ID
						break
					}
				}
				if selectedDeployment != "" {
					break
				}
			}
			if selectedDeployment == "" {
				errsystem.New(errsystem.ErrApiRequest, fmt.Errorf("no deployment found with tag '%s'", tag)).ShowErrorAndExit()
			}
		} else {
			question = "Select a deployment to rollback"
			if deleteFlag {
				question = "Select a deployment to delete"
			}
			selectedDeployment = cloudSelectDeployment(ctx, logger, apiUrl, apikey, selectedProject, question)
			if selectedDeployment == "" {
				// this will never happen because we've already checked the deployment id
				errsystem.New(errsystem.ErrApiRequest, fmt.Errorf("no deployment selected")).ShowErrorAndExit()
			}
		}

		forceFlag, _ := cmd.Flags().GetBool("force")

		if !forceFlag {
			what := "rollback"
			if deleteFlag {
				what = "delete"
			}

			if !tui.Ask(logger, "Are you sure you want to "+tui.Bold(what)+" the selected deployment?", true) {
				fmt.Println()
				tui.ShowWarning("Canceled")
				return
			}
			fmt.Println()
		}

		if deleteFlag {
			err := project.DeleteDeployment(ctx, logger, apiUrl, apikey, selectedProject, selectedDeployment)
			if err != nil {
				errsystem.New(errsystem.ErrDeleteApiKey, err, errsystem.WithContextMessage("Failed to delete deployment")).ShowErrorAndExit()
			}
			tui.ShowSuccess("Deployment deleted successfully")
		} else {
			err := project.RollbackDeployment(ctx, logger, apiUrl, apikey, selectedProject, selectedDeployment)
			if err != nil {
				errsystem.New(errsystem.ErrDeployProject, err, errsystem.WithContextMessage("Failed to rollback deployment")).ShowErrorAndExit()
			}
			tui.ShowSuccess("Deployment rolled back successfully")
		}
	},
}

// Helper to fetch projects and prompt user to select one. Returns selected project ID or empty string.
func cloudSelectProject(ctx context.Context, logger logger.Logger, apiUrl, apikey string, prompt string) string {
	var projects []project.ProjectListData
	action := func() {
		var err error
		projects, err = project.ListProjects(ctx, logger, apiUrl, apikey)
		if err != nil {
			errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to list projects")).ShowErrorAndExit()
		}
	}
	tui.ShowSpinner("fetching projects ...", action)
	if len(projects) == 0 {
		fmt.Println()
		tui.ShowWarning("no projects found")
		tui.ShowBanner("Create a new project", tui.Text("Use the ")+tui.Command("new")+tui.Text(" command to create a new project"), false)
		return ""
	}
	var options []tui.Option
	for _, p := range projects {
		options = append(options, tui.Option{
			ID:   p.ID,
			Text: tui.Bold(tui.PadRight(p.Name, 20, " ")) + tui.Muted(p.ID),
		})
	}
	selected := tui.Select(logger, prompt, "", options)
	if selected == "" {
		tui.ShowWarning("no project selected")
	}
	return selected
}

func cloudSelectDeployment(ctx context.Context, logger logger.Logger, apiUrl, apikey, projectId string, prompt string) string {
	var deployments []project.DeploymentListData
	fetchDeploymentsAction := func() {
		var err error
		deployments, err = project.ListDeployments(ctx, logger, apiUrl, apikey, projectId)
		if err != nil {
			errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to list deployments")).ShowErrorAndExit()
		}
	}
	tui.ShowSpinner("fetching deployments ...", fetchDeploymentsAction)
	if len(deployments) == 0 {
		tui.ShowWarning("no deployments found for this project")
		os.Exit(1)
	}
	var deploymentOptions []tui.Option
	for _, d := range deployments {
		date, err := time.Parse(time.RFC3339, d.CreatedAt)
		if err != nil {
			errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to parse deployment date")).ShowErrorAndExit()
		}
		var msg string
		if len(d.Message) > 60 {
			msg = d.Message[:57] + "..."
		} else {
			msg = d.Message
		}
		tags := strings.Join(d.Tags, ", ")
		if len(tags) > 50 {
			tags = tags[:50] + "..."
		}

		if d.Active {
			deploymentOptions = append(deploymentOptions, tui.Option{
				ID:   d.ID,
				Text: fmt.Sprintf("%s  %s %-50s %s", "✅", tui.Title(date.Format(time.Stamp)), tui.Bold(tags), tui.Muted(msg)),
			})
		} else {
			deploymentOptions = append(deploymentOptions, tui.Option{
				ID:   d.ID,
				Text: fmt.Sprintf("    %s %-50s %s", tui.Title(date.Format(time.Stamp)), tui.Bold(tags), tui.Muted(msg)),
			})
		}
	}
	selectedDeployment := tui.Select(logger, prompt, "", deploymentOptions)
	return selectedDeployment
}

var cloudDeploymentsCmd = &cobra.Command{
	Use:   "deployments",
	Short: "List deployments for a project",
	Long: `List all deployments for a selected project, showing which is active and their tags.

Examples:
  agentuity cloud deployments
  agentuity cloud deployments --project <projectId>
`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		apikey, _ := util.EnsureLoggedIn(ctx, logger, cmd)
		apiUrl, _, _ := util.GetURLs(logger)
		projectId, _ := cmd.Flags().GetString("project")
		format, _ := cmd.Flags().GetString("format")

		var selectedProject string
		if projectId != "" {
			selectedProject = projectId
		} else {
			selectedProject = cloudSelectProject(ctx, logger, apiUrl, apikey, "Select a project to list deployments")
		}

		if selectedProject == "" {
			return
		}

		var deployments []project.DeploymentListData
		action := func() {
			var err error
			deployments, err = project.ListDeployments(ctx, logger, apiUrl, apikey, selectedProject)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to list deployments")).ShowErrorAndExit()
			}
		}
		tui.ShowSpinner("fetching deployments ...", action)

		if len(deployments) == 0 {
			tui.ShowWarning("no deployments found for this project")
			return
		}

		if format == "json" {
			json.NewEncoder(os.Stdout).Encode(deployments)
			return
		}

		headers := []string{"Active", "Deployment Id", "Tags", "Message", "Created At"}
		rows := [][]string{}
		for _, d := range deployments {
			active := ""
			if d.Active {
				active = "✅"
			}
			tags := strings.Join(d.Tags, ", ")
			msg := d.Message
			if len(msg) > 60 {
				msg = msg[:57] + "..."
			}
			created := d.CreatedAt
			rows = append(rows, []string{active, tui.Muted(d.ID), tui.Bold(tags), tui.Text(msg), tui.Title(created)})
		}
		tui.Table(headers, rows)
	},
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
	cloudDeployCmd.Flags().String("ci-logs-url", "", "Used to set the CI logs URL for your deployment metadata")
	cloudDeployCmd.Flags().StringArray("tag", nil, "Tag(s) to associate with this deployment (can be specified multiple times)")
	cloudDeployCmd.Flags().String("description", "", "Description for the deployment")
	cloudDeployCmd.Flags().String("message", "", "A shorter description for the deployment")
	cloudDeployCmd.Flags().Bool("force", false, "Force the processing of environment files")
	cloudDeployCmd.Flags().String("dry-run", "", "Save deployment zip file to specified directory (defaults to current directory) instead of uploading")

	cloudDeployCmd.Flags().MarkHidden("deploymentId")
	cloudDeployCmd.Flags().MarkHidden("ci")
	cloudDeployCmd.Flags().MarkHidden("ci-remote-url")
	cloudDeployCmd.Flags().MarkHidden("ci-branch")
	cloudDeployCmd.Flags().MarkHidden("ci-commit")
	cloudDeployCmd.Flags().MarkHidden("ci-message")
	cloudDeployCmd.Flags().MarkHidden("ci-git-provider")
	cloudDeployCmd.Flags().MarkHidden("ci-logs-url")

	cloudDeployCmd.Flags().String("format", "text", "The output format to use for results which can be either 'text' or 'json'")
	cloudDeployCmd.Flags().String("org-id", "", "The organization to create the project in")
	cloudDeployCmd.Flags().String("templates-dir", "", "The directory to load the templates. Defaults to loading them from the github.com/agentuity/templates repository")

	rootCmd.AddCommand(cloudRollbackCmd)
	cloudCmd.AddCommand(cloudRollbackCmd)
	cloudRollbackCmd.Flags().String("tag", "", "Tag of the deployment to rollback")
	cloudRollbackCmd.Flags().String("project", "", "Project to rollback a deployment")
	cloudRollbackCmd.Flags().String("dir", "", "The directory to the project to rollback if project is not specified")
	cloudRollbackCmd.Flags().Bool("force", false, "Force the rollback or delete")
	cloudRollbackCmd.Flags().Bool("delete", false, "Delete the deployment instead of rolling back")

	cloudCmd.AddCommand(cloudDeploymentsCmd)
	cloudDeploymentsCmd.Flags().String("project", "", "Project to list deployments for")
	cloudDeploymentsCmd.Flags().String("format", "text", "The output format to use for results which can be either 'text' or 'json'")
}
