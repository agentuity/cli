package cmd

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/agentuity/cli/internal/dev"
	"github.com/agentuity/cli/internal/gravity"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/run"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/sys"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Hidden: true, // not working yet
	Use:    "run",
	Args:   cobra.NoArgs,
	Short:  "Run the production server",
	Long: `Run the production server for connecting to the Agentuity Cloud.

This command starts a local production server that connects to the Agentuity Cloud
for live routing of your agents to the machine running the server.

Examples:
  agentuity run`,
	Run: func(cmd *cobra.Command, args []string) {
		log := env.NewLogger(cmd)

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		theproject := project.EnsureProject(ctx, cmd)
		dir := theproject.Dir

		buildFolder := filepath.Join(dir, ".agentuity")
		if !sys.Exists(buildFolder) {
			log.Fatal("missing the build folder at %s. make sure you have run agentuity bundle --production", buildFolder)
		}

		var err error
		agentPort, _ := cmd.Flags().GetInt("port")
		agentPort, err = dev.FindAvailablePort(theproject, agentPort)
		if err != nil {
			log.Fatal("failed to find available port: %s", err)
		}
		proxyPort, err := dev.FindAvailableOpenPort()
		if err != nil {
			log.Fatal("failed to find available port: %s", err)
		}

		var envfile []env.EnvLineComment
		if sys.Exists(filepath.Join(dir, ".env.production")) {
			envfile, err = env.ParseEnvFileWithComments(filepath.Join(dir, ".env.production"))
			if err != nil {
				log.Fatal("failed to parse env file: %s", err)
			}
		} else if sys.Exists(filepath.Join(dir, ".env")) {
			envfile, err = env.ParseEnvFileWithComments(filepath.Join(dir, ".env"))
			if err != nil {
				log.Fatal("failed to parse env file: %s", err)
			}
		}

		sdkKey := os.Getenv("AGENTUITY_SDK_KEY")
		if sdkKey == "" {
			for _, line := range envfile {
				if line.Key == "AGENTUITY_SDK_KEY" {
					sdkKey = line.Val
					break
				}
			}
		}
		if sdkKey == "" {
			log.Fatal("missing AGENTUITY_SDK_KEY environment variable")
		}

		for _, line := range envfile {
			os.Setenv(line.Key, line.Val)
		}

		gravityurl, _ := cmd.Flags().GetString("gravity-url")
		transporturl, _ := cmd.Flags().GetString("transport-url")

		var instanceId string

		if sys.Exists("/etc/machine-id") {
			val, _ := os.ReadFile("/etc/machine-id")
			instanceId = strings.TrimSpace(string(val))
		}

		if instanceId == "" {
			instanceId = uuid.New().String()
		}

		client := gravity.New(gravity.Config{
			Context:        ctx,
			Logger:         log,
			Version:        Version,
			Project:        theproject,
			URL:            gravityurl,
			SDKKey:         sdkKey,
			EndpointID:     instanceId,
			ClientName:     "cli/run",
			ProxyPort:      uint(proxyPort),
			AgentPort:      uint(agentPort),
			Ephemeral:      true,
			DynamicProject: true,
		})

		if err := client.Start(); err != nil {
			log.Fatal("failed to start client: %s", err)
		}

		defer client.Close()

		thecmd, err := run.CreateRunProjectCmd(run.Config{
			WorkingDir:      dir,
			Project:         theproject,
			OrgId:           client.OrgID(),
			Context:         ctx,
			Logger:          log,
			TelemetryURL:    client.TelemetryURL(),
			TelemetryAPIKey: client.TelemetryAPIKey(),
			APIURL:          client.APIURL(),
			AgentPort:       agentPort,
			TransportURL:    transporturl,
		})

		if err != nil {
			log.Fatal("failed to create run project command: %s", err)
		}

		if err := thecmd.Start(); err != nil {
			log.Fatal("failed to start run project command: %s", err)
		}

		<-ctx.Done()

		if thecmd.Process != nil {
			log.Trace("sending SIGINT to agent process")
			thecmd.Process.Signal(syscall.SIGINT)
		}

		wctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		go func() {
			defer cancel()
			thecmd.Wait()
			log.Trace("agent exited")
		}()

		select {
		case <-wctx.Done():
		case <-time.After(30 * time.Second):
			log.Trace("agent stop timed out")
			util.ProcessKill(thecmd)
		}
	},
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.Flags().StringP("dir", "d", ".", "The directory to run the server in")
	runCmd.Flags().Int("port", 0, "The port to run the server on (uses project default if not provided)")
	runCmd.Flags().String("gravity-url", "grpc://devmode.agentuity.com", "The URL to the devmode/gravity server")
	runCmd.Flags().String("transport-url", "https://agentuity.ai", "The URL to the transport server")
	runCmd.Flags().MarkHidden("gravity-url")
	runCmd.Flags().MarkHidden("transport-url")
}
