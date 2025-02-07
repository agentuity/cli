package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/agentuity/cli/internal/ignore"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/provider"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
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

var cloudDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy project to the cloud",
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		dir := resolveProjectDir(logger, cmd)

		deploymentConfig := project.NewDeploymentConfig()

		// validate our project
		project := project.NewProject()
		if err := project.Load(dir); err != nil {
			logger.Fatal("error loading project: %s", err)
		}

		deploymentConfig.Provider = project.Provider

		p, err := provider.GetProviderForName(project.Provider)
		if err != nil {
			logger.Fatal("%s", err)
		}

		if err := p.ConfigureDeploymentConfig(deploymentConfig); err != nil {
			logger.Fatal("error configuring deployment config: %s", err)
		}

		if err := deploymentConfig.Write(dir); err != nil {
			logger.Fatal("error writing deployment config: %s", err)
		}

		apiUrl := viper.GetString("overrides.api_url")
		appUrl := viper.GetString("overrides.app_url")
		token := viper.GetString("auth.api_key")

		client := util.NewAPIClient(apiUrl, token)

		// Get project details
		var projectResponse projectResponse
		if err := client.Do("GET", fmt.Sprintf("/cli/project/%s", project.ProjectId), nil, &projectResponse); err != nil {
			logger.Fatal("error requesting project: %s", err)
		}
		orgId := projectResponse.Data.OrgId

		// Start deployment
		var startResponse startResponse
		if err := client.Do("PUT", fmt.Sprintf("/cli/deploy/start/%s/%s", orgId, project.ProjectId), nil, &startResponse); err != nil {
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
				logger.Fatal("error adding rule: %s. %s", rule, err)
			}
		}

		// create a temp file we're going to use for zip and upload
		tmpfile, err := os.CreateTemp("", "agentuity-deploy-*.zip")
		if err != nil {
			logger.Fatal("error creating temp file: %s", err)
		}
		defer os.Remove(tmpfile.Name())
		tmpfile.Close()

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

		of, err := os.Open(tmpfile.Name())
		if err != nil {
			logger.Fatal("error opening deloyment zip file: %s", err)
		}
		defer of.Close()

		fi, _ := os.Stat(tmpfile.Name())
		started = time.Now()

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

		// tell the api that we've completed the upload for the deployment
		if err := updateDeploymentStatus(apiUrl, token, startResponse.Data.DeploymentId, "completed"); err != nil {
			logger.Fatal("%s", err)
		}

		logger.Info("Your deployment is available at %s/projects/%s?deploymentId=%s", appUrl, project.ProjectId, startResponse.Data.DeploymentId)
	},
}

func updateDeploymentStatus(apiUrl, token, deploymentId, status string) error {
	client := util.NewAPIClient(apiUrl, token)
	payload := map[string]string{"state": status}
	return client.Do("PUT", fmt.Sprintf("/cli/deploy/upload/%s", deploymentId), payload, nil)
}

func init() {
	rootCmd.AddCommand(cloudCmd)
	cloudCmd.AddCommand(cloudDeployCmd)
	cloudDeployCmd.Flags().StringP("dir", "d", ".", "The directory to the project to deploy")
	addURLFlags(cloudCmd)
}
