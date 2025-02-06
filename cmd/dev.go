package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"regexp"
	"time"

	"github.com/agentuity/cli/internal/provider"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	csys "github.com/agentuity/go-common/sys"
	"github.com/gorilla/websocket"
	"github.com/nxadm/tail"
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

func NewLiveDevConnection(logger logger.Logger, sdkEventsFile string, websocketId string) (*LiveDevConnection, error) {
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
	// ws://localhost:8787/websocket?type=LIVE_DEV&id=oooweee
	// TODO: make this configurable
	u, err := url.Parse("wss://8b06-12-144-206-134.ngrok-free.app/websocket")
	if err != nil {
		return nil, fmt.Errorf("failed to parse url: %s", err)
	}
	u.RawQuery = url.Values{"id": {websocketId}, "type": {"LIVE_DEV"}}.Encode()

	urlString := u.String()

	logger.Debug("dialing %s", urlString)
	self.conn, _, err = websocket.DefaultDialer.Dial(urlString, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %s", err)
	}

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
			default:
				_, message, err := self.conn.ReadMessage()
				if err != nil {
					log.Println("read:", err)
					return
				}
				log.Printf("recv: %s", message)
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

var devRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the development server",
	Run: func(cmd *cobra.Command, args []string) {

		log := env.NewLogger(cmd)

		sdkEventsFile := "events.log"
		dir := resolveProjectDir(log, cmd)
		apiUrl := viper.GetString("overrides.api_url")
		websocketId, _ := cmd.Flags().GetString("websocket-id")

		liveDevConnection, err := NewLiveDevConnection(log, sdkEventsFile, websocketId)
		if err != nil {
			log.Fatal("failed to create live dev connection: %s", err)
		}
		defer liveDevConnection.Close()
		logger := logger.NewMultiLogger(log, logger.NewJSONLoggerWithSink(liveDevConnection, logger.LevelInfo))
		logger.Info("starting development agent 🤖")
		runner, err := provider.NewRunner(logger, dir, apiUrl, sdkEventsFile, args)
		if err != nil {
			logger.Fatal("failed to run development agent: %s", err)
		}
		if err := runner.Start(); err != nil {
			logger.Fatal("failed to start development agent: %s", err)
		}
		// TODO: hook up watch
		for {
			select {
			case <-runner.Done():
				logger.Info("development agent stopped")
				time.Sleep(1 * time.Second)
				os.Exit(0)
			case <-runner.Restart():
				if err := runner.Stop(); err != nil {
					logger.Warn("failed to stop development agent: %s", err)
				}
				if err := runner.Start(); err != nil {
					logger.Fatal("failed to restart development agent: %s", err)
				}
			case <-csys.CreateShutdownChannel():
				if err := runner.Stop(); err != nil {
					logger.Warn("failed to stop development agent: %s", err)
				}
				return
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(devCmd)
	devCmd.AddCommand(devRunCmd)
	devRunCmd.Flags().StringP("dir", "d", ".", "The directory to run the development server in")
	devRunCmd.Flags().String("websocket-id", "", "aaan")
}
