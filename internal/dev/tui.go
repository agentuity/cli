package dev

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/agentuity/go-common/tui"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

var (
	resumeKey         = key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "resume"), key.WithDisabled())
	pauseKey          = key.NewBinding(key.WithKeys("p"), key.WithHelp("p", "pause"))
	helpKey           = key.NewBinding(key.WithKeys("h"), key.WithHelp("h", "show help"))
	agentsKey         = key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "show agents"))
	logoColor         = lipgloss.AdaptiveColor{Light: "#11c7b9", Dark: "#00FFFF"}
	labelColor        = lipgloss.AdaptiveColor{Light: "#999999", Dark: "#FFFFFF"}
	runningColor      = lipgloss.AdaptiveColor{Light: "#00FF00", Dark: "#009900"}
	pausedColor       = lipgloss.AdaptiveColor{Light: "#FFA500", Dark: "#FFA500"}
	statusColor       = lipgloss.AdaptiveColor{Light: "#750075", Dark: "#FF5CFF"}
	runningStyle      = lipgloss.NewStyle().Foreground(runningColor)
	pausedStyle       = lipgloss.NewStyle().Foreground(pausedColor).AlignHorizontal(lipgloss.Center)
	labelStyle        = lipgloss.NewStyle().Foreground(labelColor).Bold(true)
	statusMsgStyle    = lipgloss.NewStyle().Foreground(statusColor).Margin(0)
	viewPortHelpStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#999999"}).Background(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#222222"}).AlignHorizontal(lipgloss.Left).MarginTop(1)
	statusMsgHeight   = 2
)

type model struct {
	infoBox       string
	statusMessage string
	logList       list.Model
	logItems      []list.Item
	windowSize    tea.WindowSizeMsg
	viewport      *viewport.Model
	paused        bool
	showhelp      bool
	showagents    bool
	agents        []*Agent
	selectedLog   *logItem
	spinner       spinner.Model
	spinning      bool
	devModeUrl    string
	publicUrl     string
	appUrl        string
}

type spinnerStartMsg struct{}
type spinnerStopMsg struct{}

type logItem string

func (i logItem) Title() string       { return strings.ReplaceAll(string(i), "\n", " ") }
func (i logItem) Description() string { return "" }
func (i logItem) FilterValue() string { return string(i) }

type tickMsg time.Time
type addLogMsg string
type statusMessageMsg string

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func initialModel(config DevModeConfig) *model {

	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		fmt.Println("Error getting terminal size:", err)
	}

	spinner := spinner.New(spinner.WithSpinner(spinner.Dot), spinner.WithStyle(statusMsgStyle.MarginLeft(1).MarginRight(0)))

	items := []list.Item{}

	listDelegate := list.NewDefaultDelegate()
	listDelegate.ShowDescription = false
	listDelegate.SetSpacing(0)
	listDelegate.Styles.NormalTitle = listDelegate.Styles.NormalTitle.Padding(0, 1)
	listDelegate.Styles.SelectedTitle = listDelegate.Styles.SelectedTitle.BorderLeft(false).Foreground(labelColor).Bold(true)

	l := list.New(items, listDelegate, width-2, 10)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)
	l.SetStatusBarItemName("log", "logs")
	l.Styles.NoItems = l.Styles.NoItems.MarginLeft(1)
	l.Styles.HelpStyle = l.Styles.HelpStyle.AlignHorizontal(lipgloss.Center).Width(width)

	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			resumeKey,
			pauseKey,
			helpKey,
			agentsKey,
		}
	}
	l.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{
			resumeKey,
			pauseKey,
			helpKey,
			agentsKey,
		}
	}

	m := &model{
		logList:    l,
		logItems:   items,
		spinner:    spinner,
		windowSize: tea.WindowSizeMsg{Width: width, Height: height},
		devModeUrl: config.DevModeUrl,
		publicUrl:  config.PublicUrl,
		appUrl:     config.AppUrl,
		agents:     config.Agents,
	}

	m.infoBox = m.generateInfoBox()

	infoBoxHeight := lipgloss.Height(m.infoBox)
	available := height - infoBoxHeight - statusMsgHeight
	if available < 1 {
		available = 1
	}
	m.logList.SetHeight(available)

	return m
}

func (m *model) Init() tea.Cmd {
	return tick()
}

func label(s string) string {
	return labelStyle.Render(tui.PadRight(s, 10, " "))
}

func (m *model) generateInfoBox() string {
	var statusStyle = runningStyle
	if m.paused {
		statusStyle = pausedStyle
	}
	var devmodeBox = lipgloss.NewStyle().
		Width(m.windowSize.Width-2).
		Border(lipgloss.NormalBorder()).
		BorderForeground(logoColor).
		Padding(1, 2).
		AlignVertical(lipgloss.Top).
		AlignHorizontal(lipgloss.Left).
		Foreground(labelColor)

	url := "loading..."
	if m.publicUrl != "" {
		url = tui.Link("%s", m.publicUrl) + "  " + tui.Muted("(only accessible while running)")
	}

	content := fmt.Sprintf(`%s

%s  %s
%s  %s
%s  %s`,
		tui.Bold("⨺ Agentuity DevMode")+" "+statusStyle.Render(tui.PadLeft("⏺", m.windowSize.Width-25, " ")),
		label("Dashboard"), tui.Link("%s", m.appUrl),
		label("Local"), tui.Link("%s", m.devModeUrl),
		label("Public"), url,
	)
	return devmodeBox.Render(content)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd []tea.Cmd

	switch msg := msg.(type) {
	case spinnerStartMsg:
		m.spinning = true
		break
	case spinnerStopMsg:
		m.spinning = false
		break
	case spinner.TickMsg:
		sm, c := m.spinner.Update(msg)
		m.spinner = sm
		cmd = append(cmd, c)
		break
	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			cmd = append(cmd, tea.Quit)
			break
		}
		if msg.Type == tea.KeyEscape {
			if m.showhelp {
				m.showhelp = false
				return m, nil
			}
			if m.showagents {
				m.showagents = false
				m.viewport = nil
				return m, nil
			}
			if m.selectedLog != nil {
				m.selectedLog = nil
				return m, nil
			}
		}
		if msg.Type == tea.KeyEnter && m.selectedLog == nil {
			if sel := m.logList.SelectedItem(); sel != nil {
				if log, ok := sel.(logItem); ok {
					m.selectedLog = &log
					break
				}
			}
		}
		if msg.Type == tea.KeyRunes {
			switch msg.String() {
			case "p":
				m.paused = true
				resumeKey.SetEnabled(true)
				pauseKey.SetEnabled(false)
			case "r":
				m.paused = false
				resumeKey.SetEnabled(false)
				pauseKey.SetEnabled(true)
			case "h":
				m.showhelp = true
			case "a":
				m.showagents = true
			}
			m.infoBox = m.generateInfoBox()
		}
		if m.viewport != nil {
			vp, vpCmd := m.viewport.Update(msg)
			m.viewport = &vp
			cmd = append(cmd, vpCmd)
		}
	case tea.WindowSizeMsg:
		m.windowSize = msg
		// Calculate the height for the info box
		infoBoxHeight := lipgloss.Height(m.infoBox)
		available := msg.Height - infoBoxHeight - statusMsgHeight
		if available < 1 {
			available = 1
		}
		m.logList.SetHeight(available)
		m.logList.SetWidth(m.windowSize.Width - 2)
		break
	case tickMsg:
		m.infoBox = m.generateInfoBox()
		cmd = append(cmd, tick())
		break
	case addLogMsg:
		m.logItems = append([]list.Item{logItem(msg)}, m.logItems...)
		if !m.paused {
			if m.logList.FilterState() == list.Unfiltered {
				m.logList.SetItems(m.logItems)
			}
		}
		break
	case statusMessageMsg:
		m.statusMessage = string(msg)
		break
	}

	var lcmd tea.Cmd
	m.logList, lcmd = m.logList.Update(msg)
	cmd = append(cmd, lcmd)
	return m, tea.Batch(cmd...)
}

func (m *model) View() string {

	var showModal bool
	var modalContent string
	modalWidth := m.windowSize.Width / 2
	modalHeight := m.windowSize.Height / 2
	if modalWidth < 40 {
		modalWidth = 40
	}
	if modalHeight < 10 {
		modalHeight = 10
	}

	if m.showhelp {
		showModal = true
		modalContent = lipgloss.JoinVertical(
			lipgloss.Left,
			tui.Bold("⨺ Agentuity DevMode"),
			"",
			tui.Secondary("When your project is running in DevMode, you can interact with it by sending messages to your local agents."),
			"", "",
			tui.Secondary("The best way to do this is to open the Agentuity console in your browser:"),
			"",
			tui.Link("%s", m.appUrl),
			"", "",
			tui.Secondary("You can also use curl or wget to send messages to the local agent."),
			"",
			tui.Secondary(fmt.Sprintf("To send a message to the local agent %s, use the following command:", m.agents[0].Name)),
			"", "",
			tui.Highlight(fmt.Sprintf("curl %s --json '{\"message\": \"Hello, world!\"}'", m.agents[0].LocalURL)),
			"",
			tui.Secondary(fmt.Sprintf("To send a message to the local agent %s from a remote machine, use the following command:", m.agents[0].Name)),
			"", "",
			tui.Highlight(fmt.Sprintf("curl %s --json '{\"message\": \"Hello, world!\"}'", m.agents[0].PublicURL)),
			"",
			tui.Muted("Note: The public URL is only accessible in devmode and has no authentication while in devmode. This this URL carefully."),
			"", "",
			tui.Warning("To get help or share your feedback, join our Discord community:"),
			"",
			tui.Link("https://discord.gg/vtn3hgUfuc"),
			"",
		)
	} else if m.selectedLog != nil {
		showModal = true
		modalContent = string(*m.selectedLog)
	} else if m.showagents {
		showModal = true
		modalContent = "Agents"
		var agentsContent string
		modalWidth = int(float64(m.windowSize.Width) * 0.9)
		for _, agent := range m.agents {
			agentsContent += fmt.Sprintf("%s %s\n", tui.PadRight("ID", 10, " "), tui.Muted(agent.ID))
			agentsContent += fmt.Sprintf("%s %s\n", tui.PadRight("Agent", 10, " "), tui.Title(agent.Name))
			agentsContent += fmt.Sprintf("%s %s\n", tui.PadRight("Local", 10, " "), tui.Link("%s", agent.LocalURL))
			agentsContent += fmt.Sprintf("%s %s\n", tui.PadRight("Public", 10, " "), tui.Link("%s", agent.PublicURL))
			agentsContent += "\n"
		}
		modalContent = agentsContent
	}

	if showModal {
		modal := lipgloss.NewStyle().Padding(2)
		if m.viewport == nil {
			vp := viewport.New(m.windowSize.Width, m.windowSize.Height-1)
			vp.SetYOffset(1)
			m.viewport = &vp
		}
		m.viewport.SetContent(modal.Render(modalContent))
		m.viewport.Width = m.windowSize.Width
		esc := "ESC to close"
		pct := fmt.Sprintf("%3.f%%", m.viewport.ScrollPercent()*100)
		spacer := m.windowSize.Width - lipgloss.Width(esc) - lipgloss.Width(pct) + 3
		right := lipgloss.NewStyle().AlignHorizontal(lipgloss.Right).Width(spacer).Render(pct)
		return m.viewport.View() + "\n" + viewPortHelpStyle.Width(m.windowSize.Width).Render(lipgloss.JoinHorizontal(lipgloss.Left, esc, right))
	}

	var view string

	if m.spinning {
		view = m.spinner.View() + " "
	} else {
		view = " "
	}

	return fmt.Sprintf("%s\n%s\n%s", m.infoBox, view+statusMsgStyle.Render(m.statusMessage), m.logList.View())
}

type Agent struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	LocalURL    string `json:"local_url,omitempty"`
	PublicURL   string `json:"public_url,omitempty"`
}

type DevModeUI struct {
	ctx     context.Context
	cancel  context.CancelFunc
	model   *model
	program *tea.Program
	wg      sync.WaitGroup
	once    sync.Once

	spinnerCtx    context.Context
	spinnerCancel context.CancelFunc
	aborting      bool
}

type DevModeConfig struct {
	DevModeUrl string
	PublicUrl  string
	AppUrl     string
	Agents     []*Agent
}

func NewDevModeUI(ctx context.Context, config DevModeConfig) *DevModeUI {
	ctx, cancel := context.WithCancel(ctx)
	return &DevModeUI{
		ctx:    ctx,
		cancel: cancel,
		model:  initialModel(config),
	}
}

func (d *DevModeUI) SetPublicURL(url string) {
	d.model.publicUrl = url
}

// Done returns a channel that will be closed when the program is done
func (d *DevModeUI) Done() <-chan struct{} {
	return d.ctx.Done()
}

// Close the program which will stop the program and wait for it to exit
func (d *DevModeUI) Close(abort bool) {
	d.once.Do(func() {
		d.aborting = abort
		d.program.Quit()
	})
}

// Start the program
func (d *DevModeUI) Start() {
	d.program = tea.NewProgram(
		d.model,
		tea.WithAltScreen(),
		tea.WithoutSignalHandler(),
	)
	d.wg.Add(1)
	go func() {
		defer func() {
			d.cancel()
			d.wg.Done()
			if d.aborting {
				for i := len(d.model.logItems) - 1; i >= 0; i-- {
					fmt.Println(d.model.logItems[i])
				}
			}
		}()
		_, err := d.program.Run()
		if err != nil {
			fmt.Printf("Error running program: %v\n", err)
		}
	}()
}

// Add a log message to the log list
func (d *DevModeUI) AddLog(log string, args ...any) {
	d.program.Send(addLogMsg(fmt.Sprintf(log, args...)))
}

func (d *DevModeUI) SetStatusMessage(msg string, args ...any) {
	val := fmt.Sprintf(msg, args...)
	d.program.Send(statusMessageMsg(val))
	if val != "" {
		go func() {
			select {
			case <-time.After(time.Second * 3):
				if val == d.model.statusMessage {
					d.program.Send(statusMessageMsg(""))
				}
			case <-d.ctx.Done():
				return
			}
		}()
	}
}

func (d *DevModeUI) ShowSpinner(msg string, fn func()) {
	d.SetSpinner(true)
	d.SetStatusMessage("%s", msg)
	fn()
	d.SetStatusMessage("")
	d.SetSpinner(false)
}

func (d *DevModeUI) SetSpinner(spinning bool) {
	if spinning {
		d.program.Send(spinnerStartMsg{})
		ctx, cancel := context.WithCancel(d.ctx)
		d.spinnerCtx = ctx
		d.spinnerCancel = cancel
		go func() {
			t := time.NewTicker(time.Millisecond * 200)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					d.program.Send(d.model.spinner.Tick())
				}
			}
		}()
	} else {
		d.spinnerCancel()
		d.spinnerCtx = nil
		d.program.Send(spinnerStopMsg{})
	}
}
