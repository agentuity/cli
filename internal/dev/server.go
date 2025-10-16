package dev

import (
	"fmt"

	"github.com/agentuity/cli/internal/gravity"
	"github.com/agentuity/go-common/tui"
	"github.com/charmbracelet/lipgloss"
)

type Server struct {
	args   ServerArgs
	client *gravity.Client
	config *gravity.Config
}

type ServerArgs struct {
	APIURL   string
	APIKey   string
	Hostname string
	*gravity.Config
}

// Close closes the bridge client and cleans up the connection
func (s *Server) Close() error {
	return s.client.Close()
}

func (s *Server) WebURL(appUrl string) string {
	return fmt.Sprintf("%s/devmode/%s", appUrl, s.args.EndpointID)
}

func (s *Server) PublicURL(appUrl string) string {
	return fmt.Sprintf("https://%s", s.args.Hostname)
}

func (s *Server) AgentURL(agentId string) string {
	return fmt.Sprintf("http://127.0.0.1:%d/%s", s.config.AgentPort, agentId)
}

func (s *Server) TelemetryURL() string {
	return s.client.TelemetryURL()
}

func (s *Server) TelemetryAPIKey() string {
	return s.client.TelemetryAPIKey()
}

func (s *Server) HealthCheck(devModeUrl string) error {
	return s.client.HealthCheck(devModeUrl)
}

func (s *Server) Connect() error {
	return s.client.Start()
}

func (s *Server) EvalChannel() <-chan gravity.EvalInfo {
	return s.client.EvalChannel()
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
	server := &Server{
		args:   args,
		config: args.Config,
		client: gravity.New(*args.Config),
	}
	return server, nil
}
