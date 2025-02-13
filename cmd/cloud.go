package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/agentuity/cli/internal/ignore"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/provider"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var cloudCmd = &cobra.Command{
	Use:   "cloud",
	Short: "Cloud related commands",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

type startResponse struct {
	Success bool `json:"success"`
	Data    struct {
		DeploymentId string `json:"deploymentId"`
		Url          string `json:"url"`
	}
	Message *string `json:"message,omitempty"`
}

type projectResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Id    string `json:"id"`
		OrgId string `json:"orgId"`
		Name  string `json:"name"`
	}
	Message *string `json:"message,omitempty"`
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
	apiUrl := viper.GetString("overrides.api_url")
	appUrl := viper.GetString("overrides.app_url")
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
		context := ensureProject(cmd)
		logger := context.Logger
		theproject := context.Project
		dir := context.Dir
		apiUrl := context.APIURL
		appUrl := context.APPURL
		token := context.Token

		deploymentConfig := project.NewDeploymentConfig()
		deploymentConfig.Provider = theproject.Provider

		p, err := provider.GetProviderForName(theproject.Provider)
		if err != nil {
			logger.Fatal("%s", err)
		}

		client := util.NewAPIClient(apiUrl, token)
		var le []env.EnvLine
		var envFile *provider.EnvFile
		var projectData *project.ProjectData

		action := func() {
			var err error
			projectData, err = theproject.ListProjectEnv(logger, apiUrl, token)
			if err != nil {
				logger.Fatal("error listing project env: %s", err)
			}
		}
		showSpinner(logger, "", action)

		// check to see if we have any env vars that are not in the project
		envfilename := filepath.Join(dir, ".env")
		if util.Exists(envfilename) {

			le, err = env.ParseEnvFile(envfilename)
			if err != nil {
				logger.Fatal("error parsing env file: %s. %s", envfilename, err)
			}
			envFile = &provider.EnvFile{Filepath: envfilename, Env: le}

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
				if !ask(logger, title, true) {
					printWarning("cancelled")
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
					printSuccess("Environment variables and secrets added")
				case len(envs) == 1:
					printSuccess("Environment variable added")
				case len(envs) > 1:
					printSuccess("Environment variables added")
				case len(secrets) == 1:
					printSuccess("Secret added")
				case len(secrets) > 1:
					printSuccess("Secrets added")
				}
			}
		}

		deploymentConfig.Deployment = theproject.Deployment

		// allow the provider to perform any preflight checks
		if err := p.DeployPreflightCheck(logger, provider.DeployPreflightCheckData{
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
			logger.Fatal("error performing pre-flight check: %s", err)
		}

		// have the provider set any specific deployment configuration
		if err := p.ConfigureDeploymentConfig(deploymentConfig); err != nil {
			logger.Fatal("error configuring deployment config: %s", err)
		}

		cleanup, err := deploymentConfig.Write(dir)
		if err != nil {
			logger.Fatal("error writing deployment config: %s", err)
		}
		defer cleanup()

		// Get project details
		var projectResponse projectResponse
		if err := client.Do("GET", fmt.Sprintf("/cli/project/%s", theproject.ProjectId), nil, &projectResponse); err != nil {
			logger.Fatal("error requesting project: %s", err)
		}
		orgId := projectResponse.Data.OrgId

		// Start deployment
		var startResponse startResponse
		if err := client.Do("PUT", fmt.Sprintf("/cli/deploy/start/%s/%s", orgId, theproject.ProjectId), nil, &startResponse); err != nil {
			logger.Fatal("error starting deployment: %s", err)
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
		for _, rule := range p.ProjectIgnoreRules() {
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

		showSpinner(logger, "Packaging ...", zipaction)

		of, err := os.Open(tmpfile.Name())
		if err != nil {
			logger.Fatal("error opening deloyment zip file: %s", err)
		}
		defer of.Close()

		fi, _ := os.Stat(tmpfile.Name())
		started := time.Now()
		var webhookToken string

		action = func() {
			// send the zip file to the upload endpoint provided
			req, err := http.NewRequest("PUT", startResponse.Data.Url, of)
			if err != nil {
				logger.Fatal("error creating PUT request", err)
			}
			req.ContentLength = fi.Size()
			req.Header.Set("Content-Type", "application/zip")
			req.Header.Set("Content-Length", strconv.FormatInt(fi.Size(), 10))

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				if err := updateDeploymentStatus(apiUrl, token, startResponse.Data.DeploymentId, "failed"); err != nil {
					logger.Fatal("%s", err)
				}
				logger.Fatal("error uploading deployment: %s", err)
			}
			if resp.StatusCode != http.StatusOK {
				buf, _ := io.ReadAll(resp.Body)
				if err := updateDeploymentStatus(apiUrl, token, startResponse.Data.DeploymentId, "failed"); err != nil {
					logger.Fatal("%s", err)
				}
				logger.Fatal("error uploading deployment (%s) %s", resp.Status, string(buf))
			}
			resp.Body.Close()
			logger.Debug("deployment uploaded %d bytes in %v", fi.Size(), time.Since(started))

			sources := []string{}
			destinations := []string{}
			var hasWebhook bool
			for _, io := range theproject.Inputs {
				if io.Type == "webhook" && hasWebhook {
					printWarning("Multiple source webhooks found. Only the first one will be used.")
					continue
				}
				sources = append(sources, io.ID)
				if io.Type == "webhook" {
					hasWebhook = true
				}
			}
			for _, io := range theproject.Outputs {
				destinations = append(destinations, io.ID)
			}
			// tell the api that we've completed the upload for the deployment
			if err := updateDeploymentStatusCompleted(apiUrl, token, startResponse.Data.DeploymentId, sources, destinations); err != nil {
				logger.Fatal("%s", err)
			}
			res, err := theproject.ListIO(logger, apiUrl, token, "source")
			if err != nil {
				logger.Fatal("error listing sources: %s", err)
			}
			for _, io := range res {
				if io.Type == "webhook" && io.Config != nil {
					if auth, ok := io.Config["authorization"].(map[string]any); ok {
						if val, ok := auth["token"].(string); ok {
							webhookToken = val
						}
					}
				}
			}
		}

		showSpinner(logger, "Deploying ...", action)

		printSuccess("Your project was deployed successfully. It is available at %s", link("%s/projects/%s?deploymentId=%s", appUrl, theproject.ProjectId, startResponse.Data.DeploymentId))

		if len(theproject.Inputs) > 0 {
			var found bool
			for _, io := range theproject.Inputs {
				if io.Type == "webhook" {
					found = true
					break
				}
			}
			if found {
				if webhookToken != "" {
					printSuccess("You can send a POST to %s with the following Bearer token: %s", link("%s/project/%s/run", apiUrl, theproject.ProjectId), color.BlackString(webhookToken))
				} else {
					printSuccess("You can send a POST to %s", link("%s/project/%s/run", apiUrl, theproject.ProjectId))
				}
			}
		}
	},
}

func updateDeploymentStatus(apiUrl, token, deploymentId, status string) error {
	client := util.NewAPIClient(apiUrl, token)
	payload := map[string]string{"state": status}
	return client.Do("PUT", fmt.Sprintf("/cli/deploy/upload/%s", deploymentId), payload, nil)
}

func updateDeploymentStatusCompleted(apiUrl, token, deploymentId string, sources []string, destinations []string) error {
	client := util.NewAPIClient(apiUrl, token)
	payload := map[string]any{"state": "completed", "sources": sources, "destinations": destinations}
	return client.Do("PUT", fmt.Sprintf("/cli/deploy/upload/%s", deploymentId), payload, nil)
}

func init() {
	rootCmd.AddCommand(cloudCmd)
	cloudCmd.AddCommand(cloudDeployCmd)
	cloudDeployCmd.Flags().StringP("dir", "d", ".", "The directory to the project to deploy")
	addURLFlags(cloudCmd)
}
