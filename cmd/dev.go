package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/agentuity/cli/internal/bundler"
	"github.com/agentuity/cli/internal/dev"
	"github.com/agentuity/cli/internal/envutil"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/tui"
	"github.com/bep/debounce"
	"github.com/spf13/cobra"
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
		logLevel := env.LogLevel(cmd)
		apiUrl, appUrl, transportUrl := util.GetURLs(log)

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
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithUserMessage("Failed to validate project (%s) using the provided API key from the .env file in %s. This is most likely due to the API key being invalid or the project has been deleted.\n\nYou can import this project using the following command:\n\n"+tui.Command("project import"), theproject.Project.ProjectId, dir), errsystem.WithContextMessage(fmt.Sprintf("Failed to get project: %s", err))).ShowErrorAndExit()
		}

		force, _ := cmd.Flags().GetBool("force")
		if !tui.HasTTY {
			force = true
		}
		_, project = envutil.ProcessEnvFiles(ctx, log, dir, theproject.Project, project, theproject.APIURL, apiKey, force)

		orgId := project.OrgId

		port, _ := cmd.Flags().GetInt("port")
		port, err = dev.FindAvailablePort(theproject, port)
		if err != nil {
			log.Fatal("failed to find available port: %s", err)
		}

		serverAddr, _ := cmd.Flags().GetString("server")

		if strings.Contains(apiUrl, "agentuity.io") && !strings.Contains(serverAddr, "localhost") {
			serverAddr = "localhost:12001"
		}

		server, err := dev.New(dev.ServerArgs{
			Ctx:          ctx,
			Logger:       log,
			LogLevel:     logLevel,
			APIURL:       apiUrl,
			TransportURL: transportUrl,
			APIKey:       apiKey,
			OrgId:        orgId,
			Project:      theproject,
			Version:      Version,
			UserId:       userId,
			Port:         port,
			ServerAddr:   serverAddr,
		})
		if err != nil {
			log.Fatal("failed to create live dev connection: %s", err)
		}
		defer server.Close()

		processCtx := context.Background()
		var pid int

		consoleUrl := server.WebURL(appUrl)
		devModeUrl := fmt.Sprintf("http://127.0.0.1:%d", port)

		agents := make([]*dev.Agent, 0)
		for _, agent := range theproject.Project.Agents {
			agents = append(agents, &dev.Agent{
				ID:       agent.ID,
				Name:     agent.Name,
				LocalURL: fmt.Sprintf("%s/%s", devModeUrl, agent.ID),
			})
		}

		ui := dev.NewDevModeUI(ctx, dev.DevModeConfig{
			DevModeUrl: devModeUrl,
			AppUrl:     consoleUrl,
			Agents:     agents,
		})

		ui.Start()

		defer ui.Close(false)

		tuiLogger := dev.NewTUILogger(logLevel, ui)

		if err := server.Connect(ui, tuiLogger); err != nil {
			log.Error("failed to start live dev connection: %s", err)
			ui.Close(true)
			return
		}

		publicUrl := server.PublicURL(appUrl)
		ui.SetPublicURL(publicUrl)

		for _, agent := range agents {
			agent.PublicURL = fmt.Sprintf("%s/%s", publicUrl, agent.ID)
		}

		projectServerCmd, err := dev.CreateRunProjectCmd(processCtx, tuiLogger, theproject, server, dir, orgId, port, tuiLogger)
		if err != nil {
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage("Failed to run project")).ShowErrorAndExit()
		}

		build := func(initial bool) bool {
			started := time.Now()
			var ok bool
			ui.ShowSpinner("Building project ...", func() {
				var w io.Writer = tuiLogger
				if err := bundler.Bundle(bundler.BundleContext{
					Context:    ctx,
					Logger:     tuiLogger,
					ProjectDir: dir,
					Production: false,
					DevMode:    true,
					Writer:     w,
				}); err != nil {
					if err == bundler.ErrBuildFailed {
						return
					}
					ui.Close(true)
					errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage(fmt.Sprintf("Failed to bundle project: %s", err))).ShowErrorAndExit()
				}
				ok = true
			})
			if ok && !initial {
				ui.SetStatusMessage("✨ Built in %s", time.Since(started).Round(time.Millisecond))
			}
			return ok
		}

		// Initial build must exit if it fails
		if !build(true) {
			ui.Close(true)
			return
		}

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

		tuiLogger.Trace("starting project server")
		if err := projectServerCmd.Start(); err != nil {
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage(fmt.Sprintf("Failed to start project: %s", err))).ShowErrorAndExit()
		}

		pid = projectServerCmd.Process.Pid
		tuiLogger.Trace("started project server with pid: %d", pid)

		if err := server.HealthCheck(devModeUrl); err != nil {
			tuiLogger.Error("failed to health check connection: %s", err)
			dev.KillProjectServer(tuiLogger, projectServerCmd, pid)
			ui.Close(true)
			return
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
	devCmd.Flags().Int("port", 0, "The port to run the development server on (uses project default if not provided)")
	devCmd.Flags().String("server", "echo.agentuity.cloud", "the echo server to connect to")
	devCmd.Flags().MarkHidden("server")
	devCmd.Flags().Bool("force", false, "Force the processing of environment files")
}
