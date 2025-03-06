package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agentuity/cli/internal/agent"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/templates"
	"github.com/agentuity/cli/internal/tui"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	"github.com/charmbracelet/lipgloss/tree"
	"github.com/spf13/cobra"
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
	Short:   "Delete one or more Agents",
	Aliases: []string{"rm", "del"},
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		theproject := ensureProject(cmd)
		apiUrl, _ := getURLs(logger)

		keys, state := reconcileAgentList(logger, apiUrl, theproject.Token, theproject)

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

		selected := tui.MultiSelect(logger, "Select one or more Agents to delete", "Toggle selection by pressing the spacebar\nPress enter to confirm\n", options)

		if len(selected) == 0 {
			tui.ShowWarning("no Agents selected")
			return
		}

		var deleted []string

		action := func() {
			var err error
			deleted, err = agent.DeleteAgents(logger, apiUrl, theproject.Token, theproject.Project.ProjectId, selected)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to delete agents")).ShowErrorAndExit()
			}
			for _, key := range keys {
				agent := state[key]
				if util.Exists(agent.Filename) {
					os.Remove(agent.Filename)
				}
			}
		}

		if !tui.Ask(logger, tui.Paragraph("Are you sure you want to delete the selected Agents?", "This action cannot be undone."), true) {
			tui.ShowWarning("cancelled")
			return
		}

		tui.ShowSpinner("Deleting Agents ...", action)
		tui.ShowSuccess("%s deleted successfully", util.Pluralize(len(deleted), "Agent", "Agents"))
	},
}

func getAgentInfoFlow(logger logger.Logger, remoteAgents []agent.Agent, name string, description string) (string, string, string) {
	if name == "" {
		var prompt, help string
		if len(remoteAgents) > 0 {
			prompt = "What should we name the Agent?"
			help = "The name of the Agent must be unique within the project"
		} else {
			prompt = "What should we name the initial Agent?"
			help = "The name can be changed at any time and helps identify the Agent"
		}
		name = tui.InputWithValidation(logger, prompt, help, 255, func(name string) error {
			for _, agent := range remoteAgents {
				if strings.EqualFold(agent.Name, name) {
					return fmt.Errorf("Agent already exists with the name: %s", name)
				}
			}
			return nil
		})
	}

	if description == "" {
		description = tui.Input(logger, "How should we describe what the "+name+" Agent does?", "The description of the Agent is optional but helpful for understanding the role of the Agent")
	}

	auth := tui.Select(logger, "Select your Agent's authentication method", "How do you want to secure the Agent?", []tui.Option{
		{Text: tui.PadRight("API Key", 10, " ") + tui.Muted("Bearer Token"), ID: "bearer"},
		{Text: tui.PadRight("None", 10, " ") + tui.Muted("No Authentication Required"), ID: "none"},
	})

	return name, description, auth
}

var agentCreateCmd = &cobra.Command{
	Use:     "create [name] [description] [auth_type]",
	Short:   "Create a new Agent",
	Aliases: []string{"new"},
	Args:    cobra.MaximumNArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		theproject := ensureProject(cmd)
		apikey := theproject.Token
		apiUrl, _ := getURLs(logger)

		remoteAgents, err := getAgentList(logger, apiUrl, apikey, theproject)
		if err != nil {
			errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to get agent list")).ShowErrorAndExit()
		}

		initScreenWithLogo()

		var name string
		var description string
		var authType string

		if len(args) > 0 {
			name = args[0]
		}

		if len(args) > 1 {
			description = args[1]
		}

		name, description, authType = getAgentInfoFlow(logger, remoteAgents, name, description)

		action := func() {
			agentID, err := agent.CreateAgent(logger, apiUrl, apikey, theproject.Project.ProjectId, name, description, authType)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to create Agent")).ShowErrorAndExit()
			}

			rules, err := templates.LoadTemplateRuleForIdentifier(theproject.Project.Bundler.Identifier)
			if err != nil {
				errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithAttributes(map[string]any{"identifier": theproject.Project.Bundler.Identifier})).ShowErrorAndExit()
			}

			if err := rules.NewAgent(templates.TemplateContext{
				Logger:           logger,
				AgentName:        name,
				Name:             name,
				Description:      description,
				AgentDescription: description,
				ProjectDir:       theproject.Dir,
			}); err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithAttributes(map[string]any{"name": name})).ShowErrorAndExit()
			}

			theproject.Project.Agents = append(theproject.Project.Agents, project.AgentConfig{
				ID:          agentID,
				Name:        name,
				Description: description,
			})

			if err := theproject.Project.Save(theproject.Dir); err != nil {
				errsystem.New(errsystem.ErrSaveProject, err, errsystem.WithContextMessage("Failed to save project to disk")).ShowErrorAndExit()
			}
		}
		tui.ShowSpinner("Creating Agent ...", action)
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
	tui.ShowSpinner("Fetching Agents ...", action)
	return remoteAgents, err
}

func normalAgentName(name string) string {
	return util.SafeFilename(strings.ToLower(name))
}

func reconcileAgentList(logger logger.Logger, apiUrl string, apikey string, theproject projectContext) ([]string, map[string]agentListState) {
	remoteAgents, err := getAgentList(logger, apiUrl, apikey, theproject)
	if err != nil {
		errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to get agent list")).ShowErrorAndExit()
	}

	rules, err := templates.LoadTemplateRuleForIdentifier(theproject.Project.Bundler.Identifier)
	if err != nil {
		errsystem.New(errsystem.ErrInvalidConfiguration, err,
			errsystem.WithContextMessage("Failed loading template rule"),
			errsystem.WithAttributes(map[string]any{"identifier": theproject.Project.Bundler.Identifier})).ShowErrorAndExit()
	}

	// make a map of the agents in the agentuity config file
	fileAgents := make(map[string]project.AgentConfig)
	for _, agent := range theproject.Project.Agents {
		fileAgents[normalAgentName(agent.Name)] = agent
	}

	agentFilename := rules.Filename
	agentSrcDir := filepath.Join(theproject.Dir, theproject.Project.Bundler.AgentConfig.Dir)

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
		errsystem.New(errsystem.ErrListFilesAndDirectories, err, errsystem.WithContextMessage("Failed to list agent source directory")).ShowErrorAndExit()
	}
	for _, filename := range localAgents {
		agentName := filepath.Base(filepath.Dir(filename))
		key := normalAgentName(agentName)
		if filepath.Base(filename) == agentFilename {
			if found, ok := state[key]; ok {
				state[key] = agentListState{
					Agent:       found.Agent,
					Filename:    filename,
					FoundLocal:  true,
					FoundRemote: true,
				}
				continue
			}
			if a, ok := fileAgents[key]; ok {
				state[key] = agentListState{
					Agent:       &agent.Agent{Name: a.Name, ID: a.ID, Description: a.Description},
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

func buildAgentTree(keys []string, state map[string]agentListState, project projectContext) (*tree.Tree, int, int, error) {
	agentSrcDir := filepath.Join(project.Dir, project.Project.Bundler.AgentConfig.Dir)
	var root *tree.Tree
	var files *tree.Tree
	cwd, err := os.Getwd()
	if err != nil {
		return nil, 0, 0, fmt.Errorf("failed to get current working directory: %w", err)
	}
	if filepath.Join(cwd, project.Project.Bundler.AgentConfig.Dir) == agentSrcDir {
		files = tree.Root(tui.Title(project.Project.Bundler.AgentConfig.Dir) + wrappedPipe)
		root = files
	} else {
		srcdir := tree.New().Root(tui.Title(project.Project.Bundler.AgentConfig.Dir) + wrappedPipe)
		root = tree.New().Root(tui.Muted(project.Dir) + wrappedPipe).Child(srcdir)
		files = srcdir
	}

	var localIssues, remoteIssues int

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
			sublabels = append(sublabels, tui.Warning("⚠ Agent found local but not remotely"))
			localIssues++
		} else if st.FoundRemote {
			sublabels = append(sublabels, tui.Muted("ID: ")+tui.Secondary(st.Agent.ID))
			sublabels = append(sublabels, tui.Warning("⚠ Agent found remotely but not locally"))
			remoteIssues++
		}
		if len(sublabels) > 0 {
			sublabels[len(sublabels)-1] = sublabels[len(sublabels)-1].(string) + "\n"
		}
		agentTree := tree.New().Root(label).Child(sublabels...)
		files.Child(agentTree)
	}

	return root, localIssues, remoteIssues, nil
}

func showAgentWarnings(remoteIssues int, localIssues int, deploying bool) bool {
	issues := remoteIssues + localIssues
	if issues > 0 {
		var msg string
		var title string
		if issues > 1 {
			title = "Issues"
		} else {
			title = "Issue"
		}
		localFmt := util.Pluralize(localIssues, "local agent", "local agents")
		remoteFmt := util.Pluralize(remoteIssues, "remote agent", "remote agents")
		var prefix string
		if !deploying {
			prefix = "When you deploy your project, the"
		} else {
			prefix = "The"
		}
		switch {
		case localIssues > 0 && remoteIssues > 0:
			msg = fmt.Sprintf("%s %s will be deployed and the %s will be undeployed.", prefix, localFmt, remoteFmt)
		case localIssues > 0:
			msg = fmt.Sprintf("%s %s will be deployed to the cloud and the ID will be saved.", prefix, localFmt)
		case remoteIssues > 0:
			msg = fmt.Sprintf("%s %s will be undeployed from the cloud and the ID will be removed from your project locally.", prefix, remoteFmt)
		}
		body := fmt.Sprintf("Detected %s in your project. %s\n\n", util.Pluralize(issues, "discrepancy", "discrepancies"), msg) + tui.Muted("$ ") + tui.Command("deploy")
		tui.ShowBanner(tui.Warning(fmt.Sprintf("⚠ Agent %s Detected", title)), body, false)
		if deploying {
			tui.WaitForAnyKey()
		}
		return true
	}
	return false
}

var agentListCmd = &cobra.Command{
	Use:     "list",
	Short:   "List all Agents in the project",
	Aliases: []string{"ls"},
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		project := ensureProject(cmd)
		apiUrl, _ := getURLs(logger)

		// perform the reconcilation
		keys, state := reconcileAgentList(logger, apiUrl, project.Token, project)

		if len(keys) == 0 {
			tui.ShowWarning("no Agents found")
			tui.ShowBanner("Create a new Agent", tui.Text("Use the ")+tui.Command("agent new")+tui.Text(" command to create a new Agent"), false)
			return
		}

		root, localIssues, remoteIssues, err := buildAgentTree(keys, state, project)
		if err != nil {
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage("Failed to build agent tree")).ShowErrorAndExit()
		}

		fmt.Println(root)

		if showAgentWarnings(remoteIssues, localIssues, false) {
			os.Exit(1)
		}

	},
}

var agentGetApiKeyCmd = &cobra.Command{
	Use:     "apikey [agent_name]",
	Short:   "Get the API key for an agent",
	Args:    cobra.MaximumNArgs(1),
	Aliases: []string{"key"},
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		project := ensureProject(cmd)
		apiUrl, _ := getURLs(logger)

		// perform the reconcilation
		keys, state := reconcileAgentList(logger, apiUrl, project.Token, project)

		if len(keys) == 0 {
			tui.ShowWarning("no Agents found")
			tui.ShowBanner("Create a new Agent", tui.Text("Use the ")+tui.Command("agent new")+tui.Text(" command to create a new Agent"), false)
			return
		}

		var agentID string
		var theagent *agentListState
		if len(args) > 0 {
			agentName := args[0]
			for _, v := range state {
				if v.Agent.ID == agentName || v.Agent.Name == agentName {
					theagent = &v
					agentID = v.Agent.ID
					break
				}
			}
		}
		if theagent == nil {
			if len(state) == 1 {
				for _, v := range state {
					theagent = &v
					agentID = v.Agent.ID
					break
				}
			} else {
				if tui.HasTTY {
					logger.Fatal("No TTY detected, please specify an Agent name or id")
				}
				var options []tui.Option
				for _, v := range keys {
					options = append(options, tui.Option{
						ID:   state[v].Agent.ID,
						Text: tui.PadRight(state[v].Agent.Name, 20, " ") + tui.Muted(state[v].Agent.ID),
					})
				}
				selected := tui.Select(logger, "Select an Agent", "Select the Agent you want to get the API key for", options)
				for _, v := range state {
					if v.Agent.ID == selected {
						theagent = &v
						break
					}
				}
			}
		}
		if theagent == nil {
			tui.ShowWarning("Agent not found")
			return
		}
		apikey, err := agent.GetApiKey(logger, apiUrl, project.Token, theagent.Agent.ID)
		if err != nil {
			errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to get agent API key")).ShowErrorAndExit()
		}
		if !tui.HasTTY {
			fmt.Print(apikey)
			return
		}
		if agentID != "" {
			fmt.Println()
			tui.ShowLock("Agent %s API key: %s", theagent.Agent.Name, apikey)
			tip := fmt.Sprintf(`$(agentuity agent apikey %s)`, agentID)
			tui.ShowBanner("Developer Pro Tip", tui.Paragraph("Fetch your Agent's API key into a shell command dynamically:", tip), false)
		}
	},
}

func init() {
	rootCmd.AddCommand(agentCmd)
	agentCmd.AddCommand(agentCreateCmd)
	agentCmd.AddCommand(agentListCmd)
	agentCmd.AddCommand(agentDeleteCmd)
	agentCmd.AddCommand(agentGetApiKeyCmd)
	for _, cmd := range []*cobra.Command{agentListCmd, agentCreateCmd, agentDeleteCmd, agentGetApiKeyCmd} {
		cmd.Flags().StringP("dir", "d", "", "The project directory")
	}
}
