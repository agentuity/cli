package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/templates"
	"github.com/agentuity/go-common/logger"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	width = 76

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"}).
			Background(lipgloss.AdaptiveColor{Light: "#F0F0F0", Dark: "#0D0D0D"}).
			Width(100).
			Align(lipgloss.Center).
			Padding(0).MarginBottom(1)

	itemStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"})

	selectedItemStyle = lipgloss.NewStyle().Bold(true).
				Foreground(lipgloss.AdaptiveColor{Light: "#0087D7", Dark: "#00FFFF"})

	descriptionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#626262"}).
				Width(width - 4).
				PaddingLeft(4)

	descriptionSelectedStyle = lipgloss.NewStyle().
					Foreground(lipgloss.AdaptiveColor{Light: "#0087D7", Dark: "#00A3A3"}).
					Width(width - 4).
					PaddingLeft(4)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#626262"}).
			Width(width).
			Align(lipgloss.Left)

	paginatorStyle = lipgloss.NewStyle().
			Width(width).
			Align(lipgloss.Center)

	paginatorDotStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#626262"})

	paginatorActiveDotStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#0087D7", Dark: "#00FFFF"})

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#00875F", Dark: "#00FF00"}).
			MarginTop(1)

	modalStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "#D70000", Dark: "#FF0000"}).
			Background(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#1A1A1A"}).
			Foreground(lipgloss.AdaptiveColor{Light: "#000000", Dark: "#FFFFFF"}).
			Padding(1, 2).
			Width(50).
			Align(lipgloss.Left)

	contentStyle = lipgloss.NewStyle().
			Border(lipgloss.HiddenBorder()).
			BorderForeground(lipgloss.AdaptiveColor{Light: "#666666", Dark: "#626262"}).
			Padding(1).
			Width(width)
)

type Choice struct {
	ID          string
	Name        string
	Description string
}

type Template struct {
	ID            string
	Name          string
	Description   string
	template      *templates.Template
	SkipAgentStep bool
}

type DeploymentOption struct {
	ID          string
	Name        string
	Description string
}

type nameCheckResultMsg struct {
	isAvailable bool
	err         error
	sequence    int
}
type debounceMsg struct {
	sequence int
}

type dependencyCheckMsg struct {
	err error
}

type ProjectForm struct {
	Context             context.Context                 `json:"-"`
	Logger              logger.Logger                   `json:"-"`
	Runtime             string                          `json:"runtime"`
	Template            string                          `json:"template"`
	ProjectName         string                          `json:"projectName"`
	Description         string                          `json:"description"`
	AgentName           string                          `json:"agentName"`
	AgentDescription    string                          `json:"agentDescription"`
	AgentAuthType       string                          `json:"agentAuthType"`
	DeploymentType      string                          `json:"deploymentType"`
	TemplateDir         string                          `json:"-"`
	Templates           templates.Templates             `json:"-"`
	ValidateProjectName func(name string) (bool, error) `json:"-"`
	AgentuityCommand    string                          `json:"-"`
	Provider            *templates.Template             `json:"-"`
}

// CheckDependencies checks if all required dependencies for the given runtime template are met.
// Returns an error if any dependencies are missing, with details about what needs to be installed.
func (f *ProjectForm) CheckDependencies(template *templates.Template) error {
	if template == nil {
		return fmt.Errorf("template is nil")
	}

	// Create a template context for checking requirements
	ctx := templates.TemplateContext{
		Context:          f.Context,
		Logger:           f.Logger,
		TemplateDir:      f.TemplateDir,
		AgentuityCommand: f.AgentuityCommand,
	}

	// Check each requirement
	var missing []templates.Requirement
	for _, req := range template.Requirements {
		if !req.Matches(ctx) {
			missing = append(missing, req)
		}
	}

	if len(missing) > 0 {
		var sb strings.Builder
		sb.WriteString("missing required dependencies:\n\n")
		for _, dep := range missing {
			sb.WriteString(fmt.Sprintf("• %s (version %s)\n", dep.Command, dep.Version))
			if dep.URL != "" {
				sb.WriteString(fmt.Sprintf("\nInstallation instructions:\n\n%s\n", dep.URL))
			}
		}
		return fmt.Errorf("%s", sb.String())
	}

	return nil
}

// projectFormModel represents the form state
type projectFormModel struct {
	step            int
	cursor          int
	stepCursors     map[int]int
	runtime         string
	runtimeName     string
	template        string
	projectName     textinput.Model
	description     textinput.Model
	agentName       textinput.Model
	agentDesc       textinput.Model
	agentAuthType   string
	authCursor      int
	deploymentType  string
	validationError string
	choices         []Choice
	templates       map[string][]Template
	height          int
	width           int
	spinner         spinner.Model
	checkingName    bool
	nameEnter       bool
	sequence        int
	lastChecked     string
	nameValidated   bool
	mouseY          int
	checkingDeps    bool
	depsError       string
	form            ProjectForm
	quit            bool
	showErrorModal  bool
	// New fields for scrolling
	viewport      viewport.Model
	itemHeight    int
	contentHeight int
	windowStart   int
	windowSize    int
	ready         bool
}

var deploymentOptions = []DeploymentOption{
	{
		ID:          "github-action",
		Name:        "GitHub Action",
		Description: "Deploy using GitHub Actions workflow for automated deployment and CI/CD integration.",
	},
	{
		ID:          "github-app",
		Name:        "GitHub App",
		Description: "Deploy using a GitHub App for enhanced security and seamless integration with repositories.",
	},
	{
		ID:          "none",
		Name:        "None",
		Description: "Skip GitHub integration and configure deployment manually later.",
	},
}

func initialProjectModel(initialForm ProjectForm) projectFormModel {
	s := spinner.New()
	s.Spinner = spinner.Points
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	nameInput := textinput.New()
	nameInput.Placeholder = "my-project"
	if initialForm.ProjectName != "" {
		nameInput.SetValue(initialForm.ProjectName)
	}
	nameInput.CharLimit = 50
	nameInput.Width = 40
	nameInput.Prompt = "› "
	nameInput.Focus()
	nameInput.Validate = func(s string) error {
		return nil
	}

	descInput := textinput.New()
	descInput.Placeholder = "A cool project using..."
	if initialForm.Description != "" {
		descInput.SetValue(initialForm.Description)
	}
	descInput.CharLimit = 100
	descInput.Width = 40
	descInput.Prompt = "› "

	agentNameInput := textinput.New()
	agentNameInput.Placeholder = "my-agent"
	if initialForm.AgentName != "" {
		agentNameInput.SetValue(initialForm.AgentName)
	}
	agentNameInput.CharLimit = 50
	agentNameInput.Width = 40
	agentNameInput.Prompt = "› "
	agentNameInput.Validate = func(s string) error {
		return nil
	}

	agentDescInput := textinput.New()
	agentDescInput.Placeholder = "An agent that does..."
	if initialForm.AgentDescription != "" {
		agentDescInput.SetValue(initialForm.AgentDescription)
	}
	agentDescInput.CharLimit = 100
	agentDescInput.Width = 40
	agentDescInput.Prompt = "› "
	agentDescInput.Validate = func(s string) error {
		return nil
	}

	var cursor int
	var authCursor int
	stepCursors := make(map[int]int)
	var providers []Choice
	providerTemplates := make(map[string][]Template)
	var agentAuthType string
	var deploymentType string

	if initialForm.DeploymentType != "" {
		deploymentType = initialForm.DeploymentType
	}
	if initialForm.AgentAuthType != "" {
		agentAuthType = initialForm.AgentAuthType
		if agentAuthType == "none" {
			authCursor = 0
		} else if agentAuthType == "project" {
			authCursor = 1
		} else if agentAuthType == "bearer" {
			authCursor = 2
		}
	}

	if initialForm.Runtime != "" {
		for i, template := range initialForm.Templates {
			if template.Identifier == initialForm.Runtime || template.Name == initialForm.Runtime {
				cursor = i
				break
			}
		}
	}
	for _, template := range initialForm.Templates {
		providers = append(providers, Choice{
			ID:          template.Identifier,
			Name:        template.Name,
			Description: template.Description,
		})
		templates, err := templates.LoadLanguageTemplates(initialForm.Context, initialForm.TemplateDir, template.Identifier)
		if err != nil {
			errsystem.New(errsystem.ErrLoadTemplates, err, errsystem.WithContextMessage("Failed to load templates from template provider")).ShowErrorAndExit()
		}
		for i, t := range templates {
			providerTemplates[template.Identifier] = append(providerTemplates[template.Identifier], Template{
				ID:            t.Name,
				Name:          t.Name,
				Description:   t.Description,
				template:      &template,
				SkipAgentStep: t.SkipAgentStep,
			})
			if initialForm.Template == t.Name {
				stepCursors[1] = i
			}
		}
	}

	m := projectFormModel{
		step:           0,
		cursor:         cursor,
		stepCursors:    stepCursors,
		projectName:    nameInput,
		description:    descInput,
		agentName:      agentNameInput,
		agentDesc:      agentDescInput,
		agentAuthType:  agentAuthType,
		authCursor:     authCursor,
		deploymentType: deploymentType,
		spinner:        s,
		nameValidated:  false,
		choices:        providers,
		templates:      providerTemplates,
		checkingDeps:   false,
		depsError:      "",
		form:           initialForm,
		showErrorModal: false,
		itemHeight:     3,
		ready:          false,
	}

	return m
}

func (m projectFormModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick)
}

// Create a debounced command that will trigger after delay
func debouncedCmd(sequence int) tea.Cmd {
	return tea.Tick(time.Millisecond*500, func(t time.Time) tea.Msg {
		return debounceMsg{sequence: sequence}
	})
}

func checkNameAvailability(m projectFormModel, name string, sequence int) tea.Cmd {
	return func() tea.Msg {
		if m.form.ValidateProjectName != nil {
			ok, err := m.form.ValidateProjectName(name)
			if err != nil {
				return nameCheckResultMsg{isAvailable: false, err: err, sequence: sequence}
			}
			if !ok {
				return nameCheckResultMsg{isAvailable: false, err: fmt.Errorf("name '%s' is already taken", name), sequence: sequence}
			}
		}
		return nameCheckResultMsg{isAvailable: true, err: nil, sequence: sequence}
	}
}

func queueCheckDependencies(m *projectFormModel) func() tea.Msg {
	// Start dependency check
	m.checkingDeps = true
	m.depsError = ""
	// Find the selected template
	var selectedTemplate *templates.Template
	for _, t := range m.form.Templates {
		if t.Identifier == m.runtime {
			selectedTemplate = &t
			break
		}
	}

	// Check dependencies
	if selectedTemplate != nil {
		return func() tea.Msg {
			err := m.form.CheckDependencies(selectedTemplate)
			return dependencyCheckMsg{err: err}
		}
	}
	return func() tea.Msg {
		return dependencyCheckMsg{err: fmt.Errorf("template %s not found", m.runtime)}
	}
}

func (m projectFormModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width

		// Update styles that depend on window width
		titleStyle = titleStyle.Width(m.width)
		helpStyle = helpStyle.Width(m.width)
		// Recompute window size on resize
		m.initViewport()

		return m, nil

	case tea.MouseMsg:
		if !m.ready {
			return m, nil
		}
		if msg.String() == "MouseLeft" {
			m.mouseY = msg.Y
			// Approximate top offset of list area; keep prior 6-line offset heuristic
			localIndex := (msg.Y - 6) / m.itemHeight
			clickedIndex := m.windowStart + localIndex

			switch m.step {
			case 0:
				if clickedIndex >= 0 && clickedIndex < len(m.choices) {
					m.cursor = clickedIndex
					m.ensureCursorVisible()
				}
			case 1:
				if templates, ok := m.templates[m.runtime]; ok {
					if clickedIndex >= 0 && clickedIndex < len(templates) {
						m.cursor = clickedIndex
						m.ensureCursorVisible()
					}
				}
			}
		}

	case spinner.TickMsg:
		if m.checkingName || m.checkingDeps {
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}

	case debounceMsg:
		// Only process if this is the most recent debounce request, name isn't empty, and we haven't already checked this name
		if msg.sequence == m.sequence && m.projectName.Value() != m.lastChecked && m.projectName.Value() != "" {
			m.checkingName = true
			m.validationError = ""
			m.nameValidated = false // Reset validation state while checking
			m.lastChecked = m.projectName.Value()
			cmds = append(cmds, checkNameAvailability(m, m.projectName.Value(), msg.sequence))
		} else {
			m.checkingName = false
			m.validationError = ""
			m.lastChecked = ""
		}

	case nameCheckResultMsg:
		// Only process if this is the result for the most recent check
		if msg.sequence == m.sequence {
			m.checkingName = false
			if msg.err != nil {
				m.validationError = msg.err.Error()
				m.nameValidated = false
			} else {
				m.validationError = ""
				m.nameValidated = true
				if m.nameEnter {
					m.description.Focus()
					m.projectName.Blur()
					m.cursor++
					m.stepCursors[m.step] = m.cursor
					m.nameEnter = false
				}
			}
		}

	case dependencyCheckMsg:
		m.checkingDeps = false
		if msg.err != nil {
			m.depsError = msg.err.Error()
		} else {
			m.step++
			if templates, ok := m.templates[m.runtime]; ok {
				if m.form.Template != "" {
					for i, t := range templates {
						if t.Name == m.form.Template {
							m.stepCursors[m.step] = i
							break
						}
					}
				}
				if savedCursor, exists := m.stepCursors[m.step]; exists && savedCursor < len(templates) {
					m.cursor = savedCursor
				} else {
					m.cursor = 0
					m.stepCursors[m.step] = 0
				}
				// Ensure selection is visible when entering template step
				m.ensureCursorVisible()
			} else {
				m.cursor = 0
				m.stepCursors[m.step] = 0
			}
			m.depsError = ""
		}

	case tea.KeyMsg:
		// Handle error modal dismissal first
		if m.showErrorModal && (msg.Type == tea.KeyEnter || msg.Type == tea.KeyEscape) {
			m.showErrorModal = false
			m.depsError = ""
			m.validationError = ""
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c":
			m.quit = true
			return m, tea.Quit

		case "left", "esc":
			if m.step == 3 && !m.agentName.Focused() && !m.agentDesc.Focused() {
				if m.authCursor > 0 {
					m.authCursor--
					if m.authCursor == 0 {
						m.agentAuthType = "none"
					} else if m.authCursor == 1 {
						m.agentAuthType = "project"
					} else if m.authCursor == 2 {
						m.agentAuthType = "agent"
					}
					break
				}
			}
			if m.step > 0 {
				// Store current cursor position before going back
				m.stepCursors[m.step] = m.cursor
				m.step--
				// If we're going back from deployment step, check if we need to skip agent step
				if m.step == 3 {
					if templates, ok := m.templates[m.runtime]; ok {
						for _, t := range templates {
							if t.Name == m.template && t.SkipAgentStep {
								// Skip agent step and go back to project details
								m.step--
								break
							}
						}
					}
				}
				// Restore cursor position for the previous step
				if savedCursor, ok := m.stepCursors[m.step]; ok {
					m.cursor = savedCursor
					// Update the selection based on the restored cursor position
					if m.step == 0 {
						m.runtime = m.choices[m.cursor].ID
						m.runtimeName = m.choices[m.cursor].Name
					} else if m.step == 1 {
						if templates, ok := m.templates[m.runtime]; ok {
							m.template = templates[m.cursor].Name
						}
					}
					if m.step <= 1 {
						m.ensureCursorVisible()
					}
				} else {
					m.cursor = 0
					if m.step == 0 {
						m.runtime = ""
						m.runtimeName = ""
					} else if m.step == 1 {
						m.template = ""
					}
					if m.step <= 1 {
						m.windowStart = 0
					}
				}
				m.projectName.Blur()
				m.description.Blur()
				m.agentName.Blur()
				m.agentDesc.Blur()
			}

		case "right":
			if m.step == 3 && !m.agentName.Focused() && !m.agentDesc.Focused() {
				// Toggle between None, Project API Key, and Agent API Key
				if m.authCursor < 2 {
					m.authCursor++
					if m.authCursor == 0 {
						m.agentAuthType = "none"
					} else if m.authCursor == 1 {
						m.agentAuthType = "project"
					} else if m.authCursor == 2 {
						m.agentAuthType = "agent"
					}
				}
			} else {
				// Only advance if current step is valid
				if m.step == 0 && m.cursor < len(m.choices) {
					m.runtime = m.choices[m.cursor].ID
					m.runtimeName = m.choices[m.cursor].Name
					cmds = append(cmds, queueCheckDependencies(&m))
				} else if m.step == 1 && m.cursor < len(m.templates[m.runtime]) {
					// Store current cursor position before moving forward
					m.stepCursors[m.step] = m.cursor
					m.template = m.templates[m.runtime][m.cursor].Name
					m.step++
					m.cursor = m.stepCursors[m.step]
					m.projectName.Focus()
					m.windowStart = 0
				} else if m.step == 2 {
					if m.nameValidated && !m.checkingName && m.projectName.Value() != "" {
						// Store current cursor position before moving forward
						m.stepCursors[m.step] = m.cursor
						m.step++
						m.projectName.Blur()
						m.description.Blur()
						// Check if we should skip the agent step
						if templates, ok := m.templates[m.runtime]; ok {
							for _, t := range templates {
								if t.Name == m.template && t.SkipAgentStep {
									// Skip agent step and go directly to deployment
									m.step++
									m.cursor = m.stepCursors[m.step]
									break
								}
							}
						}
						if m.step == 3 {
							m.agentName.Focus()
						}
					}
				} else if m.step == 4 {
					return m, tea.Quit
				}
			}

		case "up":
			if m.step <= 1 { // Only for list views
				if m.cursor > 0 {
					m.cursor--
					m.ensureCursorVisible()
				}
			}
			if m.step == 2 {
				if m.description.Focused() {
					// Only allow moving up if name is validated
					if m.nameValidated && !m.checkingName {
						m.description.Blur()
						m.projectName.Focus()
					}
				}
			} else if m.step == 3 {
				if m.agentDesc.Focused() {
					m.agentDesc.Blur()
					m.agentName.Focus()
				} else if m.agentName.Focused() {
					// Do nothing, already at top
				} else {
					// Move focus back to agent description
					m.agentDesc.Focus()
				}
			} else if m.step == 4 {
				if m.cursor > 0 {
					m.cursor--
				}
			}

		case "down":
			if m.step <= 1 { // Only for list views
				maxItems := 0
				if m.step == 0 {
					maxItems = len(m.choices)
				} else if m.step == 1 && m.runtime != "" {
					maxItems = len(m.templates[m.runtime])
				}

				if m.cursor < maxItems-1 {
					m.cursor++
					m.ensureCursorVisible()
				}
			}
			if m.step == 2 {
				if m.projectName.Focused() {
					// Only allow moving down if name is validated
					if m.nameValidated && !m.checkingName {
						m.projectName.Blur()
						m.description.Focus()
					}
				} else if m.description.Focused() && m.nameValidated && !m.checkingName {
					m.step++
					m.description.Blur()
					// Check if we should skip the agent step
					if templates, ok := m.templates[m.runtime]; ok {
						for _, t := range templates {
							if t.Name == m.template && t.SkipAgentStep {
								// Skip agent step and go directly to deployment
								m.step++
								m.cursor = m.stepCursors[m.step]
								break
							}
						}
					}
					if m.step == 3 {
						m.agentName.Focus()
					}
				}
			} else if m.step == 3 {
				if m.agentName.Focused() {
					m.agentName.Blur()
					m.agentDesc.Focus()
				} else if m.agentDesc.Focused() {
					// Move to auth options
					m.agentDesc.Blur()
				} else {
					// Do nothing, already at bottom
				}
			} else if m.step == 4 {
				if m.cursor < 2 { // 3 options for deployment
					m.cursor++
				}
			}

		case "enter":
			if m.step == 0 {
				m.runtime = m.choices[m.cursor].ID
				m.runtimeName = m.choices[m.cursor].Name
				cmds = append(cmds, queueCheckDependencies(&m))
			} else if m.step == 1 {
				// Store current cursor position before moving forward
				m.stepCursors[m.step] = m.cursor
				m.template = m.templates[m.runtime][m.cursor].Name
				m.step++
				m.cursor = m.stepCursors[m.step]
				// Focus the project name input when entering step 2
				m.projectName.Focus()
				m.nameEnter = false
				m.windowStart = 0

			} else if m.step == 2 {
				if m.projectName.Focused() {
					// If name field is focused, trigger immediate validation
					if m.projectName.Value() != "" && !m.checkingName {
						if err := m.projectName.Validate(m.projectName.Value()); err != nil {
							m.validationError = err.Error()
						} else {
							m.sequence++
							m.nameEnter = true
							m.checkingName = true
							m.validationError = ""
							m.nameValidated = false
							m.lastChecked = m.projectName.Value()
							cmds = append(cmds, checkNameAvailability(m, m.projectName.Value(), m.sequence))
						}
					}
				} else if m.description.Focused() && m.nameValidated && !m.checkingName {
					m.step++
					m.description.Blur()
					// Check if we should skip the agent step
					if templates, ok := m.templates[m.runtime]; ok {
						for _, t := range templates {
							if t.Name == m.template && t.SkipAgentStep {
								// Skip agent step and go directly to deployment
								m.step++
								m.cursor = m.stepCursors[m.step]
								break
							}
						}
					}
					if m.step == 3 {
						m.agentName.Focus()
					}
				}
			} else if m.step == 3 {
				if m.agentName.Focused() {
					m.agentName.Blur()
					m.agentDesc.Focus()
				} else if m.agentDesc.Focused() {
					m.agentDesc.Blur()
				} else {
					// When on auth options, confirm selection and move to deployment step
					if m.authCursor == 0 {
						m.agentAuthType = "none"
					} else if m.authCursor == 1 {
						m.agentAuthType = "project"
					} else if m.authCursor == 2 {
						m.agentAuthType = "agent"
					}
					m.step++
					m.cursor = m.stepCursors[m.step]
				}
			} else if m.step == 4 {
				// Set deployment type based on cursor position
				m.deploymentType = deploymentOptions[m.cursor].ID
				return m, tea.Quit
			}

			// Remove free scrolling keys

		case "tab", "shift+tab":
			if m.step == 2 {
				if msg.String() == "shift+tab" {
					// Shift+Tab: Move backward
					if m.description.Focused() {
						m.description.Blur()
						m.projectName.Focus()
					}
				} else {
					// Tab: Move forward
					if m.projectName.Focused() && m.nameValidated && !m.checkingName {
						m.projectName.Blur()
						m.description.Focus()
					}
				}
			} else if m.step == 3 {
				if msg.String() == "shift+tab" {
					// Shift+Tab: Move backward
					if m.agentDesc.Focused() {
						m.agentDesc.Blur()
						m.agentName.Focus()
					} else if !m.agentName.Focused() && !m.agentDesc.Focused() {
						// If on auth options, move back to description
						m.agentDesc.Focus()
					}
				} else {
					// Tab: Move forward
					if m.agentName.Focused() {
						m.agentName.Blur()
						m.agentDesc.Focus()
					} else if m.agentDesc.Focused() {
						// Move to auth options
						m.agentDesc.Blur()
					}
				}
			}
		}
	}

	// Handle text input
	if m.step == 2 {
		if m.projectName.Focused() {
			oldValue := m.projectName.Value()
			m.projectName, cmd = m.projectName.Update(msg)
			cmds = append(cmds, cmd)

			// Check if the name changed
			if oldValue != m.projectName.Value() {
				// Reset validation state when input changes
				m.nameValidated = false
				m.validationError = ""
				// First validate format
				if err := m.projectName.Validate(m.projectName.Value()); err != nil {
					m.validationError = err.Error()
					m.lastChecked = "" // Reset last checked name
				} else if m.projectName.Value() != "" {
					// Only start validation if name isn't empty
					m.sequence++
					cmds = append(cmds, debouncedCmd(m.sequence))
				}
			}
		} else if m.description.Focused() {
			m.description, cmd = m.description.Update(msg)
			cmds = append(cmds, cmd)
		}
	} else if m.step == 3 {
		if m.agentName.Focused() {
			m.agentName, cmd = m.agentName.Update(msg)
			cmds = append(cmds, cmd)
		} else if m.agentDesc.Focused() {
			m.agentDesc, cmd = m.agentDesc.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	// Handle viewport updates: set YOffset based on windowStart so items are fully shown
	if m.ready && m.step <= 1 {
		// In paged mode the content is already sliced; keep offset at 0
		if m.viewport.YOffset != 0 {
			m.viewport.SetYOffset(0)
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	// Update error modal state
	if m.depsError != "" || m.validationError != "" {
		m.showErrorModal = true
	}

	return m, tea.Batch(cmds...)
}

// Add helper methods for scroll handling
func (m *projectFormModel) initViewport() {
	// Fixed element heights
	titleBarHeight := 3  // Title + spacing
	headerHeight := 4    // Step title + description + spacing
	footerHeight := 4    // Help text + spacing
	verticalMargins := 2 // Top and bottom margins

	// Calculate total fixed height
	totalFixedHeight := titleBarHeight + headerHeight + footerHeight + verticalMargins

	// Calculate available height for viewport
	availableHeight := m.height - totalFixedHeight
	if availableHeight < 0 {
		availableHeight = 0
	}

	// Calculate content width accounting for borders
	contentWidth := width - 2 // Subtract left and right borders
	if contentWidth < 20 {    // Minimum reasonable width
		contentWidth = 20
	}

	// Initialize viewport with calculated dimensions
	m.viewport = viewport.New(contentWidth, availableHeight)
	m.viewport.Style = lipgloss.NewStyle().Padding(0, 1) // Add left/right padding within viewport

	// Set item height and mark model as ready
	m.itemHeight = 3 // Each item uses 3 lines (title + description + spacing)
	if availableHeight <= 0 {
		m.windowSize = 0
	} else {
		m.windowSize = availableHeight / m.itemHeight
		if m.windowSize < 1 {
			m.windowSize = 1
		}
	}
	m.ready = true

	// Force initial update of viewport content
	if m.step <= 1 {
		var content strings.Builder

		// Build content based on current step
		if m.step == 0 {
			for i, choice := range m.choices {
				if m.cursor == i {
					content.WriteString(fmt.Sprintf("> %s\n", selectedItemStyle.Render(choice.Name)))
					content.WriteString(descriptionSelectedStyle.PaddingLeft(2).Render(choice.Description) + "\n")
					content.WriteString("\n") // Extra line for spacing
				} else {
					content.WriteString(fmt.Sprintf("  %s\n", itemStyle.Render(choice.Name)))
					content.WriteString(descriptionStyle.PaddingLeft(2).Render(choice.Description) + "\n")
					content.WriteString("\n") // Extra line for spacing
				}
			}
		} else if templates, ok := m.templates[m.runtime]; ok {
			for i, template := range templates {
				if m.cursor == i {
					content.WriteString(fmt.Sprintf("> %s\n", selectedItemStyle.Render(template.Name)))
					content.WriteString(descriptionSelectedStyle.PaddingLeft(2).Render(template.Description) + "\n")
					content.WriteString("\n") // Extra line for spacing
				} else {
					content.WriteString(fmt.Sprintf("  %s\n", itemStyle.Render(template.Name)))
					content.WriteString(descriptionStyle.PaddingLeft(2).Render(template.Description) + "\n")
					content.WriteString("\n") // Extra line for spacing
				}
			}
		}

		// Ensure selected option will be in frame after resize
		m.ensureCursorVisible()
		m.updateViewportContent(content.String())
	}
}

func (m *projectFormModel) ensureCursorVisible() {
	if !m.ready || m.step > 1 {
		return
	}

	// Get the total number of items based on current step
	var totalItems int
	if m.step == 0 {
		totalItems = len(m.choices)
	} else if m.step == 1 {
		if templates, ok := m.templates[m.runtime]; ok {
			totalItems = len(templates)
		}
	}

	// Compute window so that selected item is fully visible
	if m.windowSize <= 0 || totalItems == 0 {
		return
	}

	// If cursor above window, move windowStart up to cursor
	if m.cursor < m.windowStart {
		m.windowStart = m.cursor
	}

	// If cursor beyond window end, shift window to include it
	windowEnd := m.windowStart + m.windowSize - 1
	if m.cursor > windowEnd {
		m.windowStart = m.cursor - (m.windowSize - 1)
	}

	if m.windowStart < 0 {
		m.windowStart = 0
	}

	// Clamp windowStart to ensure it doesn't exceed valid bounds
	maxWindowStart := totalItems - m.windowSize
	if maxWindowStart < 0 {
		maxWindowStart = 0
	}
	if m.windowStart > maxWindowStart {
		m.windowStart = maxWindowStart
	}
}

func (m *projectFormModel) updateCursorFromScroll() {
	// No free scroll; nothing to do
}

func (m projectFormModel) View() string {
	var s strings.Builder

	// If error modal is shown, render it over the main content
	if m.showErrorModal {
		// Get the error message
		errorMsg := ""
		if m.depsError != "" {
			errorMsg = m.depsError
		} else if m.validationError != "" {
			errorMsg = m.validationError
		}

		// Format the error message with proper line breaks and indentation
		lines := strings.Split(errorMsg, "\n")
		var formattedMsg strings.Builder

		formattedMsg.WriteString("Error\n\n")

		for _, line := range lines {
			formattedMsg.WriteString(line + "\n")
		}

		// Add a note about how to dismiss the modal
		formattedMsg.WriteString("\nPress Enter to continue")

		// Create the modal content
		modalContent := modalStyle.Render(strings.TrimRight(formattedMsg.String(), "\n"))
		modalHeight := strings.Count(modalContent, "\n") + 1
		modalWidth := lipgloss.Width(modalContent)

		// Calculate position to center the modal
		leftPadding := (m.width - modalWidth) / 2
		if leftPadding < 0 {
			leftPadding = 0
		}
		topPadding := (m.height - modalHeight) / 2
		if topPadding < 0 {
			topPadding = 0
		}

		// Create a full-screen overlay with the modal centered
		var overlay strings.Builder

		// Add top padding
		for i := 0; i < topPadding; i++ {
			overlay.WriteString(strings.Repeat(" ", m.width) + "\n")
		}

		// Add the modal with horizontal centering
		modalLines := strings.Split(modalContent, "\n")
		for _, line := range modalLines {
			overlay.WriteString(strings.Repeat(" ", leftPadding) + line + "\n")
		}

		// Return just the overlay when showing error
		return overlay.String()
	}

	// Regular view rendering
	s.WriteString(titleStyle.Render("⨺ Create New Agentuity Project"))
	s.WriteString("\n")

	// Build content
	var content strings.Builder

	// Handle different steps
	switch m.step {
	case 0, 1: // Use viewport for runtime and template selection
		var title, description string
		var items []struct {
			name, desc string
			selected   bool
		}

		if m.step == 0 {
			title = "Select Runtime:"
			description = "Choose the runtime environment for your project"
			for i, choice := range m.choices {
				items = append(items, struct {
					name, desc string
					selected   bool
				}{
					name:     choice.Name,
					desc:     choice.Description,
					selected: m.cursor == i,
				})
			}
		} else {
			title = "Select Template:"
			description = "Choose a framework template for your " + m.runtimeName + " project"
			if templates, ok := m.templates[m.runtime]; ok {
				for i, template := range templates {
					items = append(items, struct {
						name, desc string
						selected   bool
					}{
						name:     template.Name,
						desc:     template.Description,
						selected: m.cursor == i,
					})
				}
			}
		}

		// Fixed step header - Always rendered before viewport
		content.WriteString(titleStyle.UnsetBackground().UnsetWidth().Underline(true).Render(title))
		content.WriteString("\n")
		content.WriteString(descriptionStyle.UnsetWidth().UnsetPaddingLeft().Render(description))
		content.WriteString("\n\n")

		// Build paged content: only full items that fit
		var scrollContent strings.Builder
		// Derive visible slice
		totalItems := len(items)
		if m.windowSize <= 0 {
			m.windowSize = 1
		}
		// Make sure selection is clamped and window follows selection
		if m.cursor >= totalItems {
			m.cursor = totalItems - 1
		}
		if m.cursor < 0 {
			m.cursor = 0
		}
		m.ensureCursorVisible()
		if m.windowStart < 0 {
			m.windowStart = 0
		}
		if m.windowStart > totalItems {
			m.windowStart = 0
		}
		end := m.windowStart + m.windowSize
		if end > totalItems {
			end = totalItems
		}
		visible := items[m.windowStart:end]
		for _, item := range visible {
			if item.selected {
				scrollContent.WriteString(fmt.Sprintf("> %s\n", selectedItemStyle.Render(item.name)))
				scrollContent.WriteString(descriptionSelectedStyle.PaddingLeft(2).Render(item.desc) + "\n")
				scrollContent.WriteString("\n") // Extra line for spacing
			} else {
				scrollContent.WriteString(fmt.Sprintf("  %s\n", itemStyle.Render(item.name)))
				scrollContent.WriteString(descriptionStyle.PaddingLeft(2).Render(item.desc) + "\n")
				scrollContent.WriteString("\n") // Extra line for spacing
			}
		}

		// Add content within viewport
		if m.ready {
			m.updateViewportContent(scrollContent.String())
			content.WriteString(m.viewport.View())
			// Paged-mode indicators based on window position
			if totalItems > m.windowSize {
				var indicators strings.Builder
				if m.windowStart > 0 {
					indicators.WriteString("↑")
				} else {
					indicators.WriteString(" ")
				}
				indicators.WriteString(" • ")
				if m.windowStart+m.windowSize < totalItems {
					indicators.WriteString("↓")
				} else {
					indicators.WriteString(" ")
				}
				content.WriteString("\n" + descriptionStyle.Copy().Align(lipgloss.Center).Render(indicators.String()))
			}
		} else {
			content.WriteString(scrollContent.String())
		}

		// Add dependency check status to step 0 view
		if m.step == 0 {
			if m.checkingDeps {
				content.WriteString("\n" + descriptionStyle.UnsetWidth().UnsetPaddingLeft().UnsetMarginLeft().MarginTop(1).Render(m.spinner.View()+" checking dependencies..."))
			}
		}

	case 2:
		content.WriteString(titleStyle.UnsetBackground().UnsetWidth().Underline(true).Render("Project Details:"))
		content.WriteString("\n")
		content.WriteString(descriptionStyle.UnsetWidth().UnsetPaddingLeft().UnsetMarginLeft().Render("Enter your project details"))
		content.WriteString("\n\n")

		// Project name input (required)
		content.WriteString(selectedItemStyle.Render("Project Name") + descriptionStyle.UnsetWidth().Render(" (required)") + "\n")
		content.WriteString(m.projectName.View() + "\n")
		if m.checkingName {
			content.WriteString(descriptionStyle.UnsetWidth().UnsetPaddingLeft().UnsetMarginLeft().MarginTop(1).Render(m.spinner.View() + " checking availability"))
		} else if m.nameValidated {
			content.WriteString(successStyle.Render("✓ name is available"))
		} else {
			content.WriteString("\n")
		}
		content.WriteString("\n\n")

		// Description input (optional)
		content.WriteString(selectedItemStyle.Render("Description") + descriptionStyle.UnsetWidth().Render(" (optional)") + "\n")
		content.WriteString(m.description.View() + "\n")

	case 3:
		content.WriteString(titleStyle.UnsetBackground().UnsetWidth().Underline(true).Render("Agent Details:"))
		content.WriteString("\n")
		content.WriteString(descriptionStyle.UnsetWidth().UnsetPaddingLeft().UnsetMarginLeft().Render("Configure your initial agent"))
		content.WriteString("\n\n")

		// Agent name input (optional)
		content.WriteString(selectedItemStyle.Render("Agent Name") + descriptionStyle.UnsetWidth().Render(" (optional)") + "\n")
		content.WriteString(m.agentName.View() + "\n\n")

		// Agent description input (optional)
		content.WriteString(selectedItemStyle.Render("Agent Description") + descriptionStyle.UnsetWidth().Render(" (optional)") + "\n")
		content.WriteString(m.agentDesc.View() + "\n\n")

		// Authentication type selection
		content.WriteString(selectedItemStyle.Render("Authentication") + "\n")
		if !m.agentName.Focused() && !m.agentDesc.Focused() {
			if m.authCursor == 0 {
				content.WriteString(itemStyle.UnsetForeground().Render("  [•] None      [ ] Project API Key      [ ] Agent API Key\n"))
			} else if m.authCursor == 1 {
				content.WriteString(itemStyle.UnsetForeground().Render("  [ ] None      [•] Project API Key      [ ] Agent API Key\n"))
			} else if m.authCursor == 2 {
				content.WriteString(itemStyle.UnsetForeground().Render("  [ ] None      [ ] Project API Key      [•] Agent API Key\n"))
			}
		} else {
			if m.authCursor == 0 {
				content.WriteString(itemStyle.UnsetForeground().Render("  [•] None      [ ] Project API Key      [ ] Agent API Key\n"))
			} else if m.authCursor == 1 {
				content.WriteString(itemStyle.UnsetForeground().Render("  [ ] None      [•] Project API Key      [ ] Agent API Key\n"))
			} else if m.authCursor == 2 {
				content.WriteString(itemStyle.UnsetForeground().Render("  [ ] None      [ ] Project API Key      [•] Agent API Key\n"))
			}
		}

	case 4:
		content.WriteString(titleStyle.UnsetBackground().UnsetWidth().Underline(true).Render("Deployment Options:"))
		content.WriteString("\n")
		content.WriteString(descriptionStyle.UnsetWidth().UnsetPaddingLeft().UnsetMarginLeft().Render("Choose how you want to deploy your project"))
		content.WriteString("\n\n")

		for i, option := range deploymentOptions {
			if m.cursor == i {
				content.WriteString(fmt.Sprintf("> %s\n", selectedItemStyle.Render(option.Name)))
				content.WriteString(descriptionSelectedStyle.PaddingLeft(2).Render(option.Description) + "\n\n")
			} else {
				content.WriteString(fmt.Sprintf("  %s\n", itemStyle.Render(option.Name)))
				content.WriteString(descriptionStyle.PaddingLeft(2).Render(option.Description) + "\n\n")
			}
		}
	}

	// Paginator
	content.WriteString("\n\n")
	var dots []string
	for i := 0; i < 5; i++ {
		if i == m.step {
			dots = append(dots, paginatorActiveDotStyle.Render("●"))
		} else {
			dots = append(dots, paginatorDotStyle.Render("○"))
		}
	}
	content.WriteString(paginatorStyle.Render(strings.Join(dots, " ")))

	// Center the bordered content box
	borderedContent := contentStyle.Render(content.String())
	if m.width > 0 {
		boxWidth := width + 2 //  content width + 2 for borders
		horizontalPadding := (m.width - boxWidth) / 2
		if horizontalPadding > 0 {
			lines := strings.Split(borderedContent, "\n")
			var centeredContent strings.Builder
			for _, line := range lines {
				if line != "" {
					centeredContent.WriteString(strings.Repeat(" ", horizontalPadding) + line + "\n")
				} else {
					centeredContent.WriteString("\n")
				}
			}
			s.WriteString(centeredContent.String())
		} else {
			s.WriteString(borderedContent + "\n")
		}
	} else {
		s.WriteString(borderedContent + "\n")
	}

	// Help bar (fixed at bottom)
	help := []string{"↑ up", "↓ down"}
	// No free scrolling in list views
	switch m.step {
	case 2:
		if m.projectName.Value() != "" && m.nameValidated && !m.checkingName {
			help = append(help, "enter/→ next")
		}
	case 3:
		if !m.agentName.Focused() && !m.agentDesc.Focused() {
			help = append(help, "←/→ select", "enter confirm")
		}
		help = append(help, "enter/→ finish")
	case 4:
		help = append(help, "←/esc back")
	}
	help = append(help, "ctrl+c quit")

	helpBar := strings.Join(help, " • ")
	if m.width > 0 {
		boxWidth := width + 2
		helpBarWidth := len(helpBar)
		helpBarPadding := (boxWidth - helpBarWidth) / 2
		if helpBarPadding > 0 {
			helpBar = strings.Repeat(" ", helpBarPadding) + helpBar
		}
		windowPadding := (m.width - boxWidth) / 2
		if windowPadding > 0 {
			helpBar = strings.Repeat(" ", windowPadding) + helpBar
		}
	}
	s.WriteString("\n" + helpStyle.UnsetWidth().Render(helpBar))

	return s.String()
}

func ShowProjectUI(initial ProjectForm) ProjectForm {
	p := tea.NewProgram(
		initialProjectModel(initial),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
		tea.WithMouseAllMotion(),
	)
	m, err := p.Run()
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Get the final model
	finalModel := m.(projectFormModel)

	if finalModel.quit {
		os.Exit(1)
	}

	var provider *templates.Template
	for _, t := range initial.Templates {
		if t.Identifier == finalModel.runtime {
			finalModel.runtime = t.Name
			provider = &t
			break
		}
	}

	if provider == nil {
		panic(fmt.Sprintf("provider not found: %s", finalModel.runtime))
	}

	return ProjectForm{
		Runtime:          finalModel.runtime,
		Template:         finalModel.template,
		ProjectName:      finalModel.projectName.Value(),
		Description:      finalModel.description.Value(),
		AgentName:        finalModel.agentName.Value(),
		AgentDescription: finalModel.agentDesc.Value(),
		AgentAuthType:    finalModel.agentAuthType,
		DeploymentType:   finalModel.deploymentType,
		Provider:         provider,
	}
}

func (m *projectFormModel) updateViewportContent(content string) {
	if !m.ready {
		return
	}

	// Calculate total height in lines
	lines := strings.Count(content, "\n") + 1
	m.contentHeight = lines

	// Set content
	m.viewport.SetContent(content)

	// Ensure cursor is visible after content update
	m.ensureCursorVisible()
}
