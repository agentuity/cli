package cmd

import (
	"os"
	"path/filepath"

	"github.com/agentuity/cli/internal/env"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/project/autodetect"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Project related commands",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var projectInitCmd = &cobra.Command{
	Use:   "init [directory]",
	Short: "Initialize a new project",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		logger := newLogger(cmd)

		dir := args[0]
		if dir == "." {
			cwd, err := os.Getwd()
			if err != nil {
				logger.Fatal("failed to get current directory: %s", err)
			}
			dir = cwd
		}

		if _, err := os.Stat(dir); os.IsNotExist(err) {
			logger.Fatal("directory does not exist: %s", dir)
		}

		token := viper.GetString("auth.token")
		if token == "" {
			logger.Fatal("you are not logged in")
		}
		appUrl := viper.GetString("overrides.app_url")
		initScreenWithLogo()
		projectType, err := autodetect.Detect(logger, dir)
		if err != nil {
			logger.Fatal("failed to detect project type: %s", err)
		}
		logger.Debug("Detected project type: %s", projectType)
		result, err := project.InitProject(logger, appUrl, projectType)
		if err != nil {
			logger.Fatal("failed to initialize project: %s", err)
		}
		project := project.NewProject()
		project.ProjectId = result.ProjectId
		project.Provider = result.Provider
		if err := project.Save(dir); err != nil {
			logger.Fatal("failed to save project: %s", err)
		}
		filename := filepath.Join(dir, ".env")
		envLines, err := env.ParseEnvFile(filename)
		if err != nil {
			logger.Fatal("failed to parse .env file: %s", err)
		}
		var found bool
		for _, envLine := range envLines {
			if envLine.Key == "AGENTUITY_API_KEY" {
				envLine.Val = result.APIKey
				found = true
			}
		}
		if !found {
			envLines = append(envLines, env.EnvLine{Key: "AGENTUITY_API_KEY", Val: result.APIKey})
		}
		if err := env.WriteEnvFile(filename, envLines); err != nil {
			logger.Fatal("failed to write .env file: %s", err)
		}
		logger.Info("Project initialized successfully: %s", result.APIKey)
	},
}

func init() {
	rootCmd.AddCommand(projectCmd)
	projectCmd.AddCommand(projectInitCmd)
}
