package cmd

import (
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/agentuity/cli/internal/ignore"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/provider"
	"github.com/agentuity/cli/internal/util"
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

var cloudDeployCmd = &cobra.Command{
	Use:   "deploy",
	Short: "Deploy project to the cloud",
	Run: func(cmd *cobra.Command, args []string) {
		logger := newLogger(cmd)
		dir := resolveProjectDir(logger, cmd)

		// validate our project
		project := project.NewProject()
		if err := project.Load(dir); err != nil {
			logger.Fatal("error loading project: %s", err)
		}

		p, err := provider.GetProviderForName(project.Provider)
		if err != nil {
			logger.Fatal("%s", err)
		}

		apiUrl := viper.GetString("overrides.api_url")
		appUrl := viper.GetString("overrides.app_url")
		token := viper.GetString("auth.token")

		u, err := url.Parse(apiUrl)
		if err != nil {
			logger.Fatal("error parsing api url: %s. %s", apiUrl, err)
		}
		u.Path = "/cli/deploy/start"

		// start the deployment request to get a one-time upload url
		req, err := http.NewRequest("PUT", u.String(), nil)
		if err != nil {
			logger.Fatal("error creating url route: %s", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			logger.Fatal("error creating start request for upload: %s", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			logger.Fatal("unexpected error uploading (%s)", resp.Status)
		}
		enc := json.NewDecoder(resp.Body)
		var startResponse startResponse
		if err := enc.Decode(&startResponse); err != nil {
			logger.Fatal("error decoding start response json: %s", err)
		}
		resp.Body.Close()
		if !startResponse.Success {
			logger.Fatal("error generating start authentication: %s", startResponse.Message)
		}
		logger.Debug("upload api is %s", startResponse.Data.Url)
		logger.Debug("deployment id is %s", startResponse.Data.DeploymentId)

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
		req, err = http.NewRequest("PUT", startResponse.Data.Url, of)
		if err != nil {
			logger.Fatal("error creating PUT request", err)
		}
		req.ContentLength = fi.Size()
		req.Header.Set("Content-Type", "application/zip")
		req.Header.Set("Content-Length", strconv.FormatInt(fi.Size(), 10))

		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			logger.Fatal("error uploading deployment: %s", err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			logger.Fatal("error uploading deployment (%s)", resp.Status)
		}
		logger.Debug("deployment uploaded %d bytes in %v", fi.Size(), time.Since(started))

		// tell the api that we've completed the upload for the deployment
		u.Path = "/cli/deploy/upload/" + startResponse.Data.DeploymentId
		req, err = http.NewRequest("PUT", u.String(), nil)
		if err != nil {
			logger.Fatal("error creating upload deployment success request: %s", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			logger.Fatal("error sending upload deployment success request: %s", err)
		}

		if resp.StatusCode != http.StatusAccepted {
			logger.Fatal("error sending deployment success (%s)", resp.Status)
		}
		resp.Body.Close()

		logger.Info("Your deployment is available at %s/deployment/%s", appUrl, startResponse.Data.DeploymentId)
	},
}

func init() {
	rootCmd.AddCommand(cloudCmd)
	cloudCmd.AddCommand(cloudDeployCmd)
	cloudDeployCmd.Flags().StringP("dir", "d", ".", "The directory to the project to deploy")
	addURLFlags(cloudCmd)
}
