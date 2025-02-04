package cmd

import (
	"os"
	"path/filepath"

	"github.com/agentuity/cli/internal/env"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/provider"
	"github.com/agentuity/cli/internal/util"
	"github.com/charmbracelet/huh"
	"github.com/shopmonkeyus/go-common/logger"
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

func initProject(logger logger.Logger, appUrl string, dir string, provider string, name string, description string) *project.InitProjectResult {
	result, err := project.InitProject(logger, appUrl, provider, name, description)
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
	for i, envLine := range envLines {
		if envLine.Key == "AGENTUITY_API_KEY" {
			envLines[i].Val = result.APIKey
			found = true
		}
	}
	if !found {
		envLines = append(envLines, env.EnvLine{Key: "AGENTUITY_API_KEY", Val: result.APIKey})
	}
	if err := env.WriteEnvFile(filename, envLines); err != nil {
		logger.Fatal("failed to write .env file: %s", err)
	}
	return result
}

var projectNewCmd = &cobra.Command{
	Use:     "new",
	Short:   "Create a new project",
	Aliases: []string{"create"},
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		logger := newLogger(cmd)
		token := viper.GetString("auth.api_key")
		if token == "" {
			logger.Fatal("you are not logged in")
		}

		appUrl := viper.GetString("overrides.app_url")
		initScreenWithLogo()

		cwd, err := os.Getwd()
		if err != nil {
			logger.Fatal("failed to get current directory: %s", err)
		}

		theme := huh.ThemeCatppuccin()

		var name string

		if huh.NewInput().
			Title("What should we name the project?").
			Prompt("> ").
			CharLimit(255).
			Value(&name).WithTheme(theme).Run() != nil {
			logger.Fatal("failed to get project name")
		}

		projectDir := filepath.Join(cwd, name)
		if huh.NewInput().
			Title("What directory should the project be created in?").
			Prompt("> ").
			Placeholder(projectDir).
			Value(&projectDir).WithTheme(theme).Run() != nil {
			logger.Fatal("failed to get project directory")
		}

		providers := provider.GetProviders()
		var opts []huh.Option[string]

		for key, provider := range providers {
			opts = append(opts, huh.NewOption(provider.Name(), key))
		}

		var providerName string
		if huh.NewSelect[string]().
			Title("What framework should we use?").
			Options(opts...).
			Value(&providerName).WithTheme(theme).Run() != nil {
			logger.Fatal("failed to get project framework")
		}

		var confirm bool
		if huh.NewConfirm().
			Title("Create new project in "+projectDir+"?").
			Affirmative("Yes!").
			Negative("Cancel").
			Value(&confirm).WithTheme(theme).Run() != nil {
			logger.Fatal("failed to confirm project creation")
		}
		if !confirm {
			return
		}

		if util.Exists(projectDir) {
			if huh.NewConfirm().
				Title("The directory "+projectDir+" already exists.\nAre you sure you want to overwrite files here?").
				Affirmative("Yes!").
				Negative("Cancel").
				Value(&confirm).WithTheme(theme).Run() != nil {
				logger.Fatal("failed to confirm directory")
			}
			if !confirm {
				return
			}
		} else {
			if err := os.MkdirAll(projectDir, 0700); err != nil {
				logger.Fatal("failed to create project directory: %s", err)
			}
		}

		provider := providers[providerName]
		if err := provider.NewProject(logger, projectDir, name); err != nil {
			logger.Fatal("failed to create project: %s", err)
		}

		initProject(logger, appUrl, projectDir, providerName, name, "")

		printSuccess("Project created successfully")
	},
}

var projectInitCmd = &cobra.Command{
	Use:   "init [directory]",
	Short: "Initialize a new project",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		logger := newLogger(cmd)
		dir := resolveDir(logger, args[0], false)

		token := viper.GetString("auth.api_key")
		if token == "" {
			logger.Fatal("you are not logged in")
		}
		appUrl := viper.GetString("overrides.app_url")
		initScreenWithLogo()
		detection, err := provider.Detect(logger, dir)
		if err != nil {
			logger.Fatal("failed to detect project type: %s", err)
		}

		initProject(logger, appUrl, dir, detection.Provider, detection.Name, detection.Description)

		logger.Info("Project initialized successfully")
	},
}

func init() {
	rootCmd.AddCommand(projectCmd)
	projectCmd.AddCommand(projectInitCmd)
	projectCmd.AddCommand(projectNewCmd)
}
