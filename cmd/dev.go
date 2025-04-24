package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/agentuity/cli/internal/bundler"
	"github.com/agentuity/cli/internal/dev"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	cstr "github.com/agentuity/go-common/string"
	csys "github.com/agentuity/go-common/sys"
	"github.com/agentuity/go-common/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var devCmd = &cobra.Command{
	Use:     "dev",
	Aliases: []string{"run"},
	Args:    cobra.NoArgs,
	Short:   "Run the development server",
	Long: `Run the development server for local testing and development.

This command starts a local development server that connects to the Agentuity Cloud
for live development and testing of your agents. It watches for file changes and
automatically rebuilds your project when changes are detected.

Flags:
  --dir            The directory to run the development server in

Examples:
  agentuity dev
  agentuity dev --dir /path/to/project`,
	Run: func(cmd *cobra.Command, args []string) {
		log := env.NewLogger(cmd)
		_, appUrl, _ := util.GetURLs(log)
		websocketUrl := viper.GetString("overrides.websocket_url")
		websocketId, _ := cmd.Flags().GetString("websocket-id")

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		apiKey, userId := util.EnsureLoggedIn(ctx, log, cmd)
		theproject := project.EnsureProject(ctx, cmd)
		dir := theproject.Dir
		isDeliberateRestart := false

		checkForUpgrade(ctx, log, false)

		if theproject.NewProject {
			var projectId string
			if theproject.Project.ProjectId != "" {
				projectId = theproject.Project.ProjectId
			}
			ShowNewProjectImport(ctx, log, cmd, theproject.APIURL, apiKey, projectId, theproject.Project, dir, false)
		}

		project, err := theproject.Project.GetProject(ctx, log, theproject.APIURL, apiKey)
		if err != nil {
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithUserMessage("Failed to validate project (%s) using the provided API key from the .env file in %s. This is most likely due to the API key being invalid or the project has been deleted.", theproject.Project.ProjectId, dir), errsystem.WithContextMessage(fmt.Sprintf("Failed to get project: %s", err))).ShowErrorAndExit()
		}

		orgId := project.OrgId

		if websocketId == "" {
			websocketId = cstr.NewHash(orgId, userId)
		}

		websocketConn, err := dev.NewWebsocket(dev.WebsocketArgs{
			Ctx:          ctx,
			Logger:       log,
			WebsocketId:  websocketId,
			WebsocketUrl: websocketUrl,
			APIKey:       apiKey,
			Project:      theproject,
			Version:      Version,
			OrgId:        orgId,
		})
		if err != nil {
			log.Fatal("failed to create live dev connection: %s", err)
		}
		defer websocketConn.Close()

		port, err := dev.FindAvailablePort(theproject)
		if err != nil {
			log.Fatal("failed to find available port: %s", err)
		}

		projectServerCmd, err := dev.CreateRunProjectCmd(ctx, log, theproject, websocketConn, dir, orgId, port)
		if err != nil {
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage("Failed to run project")).ShowErrorAndExit()
		}

		build := func() {
			started := time.Now()
			tui.ShowSpinner("Building project ...", func() {
				if err := bundler.Bundle(bundler.BundleContext{
					Context:    ctx,
					Logger:     log,
					ProjectDir: dir,
					Production: false,
				}); err != nil {
					errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage(fmt.Sprintf("Failed to bundle project: %s", err))).ShowErrorAndExit()
				}
			})
			fmt.Println(tui.Text(fmt.Sprintf("✨ Built in %s", time.Since(started).Round(time.Millisecond))))
		}

		// Initial build
		build()

		// Watch for changes
		watcher, err := dev.NewWatcher(log, dir, theproject.Project.Development.Watch.Files, func(path string) {
			build()
			isDeliberateRestart = true
			log.Debug("killing project server")
			dev.KillProjectServer(projectServerCmd)
		})
		if err != nil {
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage(fmt.Sprintf("Failed to start watcher: %s", err))).ShowErrorAndExit()
		}
		defer watcher.Close(log)

		if err := projectServerCmd.Start(); err != nil {
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage(fmt.Sprintf("Failed to start project: %s", err))).ShowErrorAndExit()
		}

		websocketConn.StartReadingMessages(ctx, log, port)
		devUrl := websocketConn.WebURL(appUrl)

		// Display local interaction instructions
		displayLocalInstructions(port, theproject.Project.Agents, devUrl)

		go func() {
			for {
				defer cancel()
				projectServerCmd.Wait()
				log.Debug("project server exited")
				log.Debug("isDeliberateRestart: %t", isDeliberateRestart)
				if !isDeliberateRestart {
					return
				}

				// If it was a deliberate restart, start the new process here
				if isDeliberateRestart {
					isDeliberateRestart = false
					projectServerCmd, err = dev.CreateRunProjectCmd(ctx, log, theproject, websocketConn, dir, orgId, port)
					if err != nil {
						errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage("Failed to run project")).ShowErrorAndExit()
					}
					if err := projectServerCmd.Start(); err != nil {
						errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage(fmt.Sprintf("Failed to start project: %s", err))).ShowErrorAndExit()
					}
				}
			}
		}()

		select {
		case <-websocketConn.Done():
			log.Info("live dev connection closed, shutting down")
			dev.KillProjectServer(projectServerCmd)
			watcher.Close(log)
		case <-ctx.Done():
			log.Info("context done, shutting down")
			websocketConn.Close()
			watcher.Close(log)
		case <-csys.CreateShutdownChannel():
			log.Info("shutdown signal received, shutting down")
			dev.KillProjectServer(projectServerCmd)
			websocketConn.Close()
			watcher.Close(log)
		}
	},
}

func displayLocalInstructions(port int, agents []project.AgentConfig, devModeUrl string) {
	title := tui.Title("🚀 Local Agent Interaction")

	// Combine all elements with appropriate spacing
	fmt.Println()
	fmt.Println(title)

	// Create list of available agents
	if len(agents) > 0 {
		fmt.Println()

		for _, agent := range agents {
			// Display agent name and ID
			fmt.Println(tui.Text("  • ") + tui.PadRight(agent.Name, 20, " ") + " " + tui.Muted(agent.ID))
		}
	}

	// Get a sample agent ID if available
	sampleAgentID := "agent_ID"
	if len(agents) > 0 {
		sampleAgentID = agents[0].ID
	}

	curlCommand := fmt.Sprintf("curl -v http://localhost:%d/%s --json '{\"input\": \"Hello, world!\"}'", port, sampleAgentID)

	fmt.Println()
	fmt.Println(tui.Text("To interact with your agents locally, you can use:"))
	fmt.Println()
	fmt.Println(tui.Highlight(curlCommand))
	fmt.Println()

	fmt.Print(tui.Text("Or use the 💻 Dev Mode in our app: "))
	fmt.Println(tui.Link("%s", devModeUrl))

	fmt.Println()
}

func init() {
	rootCmd.AddCommand(devCmd)
	devCmd.Flags().StringP("dir", "d", ".", "The directory to run the development server in")
	devCmd.Flags().String("websocket-id", "", "The websocket room id to use for the development agent")
	devCmd.Flags().String("org-id", "", "The organization to run the project")
	devCmd.Flags().MarkHidden("websocket-id")
	devCmd.Flags().MarkHidden("org-id")
}
