package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/agentuity/cli/internal/bundler"
	"github.com/agentuity/cli/internal/dev"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/bridge"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/tui"
	"github.com/bep/debounce"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"
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
		fd := int(os.Stdin.Fd())
		oldState, err := term.GetState(fd)
		if err != nil {
			panic(err)
		}
		defer term.Restore(fd, oldState)

		log := env.NewLogger(cmd)
		logLevel := env.LogLevel(cmd)
		apiUrl, appUrl, transportUrl := util.GetURLs(log)

		signals := []os.Signal{os.Interrupt, syscall.SIGINT}
		if runtime.GOOS != "windows" {
			signals = append(signals, syscall.SIGTERM)
		}

		ctx, cancel := signal.NotifyContext(context.Background(), signals...)
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
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithUserMessage("Failed to validate project (%s) using the provided API key from the .env file in %s. This is most likely due to the API key being invalid or the project has been deleted.\n\nYou can import this project using the following command:\n\n"+tui.Command("project import"), theproject.Project.ProjectId, dir), errsystem.WithContextMessage(fmt.Sprintf("Failed to get project: %s", err))).ShowErrorAndExit()
		}

		projectToken := os.Getenv("AGENTUITY_API_KEY")
		if projectToken == "" {
			envFile := filepath.Join(dir, ".env")
			if util.Exists(envFile) {
				envs, err := env.ParseEnvFile(envFile)
				if err != nil {
					log.Fatal("failed to parse .env file: %s", err)
				}
				for _, kv := range envs {
					if kv.Key == "AGENTUITY_API_KEY" {
						projectToken = kv.Val
						break
					}
				}
			}
		}

		if projectToken == "" {
			log.Fatal("failed to find AGENTUITY_API_KEY in .env file or system environment variable")
		}

		orgId := project.OrgId

		port, _ := cmd.Flags().GetInt("port")
		if port == 0 {
			port, err = dev.FindAvailablePort(theproject)
			if err != nil {
				log.Fatal("failed to find available port: %s", err)
			}
		}

		var connection *bridge.BridgeConnectionInfo

		settings := viper.Get("devmode." + orgId)
		if val, ok := settings.(map[string]any); ok {
			connection = &bridge.BridgeConnectionInfo{}
			for k, v := range val {
				switch k {
				case "expires_at":
					if val, ok := v.(string); ok {
						expiresAt, err := time.Parse(time.RFC3339, val)
						if err != nil {
							log.Fatal("failed to parse expires_at: %s", err)
						}
						connection.ExpiresAt = &expiresAt
					}
				case "websocket_url":
					if val, ok := v.(string); ok {
						connection.WebsocketURL = val
					}
				case "stream_url":
					if val, ok := v.(string); ok {
						connection.StreamURL = val
					}
				case "client_url":
					if val, ok := v.(string); ok {
						connection.ClientURL = val
					}
				case "replies_url":
					if val, ok := v.(string); ok {
						connection.RepliesURL = val
					}
				case "refresh_url":
					if val, ok := v.(string); ok {
						connection.RefreshURL = val
					}
				case "control_url":
					if val, ok := v.(string); ok {
						connection.ControlURL = val
					}
				}
			}
		}

		server, err := dev.New(dev.ServerArgs{
			Ctx:          ctx,
			Logger:       log,
			LogLevel:     logLevel,
			APIURL:       apiUrl,
			TransportURL: transportUrl,
			APIKey:       apiKey,
			ProjectToken: projectToken,
			Project:      theproject,
			Version:      Version,
			OrgId:        orgId,
			UserId:       userId,
			Connection:   connection,
			Port:         port,
		})
		if err != nil {
			log.Fatal("failed to create live dev connection: %s", err)
		}
		defer server.Close()

		processCtx := context.Background()
		var pid int

		consoleUrl := server.WebURL(appUrl)
		publicUrl := server.PublicURL(appUrl)
		devModeUrl := fmt.Sprintf("http://127.0.0.1:%d", port)

		agents := make([]dev.Agent, 0)
		for _, agent := range theproject.Project.Agents {
			agents = append(agents, dev.Agent{
				ID:        agent.ID,
				Name:      agent.Name,
				LocalURL:  fmt.Sprintf("%s/%s", devModeUrl, agent.ID),
				PublicURL: fmt.Sprintf("%s/%s", publicUrl, agent.ID),
			})
		}

		ui := dev.NewDevModeUI(ctx, dev.DevModeConfig{
			DevModeUrl: devModeUrl,
			PublicUrl:  publicUrl,
			AppUrl:     consoleUrl,
			Agents:     agents,
		})

		ui.Start()

		defer ui.Close()

		tuiLogger := dev.NewTUILogger(logLevel, ui)

		if err := server.Connect(ui, tuiLogger); err != nil {
			ui.Close()
			tuiLogger.Fatal("failed to start live dev connection: %s", err)
		}

		projectServerCmd, err := dev.CreateRunProjectCmd(processCtx, tuiLogger, theproject, server, dir, orgId, port, tuiLogger)
		if err != nil {
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage("Failed to run project")).ShowErrorAndExit()
		}

		build := func(initial bool) {
			started := time.Now()
			var ok bool
			ui.ShowSpinner("Building project ...", func() {
				if err := bundler.Bundle(bundler.BundleContext{
					Context:    ctx,
					Logger:     tuiLogger,
					ProjectDir: dir,
					Production: false,
					DevMode:    !initial,
				}); err != nil {
					if err == bundler.ErrBuildFailed {
						tuiLogger.Error("build failed ...")
						return
					}
					errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage(fmt.Sprintf("Failed to bundle project: %s", err))).ShowErrorAndExit()
				}
				ok = true
			})
			if ok && !initial {
				ui.SetStatusMessage("✨ Built in %s", time.Since(started).Round(time.Millisecond))
			}
		}

		// Initial build
		build(true)

		restart := func() {
			isDeliberateRestart = true
			build(false)
			tuiLogger.Debug("killing project server")
			dev.KillProjectServer(tuiLogger, projectServerCmd, pid)
			tuiLogger.Debug("killing project server done")
		}

		ui.SetStatusMessage("starting ...")
		ui.SetSpinner(true)

		// debounce a lot of changes at once to avoid multiple restarts in succession
		debounced := debounce.New(250 * time.Millisecond)

		// Watch for changes
		watcher, err := dev.NewWatcher(tuiLogger, dir, theproject.Project.Development.Watch.Files, func(path string) {
			tuiLogger.Trace("%s has changed", path)
			debounced(restart)
		})
		if err != nil {
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage(fmt.Sprintf("Failed to start watcher: %s", err))).ShowErrorAndExit()
		}
		defer watcher.Close(tuiLogger)

		if err := projectServerCmd.Start(); err != nil {
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage(fmt.Sprintf("Failed to start project: %s", err))).ShowErrorAndExit()
		}

		pid = projectServerCmd.Process.Pid

		if err := server.HealthCheck(devModeUrl); err != nil {
			ui.Close()
			tuiLogger.Fatal("failed to health check connection: %s", err)
		}

		ui.SetStatusMessage("🚀 DevMode ready")
		ui.SetSpinner(false)

		go func() {
			for {
				tuiLogger.Trace("waiting for project server to exit (pid: %d)", pid)
				if err := projectServerCmd.Wait(); err != nil {
					if !isDeliberateRestart {
						tuiLogger.Error("project server (pid: %d) exited with error: %s", pid, err)
					}
				}
				if projectServerCmd.ProcessState != nil {
					tuiLogger.Debug("project server (pid: %d) exited with code %d", pid, projectServerCmd.ProcessState.ExitCode())
				} else {
					tuiLogger.Debug("project server (pid: %d) exited", pid)
				}
				tuiLogger.Debug("isDeliberateRestart: %t, pid: %d", isDeliberateRestart, pid)
				if !isDeliberateRestart {
					return
				}

				// If it was a deliberate restart, start the new process here
				if isDeliberateRestart {
					isDeliberateRestart = false
					tuiLogger.Trace("restarting project server")
					projectServerCmd, err = dev.CreateRunProjectCmd(processCtx, tuiLogger, theproject, server, dir, orgId, port, tuiLogger)
					if err != nil {
						errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage("Failed to run project")).ShowErrorAndExit()
					}
					if err := projectServerCmd.Start(); err != nil {
						errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage(fmt.Sprintf("Failed to start project: %s", err))).ShowErrorAndExit()
					}
					pid = projectServerCmd.Process.Pid
					tuiLogger.Trace("restarted project server (pid: %d)", pid)
				}
			}
		}()

		teardown := func() {
			watcher.Close(tuiLogger)
			server.Close()
			dev.KillProjectServer(tuiLogger, projectServerCmd, pid)
		}

		select {
		case <-ui.Done():
			teardown()
		case <-ctx.Done():
			teardown()
		}
	},
}

func init() {
	rootCmd.AddCommand(devCmd)
	devCmd.Flags().StringP("dir", "d", ".", "The directory to run the development server in")
	devCmd.Flags().String("org-id", "", "The organization to run the project")
	devCmd.Flags().Int("port", 0, "The port to run the development server on (uses project default if not provided)")
	devCmd.Flags().MarkHidden("org-id")
}
