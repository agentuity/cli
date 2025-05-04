package dev

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/agentuity/go-common/bridge"
	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

var propagator propagation.TraceContext

type packet struct {
	buf []byte
}

func (p *packet) String() string {
	return fmt.Sprintf("packet{buf: %d bytes}", len(p.buf))
}

type AgentRequest struct {
	ctx        context.Context
	logger     logger.Logger
	pending    chan *packet
	req        *http.Request
	statusCode int
	headers    map[string]string
	body       io.ReadCloser
	span       trace.Span
	started    time.Time
	completed  chan struct{}
}

var _ io.Reader = (*AgentRequest)(nil)

type AgentRequestArgs struct {
	Context   context.Context
	Logger    logger.Logger
	Tracer    trace.Tracer
	ID        string
	URL       string
	Version   string
	AgentID   string
	OrgID     string
	ProjectID string
	Headers   map[string]string
}

func NewAgentRequest(args AgentRequestArgs) (*AgentRequest, error) {
	started := time.Now()
	var err error

	sctx, logger, span := telemetry.StartSpan(args.Context, args.Logger, args.Tracer, "TriggerRun",
		trace.WithAttributes(
			attribute.Bool("@agentuity/devmode", true),
			attribute.String("trigger", "manual"),
			attribute.String("@agentuity/deploymentId", args.ID),
		),
		trace.WithSpanKind(trace.SpanKindConsumer),
	)

	defer func() {
		// only end the span if there was an error
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			span.SetAttributes(
				attribute.Int64("@agentuity/cpu_time", time.Since(started).Milliseconds()),
			)
			span.End()
		}
	}()

	span.SetAttributes(
		attribute.String("@agentuity/agentId", args.AgentID),
		attribute.String("@agentuity/orgId", args.OrgID),
		attribute.String("@agentuity/projectId", args.ProjectID),
		attribute.String("@agentuity/env", "development"),
	)

	spanContext := span.SpanContext()
	traceState := spanContext.TraceState()
	traceState, err = traceState.Insert("id", args.AgentID)
	if err != nil {
		logger.Error("failed to insert agent id into trace state: %s", err)
		err = fmt.Errorf("failed to insert agent id into trace state: %w", err)
		return nil, err
	}
	traceState, err = traceState.Insert("oid", args.OrgID)
	if err != nil {
		logger.Error("failed to insert org id into trace state: %s", err)
		err = fmt.Errorf("failed to insert org id into trace state: %w", err)
		return nil, err
	}
	traceState, err = traceState.Insert("pid", args.ProjectID)
	if err != nil {
		logger.Error("failed to insert project id into trace state: %s", err)
		err = fmt.Errorf("failed to insert project id into trace state: %w", err)
		return nil, err
	}

	ctx := trace.ContextWithSpanContext(sctx, spanContext.WithTraceState(traceState))

	agentReq := &AgentRequest{
		ctx:       ctx,
		logger:    logger,
		pending:   make(chan *packet, 10),
		completed: make(chan struct{}, 1),
		started:   started,
		span:      span,
	}
	req, err := http.NewRequestWithContext(ctx, "POST", args.URL, agentReq)
	if err != nil {
		return nil, err
	}

	for k, v := range args.Headers {
		req.Header.Set(k, v)
	}

	req.Header.Set("x-agentuity-trigger", "manual")
	req.Header.Set("User-Agent", "Agentuity CLI/"+args.Version)
	propagator.Inject(ctx, propagation.HeaderCarrier(req.Header))

	agentReq.req = req

	return agentReq, nil
}

func (r *AgentRequest) Run() error {
	var err error
	var resp *http.Response

	defer func() {
		if err != nil {
			r.span.RecordError(err)
			r.span.SetStatus(codes.Error, err.Error())
		}
	}()

	r.logger.Debug("sending request to agent: %s", r.req.URL)
	resp, err = http.DefaultClient.Do(r.req)
	if err != nil {
		return err
	}
	r.logger.Debug("sent request to agent: %s, returned: %d", r.req.URL, resp.StatusCode)
	r.statusCode = resp.StatusCode
	r.headers = make(map[string]string)
	for k, v := range resp.Header {
		r.headers[k] = strings.Join(v, ", ")
	}
	r.body = resp.Body
	r.req = nil
	r.completed <- struct{}{} // signal that the request is complete
	return nil
}

func (r *AgentRequest) Read(p []byte) (n int, err error) {
	select {
	case packet := <-r.pending:
		if packet == nil {
			r.logger.Debug("incoming buffer is EOF")
			return 0, io.EOF
		}
		if len(packet.buf) > len(p) {
			return 0, fmt.Errorf("incoming buffer is larger (%d) than the outgoing buffer (%d)", len(packet.buf), len(p))
		}
		return copy(p, packet.buf), nil
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	}
}

func (r *AgentRequest) close(client *bridge.Client, id string) {
	r.logger.Trace("closing agent request: %s", id)
	close(r.pending)
	defer func() {
		if r.body != nil {
			r.body.Close()
		}
		r.statusCode = 0
		r.headers = nil
		r.body = nil
		r.req = nil
		r.completed = nil
		if r.span != nil {
			r.span.End()
			r.span = nil
		}
	}()
	r.logger.Trace("waiting for agent request to complete: %s", id)
	<-r.completed // block for the request to complete
	r.logger.Debug("replying to agent request: %s, status: %d, headers: %v", id, r.statusCode, r.headers)
	if err := client.Reply(id, r.statusCode, r.headers, r.body); err != nil {
		r.logger.Error("failed to reply to agent request: %s", err)
	}
	r.logger.Info("processed sess_%s in %s", r.span.SpanContext().TraceID(), time.Since(r.started))
}

func (r *AgentRequest) send(buf []byte) {
	r.logger.Trace("sending buffer: %d bytes", len(buf))
	r.pending <- &packet{buf}
}
