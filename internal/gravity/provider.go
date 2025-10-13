package gravity

import (
	"context"

	"github.com/agentuity/go-common/gravity"
	"github.com/agentuity/go-common/gravity/provider"
	"github.com/agentuity/go-common/logger"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/link/channel"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
)

type networkProvider struct {
	client *gravity.GravityClient
}

// RouteTraffic configures routing for the specified network ranges
func (p *networkProvider) RouteTraffic(nets []string) error {
	return nil
}

// UnrouteTraffic removes all routing configurations
func (p *networkProvider) UnrouteTraffic() error {
	return nil
}

// Read reads a packet from the TUN interface into the provided buffer
func (p *networkProvider) Read(buffer []byte) (int, error) {
	return 0, nil
}

// Write writes a packet to the TUN interface
func (p *networkProvider) Write(packet []byte) (int, error) {
	if err := p.client.WritePacket(packet); err != nil {
		return 0, err
	}
	return len(packet), nil
}

// Running returns true if the TUN interface is currently running
func (p *networkProvider) Running() bool {
	return true
}

// Start starts the TUN interface and calls the handler with each outbound packet.
// The packet passed to the handler is NOT a copy, so it must be copied if used after the handler returns.
func (p *networkProvider) Start(handler func(packet []byte)) {
}

type cliProvider struct {
	ep        *channel.Endpoint
	logger    logger.Logger
	config    provider.Configuration
	connected chan struct{}
}

// Configure will be called to configure the provider with the given configuration
func (p *cliProvider) Configure(config provider.Configuration) error {
	p.logger.Trace("configuring devmode provider with config: %+v", config)
	p.config = config
	p.connected <- struct{}{}
	return nil
}

// Provision provisions a resource for a given spec
func (p *cliProvider) Provision(ctx context.Context, request *provider.ProvisionRequest) (*provider.Resource, error) {
	return nil, nil
}

// Deprovision deprovisions a provisioned resource
func (p *cliProvider) Deprovision(ctx context.Context, resourceID string, reason provider.DeprovisionReason) error {
	return nil
}

// Resources returns a list of all resources regardless of state
func (p *cliProvider) Resources() []*provider.Resource {
	return nil
}

// SetMetricsCollector sets the metrics collector for runtime stats collection
func (p *cliProvider) SetMetricsCollector(collector provider.ProjectRuntimeStatsCollector) {
}

// ProcessInPacket processes an inbound packet from the gravity server
func (p *cliProvider) ProcessInPacket(payload []byte) {
	if p.ep == nil {
		return
	}

	if len(payload) < 1 {
		return
	}

	// Detect IP version from the packet header
	version := header.IPVersion(payload)
	var protocol tcpip.NetworkProtocolNumber

	switch version {
	case 4:
		protocol = ipv4.ProtocolNumber
	case 6:
		protocol = ipv6.ProtocolNumber
	default:
		p.logger.Trace("dropping packet: unknown IP version %d", version)
		return
	}

	// Create packet buffer with proper payload and cleanup
	pkt := stack.NewPacketBuffer(stack.PacketBufferOptions{
		Payload: buffer.MakeWithData(append([]byte(nil), payload...)),
	})
	defer pkt.DecRef()

	p.ep.InjectInbound(protocol, pkt)
}
