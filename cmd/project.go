package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/agentuity/cli/internal/organization"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/provider"
	"github.com/agentuity/cli/internal/tui"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	Name              string
	Description       string
	EnableWebhookAuth bool
	Provider          provider.Provider
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
		Provider:          args.Provider.Name(),
		AgentName:         args.AgentName,
		AgentDescription:  args.AgentDescription,
	})
	if err != nil {
		logger.Fatal("failed to initialize project: %s", err)
	}

	if err := args.Provider.InitProject(logger, args.Dir, result); err != nil {
		logger.Fatal("failed to initialize project: %s", err)
	}

	proj := project.NewProject()
	proj.ProjectId = result.ProjectId
	proj.Name = args.Name
	proj.Description = args.Description

	proj.Bundler = &project.Bundler{
		Language:  args.Provider.Language(),
		Framework: args.Provider.Framework(),
		Runtime:   args.Provider.Runtime(),
		AgentConfig: project.AgentBundlerConfig{
			Dir: args.Provider.DefaultSrcDir(),
		},
	}

	// add the initial agent
	proj.Agents = []project.AgentConfig{
		{
			ID:          result.AgentID,
			Name:        args.AgentName,
			Description: args.AgentDescription,
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
		var opts []tui.Option
		for _, org := range orgs {
			opts = append(opts, tui.Option{ID: org.OrgId, Text: org.Name})
		}
		orgId = tui.Select(logger, "What organization should we create the project in?", "", opts)
	}
	return orgId, nil
}

var invalidProjectNames = []any{
	"test",
	"agent",
	"agentuity",
}

var docStyle = lipgloss.NewStyle().Margin(1, 2)

type projectProvider struct {
	title, desc, id string
}

func (i projectProvider) Title() string       { return i.title }
func (i projectProvider) Description() string { return i.desc }
func (i projectProvider) FilterValue() string { return i.title }
func (i projectProvider) ID() string          { return i.id }

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

func showProjectSelector(items []list.Item) string {

	m := projectSelectionModel{list: list.New(items, list.NewDefaultDelegate(), 0, 0)}
	m.list.Title = "Select your project framework"
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

	return items[m.list.Index()].(projectProvider).id
}

var projectNewCmd = &cobra.Command{
	Use:     "create [name]",
	Short:   "Create a new project",
	Aliases: []string{"new"},
	Args:    cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		apikey := viper.GetString("auth.api_key")
		if apikey == "" {
			logger.Fatal("you are not logged in")
		}

		apiUrl, appUrl := getURLs(logger)
		initScreenWithLogo()

		cwd, err := os.Getwd()
		if err != nil {
			logger.Fatal("failed to get current directory: %s", err)
		}

		orgId, err := promptForOrganization(logger, apiUrl, apikey)
		if err != nil {
			logger.Fatal("failed to get organization: %s", err)
		}

		var name string

		if len(args) > 0 {
			name = args[0]
			if exists, err := project.ProjectWithNameExists(logger, apiUrl, apikey, name); err != nil {
				logger.Fatal("failed to check if project exists: %s", err)
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
					return fmt.Errorf("failed to check if project exists: %s", err)
				} else if exists {
					return fmt.Errorf("project %s already exists in this organization. please choose another name", name)
				}
				return nil
			})
		}

		description := tui.Input(logger, "How should we describe what the "+name+" project does?", "The description of the project is optional but helpful")

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
		providers := provider.GetProviders()
		providerArg, _ := cmd.Flags().GetString("provider")

		if providerArg != "" {
			providerName = providerArg
			var found bool
			for _, provider := range providers {
				if provider.Identifier() == providerName {
					found = true
					break
				}
				for _, alias := range provider.Aliases() {
					if alias == providerName {
						found = true
						break
					}
				}
				if found {
					break
				}
			}
			if !found {
				providerName = ""
			}
		}

		if providerName == "" {

			var items []list.Item

			for key, provider := range providers {
				items = append(items, projectProvider{id: key, title: provider.Name(), desc: provider.Description()})
			}

			sort.Slice(items, func(i, j int) bool {
				return items[i].(projectProvider).title < items[j].(projectProvider).title
			})

			providerName = showProjectSelector(items)
		}

		if util.Exists(projectDir) {
			if !tui.Ask(logger, tui.Paragraph("The directory "+tui.Bold(projectDir)+" already exists.", "Are you sure you want to overwrite files here?"), true) {
				return
			}
		} else {
			if err := os.MkdirAll(projectDir, 0700); err != nil {
				logger.Fatal("failed to create project directory: %s", err)
			}
		}

		theprovider := providers[providerName]
		if theprovider == nil {
			logger.Fatal("invalid provider: %s", providerName)
		}

		var projectData *project.ProjectData

		tui.ShowSpinner(logger, "creating project ...", func() {
			if err := theprovider.NewProject(logger, projectDir, name); err != nil {
				logger.Fatal("failed to create project: %s", err)
			}
			projectData = initProject(logger, InitProjectArgs{
				BaseURL:          apiUrl,
				Dir:              projectDir,
				Token:            apikey,
				OrgId:            orgId,
				Name:             name,
				Description:      description,
				Provider:         theprovider,
				AgentName:        provider.MyFirstAgentName,
				AgentDescription: provider.MyFirstAgentDescription,
			})
		})

		tui.ShowBanner("You're ready to deploy your first Agent!",
			tui.Paragraph("Next steps:",
				tui.Secondary("1. Switch into the project directory at ")+tui.Directory(projectDir),
				tui.Secondary("2. Run ")+tui.Command("run")+tui.Secondary(" to run the project locally in development mode"),
				tui.Secondary("3. Run ")+tui.Command("deploy")+tui.Secondary(" to deploy the project to the Agentuity Agent Cloud"),
				tui.Secondary("🏠 Access your project at ")+tui.Link("%s/projects/%s", appUrl, projectData.ProjectId),
			),
			true,
		)

	},
}

var projectListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all projects",
	Aliases: []string{"ls"},
	Args:    cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		apikey := viper.GetString("auth.api_key")
		if apikey == "" {
			logger.Fatal("you are not logged in")
		}
		apiUrl, _ := getURLs(logger)
		var projects []project.ProjectListData
		action := func() {
			var err error
			projects, err = project.ListProjects(logger, apiUrl, apikey)
			if err != nil {
				logger.Fatal("failed to list projects: %s", err)
			}
		}
		tui.ShowSpinner(logger, "fetching projects ...", action)
		if len(projects) == 0 {
			tui.ShowWarning("no projects found")
			tui.ShowBanner("Create a new project", tui.Text("Use the ")+tui.Command("new")+tui.Text(" command to create a new project"), false)
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
		apikey := viper.GetString("auth.api_key")
		if apikey == "" {
			logger.Fatal("you are not logged in")
		}
		apiUrl, _ := getURLs(logger)
		var projects []project.ProjectListData
		action := func() {
			var err error
			projects, err = project.ListProjects(logger, apiUrl, apikey)
			if err != nil {
				logger.Fatal("failed to list projects: %s", err)
			}
		}
		tui.ShowSpinner(logger, "fetching projects ...", action)
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
				logger.Fatal("failed to delete projects: %s", err)
			}
		}

		tui.ShowSpinner(logger, "Deleting projects ...", action)
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
	projectNewCmd.Flags().StringP("provider", "p", "", "The provider to use for the project")
}
