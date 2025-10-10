package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/agentuity/cli/internal/deployer"
	"github.com/agentuity/cli/internal/envutil"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/mcp"
	"github.com/agentuity/cli/internal/organization"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/templates"
	"github.com/agentuity/cli/internal/ui"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	cproject "github.com/agentuity/go-common/project"
	"github.com/agentuity/go-common/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Project related commands",
	Long: `Project related commands for creating, listing, and managing projects.

Use the subcommands to manage your projects.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// detectRuntime auto-detects the runtime based on lockfiles in the directory
// If no lockfile is found, falls back to the default runtime from the template
func detectRuntime(dir string, defaultRuntime string) string {
	// Only auto-detect for JavaScript-based runtimes
	if defaultRuntime != "nodejs" && defaultRuntime != "bunjs" {
		return defaultRuntime
	}

	// Check for bun lockfiles - use bunjs runtime
	if util.Exists(filepath.Join(dir, "bun.lockb")) ||
		util.Exists(filepath.Join(dir, "bun.lock")) {
		return "bunjs"
	}

	// Check for nodejs runtime
	if util.Exists(filepath.Join(dir, "pnpm-lock.yaml")) ||
		util.Exists(filepath.Join(dir, "package-lock.json")) ||
		util.Exists(filepath.Join(dir, "yarn.lock")) {
		return "nodejs"
	}

	// No lockfile found, use the template default
	return defaultRuntime
}

func saveEnv(dir string, apikey string, projectKey string) {
	filename := filepath.Join(dir, ".env")
	envLines, err := env.ParseEnvFile(filename)
	if err != nil {
		errsystem.New(errsystem.ErrReadConfigurationFile, err, errsystem.WithContextMessage("Failed to parse .env file")).ShowErrorAndExit()
	}
	found := map[string]bool{
		"AGENTUITY_SDK_KEY":     false,
		"AGENTUITY_API_KEY":     false,
		"AGENTUITY_PROJECT_KEY": false,
	}

	for i, envLine := range envLines {
		if envLine.Key == "AGENTUITY_API_KEY" {
			envLines[i].Val = apikey
			found["AGENTUITY_API_KEY"] = true
		}
		if envLine.Key == "AGENTUITY_SDK_KEY" {
			envLines[i].Val = apikey
			found[envLine.Key] = true
		}
		if envLine.Key == "AGENTUITY_PROJECT_KEY" {
			envLines[i].Val = projectKey
			found[envLine.Key] = true
		}
	}

	if found["AGENTUITY_API_KEY"] {
		// Remove AGENTUITY_API_KEY since it's deprecated in newer SDK versions
		for i := len(envLines) - 1; i >= 0; i-- {
			if envLines[i].Key == "AGENTUITY_API_KEY" {
				envLines = append(envLines[:i], envLines[i+1:]...)
			}
		}
		found["AGENTUITY_API_KEY"] = false
	}

	if !found["AGENTUITY_SDK_KEY"] {
		envLines = append(envLines, env.EnvLine{Key: "AGENTUITY_SDK_KEY", Val: apikey})
	}
	if !found["AGENTUITY_PROJECT_KEY"] {
		envLines = append(envLines, env.EnvLine{Key: "AGENTUITY_PROJECT_KEY", Val: projectKey})
	}

	if err := env.WriteEnvFile(filename, envLines); err != nil {
		errsystem.New(errsystem.ErrWriteConfigurationFile, err, errsystem.WithContextMessage("Failed to write .env file")).ShowErrorAndExit()
	}
}

type InitProjectArgs struct {
	BaseURL           string
	Dir               string
	Token             string
	OrgId             string
	Name              string
	Description       string
	EnableWebhookAuth bool
	AuthType          string
	Provider          *templates.TemplateRules
	Agents            []cproject.AgentConfig
	Framework         string
}

func initProject(ctx context.Context, logger logger.Logger, args InitProjectArgs) *project.ProjectData {

	result, err := project.InitProject(ctx, logger, project.InitProjectArgs{
		BaseURL:           args.BaseURL,
		Token:             args.Token,
		OrgId:             args.OrgId,
		Name:              args.Name,
		Description:       args.Description,
		EnableWebhookAuth: args.EnableWebhookAuth,
		AuthType:          args.AuthType,
		Dir:               args.Dir,
		Provider:          args.Provider.Identifier,
		Agents:            args.Agents,
		Framework:         args.Framework,
	})
	if err != nil {
		errsystem.New(errsystem.ErrCreateProject, err, errsystem.WithContextMessage("Failed to init project")).ShowErrorAndExit()
	}

	proj := project.NewProject()
	proj.ProjectId = result.ProjectId
	proj.Name = args.Name
	proj.Description = args.Description

	proj.Development = &cproject.Development{
		Port: args.Provider.Development.Port,
		Watch: cproject.Watch{
			Enabled: args.Provider.Development.Watch.Enabled,
			Files:   args.Provider.Development.Watch.Files,
		},
		Command: args.Provider.Development.Command,
		Args:    args.Provider.Development.Args,
	}

	proj.Bundler = &cproject.Bundler{
		Enabled:    args.Provider.Bundle.Enabled,
		Identifier: args.Provider.Identifier,
		Language:   args.Provider.Language,
		Framework:  args.Provider.Framework,
		Runtime:    detectRuntime(args.Dir, args.Provider.Runtime),
		Ignore:     args.Provider.Bundle.Ignore,
		AgentConfig: cproject.AgentBundlerConfig{
			Dir: args.Provider.SrcDir,
		},
	}

	// copy over the deployment command and args from the template
	proj.Deployment.Command = args.Provider.Deployment.Command
	proj.Deployment.Args = args.Provider.Deployment.Args
	proj.Deployment.Resources.CPU = args.Provider.Deployment.Resources.CPU
	proj.Deployment.Resources.Memory = args.Provider.Deployment.Resources.Memory
	proj.Deployment.Resources.Disk = args.Provider.Deployment.Resources.Disk
	proj.Deployment.Mode = &cproject.Mode{
		Type: "on-demand",
	}

	// set the agents from the result
	proj.Agents = result.Agents

	if err := proj.Save(args.Dir); err != nil {
		errsystem.New(errsystem.ErrSaveProject, err, errsystem.WithContextMessage("Failed to save project to disk")).ShowErrorAndExit()
	}

	saveEnv(args.Dir, result.APIKey, result.ProjectKey)

	return result
}

func promptForProjectDetail(ctx context.Context, logger logger.Logger, apiUrl, apikey string, name string, description string, orgId string) (string, string) {
	var nameOK bool
	if name != "" {
		if exists, err := project.ProjectWithNameExists(ctx, logger, apiUrl, apikey, orgId, name); err != nil {
			errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to check if project name exists")).ShowErrorAndExit()
		} else if exists {
			tui.ShowWarning("project %s already exists in this organization. please choose another name", name)
		} else {
			nameOK = true
		}
	}
	if !nameOK {
		name = tui.InputWithValidation(logger, "What should we name the project?", "The name of the project must be unique within the organization", 255, func(name string) error {
			for _, invalid := range invalidProjectNames {
				if s, ok := invalid.(string); ok {
					if name == s {
						return fmt.Errorf("%s is not a valid project name", name)
					}
				}
			}
			if exists, err := project.ProjectWithNameExists(ctx, logger, apiUrl, apikey, orgId, name); err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to check if project name exists")).ShowErrorAndExit()
			} else if exists {
				return fmt.Errorf("project %s already exists in this organization. please choose another name", name)
			}
			return nil
		})
	}

	if description == "" {
		description = tui.Input(logger, "How should we describe what the "+name+" project does?", "The description of the project is optional but helpful")
	}

	return name, description
}

func promptForOrganization(ctx context.Context, logger logger.Logger, cmd *cobra.Command, apiUrl string, token string) string {
	orgs, err := organization.ListOrganizations(ctx, logger, apiUrl, token)
	if err != nil {
		errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to list organizations")).ShowErrorAndExit()
	}
	if len(orgs) == 0 {
		logger.Fatal("you are not a member of any organizations")
		errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithUserMessage("You are not a member of any organizations")).ShowErrorAndExit()
	}
	var orgId string
	if len(orgs) == 1 {
		orgId = orgs[0].OrgId
	} else {
		hasCLIFlag := cmd.Flags().Changed("org-id")
		prefOrgId, _ := cmd.Flags().GetString("org-id")
		if prefOrgId == "" {
			prefOrgId = viper.GetString("preferences.orgId")
		}
		if tui.HasTTY && !hasCLIFlag {
			var opts []tui.Option
			for _, org := range orgs {
				opts = append(opts, tui.Option{ID: org.OrgId, Text: org.Name, Selected: prefOrgId == org.OrgId})
			}
			orgId = tui.Select(logger, "What organization should we create the project in?", "", opts)
			viper.Set("preferences.orgId", orgId)
			viper.WriteConfig() // remember the preference
		} else {
			for _, org := range orgs {
				if org.OrgId == prefOrgId || org.Name == prefOrgId {
					return org.OrgId
				}
			}
			logger.Fatal("no TTY and no organization preference found. re-run with --org-id")
		}
	}
	return orgId
}

var invalidProjectNames = []any{
	"test",
	"agent",
	"agentuity",
}

func gitCommand(ctx context.Context, projectDir string, git string, args ...string) error {
	c := exec.CommandContext(ctx, git, args...)
	util.ProcessSetup(c)
	c.Dir = projectDir
	return c.Run()
}

func projectGitFlow(ctx context.Context, provider *templates.Template, tmplContext templates.TemplateContext, githubAction string) {
	git, err := exec.LookPath("git")
	if err != nil {
		return
	}
	switch githubAction {
	case "none":
	case "github-action":
		if err := provider.AddGitHubAction(tmplContext); err != nil {
			errsystem.New(errsystem.ErrAddingGithubActionWorkflowProject, err, errsystem.WithContextMessage("Failed to add GitHub Action Workflow to the project")).ShowErrorAndExit()
		}
		body := tui.Paragraph(
			tui.Secondary("✓ Added GitHub Action Workflow to the project."),
			tui.Secondary("Access the Project API Key from the dashboard in and set it as a secret"),
			tui.Secondary("named ")+tui.Warning("AGENTUITY_API_KEY")+tui.Secondary(" in your GitHub repository."),
		)
		tui.ShowBanner("GitHub Action", body, false)
	case "github-app":
		body := tui.Paragraph(
			tui.Secondary("After pushing your code to GitHub, visit the dashboard to connect"),
			tui.Secondary("your repository to the GitHub App for automatic deployments."),
		)
		tui.ShowBanner("GitHub App", body, false)
	}

	gitCommand(ctx, tmplContext.ProjectDir, git, "init")
	gitCommand(ctx, tmplContext.ProjectDir, git, "add", ".")
	gitCommand(ctx, tmplContext.ProjectDir, git, "commit", "-m", "[chore] Initial commit 🤖")
	gitCommand(ctx, tmplContext.ProjectDir, git, "branch", "-m", "main")
}

var projectNewCmd = &cobra.Command{
	Use:   "create [name] [description] [agent-name] [agent-description]",
	Short: "Create a new project",
	Long: `Create a new project with the specified name, description, and initial agent.

Arguments:
  [name]                The name of the project (must be unique within the organization)
  [description]         A description of what the project does
  [agent-name]          The name of the initial agent
  [agent-description]   A description of what the agent does

Examples:
  agentuity project create "My Project" "Project description" "My Agent" "Agent description" --auth bearer
  agentuity create --runtime nodejs --template "OpenAI SDK for Typescript"`,
	Aliases: []string{"new"},
	Args:    cobra.MaximumNArgs(4),
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		logger := env.NewLogger(cmd)
		apikey, _ := util.EnsureLoggedIn(ctx, logger, cmd)
		urls := util.GetURLs(logger)
		apiUrl := urls.API
		appUrl := urls.App

		initScreenWithLogo()

		cwd, err := os.Getwd()
		if err != nil {
			errsystem.New(errsystem.ErrListFilesAndDirectories, err, errsystem.WithContextMessage("Failed to get current working directory")).ShowErrorAndExit()
		}

		checkForUpgrade(ctx, logger, true)

		// Railgurd the user from creating a project in an existing project directory
		if cproject.ProjectExists(cwd) {
			if tui.HasTTY {
				fmt.Println()
				tui.ShowWarning("You are currently in an existing Agentuity project directory!")
				fmt.Println()
				fmt.Println(tui.Muted("Current directory: ") + tui.Bold(cwd))
				fmt.Println()
				fmt.Println(tui.Text("It looks like you might want to:"))
				fmt.Println(tui.Text("• ") + tui.Command(" dev") + tui.Text(" - Start development mode for this project"))
				fmt.Println(tui.Text("• ") + tui.Command(" project import") + tui.Text(" - Import this project to your organization"))
				fmt.Println(tui.Text("• ") + tui.Command(" deploy") + tui.Text(" - Deploy this project"))
				fmt.Println()

				continueAnyway := tui.Ask(logger, "Are you sure you want to create a new project here?", false)

				if !continueAnyway {
					fmt.Println()
					tui.ShowSuccess("No worries! Use one of the commands above to work with your existing project.")
					os.Exit(0)
				}

				fmt.Println()
				tui.ShowWarning("Continuing with new project creation...")
				fmt.Println()
			} else {
				// Non-interactive mode: fail with helpful error
				logger.Fatal("You are currently in an existing Agentuity project directory (%s).\n\n"+
					"If you want to work with this project, try:\n"+
					"  • %s (start development mode)\n"+
					"  • %s (import to organization)\n"+
					"  • %s (deploy project)\n\n"+
					"To create a new project, please run this command from a different directory or use --dir flag.",
					cwd,
					tui.Command("agentuity dev"),
					tui.Command("agentuity project import"),
					tui.Command("agentuity deploy"))
			}
		}

		if tui.HasTTY {
			// handle MCP server installation
			detected, err := mcp.Detect(logger, true)
			if err != nil {
				logger.Error("failed to detect MCP clients: %s", err)
			}
			if len(detected) > 0 {
				var clients []string
				for _, config := range detected {
					if !config.Installed || config.Detected {
						continue
					}
					clients = append(clients, config.Name)
				}
				if len(clients) > 0 {
					fmt.Println()
					fmt.Println(tui.Bold("Activate the Agentuity MCP to enhance the following tools:"))
					fmt.Println()
					for _, client := range clients {
						tui.ShowSuccess("%s", tui.PadRight(client, 20, " "))
					}
					fmt.Println()
					fmt.Println(tui.Muted("By installing, you will have advanced AI Agent capabilities."))
					fmt.Println()
					yesno := tui.Ask(logger, tui.Bold("Would you like to install?"), true)
					fmt.Println()
					if yesno {
						fmt.Println()
						if err := mcp.Install(ctx, logger); err != nil {
							logger.Fatal("%s", err)
						}
					} else {
						fmt.Println()
						tui.ShowWarning("You can install the Agentuity tooling later by running: \n\n\t%s", tui.Command("mcp", "install"))
					}
					fmt.Println()
					tui.WaitForAnyKeyMessage("Press any key to continue with project creation ...")
					fmt.Println()
					initScreenWithLogo() // re-clear the screen
				}
			}
		}

		orgId := promptForOrganization(ctx, logger, cmd, apiUrl, apikey)

		var name, description, agentName, agentDescription, authType, githubAction string

		if len(args) > 0 {
			name = args[0]
		}
		if len(args) > 1 {
			description = args[1]
		}
		if len(args) > 2 {
			agentName = args[2]
		}
		if len(args) > 3 {
			agentDescription = args[3]
		}

		authType, _ = cmd.Flags().GetString("auth")
		githubAction, _ = cmd.Flags().GetString("action")
		providerArg, _ := cmd.Flags().GetString("runtime")
		templateArg, _ := cmd.Flags().GetString("template")

		var providerName string
		var templateName string
		var provider *templates.Template

		tmpls, tmplDir := loadTemplates(ctx, cmd)

		// check for preferences in config
		if providerArg == "" {
			providerArg = viper.GetString("preferences.provider")
		}
		if templateArg == "" {
			templateArg = viper.GetString("preferences.template")
		}

		if providerArg != "" {
			providerName = providerArg
			var found bool
			for _, tmpl := range tmpls {
				if tmpl.Identifier == providerName {
					found = true
					provider = &tmpl
					break
				}
				if found {
					break
				}
			}
			if !found {
				providerName = ""
			}
		}

		if provider != nil && templateArg != "" {
			if ok := templates.IsValidRuntimeTemplateName(ctx, tmplDir, provider.Identifier, templateArg); !ok {
				logger.Info("invalid template name %s for %s", templateArg, provider.Name)
				templateArg = ""
			}
			templateName = templateArg
		}

		if !tui.HasTTY {
			if name == "" {
				logger.Fatal("no project name provided and no TTY detected. Please provide a project name using the arguments from the command line")
			}
		} else {

			var skipTUI bool

			validateProjectName := func(name string) (bool, error) {
				for _, invalid := range invalidProjectNames {
					if s, ok := invalid.(string); ok {
						if name == s {
							return false, fmt.Errorf("%s is not a valid project name", name)
						}
					}
				}
				exists, err := project.ProjectWithNameExists(ctx, logger, apiUrl, apikey, orgId, name)
				if err != nil {
					return false, err
				}
				return !exists, nil
			}

			if providerName != "" && templateName != "" && name != "" && agentName != "" {
				ok, err := validateProjectName(name)
				if err != nil {
					logger.Fatal("%s", err)
				}
				skipTUI = ok
			}

			if !skipTUI {
				resp := ui.ShowProjectUI(ui.ProjectForm{
					Context:             ctx,
					Logger:              logger,
					TemplateDir:         tmplDir,
					Templates:           tmpls,
					AgentuityCommand:    getAgentuityCommand(),
					Runtime:             providerName,
					Template:            templateName,
					ProjectName:         name,
					Description:         description,
					AgentName:           agentName,
					AgentDescription:    agentDescription,
					AgentAuthType:       authType,
					DeploymentType:      githubAction,
					ValidateProjectName: validateProjectName,
				})
				name = resp.ProjectName
				description = resp.Description
				agentName = resp.AgentName
				if agentName == "" {
					agentName = "my agent"
				}
				agentDescription = resp.AgentDescription
				authType = resp.AgentAuthType
				githubAction = resp.DeploymentType
				templateName = resp.Template
				providerName = resp.Runtime
				provider = resp.Provider
			}
		}
		projectDir := filepath.Join(cwd, util.SafeProjectFilename(name, provider.Language == "python"))
		dir, _ := cmd.Flags().GetString("dir")
		if dir != "" {
			absDir, err := filepath.Abs(dir)
			if err != nil {
				errsystem.New(errsystem.ErrListFilesAndDirectories, err, errsystem.WithContextMessage("Failed to get absolute path")).ShowErrorAndExit()
			}
			projectDir = absDir
		} else {
			projectDir = tui.InputWithPathCompletion(logger, "What directory should the project be created in?", "The directory to create the project in", projectDir)
		}

		force, _ := cmd.Flags().GetBool("force")

		if util.Exists(projectDir) {
			if !force {
				if tui.HasTTY {
					fmt.Println(tui.Secondary("The directory ") + tui.Bold(projectDir) + tui.Secondary(" already exists."))
					fmt.Println()
					if !tui.Ask(logger, "Delete and continue?", true) {
						return
					}
				} else {
					logger.Fatal("The directory %s already exists. Use --force to overwrite.", projectDir)
					os.Exit(1)
				}
			}
			os.RemoveAll(projectDir)
			initScreenWithLogo()
		}

		if util.Exists(projectDir) {
			if !tui.Ask(logger, tui.Paragraph("The directory "+tui.Bold(projectDir)+" already exists.", "Are you sure you want to overwrite files here?"), true) {
				return
			}
		} else {
			if err := os.MkdirAll(projectDir, 0700); err != nil {
				errsystem.New(errsystem.ErrCreateDirectory, err, errsystem.WithContextMessage("Failed to create project directory")).ShowErrorAndExit()
			}
		}

		format, _ := cmd.Flags().GetString("format")

		var projectData *project.ProjectData

		tmplContext := templates.TemplateContext{
			Context:          ctx,
			Logger:           logger,
			Name:             name,
			Description:      description,
			ProjectDir:       projectDir,
			AgentName:        agentName,
			AgentDescription: agentDescription,
			TemplateName:     templateName,
			TemplateDir:      tmplDir,
			AgentuityCommand: getAgentuityCommand(),
		}

		tui.ShowSpinner("creating project ...", func() {
			rules, existingAgents, err := provider.NewProject(tmplContext)
			if err != nil {
				errsystem.New(errsystem.ErrCreateProject, err, errsystem.WithContextMessage("Failed to create project")).ShowErrorAndExit()
			}

			// check to see if the project already has existing agents returned and if so, we're going to use those
			var agents []cproject.AgentConfig
			if len(existingAgents) > 0 {
				for _, agent := range existingAgents {
					agents = append(agents, cproject.AgentConfig{
						Name:        agent.Name,
						Description: agent.Description,
					})
				}
			} else {
				agents = []cproject.AgentConfig{
					{
						Name:        agentName,
						Description: agentDescription,
					},
				}
			}

			projectData = initProject(ctx, logger, InitProjectArgs{
				BaseURL:           apiUrl,
				Dir:               projectDir,
				Token:             apikey,
				OrgId:             orgId,
				Name:              name,
				Description:       description,
				Provider:          rules,
				Agents:            agents,
				EnableWebhookAuth: authType == "project" || authType == "webhook",
				AuthType:          authType,
				Framework:         templateName,
			})

			// remember our choices
			viper.Set("preferences.provider", provider.Identifier)
			viper.Set("preferences.template", templateName)
			viper.Set("preferences.project_dir", projectDir)
			viper.WriteConfig()

		})

		gitInfo, err := deployer.GetGitInfoRecursive(logger, projectDir)
		if err != nil {
			logger.Info("failed to get git info: %s", err)
		}
		logger.Debug("git info: %+v", gitInfo)

		// Only initialize a new git repo if there is no parent git repo
		// (i.e., if the closest .git is the projectDir itself, not a parent)
		if !gitInfo.IsRepo {
			projectGitFlow(ctx, provider, tmplContext, githubAction)
		} else {
			// Check if there's a .git directory directly in the project directory
			// If so, it's safe to run projectGitFlow; otherwise, we're in a parent git repo
			projectDirGitInfo, err := deployer.GetGitInfo(logger, projectDir)
			if err != nil {
				logger.Debug("failed to get git info for project directory: %s", err)
			}
			if projectDirGitInfo != nil && projectDirGitInfo.IsRepo {
				// There is a .git directly in projectDir, so it's safe to run projectGitFlow
				projectGitFlow(ctx, provider, tmplContext, githubAction)
			} else {
				// We're inside a parent git repository, do not create a nested repo
				logger.Info("Project is inside an existing git repository; not creating a new git repo.")
			}
		}

		if format == "json" {
			json.NewEncoder(os.Stdout).Encode(projectData)
		} else {

			var para []string
			para = append(para, tui.Secondary("1. Switch into the project directory at ")+tui.Directory(projectDir))
			para = append(para, tui.Secondary("2. Run ")+tui.Command("dev")+tui.Secondary(" to run the project locally in development mode"))
			para = append(para, tui.Secondary("3. Run ")+tui.Command("deploy")+tui.Secondary(" to deploy the project to the Agentuity Agent Cloud"))
			if authType == "project" || authType == "webhook" {
				para = append(para, tui.Secondary("4. Run ")+tui.Command("agent apikey")+tui.Secondary(" to fetch the Webhook API key for the agent"))
			}
			para = append(para, tui.Secondary("🏠 Access your project at ")+tui.Link("%s/projects/%s", appUrl, projectData.ProjectId))

			tui.ShowBanner("You're ready to deploy your first Agent!",
				tui.Paragraph("Next steps:", para...),
				false,
			)
		}

	},
}

func listProjects(ctx context.Context, logger logger.Logger, apiUrl string, apikey string, orgId string) []project.ProjectListData {
	unfilteredProjects, err := project.ListProjects(ctx, logger, apiUrl, apikey)
	if err != nil {
		errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to list projects")).ShowErrorAndExit()
	}
	if orgId == "" {
		return unfilteredProjects
	}
	var projects []project.ProjectListData
	for _, project := range unfilteredProjects {
		if project.OrgId == orgId {
			projects = append(projects, project)
		}
	}
	return projects
}

func showNoProjects(orgId string) {
	fmt.Println()
	if orgId != "" {
		tui.ShowWarning("no projects found in organization %s", orgId)
	} else {
		tui.ShowWarning("no projects found")
		tui.ShowBanner("Create a new project", tui.Text("Use the ")+tui.Command("new")+tui.Text(" command to create a new project"), false)
	}
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all projects",
	Long: `List all projects in your organization.

This command displays all projects in your organization, showing their IDs, names, and descriptions.

Examples:
  agentuity project list
  agentuity project ls`,
	Aliases: []string{"ls"},
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		logger := env.NewLogger(cmd)
		apikey, _ := util.EnsureLoggedIn(ctx, logger, cmd)
		urls := util.GetURLs(logger)
		apiUrl := urls.API

		orgId, _ := cmd.Flags().GetString("org-id")

		var projects []project.ProjectListData
		action := func() {
			projects = listProjects(ctx, logger, apiUrl, apikey, orgId)
		}
		tui.ShowSpinner("fetching projects ...", action)
		format, _ := cmd.Flags().GetString("format")
		if format == "json" {
			json.NewEncoder(os.Stdout).Encode(projects)
			return
		}
		if len(projects) == 0 {
			showNoProjects(orgId)
			return
		}

		orgProjects := make(map[string][]project.ProjectListData)
		orgNames := make(map[string]string)
		var orgs []string

		for _, p := range projects {
			if _, ok := orgProjects[p.OrgId]; !ok {
				orgProjects[p.OrgId] = []project.ProjectListData{}
				orgNames[p.OrgId] = p.OrgName
				orgs = append(orgs, p.OrgId)
			}
			orgProjects[p.OrgId] = append(orgProjects[p.OrgId], p)
		}

		sort.Slice(orgs, func(i, j int) bool {
			return orgNames[orgs[i]] < orgNames[orgs[j]]
		})

		if len(orgs) > 1 {
			for _, orgId := range orgs {
				fmt.Println()
				fmt.Println(tui.Bold(orgNames[orgId]) + " " + tui.Muted("("+orgId+")"))
				fmt.Println()

				headers := []string{tui.Title("Project Id"), tui.Title("Name"), tui.Title("Description")}
				rows := [][]string{}
				for _, project := range orgProjects[orgId] {
					desc := project.Description
					if desc == "" {
						desc = emptyProjectDescription
					}
					rows = append(rows, []string{
						tui.Muted(project.ID),
						tui.Bold(project.Name),
						tui.Text(tui.MaxWidth(desc, 30)),
					})
				}
				tui.Table(headers, rows)
			}
		} else {
			headers := []string{tui.Title("Project Id"), tui.Title("Name"), tui.Title("Description")}
			rows := [][]string{}
			for _, project := range projects {
				desc := project.Description
				if desc == "" {
					desc = emptyProjectDescription
				}
				rows = append(rows, []string{
					tui.Muted(project.ID),
					tui.Bold(project.Name),
					tui.Text(tui.MaxWidth(desc, 30)),
				})
			}
			tui.Table(headers, rows)
		}
	},
}

var projectDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete one or more projects",
	Long: `Delete one or more projects from your organization.

This command allows you to select and delete projects from your organization.
It will prompt you to select which projects to delete and confirm the deletion.

Examples:
  agentuity project delete
  agentuity project rm project_12345567890ab
  agentuity project rm`,
	Aliases: []string{"rm", "del"},
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		logger := env.NewLogger(cmd)
		apikey, _ := util.EnsureLoggedIn(ctx, logger, cmd)
		urls := util.GetURLs(logger)
		apiUrl := urls.API

		orgId, _ := cmd.Flags().GetString("org-id")

		var selected []string
		if len(args) > 0 {
			selected = args
		} else {

			var projects []project.ProjectListData
			action := func() {
				projects = listProjects(ctx, logger, apiUrl, apikey, orgId)
			}
			tui.ShowSpinner("fetching projects ...", action)
			var options []tui.Option
			for _, project := range projects {
				options = append(options, tui.Option{
					ID:   project.ID,
					Text: tui.Bold(tui.PadRight(project.Name, 20, " ")) + tui.Muted(project.ID),
				})
			}

			if len(options) == 0 {
				showNoProjects(orgId)
				return
			}

			selected = tui.MultiSelect(logger, "Select one or more projects to delete", "", options)

			if len(selected) == 0 {
				tui.ShowWarning("no projects selected")
				return
			}
		}

		force, _ := cmd.Flags().GetBool("force")

		if !force && !tui.Ask(logger, "Are you sure you want to delete the selected projects?", true) {
			tui.ShowWarning("cancelled")
			return
		}

		var deleted []string

		action := func() {
			var err error
			deleted, err = project.DeleteProjects(ctx, logger, apiUrl, apikey, selected)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to delete projects")).ShowErrorAndExit()
			}
		}

		tui.ShowSpinner("Deleting projects ...", action)
		tui.ShowSuccess("%s deleted successfully", util.Pluralize(len(deleted), "project", "projects"))
	},
}

var projectImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import a project",
	Long: `Import an existing project into your organization.

This command imports a project from the current directory into your organization.
You will be prompted to select an organization and provide project details.

Flags:
  --dir    The directory containing the project to import

Examples:
  agentuity project import
  agentuity project import --dir /path/to/project`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		name, _ := cmd.Flags().GetString("name")
		description, _ := cmd.Flags().GetString("description")
		orgId, _ := cmd.Flags().GetString("org-id")
		apikey, _ := cmd.Flags().GetString("api-key")

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		context := project.EnsureProject(ctx, cmd)
		logger := env.NewLogger(cmd)

		// headless mode for nova
		if apikey != "" && orgId != "" && name != "" && description != "" {
			context.Project.Name = name
			context.Project.Description = description
			result, err := project.ProjectImport(ctx, logger, context.APIURL, apikey, orgId, context.Project, true)
			if err != nil {
				if isCancelled(ctx) {
					os.Exit(1)
				}
				errsystem.New(errsystem.ErrImportingProject, err,
					errsystem.WithContextMessage("Error importing project")).ShowErrorAndExit()
			}
			if err := context.Project.Save(context.Dir); err != nil {
				errsystem.New(errsystem.ErrSaveProject, err,
					errsystem.WithContextMessage("Error saving project after import")).ShowErrorAndExit()
			}
			saveEnv(context.Dir, result.APIKey, result.ProjectKey)
			return
		}

		ShowNewProjectImport(ctx, logger, cmd, context.APIURL, context.Token, "", context.Project, context.Dir, true)
		force, _ := cmd.Flags().GetBool("force")
		if !tui.HasTTY {
			force = true
		}
		_, _ = envutil.ProcessEnvFiles(ctx, logger, context.Dir, context.Project, nil, context.APIURL, context.Token, force, false)

	},
}

func getConfigTemplateDir(cmd *cobra.Command) (string, bool, error) {
	if cmd.Flags().Changed("templates-dir") {
		dir, _ := cmd.Flags().GetString("templates-dir")
		if !util.Exists(dir) {
			return "", false, fmt.Errorf("templates directory %s does not exist", dir)
		}
		return dir, true, nil
	}
	dir := filepath.Join(filepath.Dir(cfgFile), "templates")
	if !util.Exists(dir) {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return "", false, err
		}
	}
	return dir, false, nil
}

func init() {
	rootCmd.AddCommand(projectCmd)
	rootCmd.AddCommand(projectNewCmd)
	projectCmd.AddCommand(projectNewCmd)
	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectDeleteCmd)
	projectCmd.AddCommand(projectImportCmd)

	for _, cmd := range []*cobra.Command{projectNewCmd, projectImportCmd} {
		cmd.Flags().StringP("dir", "d", "", "The directory for the project")
		cmd.Flags().String("org-id", "", "The organization to create the project in")
	}

	for _, cmd := range []*cobra.Command{projectNewCmd, projectListCmd} {
		cmd.Flags().String("format", "text", "The format to use for the output. Can be either 'text' or 'json'")
	}

	projectListCmd.Flags().String("org-id", "", "Filter the projects by organization")

	projectNewCmd.Flags().StringP("runtime", "r", "", "The runtime to use for the project")
	projectNewCmd.Flags().StringP("template", "t", "", "The template to use for the project")
	projectNewCmd.Flags().Bool("force", false, "Force the project to be created even if the directory already exists")
	projectNewCmd.Flags().String("templates-dir", "", "The directory to load the templates. Defaults to loading them from the github.com/agentuity/templates repository")
	projectNewCmd.Flags().String("auth", "project", "The authentication type for the agent (project, webhook, or none)")
	projectNewCmd.Flags().String("action", "github-app", "The action to take for the project (github-action, github-app, none)")

	projectImportCmd.Flags().String("name", "", "The name of the project to import")
	projectImportCmd.Flags().String("description", "", "The description of the project to import")
	projectImportCmd.Flags().Bool("force", false, "Force the processing of environment files")

	// hidden because they must be all passed together and we havent documented that
	projectImportCmd.Flags().MarkHidden("name")
	projectImportCmd.Flags().MarkHidden("description")
	projectImportCmd.Flags().MarkHidden("org-id")

	projectDeleteCmd.Flags().String("org-id", "", "Only delete the projects in the specified organization")
	projectDeleteCmd.Flags().Bool("force", false, "Force the removal without confirmation")
}
