package dev

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/message"
	cstr "github.com/agentuity/go-common/string"
	"github.com/agentuity/go-common/telemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/net/http2"
)

var propagator propagation.TraceContext

type Server struct {
	ID             string
	otelToken      string
	otelUrl        string
	Project        project.ProjectContext
	orgId          string
	userId         string
	apiurl         string
	transportUrl   string
	apiKey         string
	ctx            context.Context
	cancel         context.CancelFunc
	logger         logger.Logger
	tracer         trace.Tracer
	version        string
	once           sync.Once
	apiclient      *util.APIClient
	publicUrl      string
	port           int
	connected      chan string
	pendingLogger  logger.Logger
	expiresAt      *time.Time
	tlsCertificate *tls.Certificate
	conn           *tls.Conn
	srv            *http2.Server
	wg             sync.WaitGroup
	serverAddr     string
	cleanup        func()
}

type ServerArgs struct {
	Ctx          context.Context
	Logger       logger.Logger
	LogLevel     logger.LogLevel
	APIURL       string
	TransportURL string
	APIKey       string
	Project      project.ProjectContext
	OrgId        string
	UserId       string
	Version      string
	Port         int
	ServerAddr   string
}

type ConnectionResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"message"`
	Data    struct {
		Certificate     string `json:"certificate"`
		PrivateKey      string `json:"private_key"`
		Domain          string `json:"domain"`
		ExpiresAt       string `json:"expires_at"`
		OtelUrl         string `json:"otlp_url"`
		OtelBearerToken string `json:"otlp_token"`
	} `json:"data"`
}

// Close closes the bridge client and cleans up the connection
func (s *Server) Close() error {
	s.logger.Debug("closing connection")
	s.once.Do(func() {
		s.cancel()
		s.wg.Wait()
		if s.conn != nil {
			s.conn.Close()
			s.conn = nil
		}
		if s.cleanup != nil {
			s.cleanup()
		}
	})
	return nil
}

func (s *Server) refreshConnection() error {
	var resp ConnectionResponse
	if err := s.apiclient.Do("GET", "/cli/devmode/"+s.Project.Project.ProjectId+"/"+s.ID, nil, &resp); err != nil {
		return fmt.Errorf("failed to refresh connection: %w", err)
	}
	s.otelUrl = resp.Data.OtelUrl
	s.otelToken = resp.Data.OtelBearerToken
	tv, err := time.Parse(time.RFC3339, resp.Data.ExpiresAt)
	if err != nil {
		return fmt.Errorf("failed to parse expires at: %w", err)
	}
	s.expiresAt = &tv
	s.publicUrl = fmt.Sprintf("https://%s", resp.Data.Domain)
	cert, err := tls.X509KeyPair([]byte(resp.Data.Certificate), []byte(resp.Data.PrivateKey))
	if err != nil {
		return fmt.Errorf("failed to create tls key pair: %w", err)
	}
	s.tlsCertificate = &cert
	if s.cleanup == nil {
		ctx, logger, cleanup, err := telemetry.NewWithAPIKey(s.ctx, "@agentuity/cli", s.otelUrl, s.otelToken, s.logger)
		if err != nil {
			return fmt.Errorf("failed to create OTLP telemetry trace: %w", err)
		}
		s.ctx = ctx
		s.logger = logger
		s.cleanup = cleanup
	}

	return nil
}

func (s *Server) reconnect() {
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
	go s.connect(false)
}

func (s *Server) connect(initial bool) {
	var gerr error
	defer func() {
		if initial && gerr != nil {
			s.connected <- gerr.Error()
		}
	}()

	if err := s.refreshConnection(); err != nil {
		s.logger.Error("failed to refresh connection: %s", err)
		gerr = err
		return
	}

	var tlsConfig tls.Config
	tlsConfig.Certificates = []tls.Certificate{*s.tlsCertificate}
	tlsConfig.NextProtos = []string{"h2"}

	if strings.Contains(s.serverAddr, "localhost") || strings.Contains(s.serverAddr, "127.0.0.1") {
		tlsConfig.InsecureSkipVerify = true
	}

	if !strings.Contains(s.serverAddr, ":") {
		s.serverAddr = fmt.Sprintf("%s:443", s.serverAddr)
	}

	conn, err := tls.Dial("tcp", s.serverAddr, &tlsConfig)
	if err != nil {
		gerr = err
		s.logger.Error("failed to dial tls: %s", err)
		return
	}
	s.conn = conn

	if initial {
		s.connected <- ""
	}

	// HTTP/2 server to accept proxied requests over the tunnel connection
	h2s := &http2.Server{}

	h2s.ServeConn(conn, &http2.ServeConnOpts{
		Handler: http.HandlerFunc(s.handleStream),
		Context: s.ctx,
	})

}

type AgentsControlResponse struct {
	ProjectID   string                `json:"projectId"`
	ProjectName string                `json:"projectName"`
	Agents      []project.AgentConfig `json:"agents"`
}

func sendCORSHeaders(headers http.Header) {
	headers.Set("access-control-allow-origin", "*")
	headers.Set("access-control-expose-headers", "Content-Type")
	headers.Set("access-control-allow-headers", "Content-Type, Authorization")
	headers.Set("access-control-allow-methods", "GET, POST, OPTIONS")
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	s.wg.Add(1)
	defer s.wg.Done()

	s.logger.Trace("handleStream: %s %s", r.Method, r.URL)

	if r.Method == "OPTIONS" {
		sendCORSHeaders(w.Header())
		w.WriteHeader(http.StatusOK)
		return
	}

	switch r.URL.Path {
	case "/":
		message.CustomErrorResponse(w, "Agents, Not Humans, Live Here", "Hi! I'm an Agentuity Agent running in development mode.", "", http.StatusOK)
		return
	case "/_health":
		w.WriteHeader(http.StatusOK)
		return
	case "/_agents":
		sendCORSHeaders(w.Header())
		buf, err := json.Marshal(AgentsControlResponse{
			ProjectID:   s.Project.Project.ProjectId,
			ProjectName: s.Project.Project.Name,
			Agents:      s.Project.Project.Agents,
		})
		if err != nil {
			s.logger.Error("failed to marshal agents control response: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(buf)
		return
	case "/_control":
		sendCORSHeaders(w.Header())
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		rc := http.NewResponseController(w)
		rc.Flush()
		w.Write([]byte("event: start\ndata: connected\n\n"))
		rc.Flush()
		select {
		case <-s.ctx.Done():
		case <-r.Context().Done():
		}
		w.Write([]byte("event: stop\ndata: disconnected\n\n"))
		rc.Flush()
		return
	}

	if r.Method != "POST" {
		sendCORSHeaders(w.Header())
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	agentId := r.URL.Path[1:]
	var found bool
	for _, agent := range s.Project.Project.Agents {
		if agent.ID == agentId || strings.TrimLeft(agent.ID, "agent_") == agentId {
			found = true
			agentId = agent.ID
			break
		}
	}

	if !found {
		s.logger.Error("agent not found with id: %s", agentId)
		sendCORSHeaders(w.Header())
		w.WriteHeader(http.StatusNotFound)
		return
	}

	sctx, logger, span := telemetry.StartSpan(r.Context(), s.logger, s.tracer, "TriggerRun",
		trace.WithAttributes(
			attribute.Bool("@agentuity/devmode", true),
			attribute.String("trigger", "manual"),
			attribute.String("@agentuity/deploymentId", s.ID),
		),
		trace.WithSpanKind(trace.SpanKindConsumer),
	)

	var err error
	started := time.Now()

	defer func() {
		// only end the span if there was an error
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		} else {
			span.SetStatus(codes.Ok, "")
		}
		span.SetAttributes(
			attribute.Int64("@agentuity/cpu_time", time.Since(started).Milliseconds()),
		)
		span.End()
		s.logger.Info("processed sess_%s in %s", span.SpanContext().TraceID(), time.Since(started))
	}()

	span.SetAttributes(
		attribute.String("@agentuity/agentId", agentId),
		attribute.String("@agentuity/orgId", s.orgId),
		attribute.String("@agentuity/projectId", s.Project.Project.ProjectId),
		attribute.String("@agentuity/env", "development"),
	)

	spanContext := span.SpanContext()
	traceState := spanContext.TraceState()
	traceState, err = traceState.Insert("id", agentId)
	if err != nil {
		logger.Error("failed to insert agent id into trace state: %s", err)
		err = fmt.Errorf("failed to insert agent id into trace state: %w", err)
		return
	}
	traceState, err = traceState.Insert("oid", s.orgId)
	if err != nil {
		logger.Error("failed to insert org id into trace state: %s", err)
		err = fmt.Errorf("failed to insert org id into trace state: %w", err)
		return
	}
	traceState, err = traceState.Insert("pid", s.Project.Project.ProjectId)
	if err != nil {
		logger.Error("failed to insert project id into trace state: %s", err)
		err = fmt.Errorf("failed to insert project id into trace state: %w", err)
		return
	}

	newctx := trace.ContextWithSpanContext(sctx, spanContext.WithTraceState(traceState))

	nr := r.WithContext(newctx)
	nr.Header = r.Header.Clone()
	nr.Header.Set("x-agentuity-trigger", "manual")
	nr.Header.Set("User-Agent", "Agentuity CLI/"+s.version)
	propagator.Inject(newctx, propagation.HeaderCarrier(nr.Header))

	url, err := url.Parse(r.URL.String())
	if err != nil {
		logger.Error("failed to parse url: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	url.Scheme = "http"
	url.Host = fmt.Sprintf("127.0.0.1:%d", s.port)
	url.Path = "" // proxy sets so this acts like the base

	logger.Trace("sending to: %s", url)

	proxy := httputil.NewSingleHostReverseProxy(url)
	proxy.FlushInterval = -1 // no buffering so we can stream
	proxy.ServeHTTP(w, nr)
}

func (s *Server) WebURL(appUrl string) string {
	return fmt.Sprintf("%s/devmode/%s", appUrl, s.ID)
}

func (s *Server) PublicURL(appUrl string) string {
	return s.publicUrl
}

func (s *Server) AgentURL(agentId string) string {
	return fmt.Sprintf("http://127.0.0.1:%d/%s", s.port, agentId)
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
		if resp.StatusCode != http.StatusOK {
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
	if pl, ok := s.logger.(*PendingLogger); ok {
		pl.drain(ui, s.logger)
	}
	s.pendingLogger = s.logger
	msg := <-s.connected
	close(s.connected)
	if msg != "" {
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func (s *Server) monitor() {
	t := time.NewTicker(time.Minute * 10)
	defer t.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
			if s.expiresAt != nil && time.Now().After(*s.expiresAt) {
				s.logger.Debug("connection expired, reconnecting")
				s.reconnect()
			}
		}
	}
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

	ctx, cancel := context.WithCancel(args.Ctx)

	server := &Server{
		ID:            id,
		logger:        pendingLogger,
		ctx:           ctx,
		cancel:        cancel,
		apiurl:        args.APIURL,
		transportUrl:  args.TransportURL,
		apiKey:        args.APIKey,
		Project:       args.Project,
		orgId:         args.OrgId,
		userId:        args.UserId,
		tracer:        tracer,
		version:       args.Version,
		port:          args.Port,
		apiclient:     util.NewAPIClient(context.Background(), pendingLogger, args.APIURL, args.APIKey),
		pendingLogger: pendingLogger,
		connected:     make(chan string, 1),
		serverAddr:    args.ServerAddr,
	}

	go server.connect(true)
	go server.monitor()

	return server, nil
}
