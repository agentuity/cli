package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/agentuity/cli/internal/organization"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/provider"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	"github.com/charmbracelet/huh"
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

func initProject(logger logger.Logger, appUrl string, dir string, token string, orgId string, provider string, name string, description string) *project.ProjectData {
	result, err := project.InitProject(logger, appUrl, token, orgId, provider, name, description)
	if err != nil {
		logger.Fatal("failed to initialize project: %s", err)
	}
	project := project.NewProject()
	project.ProjectId = result.ProjectId
	project.Provider = provider
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

func promptForOrganization(logger logger.Logger, theme *huh.Theme, apiUrl string, token string) (string, error) {
	orgs, err := organization.ListOrganizations(logger, apiUrl, token)
	if err != nil {
		logger.Fatal("failed to list organizations: %s", err)
	}
	if len(orgs) == 0 {
		logger.Fatal("you are not a member of any organizations")
	}
	var orgId string
	if len(orgs) == 1 {
		orgId = orgs[0].OrgId
	} else {
		var opts []huh.Option[string]
		for _, org := range orgs {
			opts = append(opts, huh.NewOption(org.Name, org.OrgId))
		}
		if huh.NewSelect[string]().
			Title("What organization should we create the project in?").
			Options(opts...).
			Value(&orgId).WithTheme(theme).Run() != nil {
			logger.Fatal("failed to get organization")
		}
	}
	return orgId, nil
}

var projectNewCmd = &cobra.Command{
	Use:     "new",
	Short:   "Create a new project",
	Aliases: []string{"create"},
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		apikey := viper.GetString("auth.api_key")
		if apikey == "" {
			logger.Fatal("you are not logged in")
		}

		apiUrl := viper.GetString("overrides.api_url")
		initScreenWithLogo()

		cwd, err := os.Getwd()
		if err != nil {
			logger.Fatal("failed to get current directory: %s", err)
		}

		theme := huh.ThemeCatppuccin()

		orgId, err := promptForOrganization(logger, theme, apiUrl, apikey)
		if err != nil {
			logger.Fatal("failed to get organization: %s", err)
		}

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

		initProject(logger, apiUrl, projectDir, apikey, orgId, providerName, name, "")

		printSuccess("Project created successfully")
	},
}

var projectInitCmd = &cobra.Command{
	Use:   "init [directory]",
	Short: "Initialize a new project",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		dir := resolveDir(logger, args[0], false)

		token := viper.GetString("auth.api_key")
		if token == "" {
			logger.Fatal("you are not logged in")
		}
		apiUrl := viper.GetString("overrides.api_url")
		initScreenWithLogo()
		detection, err := provider.Detect(logger, dir)
		if err != nil {
			logger.Fatal("failed to detect project type: %s", err)
		}

		theme := huh.ThemeCatppuccin()
		orgId, err := promptForOrganization(logger, theme, apiUrl, token)
		if err != nil {
			logger.Fatal("failed to get organization: %s", err)
		}

		initProject(logger, apiUrl, dir, token, orgId, detection.Provider, detection.Name, detection.Description)

		logger.Info("Project initialized successfully")
	},
}

var projectEnvSetCmd = &cobra.Command{
	Use:   "env:set [directory]",
	Short: "Set environment variables for a project",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}
		dir = resolveDir(logger, dir, false)
		envFile := filepath.Join(dir, ".env")

		envLines, err := env.ParseEnvFile(envFile)
		if err != nil {
			logger.Fatal("failed to parse .env file: %s", err)
		}

		// Print local env vars to stdout
		for _, envLine := range envLines {
			fmt.Printf("%s=%s\n", envLine.Key, envLine.Val)
		}
	
		apiUrl := viper.GetString("overrides.api_url")
		apiKey := viper.GetString("auth.api_key")
		if apiKey == "" {
			logger.Fatal("you are not logged in")
		}

		project := project.NewProject()
		if err := project.Load(dir); err != nil {
			logger.Fatal("failed to load project: %s", err)
		}

		secretMap := make(map[string]interface{})
		for _, envLine := range envLines {
			secretMap[envLine.Key] = envLine.Val
		}
		
		project.SetProjectSecrets(logger, apiUrl, apiKey, secretMap)

		if err != nil {
			logger.Fatal("failed to set project env: %s", err)
		}

		printSuccess("Project environment variables set successfully")

	},
}

var projectEnvCmd = &cobra.Command{
	Use:   "env [directory]",
	Short: "Get environment variables for a project",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)

		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}
		dir = resolveDir(logger, dir, false)
		
		apiUrl := viper.GetString("overrides.api_url")
		apiKey := viper.GetString("auth.api_key")
		if apiKey == "" {
			logger.Fatal("you are not logged in")
		}

		project := project.NewProject()
		if err := project.Load(dir); err != nil {
			logger.Fatal("failed to load project: %s", err)
		}
		logger.Info("Getting environment variables for project %s", project.ProjectId)

		projectData, err := project.GetProjectEnv(logger, apiUrl, apiKey)
		if err != nil {
			logger.Fatal("failed to get project env: %s", err)
		}
		// Print remote env vars and secrets to stdout
		for key, value := range projectData.Env {
			fmt.Printf("%s=%s\n", key, value)
		}
		for key, value := range projectData.Secrets {
			fmt.Printf("%s=%s\n", key, value)
		}

	},
}

func init() {
	rootCmd.AddCommand(projectCmd)
	projectCmd.AddCommand(projectInitCmd)
	projectCmd.AddCommand(projectNewCmd)
	projectCmd.AddCommand(projectEnvCmd)
	projectCmd.AddCommand(projectEnvSetCmd)
}
