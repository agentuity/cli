package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/agentuity/cli/internal/bundler"
	"github.com/agentuity/cli/internal/dev"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	cstr "github.com/agentuity/go-common/string"
	"github.com/agentuity/go-common/tui"
	"github.com/bep/debounce"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/agentuity/cli/internal/debugagent"
	debugmon "github.com/agentuity/cli/internal/dev/debugmon"

	"github.com/agentuity/cli/internal/dev/linkify"
	"github.com/charmbracelet/glamour"
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

		orgId := project.OrgId

		if websocketId == "" {
			websocketId = cstr.NewHash(orgId, userId)
		}

		port, _ := cmd.Flags().GetInt("port")
		if port == 0 {
			port, err = dev.FindAvailablePort(theproject)
			if err != nil {
				log.Fatal("failed to find available port: %s", err)
			}
		}

		experimentalDebug, _ := cmd.Flags().GetBool("experimental-debug-agent")

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

		processCtx := context.Background()
		var pid int

		projectServerCmd, err := dev.CreateRunProjectCmd(processCtx, log, theproject, websocketConn, dir, orgId, port)
		if err != nil {
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage("Failed to run project")).ShowErrorAndExit()
		}

		var monitorOutChan chan debugmon.ErrorEvent
		if experimentalDebug {
			log.Info("Debug Agent enabled")
			monitorOutChan = make(chan debugmon.ErrorEvent, 8)

			r, w := io.Pipe()
			// Capture only stderr for error monitoring; stdout goes directly to console.
			projectServerCmd.Stdout = os.Stdout
			projectServerCmd.Stderr = io.MultiWriter(os.Stderr, w)

			mon := debugmon.New(log, monitorOutChan)
			go mon.Run(r)

			go func() {
				for evt := range monitorOutChan {
					log.Info("Debug Assist triggered – analysing error …")
					var res debugagent.Result
					var derr error
					var analysis string

					uiAction := func() {
						res, derr = debugagent.Analyze(context.Background(), debugagent.Options{
							Dir:    dir,
							Error:  evt.Raw,
							Logger: log,
						})
						analysis = linkify.LinkifyMarkdown(res.Analysis, dir)
					}
					tui.ShowSpinner("Analyzing error ...", uiAction)
					if derr != nil {
						log.Error("debug assist failed: %s", derr)
						continue
					}
					fmt.Println()
					fmt.Println(tui.Title("Debug Agent Suggestions"))
					fmt.Println()

					// Render markdown nicely using glamour
					renderer, err := glamour.NewTermRenderer(
						glamour.WithAutoStyle(),
						glamour.WithWordWrap(120),
					)
					if err != nil {
						// Fallback to plain output
						fmt.Println(tui.Text(analysis))
					} else {
						rendered, err := renderer.Render(analysis)
						if err != nil {
							fmt.Println(tui.Text(analysis))
						} else {
							fmt.Print(rendered)
						}
					}

					// Ask user if we should attempt an automatic fix
					choice := tui.Select(log, "Attempt automatic fix?", "Choose an option", []tui.Option{
						{ID: "y", Text: "Yes"},
						{ID: "e", Text: "Provide extra guidance then fix"},
						{ID: "n", Text: "No"},
					})

					if choice == "y" || choice == "e" {
						// Compose an extra prompt containing the previous analysis and optional user guidance.
						composeExtra := func(userInput string) string {
							// Limit analysis length to keep prompt compact.
							const maxAnalysis = 4000
							prior := res.Analysis
							if len(prior) > maxAnalysis {
								prior = prior[:maxAnalysis] + "\n...[truncated]"
							}
							if userInput == "" {
								return fmt.Sprintf("Here is the previous analysis you produced (for reference, not to repeat):\n\n%s", prior)
							}
							return fmt.Sprintf("Here is the previous analysis you produced (for reference, not to repeat):\n\n%s\n\nAdditional user guidance:\n\n%s", prior, userInput)
						}

						userGuidance := ""
						if choice == "e" {
							userGuidance = tui.Input(log, "Provide additional guidance", "Describe how to tweak the fix")
						}

						extraPrompt := composeExtra(userGuidance)

						fixAction := func() {
							res, derr = debugagent.Analyze(context.Background(), debugagent.Options{
								Dir:         dir,
								Error:       evt.Raw,
								Extra:       extraPrompt,
								Logger:      log,
								AllowWrites: true,
							})
						}
						tui.ShowSpinner("Applying fix ...", fixAction)
						if derr != nil {
							log.Error("auto-fix failed: %v", derr)
						} else if res.Edited {
							// Suppress monitor for a short period to avoid picking up diff/build noise.
							mon.SuppressFor(5 * time.Second)

							cmd := exec.Command("git", "--no-pager", "-C", dir, "diff", "--color", "--", ".")
							cmd.Stdout = os.Stdout
							cmd.Env = append(os.Environ(), "GIT_PAGER=cat")
							cmd.Run()
						}
					}
					fmt.Println()
				}
			}()
		}

		build := func(initial bool) {
			started := time.Now()
			var ok bool
			tui.ShowSpinner("Building project ...", func() {
				if err := bundler.Bundle(bundler.BundleContext{
					Context:    ctx,
					Logger:     log,
					ProjectDir: dir,
					Production: false,
					DevMode:    !initial,
				}); err != nil {
					if err == bundler.ErrBuildFailed {
						log.Error("build failed ...")
						return
					}
					errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage(fmt.Sprintf("Failed to bundle project: %s", err))).ShowErrorAndExit()
				}
				ok = true
			})
			if ok {
				fmt.Println(tui.Text(fmt.Sprintf("✨ Built in %s", time.Since(started).Round(time.Millisecond))))
			}
		}

		// Initial build
		build(true)

		restart := func() {
			isDeliberateRestart = true
			build(false)
			log.Debug("killing project server")
			dev.KillProjectServer(log, projectServerCmd, pid)
			log.Debug("killing project server done")
		}

		// debounce a lot of changes at once to avoid multiple restarts in succession
		debounced := debounce.New(250 * time.Millisecond)

		// Watch for changes
		watcher, err := dev.NewWatcher(log, dir, theproject.Project.Development.Watch.Files, func(path string) {
			log.Trace("%s has changed", path)
			debounced(restart)
		})
		if err != nil {
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage(fmt.Sprintf("Failed to start watcher: %s", err))).ShowErrorAndExit()
		}
		defer watcher.Close(log)

		if err := projectServerCmd.Start(); err != nil {
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage(fmt.Sprintf("Failed to start project: %s", err))).ShowErrorAndExit()
		}

		pid = projectServerCmd.Process.Pid

		websocketConn.StartReadingMessages(ctx, log, port)
		devUrl := websocketConn.WebURL(appUrl)

		// Display local interaction instructions
		displayLocalInstructions(port, theproject.Project.Agents, devUrl)

		go func() {
			for {
				log.Trace("waiting for project server to exit (pid: %d)", pid)
				if err := projectServerCmd.Wait(); err != nil {
					if !isDeliberateRestart {
						log.Error("project server (pid: %d) exited with error: %s", pid, err)
					}
				}
				if projectServerCmd.ProcessState != nil {
					log.Debug("project server (pid: %d) exited with code %d", pid, projectServerCmd.ProcessState.ExitCode())
				} else {
					log.Debug("project server (pid: %d) exited", pid)
				}
				log.Debug("isDeliberateRestart: %t, pid: %d", isDeliberateRestart, pid)
				if !isDeliberateRestart {
					return
				}

				// If it was a deliberate restart, start the new process here
				if isDeliberateRestart {
					isDeliberateRestart = false
					log.Trace("restarting project server")
					projectServerCmd, err = dev.CreateRunProjectCmd(processCtx, log, theproject, websocketConn, dir, orgId, port)
					if err != nil {
						errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage("Failed to run project")).ShowErrorAndExit()
					}
					if err := projectServerCmd.Start(); err != nil {
						errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage(fmt.Sprintf("Failed to start project: %s", err))).ShowErrorAndExit()
					}
					pid = projectServerCmd.Process.Pid
					log.Trace("restarted project server (pid: %d)", pid)
				}
			}
		}()

		teardown := func() {
			watcher.Close(log)
			websocketConn.Close()
			dev.KillProjectServer(log, projectServerCmd, pid)
		}

		select {
		case <-websocketConn.Done():
			log.Info("live dev connection closed, shutting down")
			teardown()
		case <-ctx.Done():
			log.Info("context done, shutting down")
			teardown()
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

	curlCommand := fmt.Sprintf("curl -v http://127.0.0.1:%d/%s --json '{\"input\": \"Hello, world!\"}'", port, sampleAgentID)

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
	devCmd.Flags().Int("port", 0, "The port to run the development server on (uses project default if not provided)")
	devCmd.Flags().Bool("experimental-debug-agent", false, "Enable LLM-based runtime error assistance")
	devCmd.Flags().MarkHidden("websocket-id")
	devCmd.Flags().MarkHidden("org-id")
}
