package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/agentuity/cli/internal/organization"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/provider"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	"github.com/charmbracelet/huh"
	"github.com/fatih/color"
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

type InitProjectArgs struct {
	BaseURL           string
	Dir               string
	Token             string
	OrgId             string
	Provider          string
	Name              string
	Description       string
	EnableWebhookAuth bool
}

func initProject(logger logger.Logger, args InitProjectArgs) *project.ProjectData {

	result, err := project.InitProject(logger, project.InitProjectArgs{
		BaseURL:           args.BaseURL,
		Token:             args.Token,
		OrgId:             args.OrgId,
		Provider:          args.Provider,
		Name:              args.Name,
		Description:       args.Description,
		EnableWebhookAuth: args.EnableWebhookAuth,
		Dir:               args.Dir,
	})
	if err != nil {
		logger.Fatal("failed to initialize project: %s", err)
	}
	proj := project.NewProject()
	proj.ProjectId = result.ProjectId
	proj.Provider = args.Provider

	proj.Inputs = []project.IO{
		{
			Type: "webhook",
			ID:   result.IOId,
		},
	}

	if err := proj.Save(args.Dir); err != nil {
		logger.Fatal("failed to save project: %s", err)
	}
	filename := filepath.Join(args.Dir, ".env")
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

func promptForWebhookAuth(logger logger.Logger) bool {
	return ask(logger, "Do you want to secure the agent webhook with a Bearer Token?", true)
}

func promptForOrganization(logger logger.Logger, apiUrl string, token string) (string, error) {
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

var projectNameTransformer = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
var invalidProjectNames = []any{
	"test",
	"agent",
	"agentuity",
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
		appUrl := viper.GetString("overrides.app_url")
		initScreenWithLogo()

		cwd, err := os.Getwd()
		if err != nil {
			logger.Fatal("failed to get current directory: %s", err)
		}

		theme := huh.ThemeCatppuccin()

		orgId, err := promptForOrganization(logger, apiUrl, apikey)
		if err != nil {
			logger.Fatal("failed to get organization: %s", err)
		}

		var name string

		if huh.NewInput().
			Title("What should we name the project?").
			Prompt("> ").
			CharLimit(255).Validate(func(name string) error {
			for _, invalid := range invalidProjectNames {
				if s, ok := invalid.(string); ok {
					if name == s {
						return fmt.Errorf("%s is not a valid project name", name)
					}
				}
				if r, ok := invalid.(regexp.Regexp); ok {
					if r.MatchString(name) {
						return fmt.Errorf("%s is not a valid project name", name)
					}
				}
			}
			return nil
		}).Value(&name).WithTheme(theme).Run() != nil {
			logger.Fatal("failed to get project name")
		}

		projectDir := filepath.Join(cwd, projectNameTransformer.ReplaceAllString(name, "_"))
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

		sort.Slice(opts, func(i, j int) bool {
			return opts[i].Value < opts[j].Value
		})

		var providerName string
		if huh.NewSelect[string]().
			Title("What framework should we use?").
			Options(opts...).
			Value(&providerName).WithTheme(theme).Run() != nil {
			logger.Fatal("failed to get project framework")
		}

		enableWebhookAuth := promptForWebhookAuth(logger)

		if !ask(logger, "Create new project in "+projectDir+"?", true) {
			return
		}

		if util.Exists(projectDir) {
			if !ask(logger, "The directory "+projectDir+" already exists.\nAre you sure you want to overwrite files here?", true) {
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

		projectData := initProject(logger, InitProjectArgs{
			BaseURL:           apiUrl,
			Dir:               projectDir,
			Token:             apikey,
			OrgId:             orgId,
			Provider:          providerName,
			Name:              name,
			EnableWebhookAuth: enableWebhookAuth,
		})

		fmt.Println()
		printSuccess("Project created successfully")

		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println()
		fmt.Printf("1. Switch into the project directory at %s\n", color.GreenString(projectDir))
		fmt.Printf("2. Run %s to run the project locally\n", command("dev", "run"))
		fmt.Printf("3. Run %s to deploy the project\n", command("cloud", "deploy"))
		fmt.Println()
		fmt.Printf("Access your project at %s", link("%s/projects/%s", appUrl, projectData.ProjectId))
		fmt.Println()

		if projectData.IOAuthToken != "" {
			fmt.Println()
			printLock("Your agent webhook is secured with a Bearer Token: %s", color.BlackString(projectData.IOAuthToken))
			fmt.Println()
		}

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

		orgId, err := promptForOrganization(logger, apiUrl, token)
		if err != nil {
			logger.Fatal("failed to get organization: %s", err)
		}

		enableWebhookAuth := promptForWebhookAuth(logger)

		p := initProject(logger, InitProjectArgs{
			BaseURL:           apiUrl,
			Dir:               dir,
			Token:             token,
			OrgId:             orgId,
			Provider:          detection.Provider,
			Name:              detection.Name,
			Description:       detection.Description,
			EnableWebhookAuth: enableWebhookAuth,
		})

		if p.IOAuthToken != "" {
			printLock("Your agent webhook is secured with a Bearer Token: %s", color.BlackString(p.IOAuthToken))
		}

		printSuccess("Project initialized successfully")
	},
}

func init() {
	rootCmd.AddCommand(projectCmd)
	projectCmd.AddCommand(projectInitCmd)
	projectCmd.AddCommand(projectNewCmd)
}
