package dev

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"

	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/go-common/logger"
	"github.com/gorilla/websocket"
)

type LiveDevConnection struct {
	WebSocketId string
	conn        *websocket.Conn
	OtelToken   string
	OtelUrl     string
	Project     project.ProjectContext
	Done        chan struct{}
}

type OutputPayload struct {
	ContentType string `json:"contentType"`
	Payload     string `json:"payload"`
	Trigger     string `json:"trigger"`
}

func isOutputPayload(message []byte) (*OutputPayload, error) {
	var op OutputPayload
	if err := json.Unmarshal(message, &op); err != nil {
		return nil, err
	}
	return &op, nil
}

func (c *LiveDevConnection) StartReadingMessages(logger logger.Logger) {
	go func() {
		defer close(c.Done)
		for {
			_, m, err := c.conn.ReadMessage()
			if err != nil {
				if errors.Is(err, websocket.ErrCloseSent) || errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
					logger.Debug("connection closed")
					return
				}
				logger.Error("failed to read message: %s", err)
				return
			}
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
				processInputMessage(logger, c, m)
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
					Agents: agents,
				})

				c.SendMessage(logger, agentsMessage)
			}
		}
	}()
}

func NewLiveDevConnection(logger logger.Logger, websocketId string, websocketUrl string, apiKey string, theproject project.ProjectContext) (*LiveDevConnection, error) {
	self := LiveDevConnection{
		WebSocketId: websocketId,
		Project:     theproject,
		Done:        make(chan struct{}),
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
func (c *LiveDevConnection) SendMessage(logger logger.Logger, msg Message) error {
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

func (c *LiveDevConnection) Close() error {
	if err := c.conn.Close(); err != nil {
		return err
	}
	return nil
}

func (c *LiveDevConnection) WebURL(appUrl string) string {
	return fmt.Sprintf("%s/live/%s", appUrl, c.WebSocketId)
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
		Payload     string `json:"payload"`
	} `json:"payload"`
}

// messages send by CLI to the server
func NewOutputMessage(id string, payload struct {
	ContentType string `json:"contentType"`
	Payload     string `json:"payload"`
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
	Agents []Agent `json:"agents"`
}

func NewAgentsMessage(id string, payload AgentsPayload) Message {
	payloadMap := map[string]any{
		"agents": payload.Agents,
	}

	return Message{
		ID:      id,
		Type:    "agents",
		Payload: payloadMap,
	}
}

func processInputMessage(logger logger.Logger, c *LiveDevConnection, m []byte) {
	var inputMsg InputMessage
	if err := json.Unmarshal(m, &inputMsg); err != nil {
		logger.Error("failed to unmarshal agent message: %s", err)
		return
	}

	// Decode base64 payload this wont work for images I think
	decodedPayload, err := base64.StdEncoding.DecodeString(inputMsg.Payload.Payload)
	if err != nil {
		logger.Error("failed to decode payload: %s", err)
		return
	}

	c.Project.Logger.Debug("input message: %+v", inputMsg)

	if c.Project.Project.Development == nil {
		logger.Error("development is not enabled for this project")
		return
	}

	url := fmt.Sprintf("http://localhost:%d/%s", c.Project.Project.Development.Port, inputMsg.Payload.AgentID)

	// make a json object with the payload
	payload := map[string]any{
		"contentType": inputMsg.Payload.ContentType,
		"payload":     decodedPayload,
		"trigger":     "manual",
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		logger.Error("failed to marshal payload: %s", err)
		return
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		logger.Error("failed to post to agent: %s", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("failed to read response body: %s", err)
		return
	}

	logger.Debug("response: %s", string(body))

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
