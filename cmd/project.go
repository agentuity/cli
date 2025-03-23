package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"

	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/mcp"
	"github.com/agentuity/cli/internal/organization"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/templates"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/tui"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	AgentName         string
	AgentDescription  string
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
		AgentName:         args.AgentName,
		AgentDescription:  args.AgentDescription,
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

	// add the initial agent
	proj.Agents = []project.AgentConfig{
		{
			ID:          result.AgentID,
			Name:        args.AgentName,
			Description: args.AgentDescription,
		},
	}

	if err := proj.Save(args.Dir); err != nil {
		errsystem.New(errsystem.ErrSaveProject, err, errsystem.WithContextMessage("Failed to save project to disk")).ShowErrorAndExit()
	}

	saveEnv(args.Dir, result.APIKey)

	return result
}

func promptForProjectDetail(ctx context.Context, logger logger.Logger, apiUrl, apikey string, name string, description string) (string, string) {
	var nameOK bool
	if name != "" {
		if exists, err := project.ProjectWithNameExists(ctx, logger, apiUrl, apikey, name); err != nil {
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
			if exists, err := project.ProjectWithNameExists(ctx, logger, apiUrl, apikey, name); err != nil {
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
		prefOrgId, _ := cmd.Flags().GetString("org-id")
		if prefOrgId == "" {
			prefOrgId = viper.GetString("preferences.orgId")
		}
		if tui.HasTTY {
			var opts []tui.Option
			for _, org := range orgs {
				opts = append(opts, tui.Option{ID: org.OrgId, Text: org.Name, Selected: prefOrgId == org.OrgId})
			}
			orgId = tui.Select(logger, "What organization should we create the project in?", "", opts)
			viper.Set("preferences.orgId", orgId)
			viper.WriteConfig() // remember the preference
		} else {
			for _, org := range orgs {
				if org.OrgId == prefOrgId {
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

var docStyle = lipgloss.NewStyle().Margin(1, 2)

type listItemProvider struct {
	title, desc, id string
	object          any
	selected        bool
}

func (i listItemProvider) Title() string       { return i.title }
func (i listItemProvider) Description() string { return i.desc }
func (i listItemProvider) FilterValue() string { return i.title }
func (i listItemProvider) ID() string          { return i.id }

type projectSelectionModel struct {
	list      list.Model
	cancelled bool
}

func (m *projectSelectionModel) Init() tea.Cmd {
	return nil
}

func (m *projectSelectionModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" || msg.String() == "esc" {
			m.cancelled = true
			return m, tea.Quit
		}
		if msg.String() == "enter" {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *projectSelectionModel) View() string {
	return docStyle.Render(m.list.View())
}

func showItemSelector(title string, items []list.Item) list.Item {

	selectedIndex := -1

	for i, item := range items {
		var p = item.(listItemProvider)
		p.desc = lipgloss.NewStyle().SetString(p.desc).Width(60).AlignHorizontal(lipgloss.Left).Render()
		items[i] = p
		if p.selected {
			selectedIndex = i
		}
	}

	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(4)

	m := projectSelectionModel{list: list.New(items, delegate, 0, 0)}
	m.list.Title = title
	m.list.Styles.Title = lipgloss.NewStyle().Foreground(tui.TitleColor())

	if selectedIndex != -1 {
		m.list.Select(selectedIndex)
	}

	p := tea.NewProgram(&m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}

	if m.cancelled {
		fmt.Println("Cancelled")
		os.Exit(1)
	}

	return items[m.list.Index()]
}

func projectGitFlow(ctx context.Context, logger logger.Logger) {
	git, err := exec.LookPath("git")
	if err != nil {
		return
	}
	if tui.HasTTY {
		opts := []tui.Option{
			{ID: "action", Text: tui.PadRight("GitHub Action", 20, " ") + tui.Muted("Use GitHub Action Workflow to automatically deploy")},
			{ID: "app", Text: tui.PadRight("GitHub App", 20, " ") + tui.Muted("Connect the Agentuity GitHub App to automatically deploy")},
			{ID: "none", Text: tui.PadRight("None", 20, " ") + tui.Muted("I'm not using GitHub or will setup later"), Selected: true},
		}
		choice := tui.Select(logger, "Are you using GitHub for this project?", "You can always configure later in the dashboard", opts)
		switch choice {
		case "none":
		case "action":
			body := tui.Paragraph(
				tui.Secondary("✓ Added GitHub Action Workflow to your project."),
				tui.Secondary("After you push your code, make sure you set your API Key from .env as"),
				tui.Secondary(
					fmt.Sprintf("a secret %s in your GitHub configuration.",
						tui.Highlight("AGENTUITY_API_KEY")),
				),
			)
			tui.ShowBanner("GitHub Action", body, false)
		case "app":
			tui.ShowBanner("GitHub App", tui.Secondary("After pushing your code to GitHub, visit the dashboard to connect your repository"), false)
		}
	}
	exec.CommandContext(ctx, git, "init").Run()
	exec.CommandContext(ctx, git, "add", ".").Run()
	exec.CommandContext(ctx, git, "commit", "-m", "[chore] Initial commit 🤖").Run()
	exec.CommandContext(ctx, git, "branch", "-m", "main").Run()
}

var projectNewCmd = &cobra.Command{
	Use:   "create [name] [description] [agent-name] [agent-description] [auth-type]",
	Short: "Create a new project",
	Long: `Create a new project with the specified name, description, and initial agent.

Arguments:
  [name]                The name of the project (must be unique within the organization)
  [description]         A description of what the project does
  [agent-name]          The name of the initial agent
  [agent-description]   A description of what the agent does
  [auth-type]           The authentication type for the agent (bearer or none)

Flags:
  --dir        The directory for the project
  --provider   The provider template to use for the project
  --template   The template to use for the project
	--force      Force the creation of the project even if the directory already exists

Examples:
  agentuity project create "My Project" "Project description" "My Agent" "Agent description" bearer
  agentuity create --provider nodejs --template express`,
	Aliases: []string{"new"},
	Args:    cobra.MaximumNArgs(5),
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		logger := env.NewLogger(cmd)
		apikey, _ := util.EnsureLoggedIn()
		apiUrl, appUrl, _ := util.GetURLs(logger)

		initScreenWithLogo()

		cwd, err := os.Getwd()
		if err != nil {
			errsystem.New(errsystem.ErrListFilesAndDirectories, err, errsystem.WithContextMessage("Failed to get current working directory")).ShowErrorAndExit()
		}

		checkForUpgrade(ctx, logger)

		if tui.HasTTY {
			// handle MCP server installation
			detected, err := mcp.Detect(true)
			if err != nil {
				logger.Fatal("%s", err)
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

		var name, description, agentName, agentDescription, authType string

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

		name, description = promptForProjectDetail(ctx, logger, apiUrl, apikey, name, description)

		projectDir := filepath.Join(cwd, util.SafeFilename(name))
		dir, _ := cmd.Flags().GetString("dir")
		if dir != "" {
			projectDir = dir
		} else {
			projectDir = tui.InputWithPlaceholder(logger, "What directory should the project be created in?", "The directory to create the project in", projectDir)
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

		var providerName string
		var templateName string
		var provider *templates.Template

		providerArg, _ := cmd.Flags().GetString("provider")
		templateArg, _ := cmd.Flags().GetString("template")

		tmpls, err := templates.LoadTemplates()
		if err != nil {
			errsystem.New(errsystem.ErrLoadTemplates, err, errsystem.WithContextMessage("Failed to load templates")).ShowErrorAndExit()
		}

		if len(tmpls) == 0 {
			errsystem.New(errsystem.ErrLoadTemplates, err, errsystem.WithContextMessage("No templates returned from load templates")).ShowErrorAndExit()
		}

		var selectProvider string
		var selectTemplate string

		// check for preferences in config
		if providerArg == "" {
			selectProvider = viper.GetString("preferences.provider")
		}
		if templateArg == "" {
			selectTemplate = viper.GetString("preferences.template")
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
			if ok := templates.IsValidRuntimeTemplateName(provider.Identifier, templateArg); !ok {
				logger.Fatal("invalid template name %s for %s", templateArg, provider.Name)
			}
			templateName = templateArg
		}

		if providerName == "" {
			if !tui.HasTTY {
				logger.Fatal("no provider provided and no TTY detected. Please select a provider using the --provider flag")
				os.Exit(1)
			}

			var items []list.Item

			for _, tmpls := range tmpls {
				items = append(items, listItemProvider{
					id:       tmpls.Identifier,
					title:    tmpls.Name,
					desc:     tmpls.Description,
					object:   &tmpls,
					selected: selectProvider == tmpls.Identifier,
				})
			}

			sort.Slice(items, func(i, j int) bool {
				return items[i].(listItemProvider).title < items[j].(listItemProvider).title
			})

			provider = showItemSelector("Select the project runtime", items).(listItemProvider).object.(*templates.Template)
		}

		if templateName == "" {
			if !tui.HasTTY {
				logger.Fatal("no template provided and no TTY detected. Please select a template using the --template flag")
				os.Exit(1)
			}

			templates, err := templates.LoadLanguageTemplates(provider.Identifier)
			if err != nil {
				errsystem.New(errsystem.ErrLoadTemplates, err, errsystem.WithContextMessage("Failed to load templates from template provider")).ShowErrorAndExit()
			}

			var tmplTemplates []list.Item
			for _, t := range templates {
				tmplTemplates = append(tmplTemplates, listItemProvider{
					id:       t.Name,
					title:    t.Name,
					desc:     t.Description,
					object:   &t,
					selected: t.Name == selectTemplate,
				})
			}
			templateId := showItemSelector("Select a project template", tmplTemplates)
			templateName = templateId.(listItemProvider).id
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

		if agentName == "" {
			if !tui.HasTTY {
				logger.Fatal("no agent name provided and no TTY detected. Please provide an agent name using the arguments from the command line")
				os.Exit(1)
			}
			agentName, agentDescription, authType = getAgentInfoFlow(logger, nil, agentName, agentDescription, authType)
		}

		format, _ := cmd.Flags().GetString("format")

		var projectData *project.ProjectData

		tmplContext := templates.TemplateContext{
			Context:          context.Background(),
			Logger:           logger,
			Name:             name,
			Description:      description,
			ProjectDir:       projectDir,
			AgentName:        agentName,
			AgentDescription: agentDescription,
			TemplateName:     templateName,
			AgentuityCommand: getAgentuityCommand(),
		}

		tui.ShowSpinner("checking dependencies ...", func() {
			if !provider.Matches(tmplContext) {
				if err := provider.Install(tmplContext); err != nil {
					var requirementsErr *templates.ErrRequirementsNotMet
					if errors.As(err, &requirementsErr) {
						tui.CancelSpinner()
						tui.ShowBanner("Missing Requirement", requirementsErr.Message, false)
						os.Exit(1)
					}
					errsystem.New(errsystem.ErrInstallDependencies, err, errsystem.WithContextMessage("Failed to install dependencies")).ShowErrorAndExit()
				}
			}
		})

		tui.ShowSpinner("creating project ...", func() {
			rules, err := provider.NewProject(tmplContext)
			if err != nil {
				errsystem.New(errsystem.ErrCreateProject, err, errsystem.WithContextMessage("Failed to create project")).ShowErrorAndExit()
			}

			projectData = initProject(ctx, logger, InitProjectArgs{
				BaseURL:           apiUrl,
				Dir:               projectDir,
				Token:             apikey,
				OrgId:             orgId,
				Name:              name,
				Description:       description,
				Provider:          rules,
				AgentName:         agentName,
				AgentDescription:  agentDescription,
				EnableWebhookAuth: authType != "none",
			})

			// remember our choices
			viper.Set("preferences.provider", provider.Identifier)
			viper.Set("preferences.template", templateName)
			viper.WriteConfig()

		})

		// run the git flow
		projectGitFlow(ctx, logger)

		if format == "json" {
			json.NewEncoder(os.Stdout).Encode(projectData)
		} else {

			var para []string
			para = append(para, tui.Secondary("1. Switch into the project directory at ")+tui.Directory(projectDir))
			para = append(para, tui.Secondary("2. Run ")+tui.Command("run")+tui.Secondary(" to run the project locally in development mode"))
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
		apikey, _ := util.EnsureLoggedIn()
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
		headers := []string{tui.Title("Project Id"), tui.Title("Name"), tui.Title("Description"), tui.Title("Organization")}
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
				tui.Text(project.OrgName) + " " + tui.Muted("("+project.OrgId+")"),
			})
		}
		tui.Table(headers, rows)
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
		apikey, _ := util.EnsureLoggedIn()
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
		context := project.EnsureProject(cmd)
		ShowNewProjectImport(ctx, logger, cmd, context.APIURL, context.Token, "", context.Project, context.Dir, true)
	},
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

	projectNewCmd.Flags().StringP("provider", "p", "", "The provider template to use for the project")
	projectNewCmd.Flags().StringP("template", "t", "", "The template to use for the project")
	projectNewCmd.Flags().Bool("force", false, "Force the project to be created even if the directory already exists")
}
