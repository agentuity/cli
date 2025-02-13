package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"

	"github.com/agentuity/cli/internal/provider"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"

	csys "github.com/agentuity/go-common/sys"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/nxadm/tail"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var devCmd = &cobra.Command{
	Use:   "dev",
	Short: "Development related commands",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

type LiveDevConnection struct {
	sdkEventsFile string
	websocketId   string
	sdkEventsTail *tail.Tail
	conn          *websocket.Conn
	logQueue      chan []byte
	onMessage     func(message []byte) error
}

var looksLikeJson = regexp.MustCompile(`^\{.*\}$`)
var looksLikeJSONArray = regexp.MustCompile(`^\[.*\]$`)

func decodeEvent(event string) ([]map[string]any, error) {
	if looksLikeJson.MatchString(event) {
		var payload map[string]any
		if err := json.Unmarshal([]byte(event), &payload); err != nil {
			return nil, err
		}
		return []map[string]any{payload}, nil
	}
	if looksLikeJSONArray.MatchString(event) {
		var payload []map[string]any
		if err := json.Unmarshal([]byte(event), &payload); err != nil {
			return nil, err
		}
		return payload, nil
	}
	return nil, fmt.Errorf("event does not look like a JSON object or array")
}

func (c *LiveDevConnection) SetOnMessage(onMessage func(message []byte) error) {
	c.onMessage = onMessage
}

func NewLiveDevConnection(logger logger.Logger, sdkEventsFile string, websocketId string, websocketUrl string, apiKey string) (*LiveDevConnection, error) {
	t, err := tail.TailFile(sdkEventsFile, tail.Config{Follow: true, ReOpen: true, Logger: tail.DiscardingLogger})
	if err != nil {
		return nil, err
	}

	self := LiveDevConnection{
		sdkEventsFile: sdkEventsFile,
		websocketId:   websocketId,
		sdkEventsTail: t,
		logQueue:      make(chan []byte, 100),
	}
	u, err := url.Parse(websocketUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse url: %s", err)
	}
	u.RawQuery = url.Values{"id": {websocketId}, "type": {"LIVE_DEV"}}.Encode()
	u.Path = "/websocket"
	if u.Scheme == "http" {
		u.Scheme = "ws"
	} else if u.Scheme == "https" {
		u.Scheme = "wss"
	}

	urlString := u.String()

	logger.Trace("dialing %s", urlString)
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	var httpResponse *http.Response
	self.conn, httpResponse, err = websocket.DefaultDialer.Dial(urlString, headers)
	if err != nil {
		if httpResponse != nil {
			if httpResponse.StatusCode == 401 {
				logger.Error("invalid api key")
			}
		}
		return nil, fmt.Errorf("failed to dial: %s", err)
	}

	// writer
	go func() {
		for {
			select {
			case jsonLogMessage := <-self.logQueue:
				// TODO: this is a hack to get the log message to the server
				// we should probably use a specific JSON logger for this
				payload := make(map[string]any)
				if err := json.Unmarshal(jsonLogMessage, &payload); err != nil {
					logger.Error("failed to unmarshal log message: %s", err)
					continue
				}
				if err := self.SendMessage(payload, "log"); err != nil {
					logger.Error("failed to send log message: %s", err)
					continue
				}
			case line := <-self.sdkEventsTail.Lines:
				evts, err := decodeEvent(line.Text)
				if err != nil {
					logger.Error("failed to decode event: %s", err)
					continue
				}
				for _, evt := range evts {
					if command, ok := evt["command"].(string); ok {
						if command == "event" {
							command = "session_event"
						}
						if err := self.SendMessage(evt, command); err != nil {
							logger.Error("failed to send event: %s", err)
							continue
						}
					}
				}

			}
		}
	}()

	// reader
	go func() {
		for {
			_, message, err := self.conn.ReadMessage()
			if err != nil {
				logger.Fatal("failed to read message: %s", err)
				return
			}
			logger.Debug("recv: %s", message)
			if self.onMessage == nil {
				logger.Trace("no onMessage handler set, skipping message")
				continue
			}

			if err := self.onMessage(message); err != nil {
				logger.Trace("failed to handle message: %s", err)
			}
		}
	}()

	return &self, nil
}

type Message struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

func (c *LiveDevConnection) SendMessage(payload map[string]any, messageType string) error {
	msg := Message{
		ID:      c.websocketId,
		Type:    messageType,
		Payload: payload,
	}
	buf, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if err := c.conn.WriteMessage(websocket.TextMessage, buf); err != nil {
		return err
	}
	return nil
}

// implements io.Writer to send logs
func (c *LiveDevConnection) Write(jsonLogMessage []byte) (int, error) {
	c.logQueue <- jsonLogMessage
	return len(jsonLogMessage), nil
}

func (c *LiveDevConnection) Close() error {
	if err := c.conn.Close(); err != nil {
		return err
	}
	return c.sdkEventsTail.Stop()
}

func (c *LiveDevConnection) WebURL(appUrl string) string {
	return fmt.Sprintf("%s/developer/live/%s", appUrl, c.websocketId)
}

type InputMessage struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	From    string `json:"from"`
	Payload struct {
		SessionID   string `json:"sessionId"`
		Trigger     string `json:"trigger"`
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

func isInputMessage(message []byte) (*InputMessage, error) {
	var tm InputMessage
	if err := json.Unmarshal(message, &tm); err != nil {
		return nil, err
	}
	return &tm, nil
}

func ReadOutput(logger logger.Logger, outputPath string, sessionId string) (map[string]any, error) {

	//read this file
	content, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, err
	}

	op, err := isOutputPayload(content)

	if err != nil {
		return nil, err
	}
	if op == nil {
		return nil, fmt.Errorf("output is not a valid output payload")
	}

	message := map[string]any{
		"sessionId":   sessionId,
		"contentType": op.ContentType,
		"payload":     op.Payload,
	}
	return message, nil
}

func SaveInput(logger logger.Logger, message []byte) (string, string, error) {
	inputMessage, err := isInputMessage(message)
	if err != nil {
		return "", "", fmt.Errorf("failed to parse input message: %w", err)
	}
	if inputMessage == nil {
		return "", "", fmt.Errorf("message is not a input message")
	}

	payload := inputMessage.Payload
	sessionId := inputMessage.Payload.SessionID

	dir := filepath.Join(os.TempDir(), "agentuity")

	// Ensure directory exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create directory: %w", err)
	}

	// Create input file with state
	inputPath := filepath.Join(dir, sessionId, "input")
	if err := os.MkdirAll(filepath.Dir(inputPath), 0755); err != nil {
		return "", "", fmt.Errorf("failed to create input directory: %w", err)
	}
	logger.Trace("inputPath: %s", inputPath)

	// Marshal payload struct to JSON bytes
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	if err := os.WriteFile(inputPath, payloadBytes, 0644); err != nil {
		return "", "", fmt.Errorf("failed to write input file: %w", err)
	}

	// Create empty output file
	outputPath := filepath.Join(dir, sessionId, "output")
	if err := os.WriteFile(outputPath, []byte{}, 0644); err != nil {
		return "", "", fmt.Errorf("failed to create output file: %w", err)
	}

	logger.Trace("outputPath: %s", outputPath)

	// Export environment variables
	os.Setenv("AGENTUITY_SDK_INPUT_FILE", inputPath)
	os.Setenv("AGENTUITY_SDK_OUTPUT_FILE", outputPath)
	os.Setenv("AGENTUITY_SDK_SESSION_ID", sessionId)

	return outputPath, sessionId, nil
}

var devRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the development server",
	Run: func(cmd *cobra.Command, args []string) {
		log := env.NewLogger(cmd)
		sdkEventsFile := "events.log"
		dir := resolveProjectDir(log, cmd)
		apiUrl := viper.GetString("overrides.api_url")
		appUrl := viper.GetString("overrides.app_url")
		websocketUrl := viper.GetString("overrides.websocket_url")
		websocketId, _ := cmd.Flags().GetString("websocket-id")
		apiKey := viper.GetString("auth.api_key")

		if _, err := os.Stat(sdkEventsFile); err == nil {
			if err := os.Remove(sdkEventsFile); err != nil {
				log.Trace("failed to delete sdkEventsFile: %s", err)
			}
		}

		// get 6 random characters
		if websocketId == "" {
			websocketId = uuid.New().String()[:6]
		}
		liveDevConnection, err := NewLiveDevConnection(log, sdkEventsFile, websocketId, websocketUrl, apiKey)
		if err != nil {
			log.Fatal("failed to create live dev connection: %s", err)
		}
		defer liveDevConnection.Close()
		devUrl := liveDevConnection.WebURL(appUrl)
		log.Info("development server at url: %s", devUrl)

		if err := browser.OpenURL(devUrl); err != nil {
			log.Fatal("failed to open browser: %s", err)

		}
		logger := logger.NewMultiLogger(log, logger.NewJSONLoggerWithSink(liveDevConnection, logger.LevelInfo))

		liveDevConnection.SetOnMessage(func(message []byte) error {
			logger.Trace("recv: %s", message)
			runner, err := provider.NewRunner(logger, dir, apiUrl, sdkEventsFile, args)
			if err != nil {
				logger.Fatal("failed to run development agent: %s", err)
			}
			outputPath, sessionId, err := SaveInput(logger, message)
			if err != nil {
				logger.Error("failed to save input: %s", err)
			}

			if err := runner.Start(); err != nil {
				logger.Fatal("failed to start development agent: %s", err)
			}
			<-runner.Done()

			output, err := ReadOutput(logger, outputPath, sessionId)
			if err != nil {
				logger.Error("failed to read output: %s", err)
				return nil
			}
			if output == nil {
				logger.Error("failed to read output: %s", err)
				return nil
			}

			liveDevConnection.SendMessage(output, "output")

			return nil
		})

		<-csys.CreateShutdownChannel()
	},
}

func init() {
	rootCmd.AddCommand(devCmd)
	devCmd.AddCommand(devRunCmd)
	devRunCmd.Flags().StringP("dir", "d", ".", "The directory to run the development server in")
	devRunCmd.Flags().String("websocket-id", "", "The websocket room id to use for the development agent")
	addURLFlags(devRunCmd)
}
