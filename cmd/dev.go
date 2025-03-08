package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/agentuity/cli/internal/bundler"
	"github.com/agentuity/cli/internal/dev"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/tui"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	csys "github.com/agentuity/go-common/sys"
	"github.com/google/uuid"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type AgentMessage struct {
	Type string `json:"type"`
}

type InputMessage struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	From    string `json:"from"`
	Payload struct {
		SessionID   string `json:"sessionId"`
		Trigger     string `json:"trigger"`
		AgentID     string `json:"agentId"`
		ContentType string `json:"contentType"`
		Payload     string `json:"payload"`
	} `json:"payload"`
}

type OutputPayload struct {
	ContentType string `json:"contentType"`
	Payload     string `json:"payload"`
}

func isOutputPayload(message []byte) (*OutputPayload, error) {
	var op OutputPayload
	if err := json.Unmarshal(message, &op); err != nil {
		return nil, err
	}
	return &op, nil
}

var devRunCmd = &cobra.Command{
	Use:     "run",
	Aliases: []string{"dev"},
	Args:    cobra.NoArgs,
	Short:   "Run the development server",
	Run: func(cmd *cobra.Command, args []string) {
		log := env.NewLogger(cmd)
		sdkEventsFile := "events.log"
		dir := project.ResolveProjectDir(log, cmd)
		_, appUrl := getURLs(log)
		websocketUrl := viper.GetString("overrides.websocket_url")
		websocketId, _ := cmd.Flags().GetString("websocket-id")
		apiKey, _ := util.EnsureLoggedIn()
		theproject := project.EnsureProject(cmd)

		project, err := theproject.Project.GetProject(log, theproject.APIURL, apiKey)
		if err != nil {
			log.Fatal("failed to get project: %s", err)
		}

		orgId := project.OrgId

		if _, err := os.Stat(sdkEventsFile); err == nil {
			if err := os.Remove(sdkEventsFile); err != nil {
				log.Trace("failed to delete sdkEventsFile: %s", err)
			}
		}

		// get 6 random characters
		if websocketId == "" {
			websocketId = uuid.New().String()[:6]
		}

		liveDevConnection, err := dev.NewLiveDevConnection(log, sdkEventsFile, websocketId, websocketUrl, apiKey)
		if err != nil {
			log.Fatal("failed to create live dev connection: %s", err)
		}
		defer liveDevConnection.Close()
		devUrl := liveDevConnection.WebURL(appUrl)
		log.Info("development server at url: %s", devUrl)

		// Display local interaction instructions
		displayLocalInstructions(theproject.Project.Development.Port, theproject.Project.Agents)

		if err := browser.OpenURL(devUrl); err != nil {
			log.Fatal("failed to open browser: %s", err)
		}
		logger := logger.NewMultiLogger(log, logger.NewJSONLoggerWithSink(liveDevConnection, logger.LevelInfo))

		projectServerCmd, err := dev.RunProject(log, theproject, liveDevConnection, dir, orgId)
		if err != nil {
			log.Fatal("failed to run project: %s", err)
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage("Failed to run project")).ShowErrorAndExit()
		}

		started := time.Now()
		if err := bundler.Bundle(bundler.BundleContext{
			Context:    context.Background(),
			Logger:     log,
			ProjectDir: dir,
			Production: false,
		}); err != nil {
			errsystem.New(errsystem.ErrInvalidConfiguration, err, errsystem.WithContextMessage("Failed to bundle project")).ShowErrorAndExit()
		}
		theproject.Logger.Debug("bundled in %s", time.Since(started))

		if err := projectServerCmd.Start(); err != nil {
			log.Fatal("failed to start command: %s", err)
		}

		ctx, cancel := context.WithCancel(context.Background())

		go func() {
			defer cancel()
			projectServerCmd.Wait()
		}()

		agents := make([]map[string]any, 0)
		for _, agent := range theproject.Project.Agents {
			agents = append(agents, map[string]any{
				"name":        agent.Name,
				"id":          agent.ID,
				"description": agent.Description,
			})
		}

		liveDevConnection.SetOnMessage(func(message []byte) error {
			logger.Trace("recv: %s", string(message))

			var agentMessage AgentMessage
			if err := json.Unmarshal(message, &agentMessage); err != nil {
				logger.Error("failed to unmarshal agent message: %s", err)
				return err
			}

			if agentMessage.Type == "getAgents" {
				liveDevConnection.SendMessage(map[string]any{
					"agents": agents,
				}, "agents")
				return nil
			}

			var inputMsg InputMessage
			if err := json.Unmarshal(message, &inputMsg); err != nil {
				logger.Error("failed to unmarshal input message: %s", err)
				return err
			}
			// Decode base64 payload
			decodedPayload, err := base64.StdEncoding.DecodeString(inputMsg.Payload.Payload)
			if err != nil {
				logger.Error("failed to decode payload: %s", err)
				return err
			}

			url := fmt.Sprintf("http://localhost:%d/%s", theproject.Project.Development.Port, inputMsg.Payload.AgentID)

			// make a json object with the payload
			payload := map[string]any{
				"sessionId":   inputMsg.Payload.SessionID,
				"contentType": inputMsg.Payload.ContentType,
				"payload":     decodedPayload,
				"trigger":     "manual",
			}

			jsonPayload, err := json.Marshal(payload)
			if err != nil {
				logger.Error("failed to marshal payload: %s", err)
				return err
			}

			resp, err := http.Post(url, inputMsg.Payload.ContentType, bytes.NewBuffer(jsonPayload))
			if err != nil {
				logger.Error("failed to post to agent: %s", err)
				return err
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				logger.Error("failed to read response body: %s", err)
				return err
			}

			outputPayload, err := isOutputPayload(body)

			if err != nil {
				// print the body as a string if you can
				logger.Error("failed to parse output payload: %s. body was: %s", err, string(body))
				return err
			}

			logger.Trace("response body: %s", string(body))

			liveDevConnection.SendMessage(map[string]any{
				"sessionId":   inputMsg.Payload.SessionID,
				"contentType": outputPayload.ContentType,
				"payload":     outputPayload.Payload,
			}, "output")

			return nil
		})

		select {
		case <-ctx.Done():
			log.Info("context done, shutting down")
		case <-csys.CreateShutdownChannel():
			log.Info("shutdown signal received, shutting down")
			projectServerCmd.Process.Kill()
		}
	},
}

func init() {
	rootCmd.AddCommand(devRunCmd)
	devRunCmd.Flags().StringP("dir", "d", ".", "The directory to run the development server in")
	devRunCmd.Flags().String("websocket-id", "", "The websocket room id to use for the development agent")
}

func displayLocalInstructions(port int, agents []project.AgentConfig) {
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
}
