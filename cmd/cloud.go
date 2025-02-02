package cmd

import (
	"os"
	"path/filepath"
	"time"

	"github.com/agentuity/cli/internal/ignore"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/provider"
	"github.com/agentuity/cli/internal/util"
	"github.com/spf13/cobra"
)

var cloudCmd = &cobra.Command{
	Use:   "cloud",
	Short: "Cloud related commands",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
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

		// TODO: request an upload token

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

		// zip up our directory
		started := time.Now()
		logger.Debug("creating a zip file of %s into %s", dir, tmpfile.Name())
		if err := util.ZipDir(dir, tmpfile.Name(), func(fn string, fi os.FileInfo) bool {
			notok := rules.Ignore(fn, fi)
			if notok {
				logger.Debug("❌ %s", fn)
			} else {
				logger.Debug("❎ %s", fn)
			}
			return !notok
		}); err != nil {
			logger.Fatal("error zipping project: %s", err)
		}
		logger.Debug("zip file created in %v", time.Since(started))

		// STEPS:
		// 1. Validate project
		// 2. Get a token for uploading
		// 3. Zip up the project
		// 4. Upload to cloud
		// 5. Hit the API with the upload details
	},
}

func init() {
	rootCmd.AddCommand(cloudCmd)
	cloudCmd.AddCommand(cloudDeployCmd)
	cloudDeployCmd.Flags().StringP("dir", "d", ".", "The directory to the project to deploy")
}
