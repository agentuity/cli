package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/organization"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/templates"
	"github.com/agentuity/cli/internal/tui"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
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
	Name              string
	Description       string
	EnableWebhookAuth bool
	Provider          *templates.TemplateRules
	AgentName         string
	AgentDescription  string
}

func initProject(logger logger.Logger, args InitProjectArgs) *project.ProjectData {

	result, err := project.InitProject(logger, project.InitProjectArgs{
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
	filename := filepath.Join(args.Dir, ".env")
	envLines, err := env.ParseEnvFile(filename)
	if err != nil {
		errsystem.New(errsystem.ErrReadConfigurationFile, err, errsystem.WithContextMessage("Failed to parse .env file")).ShowErrorAndExit()
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
		errsystem.New(errsystem.ErrWriteConfigurationFile, err, errsystem.WithContextMessage("Failed to write .env file")).ShowErrorAndExit()
	}
	return result
}

func promptForOrganization(logger logger.Logger, apiUrl string, token string) string {
	orgs, err := organization.ListOrganizations(logger, apiUrl, token)
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
		var opts []tui.Option
		for _, org := range orgs {
			opts = append(opts, tui.Option{ID: org.OrgId, Text: org.Name})
		}
		orgId = tui.Select(logger, "What organization should we create the project in?", "", opts)
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

	for i, item := range items {
		var p = item.(listItemProvider)
		p.desc = lipgloss.NewStyle().SetString(p.desc).Width(60).AlignHorizontal(lipgloss.Left).Render()
		items[i] = p
	}

	delegate := list.NewDefaultDelegate()
	delegate.SetHeight(4)

	m := projectSelectionModel{list: list.New(items, delegate, 0, 0)}
	m.list.Title = title
	m.list.Styles.Title = lipgloss.NewStyle().Foreground(tui.TitleColor())

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

var projectNewCmd = &cobra.Command{
	Use:     "create [name] [description] [agent-name] [agent-description] [auth-type]",
	Short:   "Create a new project",
	Aliases: []string{"new"},
	Args:    cobra.MaximumNArgs(5),
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		apikey, _ := util.EnsureLoggedIn()
		apiUrl, appUrl := util.GetURLs(logger)
		initScreenWithLogo()

		cwd, err := os.Getwd()
		if err != nil {
			errsystem.New(errsystem.ErrListFilesAndDirectories, err, errsystem.WithContextMessage("Failed to get current working directory")).ShowErrorAndExit()
		}

		orgId := promptForOrganization(logger, apiUrl, apikey)

		var name, description, agentName, agentDescription, authType string

		if len(args) > 0 {
			name = args[0]
			if exists, err := project.ProjectWithNameExists(logger, apiUrl, apikey, name); err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to check if project name exists")).ShowErrorAndExit()
			} else if exists {
				logger.Fatal("project %s already exists in this organization. please choose another name", name)
			}
		} else {
			name = tui.InputWithValidation(logger, "What should we name the project?", "The name of the project must be unique within the organization", 255, func(name string) error {
				for _, invalid := range invalidProjectNames {
					if s, ok := invalid.(string); ok {
						if name == s {
							return fmt.Errorf("%s is not a valid project name", name)
						}
					}
				}
				if exists, err := project.ProjectWithNameExists(logger, apiUrl, apikey, name); err != nil {
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

		projectDir := filepath.Join(cwd, util.SafeFilename(name))
		dir, _ := cmd.Flags().GetString("dir")
		if dir != "" {
			projectDir = dir
		} else {
			projectDir = tui.InputWithPlaceholder(logger, "What directory should the project be created in?", "The directory to create the project in", projectDir)
		}

		if util.Exists(projectDir) {
			if !tui.Ask(logger, tui.Paragraph(tui.Secondary("The directory ")+tui.Bold(projectDir)+tui.Secondary(" already exists."), "Delete and continue?"), true) {
				return
			}
			os.RemoveAll(projectDir)
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

			var items []list.Item

			for _, tmpls := range tmpls {
				items = append(items, listItemProvider{
					id:     tmpls.Identifier,
					title:  tmpls.Name,
					desc:   tmpls.Description,
					object: &tmpls,
				})
			}

			sort.Slice(items, func(i, j int) bool {
				return items[i].(listItemProvider).title < items[j].(listItemProvider).title
			})

			provider = showItemSelector("Select the project runtime", items).(listItemProvider).object.(*templates.Template)
		}

		if templateName == "" {
			templates, err := templates.LoadLanguageTemplates(provider.Identifier)
			if err != nil {
				errsystem.New(errsystem.ErrLoadTemplates, err, errsystem.WithContextMessage("Failed to load templates from template provider")).ShowErrorAndExit()
			}

			var tmplTemplates []list.Item
			for _, t := range templates {
				tmplTemplates = append(tmplTemplates, listItemProvider{
					id:     t.Name,
					title:  t.Name,
					desc:   t.Description,
					object: &t,
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
			agentName, agentDescription, authType = getAgentInfoFlow(logger, nil, agentName, agentDescription)
		}

		var projectData *project.ProjectData

		tui.ShowSpinner("creating project ...", func() {
			rules, err := provider.NewProject(templates.TemplateContext{
				Context:          context.Background(),
				Logger:           logger,
				Name:             name,
				Description:      description,
				ProjectDir:       projectDir,
				AgentName:        agentName,
				AgentDescription: agentDescription,
				TemplateName:     templateName,
			})
			if err != nil {
				errsystem.New(errsystem.ErrCreateProject, err, errsystem.WithContextMessage("Failed to create project")).ShowErrorAndExit()
			}

			projectData = initProject(logger, InitProjectArgs{
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

		})

		var para []string
		para = append(para, tui.Secondary("1. Switch into the project directory at ")+tui.Directory(projectDir))
		para = append(para, tui.Secondary("2. Run ")+tui.Command("run")+tui.Secondary(" to run the project locally in development mode"))
		para = append(para, tui.Secondary("3. Run ")+tui.Command("deploy")+tui.Secondary(" to deploy the project to the Agentuity Agent Cloud"))
		if authType != "none" {
			para = append(para, tui.Secondary("4. Run ")+tui.Command("agent apikey")+tui.Secondary(" to fetch the API key for the agent"))
		}
		para = append(para, tui.Secondary("🏠 Access your project at ")+tui.Link("%s/projects/%s", appUrl, projectData.ProjectId))

		tui.ShowBanner("You're ready to deploy your first Agent!",
			tui.Paragraph("Next steps:", para...),
			true,
		)

	},
}

func showNoProjects() {
	fmt.Println()
	tui.ShowWarning("no projects found")
	tui.ShowBanner("Create a new project", tui.Text("Use the ")+tui.Command("new")+tui.Text(" command to create a new project"), false)
}

var projectListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all projects",
	Aliases: []string{"ls"},
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		apikey, _ := util.EnsureLoggedIn()
		apiUrl, _ := util.GetURLs(logger)
		var projects []project.ProjectListData
		action := func() {
			var err error
			projects, err = project.ListProjects(logger, apiUrl, apikey)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to list projects")).ShowErrorAndExit()
			}
		}
		tui.ShowSpinner("fetching projects ...", action)
		if len(projects) == 0 {
			showNoProjects()
			return
		}
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
	},
}

var projectDeleteCmd = &cobra.Command{
	Use:     "delete",
	Short:   "Delete one or more projects",
	Aliases: []string{"rm", "del"},
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		apikey, _ := util.EnsureLoggedIn()
		apiUrl, _ := util.GetURLs(logger)
		var projects []project.ProjectListData
		action := func() {
			var err error
			projects, err = project.ListProjects(logger, apiUrl, apikey)
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

		if !tui.Ask(logger, tui.Paragraph("Are you sure you want to delete the selected projects?", "This action cannot be undone."), true) {
			tui.ShowWarning("cancelled")
			return
		}

		var deleted []string

		action = func() {
			var err error
			deleted, err = project.DeleteProjects(logger, apiUrl, apikey, selected)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to delete projects")).ShowErrorAndExit()
			}
		}

		tui.ShowSpinner("Deleting projects ...", action)
		tui.ShowSuccess("%s deleted successfully", util.Pluralize(len(deleted), "project", "projects"))
	},
}

func init() {
	rootCmd.AddCommand(projectCmd)
	rootCmd.AddCommand(projectNewCmd)
	projectCmd.AddCommand(projectNewCmd)
	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectDeleteCmd)

	projectNewCmd.Flags().StringP("dir", "d", "", "The directory to create the project in")
	projectNewCmd.Flags().StringP("provider", "p", "", "The provider template to use for the project")
	projectNewCmd.Flags().StringP("template", "t", "", "The template to use for the project")
}
