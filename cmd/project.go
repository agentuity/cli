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
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
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
	Name              string
	Description       string
	EnableWebhookAuth bool
	Provider          provider.Provider
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
	})
	if err != nil {
		logger.Fatal("failed to initialize project: %s", err)
	}
	proj := project.NewProject()
	proj.ProjectId = result.ProjectId

	proj.Inputs = []project.IO{
		{
			Type: "webhook",
			ID:   result.IOId,
		},
	}

	proj.Bundler = &project.Bundler{
		Language:  args.Provider.Language(),
		Framework: args.Provider.Framework(),
		Runtime:   args.Provider.Runtime(),
		Agents: project.Agent{
			Dir: args.Provider.DefaultSrcDir(),
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

func showProjectSelector(theme *huh.Theme, items []list.Item) string {

	// render the items with the theme
	for i, item := range items {
		val := item.(projectProvider)
		val.title = theme.Focused.Title.Render(val.title)
		val.desc = theme.Focused.Description.Render(val.desc)
		items[i] = val
	}

	m := projectSelectionModel{list: list.New(items, list.NewDefaultDelegate(), 0, 0)}
	m.list.Title = "Select your project framework"
	m.list.Styles.Title = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FFFF"))

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

		theme := huh.ThemeCatppuccin()

		orgId, err := promptForOrganization(logger, apiUrl, apikey)
		if err != nil {
			logger.Fatal("failed to get organization: %s", err)
		}

		var name string

		if len(args) > 0 {
			name = args[0]
		} else {
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
		}

		projectDir := filepath.Join(cwd, projectNameTransformer.ReplaceAllString(name, "_"))
		dir, _ := cmd.Flags().GetString("dir")
		if dir != "" {
			projectDir = dir
		} else {
			if huh.NewInput().
				Title("What directory should the project be created in?").
				Prompt("> ").
				Placeholder(projectDir).
				Value(&projectDir).WithTheme(theme).Run() != nil {
				logger.Fatal("failed to get project directory")
			}
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

			providerName = showProjectSelector(theme, items)
		}

		enableWebhookAuth := promptForWebhookAuth(logger)

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
		if provider == nil {
			logger.Fatal("invalid provider: %s", providerName)
		}

		var projectData *project.ProjectData

		showSpinner(logger, "creating project ...", func() {
			if err := provider.NewProject(logger, projectDir, name); err != nil {
				logger.Fatal("failed to create project: %s", err)
			}
			projectData = initProject(logger, InitProjectArgs{
				BaseURL:           apiUrl,
				Dir:               projectDir,
				Token:             apikey,
				OrgId:             orgId,
				Name:              name,
				EnableWebhookAuth: enableWebhookAuth,
				Provider:          provider,
			})
		})

		fmt.Println()
		printSuccess("Project created successfully")

		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println()
		fmt.Printf("1. Switch into the project directory at %s\n", color.GreenString(projectDir))
		fmt.Printf("2. Run %s to run the project locally in development mode\n", command("run"))
		fmt.Printf("3. Run %s to deploy the project to the Agentuity Agent Cloud\n", command("deploy"))
		fmt.Println()
		fmt.Printf("🏠 Access your project at %s", link("%s/projects/%s", appUrl, projectData.ProjectId))
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

		provider, err := provider.GetProviderForName(detection.Provider)
		if err != nil {
			logger.Fatal("failed to get provider: %s. %s", detection.Provider, err)
		}

		p := initProject(logger, InitProjectArgs{
			BaseURL:           apiUrl,
			Dir:               dir,
			Token:             token,
			OrgId:             orgId,
			Provider:          provider,
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
	rootCmd.AddCommand(projectNewCmd)
	projectCmd.AddCommand(projectInitCmd)
	projectCmd.AddCommand(projectNewCmd)
	projectNewCmd.Flags().StringP("dir", "d", "", "The directory to create the project in")
	projectNewCmd.Flags().StringP("provider", "p", "", "The provider to use for the project")
}
