package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agentuity/cli/internal/agent"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/templates"
	"github.com/agentuity/cli/internal/tui"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	"github.com/charmbracelet/lipgloss/tree"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const emptyProjectDescription = "No description provided"

var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Agent related commands",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var agentDeleteCmd = &cobra.Command{
	Use:     "delete",
	Short:   "Delete one or more agents",
	Aliases: []string{"rm", "del"},
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		apikey := viper.GetString("auth.api_key")
		if apikey == "" {
			logger.Fatal("you are not logged in")
		}
		theproject := ensureProject(cmd)
		apiUrl, _ := getURLs(logger)

		keys, state := reconcileAgentList(logger, apiUrl, apikey, theproject)

		var options []tui.Option
		for _, key := range keys {
			agent := state[key]
			if agent.FoundRemote {
				options = append(options, tui.Option{
					ID:   agent.Agent.ID,
					Text: tui.PadRight(agent.Agent.Name, 20, " ") + tui.Muted(agent.Agent.ID),
				})
			}
		}

		selected := tui.MultiSelect(logger, "Select one or more agents to delete", "Toggle selection by pressing the spacebar\nPress enter to confirm\n", options)

		if len(selected) == 0 {
			tui.ShowWarning("no agents selected")
			return
		}

		var deleted []string

		action := func() {
			var err error
			deleted, err = agent.DeleteAgents(logger, apiUrl, apikey, theproject.Project.ProjectId, selected)
			if err != nil {
				logger.Fatal("failed to delete agents: %s", err)
			}
			for _, key := range keys {
				agent := state[key]
				if util.Exists(agent.Filename) {
					os.Remove(agent.Filename)
				}
			}
		}

		if !tui.Ask(logger, tui.Paragraph("Are you sure you want to delete the selected agents?", "This action cannot be undone."), true) {
			tui.ShowWarning("cancelled")
			return
		}

		tui.ShowSpinner(logger, "Deleting agents ...", action)
		tui.ShowSuccess("%s deleted successfully", util.Pluralize(len(deleted), "Agent", "Agents"))
	},
}

var agentCreateCmd = &cobra.Command{
	Use:     "create",
	Short:   "Create a new agent",
	Aliases: []string{"new"},
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		apikey := viper.GetString("auth.api_key")
		if apikey == "" {
			logger.Fatal("you are not logged in")
		}
		theproject := ensureProject(cmd)
		apiUrl, _ := getURLs(logger)

		remoteAgents, err := getAgentList(logger, apiUrl, apikey, theproject)
		if err != nil {
			logger.Fatal("failed to list agents: %s", err)
		}

		initScreenWithLogo()

		name := tui.InputWithValidation(logger, "What should we name the agent?", "The name of the agent must be unique within the project", 255, func(name string) error {
			for _, agent := range remoteAgents {
				if strings.EqualFold(agent.Name, name) {
					return fmt.Errorf("agent %s already exists with this name", name)
				}
			}
			return nil
		})

		description := tui.Input(logger, "How should we describe what the "+name+" agent does?", "The description of the agent is optional but helpful for understanding the role of the agent")

		action := func() {
			agentID, err := agent.CreateAgent(logger, apiUrl, apikey, theproject.Project.ProjectId, name, description)
			if err != nil {
				logger.Fatal("failed to create agent: %s", err)
			}

			rules, err := templates.LoadTemplateRuleForIdentifier(theproject.Project.Bundler.Identifier)
			if err != nil {
				logger.Fatal("failed to load template rules for %s: %s", theproject.Project.Bundler.Identifier, err)
			}

			if err := rules.NewAgent(templates.TemplateContext{
				Logger:      logger,
				Name:        name,
				Description: description,
				ProjectDir:  theproject.Dir,
			}); err != nil {
				logger.Fatal("failed to create agent: %s", err)
			}

			theproject.Project.Agents = append(theproject.Project.Agents, project.AgentConfig{
				ID:          agentID,
				Name:        name,
				Description: description,
			})

			if err := theproject.Project.Save(theproject.Dir); err != nil {
				logger.Fatal("failed to save project: %s", err)
			}
		}
		tui.ShowSpinner(logger, "Creating agent ...", action)
		tui.ShowSuccess("Agent created successfully")
	},
}

type agentListState struct {
	Agent       *agent.Agent
	Filename    string
	FoundLocal  bool
	FoundRemote bool
}

func getAgentList(logger logger.Logger, apiUrl string, apikey string, project projectContext) ([]agent.Agent, error) {
	var remoteAgents []agent.Agent
	var err error
	action := func() {
		remoteAgents, err = agent.ListAgents(logger, apiUrl, apikey, project.Project.ProjectId)
	}
	tui.ShowSpinner(logger, "Fetching agents ...", action)
	return remoteAgents, err
}

func normalAgentName(name string) string {
	return util.SafeFilename(strings.ToLower(name))
}

func reconcileAgentList(logger logger.Logger, apiUrl string, apikey string, project projectContext) ([]string, map[string]agentListState) {
	remoteAgents, err := getAgentList(logger, apiUrl, apikey, project)
	if err != nil {
		logger.Fatal("failed to fetch agents for project: %s", err)
	}
	var agentFilename string // FIXME
	// agentFilename := project.Provider.AgentFilename()
	agentSrcDir := filepath.Join(project.Dir, project.Project.Bundler.AgentConfig.Dir)

	// perform the reconcilation
	state := make(map[string]agentListState)
	for _, agent := range remoteAgents {
		state[normalAgentName(agent.Name)] = agentListState{
			Agent:       &agent,
			Filename:    filepath.Join(agentSrcDir, util.SafeFilename(agent.Name), agentFilename),
			FoundLocal:  util.Exists(filepath.Join(agentSrcDir, util.SafeFilename(agent.Name), agentFilename)),
			FoundRemote: true,
		}
	}
	localAgents, err := util.ListDir(agentSrcDir)
	if err != nil {
		logger.Fatal("failed to list local agents: %s", err)
	}
	for _, filename := range localAgents {
		if filepath.Base(filename) == agentFilename {
			agentName := filepath.Base(filepath.Dir(filename))
			key := normalAgentName(agentName)
			if found, ok := state[key]; ok {
				state[key] = agentListState{
					Agent:       found.Agent,
					Filename:    filename,
					FoundLocal:  true,
					FoundRemote: true,
				}
			} else {
				state[key] = agentListState{
					Agent:       &agent.Agent{Name: agentName},
					Filename:    filename,
					FoundLocal:  true,
					FoundRemote: false,
				}
			}
		}
	}

	keys := make([]string, 0, len(state))
	for k := range state {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	return keys, state
}

var wrappedPipe = "\n│"

var agentListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all the agents in the project which are deployed",
	Aliases: []string{"ls"},
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		apikey := viper.GetString("auth.api_key")
		if apikey == "" {
			logger.Fatal("you are not logged in")
		}
		project := ensureProject(cmd)
		apiUrl, _ := getURLs(logger)

		// perform the reconcilation
		keys, state := reconcileAgentList(logger, apiUrl, apikey, project)

		if len(keys) == 0 {
			tui.ShowWarning("no Agents found")
			tui.ShowBanner("Create a new Agent", tui.Text("Use the ")+tui.Command("agent new")+tui.Text(" command to create a new Agent"), false)
			return
		}

		agentSrcDir := filepath.Join(project.Dir, project.Project.Bundler.AgentConfig.Dir)
		var root *tree.Tree
		var files *tree.Tree
		cwd, err := os.Getwd()
		if err != nil {
			logger.Fatal("failed to get current working directory: %s", err)
		}
		if filepath.Join(cwd, project.Project.Bundler.AgentConfig.Dir) == agentSrcDir {
			files = tree.Root(tui.Title(project.Project.Bundler.AgentConfig.Dir) + wrappedPipe)
			root = files
		} else {
			srcdir := tree.New().Root(tui.Title(project.Project.Bundler.AgentConfig.Dir) + wrappedPipe)
			root = tree.New().Root(tui.Muted(project.Dir) + wrappedPipe).Child(srcdir)
			files = srcdir
		}

		for _, k := range keys {
			st := state[k]
			label := tui.PadRight(tui.Bold(st.Agent.Name), 20, " ")
			var sublabels []any
			if st.FoundLocal && st.FoundRemote {
				sublabels = append(sublabels, tui.Muted("ID: ")+tui.Secondary(st.Agent.ID))
				desc := st.Agent.Description
				if desc == "" {
					desc = emptyProjectDescription
				}
				sublabels = append(sublabels, tui.Muted("Description: ")+tui.Secondary(desc))
			} else if st.FoundLocal {
				sublabels = append(sublabels, tui.Warning("⚠ agent found local but not remotely"))
			} else if st.FoundRemote {
				sublabels = append(sublabels, tui.Muted("ID: ")+tui.Secondary(st.Agent.ID))
				sublabels = append(sublabels, tui.Warning("⚠ agent found remotely but not locally"))
			}
			if len(sublabels) > 0 {
				sublabels[len(sublabels)-1] = sublabels[len(sublabels)-1].(string) + "\n"
			}
			agentTree := tree.New().Root(label).Child(sublabels...)
			files.Child(agentTree)
		}
		fmt.Println(root)
	},
}

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.AddCommand(agentCreateCmd)
	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentDeleteCmd)
	for _, cmd := range []*cobra.Command{agentListCmd, agentCreateCmd, agentDeleteCmd} {
		cmd.Flags().StringP("dir", "d", "", "The project directory")
	}
}
