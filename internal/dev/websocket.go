package dev

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
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
}

func (c *LiveDevConnection) StartReadingMessages(logger logger.Logger) {
	go func() {
		for {
			_, m, err := c.conn.ReadMessage()
			if err != nil {
				logger.Fatal("failed to read message: %s", err)
				return
			}
			logger.Debug("recv: %s", m)

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
				agents := make([]struct {
					Name        string `json:"name"`
					ID          string `json:"id"`
					Description string `json:"description"`
				}, 0)
				for _, agent := range c.Project.Project.Agents {
					agents = append(agents, struct {
						Name        string `json:"name"`
						ID          string `json:"id"`
						Description string `json:"description"`
					}{
						Name:        agent.Name,
						ID:          agent.ID,
						Description: agent.Description,
					})
				}
				c.SendMessage(NewAgentsMessage(c.WebSocketId, struct {
					Agents []struct {
						Name        string `json:"name"`
						ID          string `json:"id"`
						Description string `json:"description"`
					} `json:"agents"`
				}{
					Agents: agents,
				}))
			}
		}
	}()
}

func NewLiveDevConnection(logger logger.Logger, websocketId string, websocketUrl string, apiKey string, theproject project.ProjectContext) (*LiveDevConnection, error) {
	self := LiveDevConnection{
		WebSocketId: websocketId,
		Project:     theproject,
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

	// Start reading messages
	return &self, nil
}

// Update SendMessage to accept the MessageType interface
func (c *LiveDevConnection) SendMessage(msg Message) error {
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
	ID      string
	Type    string
	Payload map[string]any
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
	ContentType string `json:"content_type"`
	Payload     string `json:"payload"`
}) Message {
	payloadMap := map[string]any{
		"content_type": payload.ContentType,
		"payload":      payload.Payload,
	}
	return Message{
		ID:      id,
		Type:    "output",
		Payload: payloadMap,
	}
}

func NewAgentsMessage(id string, payload struct {
	Agents []struct {
		Name        string `json:"name"`
		ID          string `json:"id"`
		Description string `json:"description"`
	} `json:"agents"`
}) Message {
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

	// Decode base64 payload
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
		"sessionId":   inputMsg.Payload.SessionID,
		"contentType": inputMsg.Payload.ContentType,
		"payload":     decodedPayload,
		"trigger":     "manual",
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		logger.Error("failed to marshal payload: %s", err)
		return
	}

	resp, err := http.Post(url, inputMsg.Payload.ContentType, bytes.NewBuffer(jsonPayload))
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

	c.SendMessage(NewOutputMessage(inputMsg.ID, struct {
		ContentType string `json:"content_type"`
		Payload     string `json:"payload"`
	}{
		ContentType: "application/json",
		Payload:     string(body),
	}))

	return
}
