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
	"github.com/agentuity/go-common/telemetry"
	"github.com/cenkalti/backoff/v4"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var propagator propagation.TraceContext

type Websocket struct {
	webSocketId    string
	conn           *websocket.Conn
	OtelToken      string
	OtelUrl        string
	Project        project.ProjectContext
	orgId          string
	done           chan struct{}
	apiKey         string
	websocketUrl   string
	maxRetries     int
	retryCount     int
	parentCtx      context.Context
	ctx            context.Context
	logger         logger.Logger
	cleanup        func()
	tracer         trace.Tracer
	version        string
	binaryProtocol bool
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

func (c *Websocket) Done() <-chan struct{} {
	return c.done
}

const maxHealthCheckDuration = time.Second * 30

func isConnectionErrorRetryable(err error) bool {
	if strings.Contains(err.Error(), "connection refused") {
		return true
	}
	if strings.Contains(err.Error(), "connection reset by peer") {
		return true
	}
	if strings.Contains(err.Error(), "No connection could be made because the target machine actively refused it") { // windows
		return true
	}
	return false
}

func (c *Websocket) getAgentProtocol(ctx context.Context, port int) (bool, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/_health", port)
	started := time.Now()
	var i int
	for time.Since(started) < maxHealthCheckDuration {
		i++
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return false, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			c.logger.Debug("healthcheck attempt #%d failed: %s", i, err)
			if isConnectionErrorRetryable(err) {
				time.Sleep(time.Millisecond * time.Duration(100*i+1))
				continue
			}
			return false, err
		}
		c.logger.Debug("healthcheck attempt #%d succeeded with status code %d", i, resp.StatusCode)
		defer resp.Body.Close()
		if resp.StatusCode == 200 {
			return resp.Header.Get("x-agentuity-binary") == "true", nil
		}
	}
	return false, fmt.Errorf("failed to inspect agents after %s", maxHealthCheckDuration)
}

func (c *Websocket) getAgentWelcome(ctx context.Context, port int) (map[string]Welcome, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/welcome", port)
	for i := 0; i < 5; i++ {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			if isConnectionErrorRetryable(err) {
				time.Sleep(time.Millisecond * time.Duration(100*i+1))
				continue
			}
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode == 404 {
			return nil, nil // this is ok, just means no agents have inspect
		}
		res := make(map[string]Welcome)
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			return nil, err
		}
		return res, nil
	}
	return nil, fmt.Errorf("failed to inspect agents after 5 attempts")
}

func (c *Websocket) StartReadingMessages(ctx context.Context, logger logger.Logger, port int) {
	var err error
	c.binaryProtocol, err = c.getAgentProtocol(ctx, port)
	if err != nil {
		logger.Fatal("Your project failed to start. This typically happens when the project cannot compile or this is an underlying issue with starting the project.")
		return
	}

	go func() {
		defer close(c.done)
		for c.conn != nil {
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
						if err := c.connect(logger, true); err != nil {
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
					if err := c.connect(logger, true); err != nil {
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
				welcome, err := c.getAgentWelcome(ctx, port)
				if err != nil {
					logger.Error("failed to inspect agents: %s", err)
					continue
				}
				for i, agent := range agents {
					if val, ok := welcome[agent.ID]; ok {
						agents[i].Welcome = &val
					}
				}
				agentsMessage := NewAgentsMessage(c.webSocketId, c.Project.Project.ProjectId, AgentsPayload{
					ProjectID:   c.Project.Project.ProjectId,
					ProjectName: c.Project.Project.Name,
					Agents:      agents,
				})

				c.SendMessage(logger, agentsMessage)
			}
		}
	}()
}

func (c *Websocket) connect(logger logger.Logger, close bool) error {
	if close {
		// Close existing connection if it exists
		if c.cleanup != nil {
			c.cleanup()
		}
		if c.conn != nil {
			c.conn.Close()
		}
	}

	u, err := url.Parse(c.websocketUrl)
	if err != nil {
		return fmt.Errorf("failed to parse url: %s", err)
	}
	u.Path = fmt.Sprintf("/websocket/devmode/%s", c.webSocketId)
	u.RawQuery = url.Values{
		"from":      []string{"cli"},
		"projectId": []string{c.Project.Project.ProjectId},
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
	logger.Trace("connecting to %s", urlString)
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

	c.ctx, c.logger, c.cleanup, err = telemetry.NewWithAPIKey(c.parentCtx, "@agentuity/cli", c.OtelUrl, c.OtelToken, logger)
	if err != nil {
		return fmt.Errorf("failed to create OTLP telemetry trace: %w", err)
	}

	logger.Debug("successfully connected")
	return nil
}

type WebsocketArgs struct {
	Ctx          context.Context
	Logger       logger.Logger
	WebsocketId  string
	WebsocketUrl string
	APIKey       string
	Project      project.ProjectContext
	OrgId        string
	Version      string
}

func NewWebsocket(args WebsocketArgs) (*Websocket, error) {
	tracer := otel.Tracer("@agentuity/cli", trace.WithInstrumentationVersion(args.Version))
	ws := Websocket{
		parentCtx:    args.Ctx,
		webSocketId:  args.WebsocketId,
		Project:      args.Project,
		done:         make(chan struct{}),
		apiKey:       args.APIKey,
		websocketUrl: args.WebsocketUrl,
		maxRetries:   5,
		retryCount:   0,
		tracer:       tracer,
		orgId:        args.OrgId,
		version:      args.Version,
	}
	u, err := url.Parse(args.WebsocketUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to parse url: %s", err)
	}
	u.Path = fmt.Sprintf("/websocket/devmode/%s", args.WebsocketId)
	u.RawQuery = url.Values{
		"from": []string{"cli"},
	}.Encode()

	if u.Scheme == "http" {
		u.Scheme = "ws"
	} else if u.Scheme == "https" {
		u.Scheme = "wss"
	}

	if err := ws.connect(args.Logger, false); err != nil {
		return nil, err
	}

	return &ws, nil
}

// Update SendMessage to accept the MessageType interface
func (c *Websocket) SendMessage(logger logger.Logger, msg Message) error {
	buf, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if c.conn == nil {
		return nil
	}
	if err := c.conn.WriteMessage(websocket.TextMessage, buf); err != nil {
		return err
	}
	return nil
}

func (c *Websocket) Close() error {
	c.SendMessage(c.logger, NewCloseMessage(uuid.New().String(), c.Project.Project.ProjectId))
	if c.conn != nil {
		closeHandler := c.conn.CloseHandler()
		if err := closeHandler(1000, "User requested shutdown"); err != nil {
			c.logger.Error("failed to close connection: %s", err)
			return err
		}
	}
	if c.cleanup != nil {
		c.cleanup()
		c.cleanup = nil
	}
	return nil
}

func (c *Websocket) WebURL(appUrl string) string {
	return fmt.Sprintf("%s/devmode/%s", appUrl, c.webSocketId)
}

type Message struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload"`
	ProjectId string         `json:"projectId"`
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
func NewOutputMessage(id string, projectId string, payload struct {
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
		ID:        id,
		Type:      "output",
		Payload:   payloadMap,
		ProjectId: projectId,
	}

}

func NewCloseMessage(id string, projectId string) Message {
	payloadMap := map[string]any{}
	return Message{
		ID:        id,
		Type:      "close",
		Payload:   payloadMap,
		ProjectId: projectId,
	}
}

type Welcome struct {
	Message string `json:"welcome"`
	Prompts []struct {
		Data        string `json:"data"`
		ContentType string `json:"contentType"`
	} `json:"prompts,omitempty"`
}

type Agent struct {
	Name        string   `json:"name"`
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Welcome     *Welcome `json:"welcome,omitempty"`
}

type AgentsPayload struct {
	Agents      []Agent `json:"agents"`
	ProjectID   string  `json:"projectId"`
	ProjectName string  `json:"projectName"`
}

func NewAgentsMessage(id string, projectId string, payload AgentsPayload) Message {
	payloadMap := map[string]any{
		"agents":      payload.Agents,
		"projectId":   projectId,
		"projectName": payload.ProjectName,
	}

	return Message{
		ID:        id,
		Type:      "agents",
		ProjectId: projectId,
		Payload:   payloadMap,
	}
}

func processInputMessage(plogger logger.Logger, c *Websocket, m []byte, port int) {
	started := time.Now()
	ctx, logger, span := telemetry.StartSpan(c.ctx, plogger, c.tracer, "TriggerRun",
		trace.WithAttributes(
			attribute.String("@agentuity/devmode", "true"),
			attribute.String("trigger", "manual"),
			attribute.String("@agentuity/deploymentId", c.webSocketId),
		),
		trace.WithSpanKind(trace.SpanKindConsumer),
	)
	defer span.End()

	var inputMsg InputMessage
	var outputMessage *Message
	var err error

	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			msg := NewOutputMessage(inputMsg.ID, c.Project.Project.ProjectId, OutputPayload{
				ContentType: "text/plain",
				Payload:     []byte(err.Error()),
				Trigger:     "",
			})
			outputMessage = &msg
		} else {
			span.SetStatus(codes.Ok, "")
		}
		if outputMessage != nil {
			c.SendMessage(plogger, *outputMessage)
		}
		span.SetAttributes(
			attribute.Int64("@agentuity/cpu_time", time.Since(started).Milliseconds()),
		)
		plogger.Info("processed sess_%s in %s", span.SpanContext().TraceID(), time.Since(started))
	}()

	if lerr := json.Unmarshal(m, &inputMsg); lerr != nil {
		logger.Error("failed to unmarshal agent message: %s", lerr)
		err = lerr
		return
	}

	span.SetAttributes(
		attribute.String("@agentuity/agentId", inputMsg.Payload.AgentID),
		attribute.String("@agentuity/orgId", c.orgId),
		attribute.String("@agentuity/projectId", c.Project.Project.ProjectId),
		attribute.String("@agentuity/env", "development"),
	)

	spanContext := span.SpanContext()
	traceState := spanContext.TraceState()
	traceState, err = traceState.Insert("id", inputMsg.Payload.AgentID)
	if err != nil {
		logger.Error("failed to insert agent id into trace state: %s", err)
		err = fmt.Errorf("failed to insert agent id into trace state: %w", err)
		return
	}
	traceState, err = traceState.Insert("oid", c.orgId)
	if err != nil {
		logger.Error("failed to insert org id into trace state: %s", err)
		err = fmt.Errorf("failed to insert org id into trace state: %w", err)
		return
	}
	traceState, err = traceState.Insert("pid", c.Project.Project.ProjectId)
	if err != nil {
		logger.Error("failed to insert project id into trace state: %s", err)
		err = fmt.Errorf("failed to insert project id into trace state: %w", err)
		return
	}
	ctx = trace.ContextWithSpanContext(ctx, spanContext.WithTraceState(traceState))

	c.Project.Logger.Debug("input message: %+v", inputMsg)

	if c.Project.Project.Development == nil {
		logger.Error("development is not enabled for this project")
		err = errors.New("development is not enabled for this project")
		return
	}

	var inputPayload []byte
	url := fmt.Sprintf("http://127.0.0.1:%d/%s", port, inputMsg.Payload.AgentID)
	contentType := "application/json"

	if !c.binaryProtocol {
		// TODO: remove this once were all off the old protocol
		// make a json object with the payload
		payload := map[string]any{
			"contentType": inputMsg.Payload.ContentType,
			"payload":     inputMsg.Payload.Payload,
			"trigger":     "manual",
		}

		var lerr error
		inputPayload, lerr = json.Marshal(payload)
		if lerr != nil {
			logger.Error("failed to marshal payload: %s", lerr)
			err = fmt.Errorf("failed to marshal payload: %w", lerr)
			return
		}
	} else {
		contentType = inputMsg.Payload.ContentType
		inputPayload = inputMsg.Payload.Payload
	}
	logger.Debug("sending payload: %s to %s", string(inputPayload), url)

	req, lerr := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(inputPayload))
	if lerr != nil {
		logger.Error("failed to create request: %s", lerr)
		err = fmt.Errorf("failed to create HTTP request: %w", lerr)
		return
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "Agentuity CLI/"+c.version)
	propagator.Inject(ctx, propagation.HeaderCarrier(req.Header))

	logger.Debug("sending request to %s with trace id: %s", url, spanContext.TraceID())

	expBackoff := backoff.NewExponentialBackOff()
	expBackoff.InitialInterval = 500 * time.Millisecond
	expBackoff.MaxInterval = 5 * time.Second
	expBackoff.MaxElapsedTime = 30 * time.Second // Max total time as requested
	expBackoff.Multiplier = 2.0
	expBackoff.RandomizationFactor = 0.3 // Add jitter

	var resp *http.Response
	operation := func() error {
		var err error
		resp, err = http.DefaultClient.Do(req)
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
		err = fmt.Errorf("failed to post to agent: %w", err)
		return
	}
	defer resp.Body.Close()

	body, lerr := io.ReadAll(resp.Body)
	if lerr != nil {
		logger.Error("failed to read response body: %s", lerr)
		err = fmt.Errorf("failed to read response body: %w", lerr)
		return
	}

	if resp.StatusCode != 200 {
		err = fmt.Errorf("the Agent produced an error: %s", string(body))
		return
	}

	logger.Debug("response: %s (status code: %d)", string(body), resp.StatusCode)

	var trigger string
	if c.binaryProtocol {
		trigger = resp.Header.Get("x-agentuity-trigger")
		contentType = resp.Header.Get("content-type")
	} else {
		// TODO: remove this once were all off the old protocol
		output, lerr := isOutputPayload(body)
		if lerr != nil {
			err = fmt.Errorf("the Agent produced an error")
			return
		}
		trigger = output.Trigger
		contentType = output.ContentType
		body = output.Payload
	}

	msg := NewOutputMessage(inputMsg.ID, c.Project.Project.ProjectId, OutputPayload{
		ContentType: contentType,
		Payload:     body,
		Trigger:     trigger,
	})
	outputMessage = &msg
}
