package dev

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/bridge"
	"github.com/agentuity/go-common/logger"
	cstr "github.com/agentuity/go-common/string"
	"github.com/agentuity/go-common/telemetry"
	"github.com/spf13/viper"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type Server struct {
	ID            string
	otelToken     string
	otelUrl       string
	Project       project.ProjectContext
	orgId         string
	userId        string
	apiurl        string
	transportUrl  string
	apiKey        string
	ctx           context.Context
	logger        logger.Logger
	tracer        trace.Tracer
	version       string
	bridge        *bridge.Client
	once          sync.Once
	apiclient     *util.APIClient
	registered    bool
	publicUrl     string
	port          int
	connected     chan struct{}
	pendingLogger *PendingLogger
	pending       map[string]*AgentRequest
	pendingMu     sync.RWMutex
	cleanup       func()
}

type ServerArgs struct {
	Ctx          context.Context
	Logger       logger.Logger
	LogLevel     logger.LogLevel
	APIURL       string
	TransportURL string
	APIKey       string
	ProjectToken string
	Project      project.ProjectContext
	OrgId        string
	UserId       string
	Version      string
	Connection   *bridge.BridgeConnectionInfo
	Port         int
}

var _ bridge.Handler = (*Server)(nil)

func (c *Server) WebURL(appUrl string) string {
	return fmt.Sprintf("%s/devmode/%s", appUrl, c.ID)
}

func (c *Server) PublicURL(appUrl string) string {
	return c.publicUrl
}

func (s *Server) AgentURL(agentId string) string {
	return fmt.Sprintf("http://127.0.0.1:%d/%s", s.port, agentId)
}

// Close closes the bridge client and cleans up the connection
func (s *Server) Close() error {
	s.logger.Debug("closing bridge client")
	s.once.Do(func() {
		if s.registered {
			if err := s.apiclient.Do("DELETE", "/cli/devmode/"+s.ID, map[string]string{"orgId": s.orgId}, nil); err != nil {
				s.logger.Error("failed to unregister devmode connection: %s", err)
			}
		}
		s.bridge.Close()
		s.cleanup()
	})
	return nil
}

type ConnectionResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"message"`
	Data    struct {
		OtelUrl         string `json:"otlpUrl"`
		OtelBearerToken string `json:"otlpBearerToken"`
	} `json:"data"`
}

// OnConnect is called when the bridge client is connected to the bridge server
func (s *Server) OnConnect(client *bridge.Client) error {
	s.logger.Debug("on connect")

	defer func() {
		s.connected <- struct{}{} // signal that the connection is established (even if there was an error)
	}()

	payload := map[string]string{
		"orgId":        s.orgId,
		"publicURL":    client.ClientURL(),
		"websocketURL": client.WebsocketURL(),
	}

	var response ConnectionResponse

	if err := s.apiclient.Do("PUT", "/cli/devmode/"+s.ID, payload, &response); err != nil {
		return fmt.Errorf("failed to register devmode connection with api server: %w", err)
	}

	s.otelUrl = response.Data.OtelUrl
	s.otelToken = response.Data.OtelBearerToken

	tctx, _, cleanup, err := telemetry.NewWithAPIKey(s.ctx, "@agentuity/cli", s.otelUrl, s.otelToken, s.logger)
	if err != nil {
		return fmt.Errorf("failed to create telemetry client: %w", err)
	}

	s.ctx = tctx
	s.cleanup = cleanup

	s.saveConnection(client)

	return nil
}

// OnDisconnect is called when the bridge client is disconnected from the bridge server
func (s *Server) OnDisconnect(client *bridge.Client) {
	s.logger.Debug("on disconnect")
}

// OnHeader is called when a header is received from the bridge. this will only be called once before any data is sent.
func (s *Server) OnHeader(client *bridge.Client, id string, agentId string, headers map[string]string) {
	s.logger.Debug("on header, id: %s, agent: %s, headers: %s", id, agentId, headers)

	req := AgentRequestArgs{
		Context:   s.ctx,
		Logger:    s.logger,
		Tracer:    s.tracer,
		ID:        s.ID,
		Version:   s.version,
		URL:       s.AgentURL(agentId),
		Headers:   headers,
		AgentID:   agentId,
		OrgID:     s.orgId,
		ProjectID: s.Project.Project.ProjectId,
	}
	agentReq, err := NewAgentRequest(req)
	if err != nil {
		s.logger.Error("failed to create request: %s", err)
		return
	}
	s.pendingMu.Lock()
	s.pending[id] = agentReq
	s.pendingMu.Unlock()

	go agentReq.Run() // run this in a new goroutine so as not to block the main bridge thread
	s.logger.Debug("header exiting: %s, agent: %s", id, agentId)
}

// OnData is called when a data is received from the bridge. this will be called multiple times if the data is large.
func (s *Server) OnData(client *bridge.Client, id string, agentId string, data []byte) {
	s.logger.Debug("on data: id: %s, agent: %s", id, agentId)
	s.pendingMu.RLock()
	defer s.pendingMu.RUnlock()
	if req, ok := s.pending[id]; ok {
		req.send(data)
	} else {
		s.logger.Error("no pending request for id: %s and agent: %s", id, agentId)
	}
}

// OnClose is called when the bridge request is completed and no more data will be sent
func (s *Server) OnClose(client *bridge.Client, id string, agentId string) {
	s.logger.Debug("on close: id: %s, param: %s", id, agentId)
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	if req, ok := s.pending[id]; ok {
		delete(s.pending, id)
		req.close(client, id)
	} else {
		s.logger.Error("no pending request for id: %s and agent: %s", id, agentId)
	}
}

// OnError is called when an error occurs at any point in the bridge client
func (s *Server) OnError(client *bridge.Client, err error) {
	s.logger.Error("an error occurred: %s", err)
}

// OnControl is called when a control event is received from the bridge. you can respond with a control event to the bridge by returning a non-nil value.
func (s *Server) OnControl(client *bridge.Client, id string, data []byte) ([]byte, error) {
	s.logger.Debug("on control: id: %s, data: %s", id, string(data))
	return nil, nil
}

// OnRefresh is called when the bridge client has refreshed its connection
func (s *Server) OnRefresh(client *bridge.Client) {
	s.saveConnection(client)
}

func (s *Server) saveConnection(client *bridge.Client) {
	s.registered = true
	kv := map[string]string{}
	conn := client.ConnectionInfo()
	s.publicUrl = conn.ClientURL
	if conn.ExpiresAt != nil {
		kv["expires_at"] = conn.ExpiresAt.Format(time.RFC3339)
	}
	if conn.WebsocketURL != "" {
		kv["websocket_url"] = conn.WebsocketURL
	}
	if conn.StreamURL != "" {
		kv["stream_url"] = conn.StreamURL
	}
	if conn.ClientURL != "" {
		kv["client_url"] = conn.ClientURL
	}
	if conn.RepliesURL != "" {
		kv["replies_url"] = conn.RepliesURL
	}
	if conn.RefreshURL != "" {
		kv["refresh_url"] = conn.RefreshURL
	}
	if conn.ControlURL != "" {
		kv["control_url"] = conn.ControlURL
	}
	viper.Set("devmode."+s.orgId, kv)
	viper.WriteConfig()
}

func (s *Server) HealthCheck(devModeUrl string) error {
	started := time.Now()
	var i int
	for time.Since(started) < 15*time.Second {
		i++
		s.logger.Trace("health check request: %s", fmt.Sprintf("%s/_health", devModeUrl))
		req, err := http.NewRequestWithContext(s.ctx, "GET", fmt.Sprintf("%s/_health", devModeUrl), nil)
		if err != nil {
			return fmt.Errorf("failed to create health check request: %w", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			s.logger.Trace("health check request failed: %s", err)
			dur := time.Millisecond * 150 * time.Duration(math.Pow(float64(i), 2))
			time.Sleep(dur)
			continue
		}
		s.logger.Trace("health check request returned status code: %d", resp.StatusCode)
		if resp.StatusCode != 200 {
			s.logger.Trace("health check returned status code: %d", resp.StatusCode)
			dur := time.Millisecond * 150 * time.Duration(math.Pow(float64(i), 2))
			time.Sleep(dur)
			continue
		}
		return nil
	}
	return fmt.Errorf("health check failed after %s", time.Since(started))
}

func (s *Server) Connect(ui *DevModeUI, tuiLogger logger.Logger) error {
	s.logger = tuiLogger
	s.pendingLogger.drain(ui, s.logger)
	s.pendingLogger = nil
	<-s.connected
	close(s.connected)
	return nil
}

func New(args ServerArgs) (*Server, error) {
	id := cstr.NewHash(args.OrgId, args.UserId)
	tracer := otel.Tracer("@agentuity/cli", trace.WithInstrumentationAttributes(
		attribute.String("id", id),
		attribute.String("@agentuity/orgId", args.OrgId),
		attribute.String("@agentuity/userId", args.UserId),
		attribute.Bool("@agentuity/devmode", true),
		attribute.String("name", "@agentuity/cli"),
		attribute.String("version", args.Version),
	), trace.WithInstrumentationVersion(args.Version))

	pendingLogger := NewPendingLogger(args.LogLevel)

	server := &Server{
		ID:            id,
		logger:        pendingLogger,
		ctx:           args.Ctx,
		apiurl:        args.APIURL,
		transportUrl:  args.TransportURL,
		apiKey:        args.APIKey,
		Project:       args.Project,
		orgId:         args.OrgId,
		userId:        args.UserId,
		tracer:        tracer,
		version:       args.Version,
		port:          args.Port,
		apiclient:     util.NewAPIClient(context.Background(), args.Logger, args.APIURL, args.APIKey),
		pendingLogger: pendingLogger,
		pending:       make(map[string]*AgentRequest),
		connected:     make(chan struct{}, 1),
	}

	server.bridge = bridge.New(bridge.Options{
		Context:        args.Ctx,
		Logger:         pendingLogger,
		APIKey:         args.ProjectToken,
		URL:            args.TransportURL,
		ConnectionInfo: args.Connection,
		Handler:        server,
	})

	if err := server.bridge.Connect(); err != nil {
		return nil, err
	}

	return server, nil
}
