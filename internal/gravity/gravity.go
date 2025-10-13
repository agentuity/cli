package gravity

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/go-common/gravity"
	"github.com/agentuity/go-common/gravity/proto"
	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/network"
	cnet "github.com/agentuity/go-common/network"
	cproject "github.com/agentuity/go-common/project"
	cstr "github.com/agentuity/go-common/string"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/waiter"
)

const (
	nicID = 1
	mtu   = 1280 // Reduced to IPv6 minimum to avoid MTU blackhole issues
)

type Client struct {
	context          context.Context
	logger           logger.Logger
	version          string
	orgID            string
	projectID        string
	project          project.ProjectContext
	endpointID       string
	url              string
	sdkKey           string
	proxyPort        uint
	agentPort        uint
	connectProxyPort *uint
	ephemeral        bool
	clientname       string
	dynamicHostname  bool
	dynamicProject   bool
	server           *http.Server
	connectProxy     *http.Server
	client           *gravity.GravityClient
	once             sync.Once
	stack            *stack.Stack
	endpoint         *channel.Endpoint
	provider         *cliProvider
}

type Config struct {
	Context          context.Context
	Logger           logger.Logger
	Version          string // of the cli
	OrgID            string
	Project          project.ProjectContext
	EndpointID       string
	URL              string
	SDKKey           string
	ProxyPort        uint
	AgentPort        uint
	ConnectProxyPort *uint
	Ephemeral        bool
	ClientName       string
	DynamicHostname  bool
	DynamicProject   bool
}

func New(config Config) *Client {
	return &Client{
		context:          config.Context,
		logger:           config.Logger,
		version:          config.Version,
		orgID:            config.OrgID,
		projectID:        config.Project.Project.ProjectId,
		project:          config.Project,
		endpointID:       config.EndpointID,
		url:              config.URL,
		sdkKey:           config.SDKKey,
		ephemeral:        config.Ephemeral,
		proxyPort:        config.ProxyPort,
		agentPort:        config.AgentPort,
		connectProxyPort: config.ConnectProxyPort,
		clientname:       config.ClientName,
		dynamicHostname:  config.DynamicHostname,
		dynamicProject:   config.DynamicProject,
	}
}

// APIURL returns the API URL of the client.
func (c *Client) APIURL() string {
	return c.client.GetAPIURL()
}

// APIKey returns the API key of the client.
func (c *Client) APIKey() string {
	return c.client.GetSecret()
}

// TelemetryURL returns the telemetry URL of the client.
func (c *Client) TelemetryURL() string {
	return c.provider.config.TelemetryURL
}

// TelemetryAPIKey returns the telemetry API key of the client.
func (c *Client) TelemetryAPIKey() string {
	return c.provider.config.TelemetryAPIKey
}

// Hostname returns the hostname of the client.
func (c *Client) Hostname() string {
	return c.provider.config.Hostname
}

// OrgID returns the organization ID of the client.
func (c *Client) OrgID() string {
	return c.provider.config.OrgID
}

// EndpointID returns the endpoint ID of the client.
func (c *Client) EndpointID() string {
	return c.endpointID
}

// For each TCP connection: connect to local HTTPS server and proxy bytes.
func (c *Client) bridgeToLocalTLS(remote *gonet.TCPConn) {
	logger := c.logger
	logger.Debug("bridgeToLocalTLS: starting...")
	defer remote.Close()
	addr := fmt.Sprintf("127.0.0.1:%d", c.proxyPort)
	logger.Debug("bridgeToLocalTLS: attempting to dial %s...", addr)
	local, err := net.Dial("tcp", addr)

	if err != nil {
		logger.Error("dial error: %v", err)
		return
	}
	logger.Info("connected to local HTTPS server: %s", local.RemoteAddr().String())
	defer local.Close()

	logger.Trace("bridgeToLocalTLS: starting copy operations...")
	go func() {
		logger.Trace("bridgeToLocalTLS: copying netstack -> local server...")
		n, err := io.Copy(local, remote)
		logger.Trace("bridgeToLocalTLS: netstack -> local server finished (copied %d bytes, err: %v)", n, err)
	}()
	logger.Trace("bridgeToLocalTLS: copying local server -> netstack...")
	n, err := io.Copy(remote, local)
	logger.Trace("bridgeToLocalTLS: local server -> netstack finished (copied %d bytes, err: %v)", n, err)
}

// handleConnect handles HTTP CONNECT requests and bridges them through the netstack.
func (c *Client) handleConnect(w http.ResponseWriter, r *http.Request) {
	logger := c.logger
	logger.Trace("CONNECT request: %s", r.Host)

	if r.Method != http.MethodConnect {
		logger.Debug("non-CONNECT request rejected: %s %s", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse host and port
	host, portStr, err := net.SplitHostPort(r.Host)
	if err != nil {
		logger.Error("invalid CONNECT target: %s (%v)", r.Host, err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	port, err := net.LookupPort("tcp", portStr)
	if err != nil {
		logger.Error("invalid port in CONNECT target: %s (%v)", portStr, err)
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Check if this is an agentuity domain
	agentuityDomains := []string{".agentuity.io", ".agentuity.cloud", ".agentuity.run", ".agentuity.com", "agentuity.ai"}
	isAgentuityDomain := false
	for _, domain := range agentuityDomains {
		if strings.HasSuffix(host, domain) || host == domain[1:] {
			isAgentuityDomain = true
			break
		}
	}

	var remoteConn net.Conn

	if isAgentuityDomain {
		// Route through gravity tunnel
		logger.Trace("CONNECT to %s via gravity tunnel", host)

		ip := network.Addresses["catalyst"]

		if strings.HasSuffix(host, ".agentuity.cloud") {
			part := strings.Split(host, ".agentuity.cloud")[0]
			if customip, ok := network.Addresses[part]; ok {
				ip = customip
			}
		} else if strings.HasSuffix(host, ".agentuity.io") {
			part := strings.Split(host, ".agentuity.io")[0]
			if customip, ok := network.Addresses[part]; ok {
				ip = customip
			}
		}

		var protocolNumber tcpip.NetworkProtocolNumber
		var addr tcpip.Address

		if ip4 := ip.To4(); ip4 != nil {
			protocolNumber = ipv4.ProtocolNumber
			var addr4 [4]byte
			copy(addr4[:], ip4)
			addr = tcpip.AddrFrom4(addr4)
		} else {
			protocolNumber = ipv6.ProtocolNumber
			ip6 := ip.To16()
			var addr6 [16]byte
			copy(addr6[:], ip6)
			addr = tcpip.AddrFrom16(addr6)
		}

		// Dial through netstack
		fullAddr := tcpip.FullAddress{
			Addr: addr,
			Port: uint16(port),
		}

		remoteConn, err = gonet.DialTCP(c.stack, fullAddr, protocolNumber)
		if err != nil {
			logger.Error("failed to dial %s: %v", host, err)
			http.Error(w, "Connection failed", http.StatusBadGateway)
			return
		}
	} else {
		// Direct connection for non-agentuity domains
		logger.Trace("CONNECT to %s directly", host)
		remoteConn, err = net.Dial("tcp", net.JoinHostPort(host, portStr))
		if err != nil {
			logger.Error("failed to dial %s: %v", host, err)
			http.Error(w, "Connection failed", http.StatusBadGateway)
			return
		}
	}

	// Hijack the client connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		logger.Error("ResponseWriter does not support hijacking")
		remoteConn.Close()
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		logger.Error("failed to hijack connection: %v", err)
		remoteConn.Close()
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Send success response with dynamic HTTP version
	response := fmt.Sprintf("%s 200 Connection Established\r\n\r\n", r.Proto)
	if _, err := clientConn.Write([]byte(response)); err != nil {
		logger.Error("failed to send 200 response: %v", err)
		clientConn.Close()
		remoteConn.Close()
		return
	}

	logger.Debug("proxying CONNECT for %s", r.Host)

	// Bidirectional copy
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		n, err := io.Copy(remoteConn, clientConn)
		if err != nil {
			logger.Trace("client -> remote error: %v", err)
		}
		logger.Trace("client -> remote: %d bytes", n)
		// Close write side if possible
		if tcpConn, ok := remoteConn.(interface{ CloseWrite() error }); ok {
			tcpConn.CloseWrite()
		}
	}()

	go func() {
		defer wg.Done()
		n, err := io.Copy(clientConn, remoteConn)
		if err != nil {
			logger.Trace("remote -> client error: %v", err)
		}
		logger.Trace("remote -> client: %d bytes", n)
	}()

	wg.Wait()

	clientConn.Close()
	remoteConn.Close()
	logger.Debug("CONNECT session completed for %s", r.Host)
}

// startConnectProxy starts the HTTP CONNECT proxy server if configured.
func (c *Client) startConnectProxy() error {
	if c.connectProxyPort == nil {
		return nil
	}

	logger := c.logger
	port := *c.connectProxyPort

	logger.Debug("starting CONNECT proxy on port %d", port)

	server := &http.Server{
		Addr: fmt.Sprintf("127.0.0.1:%d", port),
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.handleConnect(w, r)
		}),
		ReadTimeout:  0,
		WriteTimeout: 0,
	}
	c.connectProxy = server

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("CONNECT proxy server failed: %v", err)
		}
	}()

	logger.Info("CONNECT proxy listening on http://127.0.0.1:%d", port)
	return nil
}

// Close will close the client and all the associated services.
func (c *Client) Close() error {
	var err error
	c.once.Do(func() {
		c.logger.Debug("closing client")
		err = c.cleanup()
		c.logger.Debug("closed")
	})
	return err
}

// Close will close the client and all the associated services.
func (c *Client) cleanup() error {
	var err error
	if c.client != nil {
		c.client.Close()
		c.client = nil
	}
	if c.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err = c.server.Shutdown(ctx)
		c.server = nil
	}
	if c.connectProxy != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if shutdownErr := c.connectProxy.Shutdown(ctx); shutdownErr != nil && err == nil {
			err = shutdownErr
		}
		c.connectProxy = nil
	}
	if c.endpoint != nil {
		c.endpoint.Close()
		c.endpoint = nil
	}
	if c.stack != nil {
		c.stack.Close()
		c.stack = nil
	}
	return err
}

type AgentWelcome struct {
	cproject.AgentConfig
	Welcome
}

type AgentsControlResponse struct {
	ProjectID   string         `json:"projectId"`
	ProjectName string         `json:"projectName"`
	Agents      []AgentWelcome `json:"agents"`
}

type Welcome struct {
	Message string `json:"welcome"`
	Prompts []struct {
		Data        string `json:"data"`
		ContentType string `json:"contentType"`
	} `json:"prompts,omitempty"`
}

func (c *Client) getWelcome(ctx context.Context, port int) (map[string]Welcome, error) {
	url := fmt.Sprintf("http://127.0.0.1:%d/welcome", port)
	for i := range 5 {
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

func (c *Client) getAgents(ctx context.Context, project *cproject.Project) (*AgentsControlResponse, error) {
	var resp = &AgentsControlResponse{
		ProjectID:   project.ProjectId,
		ProjectName: project.Name,
	}
	welcome, err := c.getWelcome(ctx, int(c.agentPort))
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

func (c *Client) HealthCheck(devModeUrl string) error {
	started := time.Now()
	var i int
	for time.Since(started) < 30*time.Second {
		i++
		c.logger.Trace("health check request [#%d (%s)]: %s", i, time.Since(started), fmt.Sprintf("%s/_health", devModeUrl))
		req, err := http.NewRequestWithContext(c.context, "GET", fmt.Sprintf("%s/_health", devModeUrl), nil)
		if err != nil {
			return fmt.Errorf("failed to create health check request: %w", err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			c.logger.Trace("health check request failed: %s", err)
			dur := time.Millisecond * 150 * time.Duration(math.Pow(float64(i), 2))
			time.Sleep(dur)
			continue
		}
		c.logger.Trace("health check request returned status code: %d", resp.StatusCode)
		if resp.StatusCode != http.StatusOK {
			c.logger.Trace("health check returned status code: %d", resp.StatusCode)
			dur := time.Millisecond * 150 * time.Duration(math.Pow(float64(i), 2))
			time.Sleep(dur)
			continue
		}
		return nil
	}
	return fmt.Errorf("health check failed after %s", time.Since(started))
}

// Start will start the client and all the associated services.
func (c *Client) Start() error {
	var success bool

	defer func() {
		if !success {
			c.Close()
		}
	}()

	c.logger.Debug("proxy port: %d, agent port: %d", c.proxyPort, c.agentPort)

	ipv4addr, err := getPrivateIPv4()
	if err != nil {
		return err
	}
	ipv6Address := cnet.NewIPv6Address(cnet.GetRegion(""), cnet.NetworkHadron, c.orgID, c.endpointID, ipv4addr)

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("error getting hostname: %w", err)
	}

	var dynamicProjectRouting string
	if c.dynamicProject {
		dynamicProjectRouting = c.projectID
	}

	capabilities := &proto.ClientCapabilities{
		DynamicHostname:       c.dynamicHostname,
		DynamicProjectRouting: dynamicProjectRouting,
	}

	resp, err := gravity.Provision(gravity.ProvisionRequest{
		Context:      c.context,
		GravityURL:   c.url,
		InstanceID:   c.endpointID,
		Region:       "unknown",
		Provider:     "other", // TODO: change this to support this provider
		PrivateIP:    ipv4addr,
		Token:        c.sdkKey,
		Hostname:     hostname,
		Ephemeral:    c.ephemeral,
		Capabilities: capabilities,
	})
	if err != nil {
		return fmt.Errorf("failed to provision machine: %w", err)
	}

	// FIXME: cert expires

	log := c.logger

	log.Debug("machine provisioned")

	cert, err := tls.X509KeyPair(resp.Certificate, resp.PrivateKey)
	if err != nil {
		return fmt.Errorf("failed to load client certificate: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(resp.CaCertificate) {
		return fmt.Errorf("failed to parse CA certificate")
	}

	tlsConfig := &tls.Config{
		Certificates:     []tls.Certificate{cert},
		RootCAs:          caCertPool,
		MinVersion:       tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{tls.X25519, tls.X25519MLKEM768, tls.CurveP256},
		NextProtos:       []string{"h2", "http/1.1"}, // Prefer HTTP/2
	}

	upstreamURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", c.agentPort))
	if err != nil {
		return fmt.Errorf("failed to parse upstream URL: %w", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(upstreamURL)
	proxy.FlushInterval = -1

	server := &http.Server{
		Addr:                         fmt.Sprintf(":%d", c.proxyPort),
		TLSConfig:                    tlsConfig,
		DisableGeneralOptionsHandler: true,
		ReadTimeout:                  0,
		WriteTimeout:                 0,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Debug("incoming request: %s %s", r.Method, r.URL.Path)
			if c.ephemeral {
				switch r.URL.Path {
				case "/_health":
					sendCORSHeaders(w.Header())
					w.WriteHeader(http.StatusOK)
					return
				case "/_agents":
					sendCORSHeaders(w.Header())
					agents, err := c.getAgents(r.Context(), c.project.Project)
					if err != nil {
						c.logger.Error("failed to marshal agents control response: %s", err)
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
					c.HealthCheck(fmt.Sprintf("http://127.0.0.1:%d", c.agentPort)) // make sure the server is running
					io.WriteString(w, "event: start\ndata: connected\n\n")
					agents, err := c.getAgents(r.Context(), c.project.Project)
					if err != nil {
						c.logger.Error("failed to marshal agents control response: %s", err)
						io.WriteString(w, fmt.Sprintf("event: error\ndata: %q\n\n", err.Error()))
						rc.Flush()
						return
					}
					io.WriteString(w, fmt.Sprintf("event: agents\ndata: %s\n\n", cstr.JSONStringify(agents)))
					rc.Flush()
					select {
					case <-c.context.Done():
					case <-r.Context().Done():
					}
					io.WriteString(w, "event: stop\ndata: disconnected\n\n")
					rc.Flush()
					return
				default:
				}
			}
			started := time.Now()
			proxy.ServeHTTP(w, r)
			tp := r.Header.Get("traceparent")
			if tp != "" {
				tok := strings.Split(tp, "-")
				c.logger.Info("%s %s (sess_%s) in %s", r.Method, r.URL.Path, tok[1], time.Since(started))
			}
		}),
	}
	c.server = server

	go func() {
		if err := server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Fatal("failed to start gravity proxy HTTPS server: %v", err)
		}
	}()

	// Create netstack.
	s := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol, ipv6.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol},
	})
	c.stack = s

	// NIC that we can write raw packets into.
	linkEP := channel.New(1024, mtu, "")
	if err := s.CreateNIC(nicID, linkEP); err != nil {
		return fmt.Errorf("failed to create virtual NIC: %s", err)
	}
	c.endpoint = linkEP

	// Add IPv6 address
	ipBytes := net.ParseIP(ipv6Address.String()).To16()
	var addr6 [16]byte
	copy(addr6[:], ipBytes)
	if err := s.AddProtocolAddress(nicID,
		tcpip.ProtocolAddress{
			Protocol: ipv6.ProtocolNumber,
			AddressWithPrefix: tcpip.AddressWithPrefix{
				Address:   tcpip.AddrFrom16(addr6),
				PrefixLen: 64,
			},
		},
		stack.AddressProperties{},
	); err != nil {
		return fmt.Errorf("failed to create IPv6 protocol address: %s", err)
	}

	// Add IPv4 address
	ipv4Bytes := net.ParseIP(ipv4addr).To4()
	var addr4 [4]byte
	copy(addr4[:], ipv4Bytes)
	if err := s.AddProtocolAddress(nicID,
		tcpip.ProtocolAddress{
			Protocol: ipv4.ProtocolNumber,
			AddressWithPrefix: tcpip.AddressWithPrefix{
				Address:   tcpip.AddrFrom4(addr4),
				PrefixLen: 24,
			},
		},
		stack.AddressProperties{},
	); err != nil {
		return fmt.Errorf("failed to create IPv4 protocol address: %s", err)
	}

	// Add default routes for both IPv4 and IPv6
	subnet4, err := tcpip.NewSubnet(tcpip.AddrFromSlice(make([]byte, 4)), tcpip.MaskFromBytes(make([]byte, 4)))
	if err != nil {
		return fmt.Errorf("failed to create IPv4 subnet: %w", err)
	}
	subnet6, err := tcpip.NewSubnet(tcpip.AddrFromSlice(make([]byte, 16)), tcpip.MaskFromBytes(make([]byte, 16)))
	if err != nil {
		return fmt.Errorf("failed to create IPv6 subnet: %w", err)
	}
	s.SetRouteTable([]tcpip.Route{
		{
			Destination: subnet4,
			NIC:         nicID,
		},
		{
			Destination: subnet6,
			NIC:         nicID,
		},
	})

	// Start a TCP forwarder for every incoming connection using working pattern
	fwd := tcp.NewForwarder(s, 1024, 1024, func(r *tcp.ForwarderRequest) {
		wq := new(waiter.Queue)
		id := r.ID()
		log.Debug("incoming TCP connection: %s → %s", id.RemoteAddress, id.LocalAddress)

		log.Debug("about to call CreateEndpoint...")
		ep, err := r.CreateEndpoint(wq)
		log.Debug("CreateEndpoint returned: err=%v", err)

		if err != nil {
			log.Error("endpoint creation error: %v", err)
			r.Complete(true)
			return
		}

		r.Complete(false)

		tcpConn := gonet.NewTCPConn(wq, ep)
		log.Debug("created TCP conn, starting bridge to local server")
		go c.bridgeToLocalTLS(tcpConn)
	})
	s.SetTransportProtocolHandler(tcp.ProtocolNumber, fwd.HandlePacket)

	var network networkProvider
	var provider cliProvider
	provider.logger = log
	provider.ep = linkEP
	c.provider = &provider
	provider.connected = make(chan struct{}, 1)

	// Add egress pump to drain outbound packets from the channel endpoint
	go func() {
		log.Debug("starting egress pump...")
		for {
			select {
			case <-c.context.Done():
				return
			default:
				pkt := linkEP.ReadContext(c.context)
				if pkt == nil {
					continue
				}
				// Extract the raw packet data
				buf := pkt.ToBuffer()
				data := buf.Flatten()
				log.Trace("sending outbound packet (%d bytes)", len(data))
				// Send the raw IP packet to Gravity
				_, err := network.Write(data)
				pkt.DecRef() // free gvisor buffer
				if err != nil {
					log.Error("failed to send outbound packet: %v", err)
				}
			}
		}
	}()

	client, err := gravity.New(gravity.GravityConfig{
		Context:       c.context,
		Logger:        log,
		URL:           c.url,
		ClientName:    c.clientname,
		ClientVersion: c.version,
		AuthToken:     resp.ClientToken,
		Cert:          string(resp.Certificate),
		Key:           string(resp.PrivateKey),
		CACert:        string(resp.CaCertificate),
		InstanceID:    c.endpointID,
		ReportStats:   false,
		WorkingDir:    ".",
		ConnectionPoolConfig: &gravity.ConnectionPoolConfig{
			PoolSize:             1,
			StreamsPerConnection: 1,
			AllocationStrategy:   gravity.WeightedRoundRobin,
			HealthCheckInterval:  time.Second * 30,
			FailoverTimeout:      time.Second,
		},
		Capabilities:      capabilities,
		NetworkInterface:  &network,
		Provider:          &provider,
		IP4Address:        ipv4addr,
		IP6Address:        ipv6Address.String(),
		SkipAutoReconnect: true,
	})
	if err != nil {
		return fmt.Errorf("failed to create gravity client: %w", err)
	}
	c.client = client
	network.client = client

	if err := client.Start(); err != nil {
		return fmt.Errorf("failed to start the gravity client: %w", err)
	}

	select {
	case <-c.context.Done():
		return c.context.Err()
	case <-time.After(time.Second * 10):
		return fmt.Errorf("timed out waiting for provider connection")
	case <-provider.connected:
		break
	}

	// Start CONNECT proxy if configured
	if err := c.startConnectProxy(); err != nil {
		return fmt.Errorf("failed to start CONNECT proxy: %w", err)
	}

	go func() {
		log.Debug("waiting on provider disconnect")
		client.Disconnected(c.context)
		log.Debug("provider disconnected")
		select {
		case <-c.context.Done():
			log.Debug("provider disconnected but context canceled")
			c.Close()
			return
		default:
			log.Debug("provider disconnected, restarting")
			c.cleanup()
			log.Info("reconnecting to server ... one moment")
			select {
			case <-c.context.Done():
				return
			case <-time.After(2 * time.Second):
			}
			if err := c.Start(); err != nil {
				log.Fatal("failed to re-connect to the devmode server: %s", err)
			}
			log.Info("reconnected to devmode server")
		}
	}()

	success = true

	return nil
}
