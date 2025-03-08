package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/agentuity/cli/internal/bundler"
	"github.com/agentuity/cli/internal/dev"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/tui"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	csys "github.com/agentuity/go-common/sys"
	"github.com/google/uuid"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var devRunCmd = &cobra.Command{
	Use:     "run",
	Aliases: []string{"dev"},
	Args:    cobra.NoArgs,
	Short:   "Run the development server",
	Run: func(cmd *cobra.Command, args []string) {
		log := env.NewLogger(cmd)
		dir := project.ResolveProjectDir(log, cmd)
		_, appUrl := getURLs(log)
		websocketUrl := viper.GetString("overrides.websocket_url")
		websocketId, _ := cmd.Flags().GetString("websocket-id")
		apiKey, _ := util.EnsureLoggedIn()
		theproject := project.EnsureProject(cmd)

		// get project from api
		project, err := theproject.Project.GetProject(log, theproject.APIURL, apiKey)
		if err != nil {
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage(fmt.Sprintf("Failed to get project: %s", err))).ShowErrorAndExit()
		}
		orgId := project.OrgId

		// need to fixs this!!!!!!!
		if websocketId == "" {
			websocketId = uuid.New().String()[:6]
		}

		liveDevConnection, err := dev.NewLiveDevConnection(log, websocketId, websocketUrl, apiKey, theproject)
		if err != nil {
			log.Fatal("failed to create live dev connection: %s", err)
		}
		defer liveDevConnection.Close()

		projectServerCmd, err := dev.CreateRunProjectCmd(log, theproject, liveDevConnection, dir, orgId)
		if err != nil {
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage("Failed to run project")).ShowErrorAndExit()
		}

		fmt.Println(tui.Text(fmt.Sprintf("🔨 Building project...")))
		started := time.Now()
		if err := bundler.Bundle(bundler.BundleContext{
			Context:    context.Background(),
			Logger:     log,
			ProjectDir: dir,
			Production: false,
		}); err != nil {
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage(fmt.Sprintf("Failed to bundle project: %s", err))).ShowErrorAndExit()
		}

		theproject.Logger.Debug("bundled in %s", time.Since(started))

		if err := projectServerCmd.Start(); err != nil {
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage(fmt.Sprintf("Failed to start project: %s", err))).ShowErrorAndExit()
		}

		liveDevConnection.StartReadingMessages(log)
		devUrl := liveDevConnection.WebURL(appUrl)

		// Display local interaction instructions
		displayLocalInstructions(theproject.Project.Development.Port, theproject.Project.Agents, devUrl)

		if err := browser.OpenURL(devUrl); err != nil {
			log.Fatal("failed to open browser: %s", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			defer cancel()
			projectServerCmd.Wait()
		}()

		select {
		case <-ctx.Done():
			log.Info("context done, shutting down")
			liveDevConnection.Close()
		case <-csys.CreateShutdownChannel():
			log.Info("shutdown signal received, shutting down")
			projectServerCmd.Process.Kill()
			liveDevConnection.Close()
		}
	},
}

func init() {
	rootCmd.AddCommand(devRunCmd)
	devRunCmd.Flags().StringP("dir", "d", ".", "The directory to run the development server in")
	devRunCmd.Flags().String("websocket-id", "", "The websocket room id to use for the development agent")
}

func displayLocalInstructions(port int, agents []project.AgentConfig, devModeUrl string) {
	title := tui.Title("🚀 Local Agent Interaction")

	// Combine all elements with appropriate spacing
	fmt.Println()
	fmt.Println(title)

	// Create list of available agents
	if len(agents) > 0 {
		fmt.Println()
		fmt.Println(tui.Bold("Available agents:"))

		for _, agent := range agents {
			// Display agent name and ID
			fmt.Println(tui.Text("  • " + agent.Name))
			fmt.Println(tui.Secondary("    ID: " + agent.ID))
		}
	}

	// Get a sample agent ID if available
	sampleAgentID := "agent_ID"
	if len(agents) > 0 {
		sampleAgentID = agents[0].ID
	}

	curlCommand := fmt.Sprintf("curl -v http://localhost:%d/run/%s --json '{\"input\": \"Hello, world!\"}'", port, sampleAgentID)

	fmt.Println()
	fmt.Println(tui.Text("To interact with your agents locally, you can use:"))
	fmt.Println(tui.Command(curlCommand))
	fmt.Println()

	fmt.Print(tui.Text("Or use the 💻 Live Mode in our app: "))
	fmt.Println(tui.Link("%s", devModeUrl))

	fmt.Println()
}
