package dev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/go-common/logger"
	"github.com/cenkalti/backoff/v4"
	"github.com/gorilla/websocket"
)

type Websocket struct {
	WebSocketId  string
	conn         *websocket.Conn
	OtelToken    string
	OtelUrl      string
	Project      project.ProjectContext
	Done         chan struct{}
	apiKey       string
	websocketUrl string
	maxRetries   int
	retryCount   int
}

type OutputPayload struct {
	ContentType string `json:"contentType"`
	Payload     []byte `json:"payload"`
	Trigger     string `json:"trigger"`
}

func isOutputPayload(message []byte) (*OutputPayload, error) {
	var op OutputPayload
	if err := json.Unmarshal(message, &op); err != nil {
		return nil, err
	}
	return &op, nil
}

func isContextCanceled(ctx context.Context, err error) bool {
	if errors.Is(err, context.Canceled) {
		return true
	}
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func (c *Websocket) StartReadingMessages(ctx context.Context, logger logger.Logger, port int) {
	go func() {
		defer close(c.Done)
		for {
			_, m, err := c.conn.ReadMessage()
			if err != nil {
				if isContextCanceled(ctx, err) {
					logger.Debug("shutdown in progress, exiting")
					return
				}
				if errors.Is(err, websocket.ErrCloseSent) || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
					logger.Debug("connection closed")
					if c.retryCount < c.maxRetries {
						logger.Info("attempting to reconnect, retry %d of %d", c.retryCount+1, c.maxRetries)
						if err := c.reconnect(logger); err != nil {
							logger.Error("failed to reconnect: %s", err)
							c.retryCount++
							continue
						}
						c.retryCount = 0
						continue
					}
					return
				}
				logger.Error("failed to read message: %s", err)
				if c.retryCount < c.maxRetries {
					logger.Info("attempting to reconnect, retry %d of %d", c.retryCount+1, c.maxRetries)
					if err := c.reconnect(logger); err != nil {
						logger.Error("failed to reconnect: %s", err)
						c.retryCount++
						continue
					}
					c.retryCount = 0
					continue
				}
				return
			}
			// Reset retry count on successful message
			c.retryCount = 0

			logger.Trace("recv: %s", string(m))

			var message Message
			if err := json.Unmarshal(m, &message); err != nil {
				logger.Error("failed to unmarshal agent message: %s", err)
				return
			}

			if message.Type == "input" {
				var inputMsg InputMessage
				if err := json.Unmarshal(m, &inputMsg); err != nil {
					logger.Error("failed to unmarshal agent message: %s", err)
					return
				}
				processInputMessage(logger, c, m, port)
			}
			if message.Type == "getAgents" {
				agents := make([]Agent, 0)
				for _, agent := range c.Project.Project.Agents {
					agents = append(agents, Agent{
						Name:        agent.Name,
						ID:          agent.ID,
						Description: agent.Description,
					})
				}
				logger.Trace("sending agents: %+v", agents)

				agentsMessage := NewAgentsMessage(c.WebSocketId, AgentsPayload{
					ProjectID:   c.Project.Project.ProjectId,
					ProjectName: c.Project.Project.Name,
					Agents:      agents,
				})

				c.SendMessage(logger, agentsMessage)
			}
		}
	}()
}

func (c *Websocket) reconnect(logger logger.Logger) error {
	// Close existing connection if it exists
	if c.conn != nil {
		_ = c.conn.Close()
	}

	u, err := url.Parse(c.websocketUrl)
	if err != nil {
		return fmt.Errorf("failed to parse url: %s", err)
	}
	u.Path = fmt.Sprintf("/websocket/devmode/%s", c.WebSocketId)
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
	headers.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

	// connect to the websocket
	logger.Trace("reconnecting to %s", urlString)
	var httpResponse *http.Response
	c.conn, httpResponse, err = websocket.DefaultDialer.Dial(urlString, headers)
	if err != nil {
		if httpResponse != nil {
			if httpResponse.StatusCode == 401 {
				logger.Error("invalid api key")
			}
		}
		return fmt.Errorf("failed to dial: %s", err)
	}

	// get the otel token and url from the headers
	c.OtelToken = httpResponse.Header.Get("X-AGENTUITY-OTLP-BEARER-TOKEN")
	if c.OtelToken == "" {
		return errsystem.New(errsystem.ErrAuthenticateOtelServer, nil, errsystem.WithUserMessage("Failed to authenticate with otel server"))
	}
	c.OtelUrl = httpResponse.Header.Get("X-AGENTUITY-OTLP-URL")
	if c.OtelUrl == "" {
		return errsystem.New(errsystem.ErrAuthenticateOtelServer, nil, errsystem.WithUserMessage("Failed to get otel server url"))
	}

	logger.Info("successfully reconnected")
	return nil
}

func NewWebsocket(logger logger.Logger, websocketId string, websocketUrl string, apiKey string, theproject project.ProjectContext) (*Websocket, error) {
	self := Websocket{
		WebSocketId:  websocketId,
		Project:      theproject,
		Done:         make(chan struct{}),
		apiKey:       apiKey,
		websocketUrl: websocketUrl,
		maxRetries:   5,
		retryCount:   0,
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

	return &self, nil
}

// Update SendMessage to accept the MessageType interface
func (c *Websocket) SendMessage(logger logger.Logger, msg Message) error {
	logger.Trace("sending message: %+v", msg)
	buf, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	if err := c.conn.WriteMessage(websocket.TextMessage, buf); err != nil {
		return err
	}
	return nil
}

func (c *Websocket) Close() error {
	if err := c.conn.Close(); err != nil {
		return err
	}
	return nil
}

func (c *Websocket) WebURL(appUrl string) string {
	return fmt.Sprintf("%s/devmode/%s", appUrl, c.WebSocketId)
}

type Message struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

// messages send by server to CLI
type InputMessage struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	From    string `json:"from"`
	Payload struct {
		SessionID   string `json:"sessionId"`
		Trigger     string `json:"trigger"`
		AgentID     string `json:"agentId"`
		ContentType string `json:"contentType"`
		Payload     []byte `json:"payload"`
	} `json:"payload"`
}

// messages send by CLI to the server
func NewOutputMessage(id string, payload struct {
	ContentType string `json:"contentType"`
	Payload     []byte `json:"payload"`
	Trigger     string `json:"trigger"`
}) Message {
	payloadMap := map[string]any{
		"contentType": payload.ContentType,
		"payload":     payload.Payload,
		"trigger":     payload.Trigger,
	}
	return Message{
		ID:      id,
		Type:    "output",
		Payload: payloadMap,
	}

}

type Agent struct {
	Name        string `json:"name"`
	ID          string `json:"id"`
	Description string `json:"description"`
}

type AgentsPayload struct {
	Agents      []Agent `json:"agents"`
	ProjectID   string  `json:"projectId"`
	ProjectName string  `json:"projectName"`
}

func NewAgentsMessage(id string, payload AgentsPayload) Message {
	payloadMap := map[string]any{
		"agents":      payload.Agents,
		"projectId":   payload.ProjectID,
		"projectName": payload.ProjectName,
	}

	return Message{
		ID:      id,
		Type:    "agents",
		Payload: payloadMap,
	}
}

func processInputMessage(logger logger.Logger, c *Websocket, m []byte, port int) {
	var inputMsg InputMessage
	if err := json.Unmarshal(m, &inputMsg); err != nil {
		logger.Error("failed to unmarshal agent message: %s", err)
		return
	}

	c.Project.Logger.Debug("input message: %+v", inputMsg)

	if c.Project.Project.Development == nil {
		logger.Error("development is not enabled for this project")
		return
	}

	url := fmt.Sprintf("http://localhost:%d/%s", port, inputMsg.Payload.AgentID)

	// make a json object with the payload
	payload := map[string]any{
		"contentType": inputMsg.Payload.ContentType,
		"payload":     inputMsg.Payload.Payload,
		"trigger":     "manual",
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		logger.Error("failed to marshal payload: %s", err)
		return
	}
	logger.Debug("sending payload: %s to %s", string(jsonPayload), url)

	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 500 * time.Millisecond
	expBackoff.MaxInterval = 5 * time.Second
	expBackoff.MaxElapsedTime = 30 * time.Second // Max total time as requested
	expBackoff.Multiplier = 2.0
	expBackoff.RandomizationFactor = 0.3 // Add jitter

	var resp *http.Response
	operation := func() error {
		var err error
		resp, err = http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				logger.Warn("connection timeout to agent, retrying...")
				return err
			}
			if strings.Contains(err.Error(), "connection refused") {
				logger.Warn("connection refused to agent, retrying...")
				return err
			}
			logger.Error("failed to post to agent: %s", err)
			return backoff.Permanent(err)
		}
		return nil
	}

	err = backoff.Retry(operation, expBackoff)
	if err != nil {
		logger.Error("all attempts to post to agent failed: %s", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("failed to read response body: %s", err)
		return
	}

	logger.Debug("response: %s (status code: %d)", string(body), resp.StatusCode)

	output, err := isOutputPayload(body)
	if err != nil {
		logger.Error("failed to check if response is output payload: %s", err)
		return
	}

	outputMessage := NewOutputMessage(inputMsg.ID, OutputPayload{
		ContentType: output.ContentType,
		Payload:     output.Payload,
		Trigger:     output.Trigger,
	})

	if err := c.SendMessage(logger, outputMessage); err != nil {
		logger.Error("failed to send output message: %s", err)
	}
}
