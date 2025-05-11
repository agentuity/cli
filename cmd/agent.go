package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"

	"github.com/agentuity/cli/internal/agent"
	"github.com/agentuity/cli/internal/codeagent"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/templates"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/slice"
	"github.com/agentuity/go-common/tui"
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
	Use:     "delete [id]",
	Short:   "Delete one or more Agents",
	Args:    cobra.MaximumNArgs(1),
	Aliases: []string{"rm", "del"},
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		theproject := project.EnsureProject(ctx, cmd)
		apiUrl, _, _ := util.GetURLs(logger)

		if !tui.HasTTY && len(args) == 0 {
			logger.Fatal("No TTY detected, please specify an Agent id from the command line")
		}

		keys, state := reconcileAgentList(logger, cmd, apiUrl, theproject.Token, theproject)
		var selected []string

		if len(args) > 0 {
			id := args[0]
			if _, ok := state[id]; ok {
				selected = append(selected, id)
			} else {
				logger.Fatal("Agent with id %s not found", id)
			}
		} else {
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

			selected = tui.MultiSelect(logger, "Select one or more Agents to delete from Agentuity Cloud", "Toggle selection by pressing the spacebar\nPress enter to confirm\n", options)

			if len(selected) == 0 {
				tui.ShowWarning("no Agents selected")
				return
			}
		}

		var deleted []string
		var maybedelete []string

		action := func() {
			var err error
			deleted, err = agent.DeleteAgents(context.Background(), logger, apiUrl, theproject.Token, theproject.Project.ProjectId, selected)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to delete agents")).ShowErrorAndExit()
			}
			for _, key := range keys {
				agent := state[key]
				if slices.Contains(deleted, agent.Agent.ID) && util.Exists(agent.Filename) {
					maybedelete = append(maybedelete, agent.Filename)
				}
			}
			var agents []project.AgentConfig
			for _, agent := range theproject.Project.Agents {
				if !slice.Contains(deleted, agent.ID) {
					agents = append(agents, agent)
				}
			}
			theproject.Project.Agents = agents
			if err := theproject.Project.Save(theproject.Dir); err != nil {
				errsystem.New(errsystem.ErrSaveProject, err, errsystem.WithContextMessage("saving project after agent delete")).ShowErrorAndExit()
			}
		}

		force, _ := cmd.Flags().GetBool("force")

		if !force && !tui.HasTTY {
			logger.Fatal("No TTY detected, please --force to delete the selected Agents and pass in the Agent id from the command line")
		}

		if !force && !tui.Ask(logger, "Are you sure you want to delete the selected Agents from Agentuity Cloud?", true) {
			tui.ShowWarning("cancelled")
			return
		}

		tui.ShowSpinner("Deleting Agents ...", action)

		var filedeletes []string

		if len(maybedelete) > 0 {
			if !force {
				filetext := util.Pluralize(len(maybedelete), "source file", "source files")
				var opts []tui.Option
				for _, f := range maybedelete {
					rel, _ := filepath.Rel(theproject.Dir, f)
					opts = append(opts, tui.Option{
						ID:       f,
						Text:     rel,
						Selected: true,
					})
				}
				filedeletes = tui.MultiSelect(logger, fmt.Sprintf("Would you like to delete the %s?", filetext), "Press spacebar to toggle file selection. Press enter to continue.", opts)
			} else {
				filedeletes = maybedelete
			}
		}

		if len(filedeletes) > 0 {
			ad := filepath.Join(theproject.Dir, ".agentuity", "backup")
			if !util.Exists(ad) {
				os.MkdirAll(ad, 0755)
			}
			for _, f := range filedeletes {
				fd := filepath.Dir(f)
				util.CopyDir(fd, filepath.Join(ad, filepath.Base(fd))) // make a backup
				os.Remove(f)
				files, _ := util.ListDir(fd)
				if len(files) == 0 {
					os.Remove(fd)
				}
			}
			tui.ShowSuccess("A backup was made temporarily in %s", ad)
		}

		tui.ShowSuccess("%s deleted successfully", util.Pluralize(len(deleted), "Agent", "Agents"))
	},
}

func getAgentAuthType(logger logger.Logger, authType string) string {
	if authType != "" {
		switch authType {
		case "bearer", "none":
			return authType
		default:
		}
	}
	auth := tui.Select(logger, "Select your Agent's webhook authentication method", "Do you want to secure the webhook or make it publicly available?", []tui.Option{
		{Text: tui.PadRight("API Key", 10, " ") + tui.Muted("Bearer Token (will be generated for you)"), ID: "bearer"},
		{Text: tui.PadRight("None", 10, " ") + tui.Muted("No Authentication Required"), ID: "none"},
	})
	return auth
}

func getAgentInfoFlow(logger logger.Logger, remoteAgents []agent.Agent, name string, description string, authType string) (string, string, string) {
	if name == "" {
		if !tui.HasTTY {
			logger.Fatal("No TTY detected, please specify an Agent name from the command line")
		}
		var prompt, help string
		if len(remoteAgents) > 0 {
			prompt = "What should we name the Agent?"
			help = "The name of the Agent must be unique within the project"
		} else {
			prompt = "What should we name the initial Agent?"
			help = "The name can be changed at any time and helps identify the Agent"
		}
		name = tui.InputWithValidation(logger, prompt, help, 255, func(name string) error {
			if name == "" {
				return fmt.Errorf("Agent name cannot be empty")
			}
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

	if authType == "" && !tui.HasTTY {
		logger.Fatal("No TTY detected, please specify an Agent authentication type from the command line")
	}

	auth := getAgentAuthType(logger, authType)

	return name, description, auth
}

var agentCreateCmd = &cobra.Command{
	Use:     "create [name] [description] [auth_type]",
	Short:   "Create a new Agent",
	Aliases: []string{"new"},
	Args:    cobra.MaximumNArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		logger := env.NewLogger(cmd)
		theproject := project.EnsureProject(ctx, cmd)
		apikey := theproject.Token
		apiUrl, _, _ := util.GetURLs(logger)

		var remoteAgents []agent.Agent

		if theproject.NewProject {
			var projectId string
			if theproject.Project != nil {
				projectId = theproject.Project.ProjectId
			}
			ShowNewProjectImport(ctx, logger, cmd, apiUrl, apikey, projectId, theproject.Project, theproject.Dir, false)
		} else {
			initScreenWithLogo()
		}

		checkForUpgrade(ctx, logger, false)

		loadTemplates(ctx, cmd)

		var err error
		remoteAgents, err = getAgentList(logger, apiUrl, apikey, theproject)
		if err != nil {
			errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to get agent list")).ShowErrorAndExit()
		}

		var name string
		var description string
		var authType string

		if len(args) > 0 {
			name = args[0]
		}

		if len(args) > 1 {
			description = args[1]
		}

		if len(args) > 2 {
			authType = args[2]
		}

		force, _ := cmd.Flags().GetBool("force")

		// if we have a force flag and a name passed in, delete the existing agent if found
		if force && name != "" {
			for _, a := range remoteAgents {
				if strings.EqualFold(a.Name, name) {
					if _, err := agent.DeleteAgents(ctx, logger, apiUrl, apikey, theproject.Project.ProjectId, []string{a.ID}); err != nil {
						errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to delete existing Agent")).ShowErrorAndExit()
					}
					for i, ea := range theproject.Project.Agents {
						if ea.ID == a.ID {
							theproject.Project.Agents = append(theproject.Project.Agents[:i], theproject.Project.Agents[i+1:]...)
							break
						}
					}
					break
				}
			}
		}

		name, description, authType = getAgentInfoFlow(logger, remoteAgents, name, description, authType)

		action := func() {
			agentID, err := agent.CreateAgent(ctx, logger, apiUrl, apikey, theproject.Project.ProjectId, name, description, authType)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to create Agent")).ShowErrorAndExit()
			}

			tmpdir, _, err := getConfigTemplateDir(cmd)
			if err != nil {
				errsystem.New(errsystem.ErrLoadTemplates, err, errsystem.WithContextMessage("Failed to load templates from directory")).ShowErrorAndExit()
			}

			rules, err := templates.LoadTemplateRuleForIdentifier(tmpdir, theproject.Project.Bundler.Identifier)
			if err != nil {
				errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithAttributes(map[string]any{"identifier": theproject.Project.Bundler.Identifier})).ShowErrorAndExit()
			}

			template, err := templates.LoadTemplateForRuntime(context.Background(), tmpdir, theproject.Project.Bundler.Identifier)
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
				TemplateDir:      tmpdir,
				Template:         template,
				AgentuityCommand: getAgentuityCommand(),
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

			// --- New: ask what the agent should do & generate code ---------------------
			goal, _ := cmd.Flags().GetString("goal")
			codeOptIn, _ := cmd.Flags().GetBool("experimental-code-agent")
			if goal == "" && tui.HasTTY {
				goal = tui.Input(logger, "Describe what the "+name+" Agent should do", "Enter a brief description or objective for the Agent (multi-line supported; hit <enter> on an empty line to finish)")
			}
			if goal != "" && codeOptIn {
				dir := filepath.Join(theproject.Dir, theproject.Project.Bundler.AgentConfig.Dir, util.SafeFilename(name))
				genOpts := codeagent.Options{Dir: dir, Goal: goal, Logger: logger}
				codegenAction := func() {
					if err := codeagent.Generate(ctx, genOpts); err != nil {
						tui.ShowWarning("Agent code generation failed: %s", err)
					}
				}
				tui.ShowSpinner("Crafting Agent code ...", codegenAction)
			}
			// ---------------------------------------------------------------------------
		}
		tui.ShowSpinner("Creating Agent ...", action)

		format, _ := cmd.Flags().GetString("format")
		if format == "json" {
			json.NewEncoder(os.Stdout).Encode(theproject.Project.Agents[len(theproject.Project.Agents)-1])
		} else {
			tui.ShowSuccess("Agent created successfully")
		}

	},
}

type agentListState struct {
	Agent       *agent.Agent `json:"agent"`
	Filename    string       `json:"filename"`
	FoundLocal  bool         `json:"foundLocal"`
	FoundRemote bool         `json:"foundRemote"`
	Rename      bool         `json:"rename"`
	RenameFrom  string       `json:"renameFrom"`
}

func getAgentList(logger logger.Logger, apiUrl string, apikey string, project project.ProjectContext) ([]agent.Agent, error) {
	var remoteAgents []agent.Agent
	var err error
	action := func() {
		remoteAgents, err = agent.ListAgents(context.Background(), logger, apiUrl, apikey, project.Project.ProjectId)
	}
	tui.ShowSpinner("Fetching Agents ...", action)
	return remoteAgents, err
}

func normalAgentName(name string) string {
	return util.SafeFilename(strings.ToLower(name))
}

func reconcileAgentList(logger logger.Logger, cmd *cobra.Command, apiUrl string, apikey string, theproject project.ProjectContext) ([]string, map[string]agentListState) {
	remoteAgents, err := getAgentList(logger, apiUrl, apikey, theproject)
	if err != nil {
		errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to get agent list")).ShowErrorAndExit()
	}

	tmpdir, _, err := getConfigTemplateDir(cmd)
	if err != nil {
		errsystem.New(errsystem.ErrLoadTemplates, err, errsystem.WithContextMessage("Failed to load templates from directory")).ShowErrorAndExit()
	}

	rules, err := templates.LoadTemplateRuleForIdentifier(tmpdir, theproject.Project.Bundler.Identifier)
	if err != nil {
		errsystem.New(errsystem.ErrInvalidConfiguration, err,
			errsystem.WithContextMessage("Failed loading template rule"),
			errsystem.WithAttributes(map[string]any{"identifier": theproject.Project.Bundler.Identifier})).ShowErrorAndExit()
	}

	// make a map of the agents in the agentuity config file
	fileAgents := make(map[string]project.AgentConfig)
	fileAgentsByID := make(map[string]project.AgentConfig)
	for _, agent := range theproject.Project.Agents {
		fileAgents[normalAgentName(agent.Name)] = agent
		fileAgentsByID[agent.ID] = agent
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
		// var found bool
		// for _, agent := range remoteAgents {
		// 	if localAgent, ok := fileAgentsByID[agent.ID]; ok {
		// 		if localAgent.Name == agentName {
		// 			oldkey := normalAgentName(agent.Name)
		// 			agent.Name = localAgent.Name
		// 			state[key] = agentListState{
		// 				Agent:       &agent,
		// 				Filename:    filename,
		// 				FoundLocal:  true,
		// 				FoundRemote: true,
		// 				Rename:      true,
		// 				RenameFrom:  oldkey,
		// 			}
		// 			delete(state, oldkey)
		// 			found = true
		// 			break
		// 		}
		// 	}
		// }
		// if found {
		// 	continue
		// }
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

func buildAgentTree(keys []string, state map[string]agentListState, project project.ProjectContext) (*tree.Tree, int, int, error) {
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
			if st.Rename {
				label += " " + tui.Warning("⚠ Renaming from "+st.RenameFrom)
			}
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
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		project := project.EnsureProject(ctx, cmd)
		apiUrl, _, _ := util.GetURLs(logger)

		// perform the reconcilation
		keys, state := reconcileAgentList(logger, cmd, apiUrl, project.Token, project)

		if len(keys) == 0 {
			tui.ShowWarning("no Agents found")
			tui.ShowBanner("Create a new Agent", tui.Text("Use the ")+tui.Command("agent new")+tui.Text(" command to create a new Agent"), false)
			return
		}

		format, _ := cmd.Flags().GetString("format")
		if format == "json" {
			json.NewEncoder(os.Stdout).Encode(state)
		} else {
			root, localIssues, remoteIssues, err := buildAgentTree(keys, state, project)
			if err != nil {
				errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage("Failed to build agent tree")).ShowErrorAndExit()
			}
			fmt.Println(root)
			if showAgentWarnings(remoteIssues, localIssues, false) {
				os.Exit(1)
			}
		}

	},
}

var agentGetApiKeyCmd = &cobra.Command{
	Use:   "apikey [agent_name]",
	Short: "Get the API key for an agent",
	Long: `Get the API key for an agent by name or ID.

Arguments:
  [agent_name]  The name or ID of the agent to get the API key for

If no agent name is provided, you will be prompted to select an agent.

Examples:
  agentuity agent apikey "My Agent"
  agentuity agent apikey agent_ID
  agentuity agent apikey`,
	Args:    cobra.MaximumNArgs(1),
	Aliases: []string{"key"},
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		project := project.EnsureProject(ctx, cmd)
		apiUrl, _, _ := util.GetURLs(logger)

		// perform the reconcilation
		keys, state := reconcileAgentList(logger, cmd, apiUrl, project.Token, project)

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
				if !tui.HasTTY {
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
		apikey, err := agent.GetApiKey(context.Background(), logger, apiUrl, project.Token, theagent.Agent.ID)
		if err != nil {
			errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithContextMessage("Failed to get agent API key")).ShowErrorAndExit()
		}
		if !tui.HasTTY {
			if apikey != "" {
				fmt.Print(apikey)
				return
			}
		}
		if apikey != "" {
			fmt.Println()
			tui.ShowLock("Agent %s API key: %s", theagent.Agent.Name, apikey)
			tip := fmt.Sprintf(`$(agentuity agent apikey %s)`, agentID)
			tui.ShowBanner("Developer Pro Tip", tui.Paragraph("Fetch your Agent's API key into a shell command dynamically:", tip), false)
			return
		} else {
			tui.ShowWarning("No API key found for Agent %s (%s)", theagent.Agent.Name, theagent.Agent.ID)
		}
		os.Exit(1) // no key
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
		cmd.Flags().String("templates-dir", "", "The directory to load the templates. Defaults to loading them from the github.com/agentuity/templates repository")
	}
	for _, cmd := range []*cobra.Command{agentListCmd, agentCreateCmd} {
		cmd.Flags().String("format", "text", "The format to use for the output. Can be either 'text' or 'json'")
	}
	agentListCmd.Flags().String("org-id", "", "The organization to create the project in on import")
	for _, cmd := range []*cobra.Command{agentCreateCmd, agentDeleteCmd} {
		cmd.Flags().Bool("force", false, "Force the creation of the agent even if it already exists")
	}
	agentCreateCmd.Flags().String("goal", "", "A description of what the agent should do (optional)")
	agentCreateCmd.Flags().Bool("experimental-code-agent", false, "Enable experimental code agent")
}
