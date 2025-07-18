package dev

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	"github.com/agentuity/go-common/tui"
	"github.com/charmbracelet/lipgloss"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/net/http2"
)

const (
	maxConnectionFailures = 20
	maxReconnectBaseDelay = time.Millisecond * 250
	maxReconnectMaxDelay  = time.Second * 10
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
	connected      chan error
	expiresAt      *time.Time
	tlsCertificate *tls.Certificate
	conn           *tls.Conn
	wg             sync.WaitGroup
	cleanup        func()

	// Connection state
	connectionLock    sync.Mutex
	reconnectFailures int
	connectionFailed  time.Time
	connectionStarted time.Time
	reconnectMutex    sync.Mutex
	hostname          string
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
		Hostname        string `json:"hostname,omitempty"`
	} `json:"data"`
}

// Close closes the bridge client and cleans up the connection
func (s *Server) Close() error {
	s.logger.Debug("closing connection")
	s.once.Do(func() {
		s.closeConnection()
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

func (s *Server) closeConnection() {
	if err := s.apiclient.Do("DELETE", "/cli/devmode/"+s.Project.Project.ProjectId+"/"+s.ID, nil, nil); err != nil {
		s.logger.Error("failed to send close connection: %s", err)
	}
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
	s.hostname = resp.Data.Hostname

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

	s.logger.Trace("connecting to devmode server")

	// hold a connection lock to prevent multiple go routines from trying to reconnect
	// before the previous connect goroutine has finished
	s.connectionLock.Lock()
	defer s.connectionLock.Unlock()

	defer func() {
		if initial && gerr != nil {
			s.connected <- gerr
		}
		s.logger.Debug("connection closed")
		select {
		case <-s.ctx.Done():
			return
		default:
			var count int
			var started time.Time
			s.reconnectMutex.Lock()
			if s.reconnectFailures == 0 {
				s.connectionFailed = time.Now()
				s.logger.Warn("lost connection to the dev server, reconnecting ...")
			}
			s.reconnectFailures++
			started = s.connectionFailed
			count = s.reconnectFailures
			s.reconnectMutex.Unlock()
			if count >= maxConnectionFailures {
				s.logger.Fatal("Too many connection failures, giving up after %d attempts (%s). You may need to re-run `agentuity dev`. If this error persists, please contact support.", count, time.Since(started))
				return
			}
			baseDelay := maxReconnectBaseDelay
			wait := baseDelay * time.Duration(math.Pow(2, float64(count-1)))
			if wait > maxReconnectMaxDelay {
				wait = maxReconnectMaxDelay
			}
			s.logger.Debug("reconnecting in %s after %d connection failures (%s)", wait, count, time.Since(started))
			time.Sleep(wait)
			s.reconnect()
		}
	}()

	s.logger.Trace("refreshing connection metadata")
	refreshStart := time.Now()
	if err := s.refreshConnection(); err != nil {
		if !initial {
			s.logger.Error("failed to refresh connection: %s", err)
		}
		// initial will bubble this up
		gerr = err
		return
	}

	s.logger.Trace("refreshed connection metadata in %v", time.Since(refreshStart))

	var tlsConfig tls.Config
	tlsConfig.Certificates = []tls.Certificate{*s.tlsCertificate}
	tlsConfig.NextProtos = []string{"h2"}

	hostname := s.hostname

	if strings.Contains(hostname, "localhost") || strings.Contains(hostname, "127.0.0.1") {
		tlsConfig.InsecureSkipVerify = true
	}

	if !strings.Contains(hostname, ":") {
		hostname = fmt.Sprintf("%s:443", hostname)
	}

	s.logger.Trace("dialing devmode server: %s", hostname)
	dialStart := time.Now()
	conn, err := tls.Dial("tcp", hostname, &tlsConfig)
	if err != nil {
		gerr = err
		s.logger.Warn("failed to dial devmode server: %s (%s), will retry ...", hostname, err)
		return
	}
	s.conn = conn
	s.logger.Trace("dialed devmode server in %v", time.Since(dialStart))

	if initial {
		s.connected <- nil
	}

	// if we successfully connect, reset our connection failures
	s.reconnectMutex.Lock()
	if s.reconnectFailures > 0 && !s.connectionFailed.IsZero() {
		s.logger.Debug("reconnection successful after %s (%d attempts)", time.Since(s.connectionFailed), s.reconnectFailures)
		s.logger.Info("✅ connection to the dev server re-established")
	}
	s.reconnectFailures = 0
	s.connectionStarted = time.Now()
	s.connectionFailed = time.Time{}
	s.reconnectMutex.Unlock()

	s.logger.Debug("connection established to %s", hostname)

	// HTTP/2 server to accept proxied requests over the tunnel connection
	h2s := &http2.Server{}

	h2s.ServeConn(conn, &http2.ServeConnOpts{
		Handler: http.HandlerFunc(s.handleStream),
		Context: s.ctx,
	})

}

type AgentWelcome struct {
	project.AgentConfig
	Welcome
}

type AgentsControlResponse struct {
	ProjectID   string         `json:"projectId"`
	ProjectName string         `json:"projectName"`
	Agents      []AgentWelcome `json:"agents"`
}

func (s *Server) getAgents(ctx context.Context, project *project.Project) (*AgentsControlResponse, error) {
	var resp = &AgentsControlResponse{
		ProjectID:   project.ProjectId,
		ProjectName: project.Name,
	}
	welcome, err := s.getWelcome(ctx, s.port)
	if err != nil {
		return nil, err
	}
	for _, agent := range project.Agents {
		var w Welcome
		if welcome != nil {
			w = welcome[agent.ID]
		}
		resp.Agents = append(resp.Agents, AgentWelcome{
			AgentConfig: agent,
			Welcome:     w,
		})
	}
	return resp, nil
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
		agents, err := s.getAgents(r.Context(), s.Project.Project)
		if err != nil {
			s.logger.Error("failed to marshal agents control response: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, cstr.JSONStringify(agents))
		return
	case "/_control":
		sendCORSHeaders(w.Header())
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		rc := http.NewResponseController(w)
		rc.Flush()
		s.HealthCheck(fmt.Sprintf("http://127.0.0.1:%d", s.port)) // make sure the server is running
		w.Write([]byte("event: start\ndata: connected\n\n"))
		agents, err := s.getAgents(r.Context(), s.Project.Project)
		if err != nil {
			s.logger.Error("failed to marshal agents control response: %s", err)
			w.Write([]byte(fmt.Sprintf("event: error\ndata: %q\n\n", err.Error())))
			rc.Flush()
			return
		}
		w.Write([]byte(fmt.Sprintf("event: agents\ndata: %s\n\n", cstr.JSONStringify(agents))))
		rc.Flush()
		select {
		case <-s.ctx.Done():
		case <-r.Context().Done():
		}
		w.Write([]byte("event: stop\ndata: disconnected\n\n"))
		rc.Flush()
		return
	}

	agentId := r.URL.Path[1:]
	var found bool
	for _, agent := range s.Project.Project.Agents {
		if agent.ID == agentId || strings.TrimLeft(agent.ID, "agent_") == agentId {
			found = true
			agentId = agent.ID
			r.URL.Path = fmt.Sprintf("/%s", agentId) // in case we used the short version of the agent id
			break
		}
	}

	if !found {
		s.logger.Error("agent not found with id: %s", agentId)
		sendCORSHeaders(w.Header())
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if r.Method == "GET" {
		message.CustomErrorResponse(w, "Agents, Not Humans, Live Here", "Hi! I'm an Agentuity Agent running in development mode.", "", http.StatusOK)
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
	traceState, err = traceState.Insert("d", "1")
	if err != nil {
		logger.Error("failed to insert devmode status into trace state: %s", err)
		err = fmt.Errorf("failed to insert devmode status into trace state: %w", err)
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

type Welcome struct {
	Message string `json:"welcome"`
	Prompts []struct {
		Data        string `json:"data"`
		ContentType string `json:"contentType"`
	} `json:"prompts,omitempty"`
}

func (s *Server) getWelcome(ctx context.Context, port int) (map[string]Welcome, error) {
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

func (s *Server) HealthCheck(devModeUrl string) error {
	started := time.Now()
	var i int
	for time.Since(started) < 30*time.Second {
		i++
		s.logger.Trace("health check request [#%d (%s)]: %s", i, time.Since(started), fmt.Sprintf("%s/_health", devModeUrl))
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

func (s *Server) Connect() error {
	err := <-s.connected
	close(s.connected)
	if err != nil {
		return err
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

var (
	logoColor  = lipgloss.AdaptiveColor{Light: "#11c7b9", Dark: "#00FFFF"}
	labelColor = lipgloss.AdaptiveColor{Light: "#999999", Dark: "#FFFFFF"}
	labelStyle = lipgloss.NewStyle().Foreground(labelColor).Bold(true)
)

func label(s string) string {
	return labelStyle.Render(tui.PadRight(s, 10, " "))
}

func (s *Server) GenerateInfoBox(publicUrl string, appUrl string, devModeUrl string) string {
	var devmodeBox = lipgloss.NewStyle().
		Width(100).
		Border(lipgloss.NormalBorder()).
		BorderForeground(logoColor).
		Padding(1, 2).
		AlignVertical(lipgloss.Top).
		AlignHorizontal(lipgloss.Left).
		Foreground(labelColor)

	url := "loading..."
	if publicUrl != "" {
		url = tui.Link("%s", publicUrl) + "  " + tui.Muted("(only accessible while running)")
	}

	content := fmt.Sprintf(`%s

%s  %s
%s  %s
%s  %s`,
		tui.Bold("⨺ Agentuity DevMode"),
		label("DevMode"), tui.Link("%s", appUrl),
		label("Local"), tui.Link("%s", devModeUrl),
		label("Public"), url,
	)
	return devmodeBox.Render(content)
}

func New(args ServerArgs) (*Server, error) {
	id := cstr.NewHash(args.Project.Project.ProjectId, args.UserId)
	tracer := otel.Tracer("@agentuity/cli", trace.WithInstrumentationAttributes(
		attribute.String("id", id),
		attribute.String("@agentuity/orgId", args.OrgId),
		attribute.String("@agentuity/userId", args.UserId),
		attribute.String("@agentuity/projectId", args.Project.Project.ProjectId),
		attribute.Bool("@agentuity/devmode", true),
		attribute.String("name", "@agentuity/cli"),
		attribute.String("version", args.Version),
	), trace.WithInstrumentationVersion(args.Version))

	ctx, cancel := context.WithCancel(args.Ctx)

	server := &Server{
		ID:           id,
		logger:       args.Logger,
		ctx:          ctx,
		cancel:       cancel,
		apiurl:       args.APIURL,
		transportUrl: args.TransportURL,
		apiKey:       args.APIKey,
		Project:      args.Project,
		orgId:        args.OrgId,
		userId:       args.UserId,
		tracer:       tracer,
		version:      args.Version,
		port:         args.Port,
		apiclient:    util.NewAPIClient(context.WithoutCancel(ctx), args.Logger, args.APIURL, args.APIKey),
		connected:    make(chan error, 1),
	}

	go server.connect(true)
	go server.monitor()

	return server, nil
}
