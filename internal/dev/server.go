package dev

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
	cstr "github.com/agentuity/go-common/string"
	"github.com/xtaci/smux"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

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
	pending        map[string]*AgentRequest
	pendingMu      sync.RWMutex
	expiresAt      *time.Time
	tlsCertificate *tls.Certificate
	conn           *tls.Conn
	session        *smux.Session
	wg             sync.WaitGroup
	serverAddr     string
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
		if s.session != nil {
			s.session.Close()
			s.session = nil
		}
		if s.conn != nil {
			s.conn.Close()
			s.conn = nil
		}
		s.wg.Wait()
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
	return nil
}

func (s *Server) reconnect() {
	if s.session != nil {
		s.session.Close()
		s.session = nil
	}
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
	go s.connect(false)
}

func (s *Server) connect(initial bool) {
	s.wg.Add(1)
	var gerr error
	defer func() {
		if initial && gerr != nil {
			s.connected <- gerr.Error()
		}
		s.wg.Done()
	}()

	if err := s.refreshConnection(); err != nil {
		s.logger.Error("failed to refresh connection: %s", err)
		gerr = err
		return
	}

	var tlsConfig tls.Config
	tlsConfig.Certificates = []tls.Certificate{*s.tlsCertificate}

	if strings.Contains(s.serverAddr, "localhost") || strings.Contains(s.serverAddr, "127.0.0.1") {
		tlsConfig.InsecureSkipVerify = true
	}

	conn, err := tls.Dial("tcp", s.serverAddr, &tlsConfig)
	if err != nil {
		gerr = err
		s.logger.Error("failed to dial tls: %s", err)
		return
	}
	s.conn = conn

	sess, err := smux.Client(conn, nil)
	if err != nil {
		gerr = err
		s.logger.Error("failed to start smux session: %s", err)
		return
	}
	s.session = sess

	if initial {
		s.connected <- ""
	}

	for {
		stream, err := sess.AcceptStream()
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, io.ErrClosedPipe) {
				break
			}
			s.logger.Error("Stream accept failed: %s", err)
			break
		}
		go s.handleStream(s.ctx, s.logger, stream, s.tlsCertificate.Leaf.Subject.CommonName)
	}
}

type AgentsControlResponse struct {
	ProjectID   string                `json:"projectId"`
	ProjectName string                `json:"projectName"`
	Agents      []project.AgentConfig `json:"agents"`
}

func (s *Server) handleStream(ctx context.Context, logger logger.Logger, stream net.Conn, hostname string) {
	s.wg.Add(1)
	defer func() {
		stream.Close()
		s.wg.Done()
	}()

	// Read request from stream
	req, err := http.ReadRequest(bufio.NewReader(stream))
	if err != nil {
		logger.Error("Failed to parse HTTP request: %v", err)
		return
	}

	switch req.URL.Path {
	case "/_health":
		resp := &http.Response{
			StatusCode: http.StatusOK,
		}
		select {
		case <-s.ctx.Done():
			resp.StatusCode = http.StatusServiceUnavailable
		default:
		}
		resp.Write(stream)
		return
	case "/_agents":
		resp := &http.Response{
			StatusCode: http.StatusOK,
		}
		buf, err := json.Marshal(AgentsControlResponse{
			ProjectID:   s.Project.Project.ProjectId,
			ProjectName: s.Project.Project.Name,
			Agents:      s.Project.Project.Agents,
		})
		if err != nil {
			logger.Error("Failed to marshal agents control response: %v", err)
			resp.StatusCode = http.StatusInternalServerError
			resp.Body = io.NopCloser(strings.NewReader(err.Error()))
			resp.Write(stream)
			return
		}
		resp.Body = io.NopCloser(bytes.NewReader(buf))
		resp.Write(stream)
		return
	}

	req = req.WithContext(ctx)

	// Forward to local server
	req.RequestURI = ""
	req.URL.Scheme = "http"
	req.URL.Host = fmt.Sprintf("127.0.0.1:%d", s.port)
	req.Header.Set("Host", hostname)

	s.logger.Debug("forwarding request to local server: %s", req.URL.String())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Error("Failed to contact local target: %v", err)
		resp = &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(strings.NewReader("Local target error")),
		}
		resp.Write(stream)
		return
	}
	defer resp.Body.Close()

	s.logger.Debug("received response from local server: %s, status code: %d", req.URL.String(), resp.StatusCode)

	// TODO: fix streaming

	// Send response back
	err = resp.Write(stream)
	if err != nil {
		logger.Error("Failed to write response to stream: %v", err)
	}
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
		pending:       make(map[string]*AgentRequest),
		connected:     make(chan string, 1),
		serverAddr:    args.ServerAddr,
	}

	go server.connect(true)
	go server.monitor()

	return server, nil
}
