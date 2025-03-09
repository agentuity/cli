package dev

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"

	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/go-common/logger"
	"github.com/gorilla/websocket"
	"github.com/nxadm/tail"
)

type LiveDevConnection struct {
	sdkEventsFile string
	WebSocketId   string
	sdkEventsTail *tail.Tail
	conn          *websocket.Conn
	logQueue      chan []byte
	onMessage     func(message []byte) error
	OtelToken     string
	OtelUrl       string
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
		WebSocketId:   websocketId,
		sdkEventsTail: t,
		logQueue:      make(chan []byte, 100),
	}
	u, err := url.Parse(websocketUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse url: %s", err)
	}
	u.Path = fmt.Sprintf("/websocket/devmode/%s", websocketId)
	u.RawQuery = url.Values{
		"from": []string{"cli"},
	}.Encode()

	if u.Scheme == "http" {
		u.Scheme = "ws"
	} else if u.Scheme == "https" {
		u.Scheme = "wss"
	}

	urlString := u.String()
	headers := http.Header{}
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	// connect to the websocket
	logger.Trace("dialing %s", urlString)
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

	// get the otel token and url from the headers
	self.OtelToken = httpResponse.Header.Get("X-AGENTUITY-OTLP-BEARER-TOKEN")
	if self.OtelToken == "" {
		errsystem.New(errsystem.ErrAuthenticateOtelServer, nil, errsystem.WithUserMessage("Failed to authenticate with otel server"))
	}
	self.OtelUrl = httpResponse.Header.Get("X-AGENTUITY-OTLP-URL")
	if self.OtelUrl == "" {
		errsystem.New(errsystem.ErrAuthenticateOtelServer, nil, errsystem.WithUserMessage("Failed to get otel server url"))
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
				if errors.Is(err, websocket.ErrCloseSent) || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
					logger.Trace("connection closed")
					return
				}
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
		ID:      c.WebSocketId,
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
	return fmt.Sprintf("%s/live/%s", appUrl, c.WebSocketId)
}
