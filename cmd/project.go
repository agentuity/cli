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

	"github.com/agentuity/cli/internal/codeagent"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/mcp"
	"github.com/agentuity/cli/internal/organization"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/templates"
	"github.com/agentuity/cli/internal/ui"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
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

func saveEnv(dir string, apikey string) {
	filename := filepath.Join(dir, ".env")
	envLines, err := env.ParseEnvFile(filename)
	if err != nil {
		errsystem.New(errsystem.ErrReadConfigurationFile, err, errsystem.WithContextMessage("Failed to parse .env file")).ShowErrorAndExit()
	}
	var found bool
	for i, envLine := range envLines {
		if envLine.Key == "AGENTUITY_API_KEY" {
			envLines[i].Val = apikey
			found = true
		}
	}
	if !found {
		envLines = append(envLines, env.EnvLine{Key: "AGENTUITY_API_KEY", Val: apikey})
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
	Provider          *templates.TemplateRules
	Agents            []project.AgentConfig
}

func initProject(ctx context.Context, logger logger.Logger, args InitProjectArgs) *project.ProjectData {

	result, err := project.InitProject(ctx, logger, project.InitProjectArgs{
		BaseURL:           args.BaseURL,
		Token:             args.Token,
		OrgId:             args.OrgId,
		Name:              args.Name,
		Description:       args.Description,
		EnableWebhookAuth: args.EnableWebhookAuth,
		Dir:               args.Dir,
		Provider:          args.Provider.Identifier,
		Agents:            args.Agents,
	})
	if err != nil {
		errsystem.New(errsystem.ErrCreateProject, err, errsystem.WithContextMessage("Failed to init project")).ShowErrorAndExit()
	}

	proj := project.NewProject()
	proj.ProjectId = result.ProjectId
	proj.Name = args.Name
	proj.Description = args.Description

	proj.Development = &project.Development{
		Port: args.Provider.Development.Port,
		Watch: project.Watch{
			Enabled: args.Provider.Development.Watch.Enabled,
			Files:   args.Provider.Development.Watch.Files,
		},
		Command: args.Provider.Development.Command,
		Args:    args.Provider.Development.Args,
	}

	proj.Bundler = &project.Bundler{
		Enabled:    args.Provider.Bundle.Enabled,
		Identifier: args.Provider.Identifier,
		Language:   args.Provider.Language,
		Framework:  args.Provider.Framework,
		Runtime:    args.Provider.Runtime,
		Ignore:     args.Provider.Bundle.Ignore,
		AgentConfig: project.AgentBundlerConfig{
			Dir: args.Provider.SrcDir,
		},
	}

	// copy over the deployment command and args from the template
	proj.Deployment.Command = args.Provider.Deployment.Command
	proj.Deployment.Args = args.Provider.Deployment.Args
	proj.Deployment.Resources.CPU = args.Provider.Deployment.Resources.CPU
	proj.Deployment.Resources.Memory = args.Provider.Deployment.Resources.Memory

	// set the agents from the result
	proj.Agents = result.Agents

	if err := proj.Save(args.Dir); err != nil {
		errsystem.New(errsystem.ErrSaveProject, err, errsystem.WithContextMessage("Failed to save project to disk")).ShowErrorAndExit()
	}

	saveEnv(args.Dir, result.APIKey)

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
		apiUrl, appUrl, _ := util.GetURLs(logger)

		initScreenWithLogo()

		cwd, err := os.Getwd()
		if err != nil {
			errsystem.New(errsystem.ErrListFilesAndDirectories, err, errsystem.WithContextMessage("Failed to get current working directory")).ShowErrorAndExit()
		}

		checkForUpgrade(ctx, logger, true)

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

		var name, description, agentName, agentDescription, authType, githubAction, agentGoal string

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
		goalFlag, _ := cmd.Flags().GetString("goal")
		agentGoal = goalFlag
		experimentalCode, _ := cmd.Flags().GetBool("experimental-code-agent")

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
				if agentGoal == "" {
					agentGoal = tui.Input(logger, "Describe what the initial agent should do", "Enter a brief description of the agent's functionality")
				}
			}
		}

		projectDir := filepath.Join(cwd, util.SafeFilename(name))
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
			var agents []project.AgentConfig
			if len(existingAgents) > 0 {
				for _, agent := range existingAgents {
					agents = append(agents, project.AgentConfig{
						Name:        agent.Name,
						Description: agent.Description,
					})
				}
			} else {
				agents = []project.AgentConfig{
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
				EnableWebhookAuth: authType != "none",
			})

			// remember our choices
			viper.Set("preferences.provider", provider.Identifier)
			viper.Set("preferences.template", templateName)
			viper.Set("preferences.project_dir", projectDir)
			viper.WriteConfig()

		})

		projectGitFlow(ctx, provider, tmplContext, githubAction)

		// run code generation for the initial agent if a goal is provided
		if agentGoal != "" && experimentalCode {
			// determine the agent source directory via template rules
			dirRule, err := templates.LoadTemplateRuleForIdentifier(tmplDir, provider.Identifier)
			if err == nil {
				dir := filepath.Join(projectDir, dirRule.SrcDir, util.SafeFilename(agentName))
				genOpts := codeagent.Options{Dir: dir, Goal: agentGoal, Logger: logger}
				codegenAction := func() {
					if err := codeagent.Generate(ctx, genOpts); err != nil {
						logger.Warn("Agent code generation failed: %s", err)
					}
				}
				tui.ShowSpinner("Crafting Agent code ...", codegenAction)
			}
		}

		if format == "json" {
			json.NewEncoder(os.Stdout).Encode(projectData)
		} else {

			var para []string
			para = append(para, tui.Secondary("1. Switch into the project directory at ")+tui.Directory(projectDir))
			para = append(para, tui.Secondary("2. Run ")+tui.Command("dev")+tui.Secondary(" to run the project locally in development mode"))
			para = append(para, tui.Secondary("3. Run ")+tui.Command("deploy")+tui.Secondary(" to deploy the project to the Agentuity Agent Cloud"))
			if authType != "none" {
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

func showNoProjects() {
	fmt.Println()
	tui.ShowWarning("no projects found")
	tui.ShowBanner("Create a new project", tui.Text("Use the ")+tui.Command("new")+tui.Text(" command to create a new project"), false)
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
		apiUrl, _, _ := util.GetURLs(logger)

		var projects []project.ProjectListData
		action := func() {
			var err error
			projects, err = project.ListProjects(ctx, logger, apiUrl, apikey)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to list projects")).ShowErrorAndExit()
			}
		}
		tui.ShowSpinner("fetching projects ...", action)
		format, _ := cmd.Flags().GetString("format")
		if format == "json" {
			json.NewEncoder(os.Stdout).Encode(projects)
			return
		}
		if len(projects) == 0 {
			showNoProjects()
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
  agentuity project rm`,
	Aliases: []string{"rm", "del"},
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		logger := env.NewLogger(cmd)
		apikey, _ := util.EnsureLoggedIn(ctx, logger, cmd)
		apiUrl, _, _ := util.GetURLs(logger)
		var projects []project.ProjectListData
		action := func() {
			var err error
			projects, err = project.ListProjects(ctx, logger, apiUrl, apikey)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to list projects")).ShowErrorAndExit()
			}
		}
		tui.ShowSpinner("fetching projects ...", action)
		var options []tui.Option
		for _, project := range projects {
			desc := project.Description
			if desc == "" {
				desc = emptyProjectDescription
			}
			options = append(options, tui.Option{
				ID:   project.ID,
				Text: tui.Bold(tui.PadRight(project.Name, 20, " ")) + tui.Muted(project.ID),
			})
		}

		if len(options) == 0 {
			showNoProjects()
			return
		}

		selected := tui.MultiSelect(logger, "Select one or more projects to delete", "", options)

		if len(selected) == 0 {
			tui.ShowWarning("no projects selected")
			return
		}

		if !tui.Ask(logger, "Are you sure you want to delete the selected projects?", true) {
			tui.ShowWarning("cancelled")
			return
		}

		var deleted []string

		action = func() {
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
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		logger := env.NewLogger(cmd)
		context := project.EnsureProject(ctx, cmd)
		ShowNewProjectImport(ctx, logger, cmd, context.APIURL, context.Token, "", context.Project, context.Dir, true)
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

	projectNewCmd.Flags().StringP("runtime", "r", "", "The runtime to use for the project")
	projectNewCmd.Flags().StringP("template", "t", "", "The template to use for the project")
	projectNewCmd.Flags().Bool("force", false, "Force the project to be created even if the directory already exists")
	projectNewCmd.Flags().String("templates-dir", "", "The directory to load the templates. Defaults to loading them from the github.com/agentuity/templates repository")
	projectNewCmd.Flags().String("auth", "bearer", "The authentication type for the agent (bearer or none)")
	projectNewCmd.Flags().String("action", "github-app", "The action to take for the project (github-action, github-app, none)")
	projectNewCmd.Flags().String("goal", "", "A description of what the initial agent should do (optional)")
	projectNewCmd.Flags().Bool("experimental-code-agent", false, "Enable experimental code agent generation")
}
