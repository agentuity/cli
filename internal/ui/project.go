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

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#D70000", Dark: "#FF0000"}).
			MarginTop(1)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "#00875F", Dark: "#00FF00"}).
			MarginTop(1)

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
		sb.WriteString("missing required dependencies:\n")
		for _, dep := range missing {
			sb.WriteString(fmt.Sprintf("• %s (version %s)\n", dep.Command, dep.Version))
			if dep.URL != "" {
				sb.WriteString(fmt.Sprintf("  Installation instructions: %s\n", dep.URL))
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
	// New fields for scrolling
	viewport      viewport.Model
	scrollOffset  int
	itemHeight    int
	contentHeight int
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
		if agentAuthType == "apikey" {
			authCursor = 1
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
		itemHeight:     4, // Each item takes about 4 lines with spacing
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

		if !m.ready {
			m.initViewport()
		} else {
			// Fixed elements heights (always including both scroll indicators)
			titleBarHeight := 3   // Title + description + spacing
			headerHeight := 4     // Step title + description + spacing
			footerHeight := 4     // Help text + spacing
			verticalMargins := 2  // Top and bottom margins
			scrollIndicators := 4 // Space for both scroll indicators (↑ and ↓)

			// Calculate total fixed height
			totalFixedHeight := titleBarHeight + headerHeight + footerHeight + verticalMargins + scrollIndicators

			// Update viewport dimensions
			m.viewport.Width = m.width - 4 // Account for left/right borders/padding
			m.viewport.Height = m.height - totalFixedHeight
		}

		return m, nil

	case tea.MouseMsg:
		if !m.ready {
			return m, nil
		}

		switch msg.String() {
		case "MouseWheelUp":
			m.viewport.LineUp(1)
			// Update cursor based on scroll position if needed
			m.updateCursorFromScroll()
		case "MouseWheelDown":
			m.viewport.LineDown(1)
			// Update cursor based on scroll position if needed
			m.updateCursorFromScroll()
		case "MouseLeft":
			m.mouseY = msg.Y
			// Convert mouse Y to list index considering scroll position
			clickedIndex := (msg.Y - 6 + m.viewport.YOffset) / m.itemHeight

			switch m.step {
			case 0:
				if clickedIndex >= 0 && clickedIndex < len(m.choices) {
					m.cursor = clickedIndex
					if m.runtime == m.choices[clickedIndex].ID {
						m.stepCursors[m.step] = m.cursor
						m.runtime = m.choices[m.cursor].ID
						m.runtimeName = m.choices[m.cursor].Name
						m.step++
						m.cursor = m.stepCursors[m.step]
					}
				}
			case 1:
				if templates, ok := m.templates[m.runtime]; ok {
					if clickedIndex >= 0 && clickedIndex < len(templates) {
						m.cursor = clickedIndex
						if m.template == templates[clickedIndex].Name {
							m.stepCursors[m.step] = m.cursor
							m.template = templates[m.cursor].Name
							m.step++
							m.cursor = 0
							m.projectName.Focus()
						}
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
			} else {
				m.cursor = 0
				m.stepCursors[m.step] = 0
			}
			m.depsError = ""
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quit = true
			return m, tea.Quit

		case "left", "esc":
			if m.step == 3 && !m.agentName.Focused() && !m.agentDesc.Focused() {
				if m.authCursor == 1 {
					// When on auth options and API Key is selected, focus None
					m.authCursor = 0
					m.agentAuthType = "none"
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
				} else {
					m.cursor = 0
					if m.step == 0 {
						m.runtime = ""
						m.runtimeName = ""
					} else if m.step == 1 {
						m.template = ""
					}
				}
				m.projectName.Blur()
				m.description.Blur()
				m.agentName.Blur()
				m.agentDesc.Blur()
			}

		case "right":
			if m.step == 3 && !m.agentName.Focused() && !m.agentDesc.Focused() {
				// Toggle between None and API Key
				m.authCursor = 1
				m.agentAuthType = "apikey"
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
					// Ensure cursor is visible after moving up
					if m.ready {
						m.ensureCursorVisible()
					}
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
					// Ensure cursor is visible after moving down
					if m.ready {
						m.ensureCursorVisible()
					}
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
					} else {
						m.agentAuthType = "apikey"
					}
					m.step++
					m.cursor = m.stepCursors[m.step]
				}
			} else if m.step == 4 {
				// Set deployment type based on cursor position
				m.deploymentType = deploymentOptions[m.cursor].ID
				return m, tea.Quit
			}

		case "pgup":
			if m.step <= 1 {
				m.viewport.HalfViewUp()
				m.updateCursorFromScroll()
			}

		case "pgdown":
			if m.step <= 1 {
				m.viewport.HalfViewDown()
				m.updateCursorFromScroll()
			}

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

	// Handle viewport updates
	if m.ready {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

// Add helper methods for scroll handling
func (m *projectFormModel) initViewport() {
	// Fixed element heights (always including both scroll indicators)
	titleBarHeight := 3   // Title + description + spacing
	headerHeight := 4     // Step title + description + spacing
	footerHeight := 4     // Help text + spacing
	verticalMargins := 2  // Top and bottom margins
	scrollIndicators := 4 // Space for both scroll indicators (↑ and ↓)

	// Calculate total fixed height (including both scroll indicators)
	totalFixedHeight := titleBarHeight + headerHeight + footerHeight + verticalMargins + scrollIndicators

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
	m.itemHeight = 6 // Each item takes 6 lines (title + description + spacing)
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

		m.updateViewportContent(content.String())
	}
}

func (m *projectFormModel) ensureCursorVisible() {
	if !m.ready {
		return
	}

	// Calculate the actual position of the cursor in the content
	cursorPos := m.cursor * m.itemHeight

	// Calculate visible area
	visibleStart := m.viewport.YOffset
	visibleEnd := visibleStart + m.viewport.Height - m.itemHeight

	// If cursor is above visible area, scroll up
	if cursorPos < visibleStart {
		m.viewport.SetYOffset(cursorPos)
	}

	// If cursor is below visible area, scroll down
	if cursorPos > visibleEnd {
		// Set offset to show cursor at the bottom of viewport
		m.viewport.SetYOffset(cursorPos - m.viewport.Height + m.itemHeight)
	}
}

func (m *projectFormModel) updateCursorFromScroll() {
	if !m.ready {
		return
	}

	// Update cursor based on scroll position
	viewTop := m.viewport.YOffset

	// Find the first fully visible item
	newCursor := viewTop / m.itemHeight

	// Ensure cursor stays within bounds
	maxItems := 0
	if m.step == 0 {
		maxItems = len(m.choices)
	} else if m.step == 1 && m.runtime != "" {
		maxItems = len(m.templates[m.runtime])
	}

	if newCursor >= maxItems {
		newCursor = maxItems - 1
	}
	if newCursor < 0 {
		newCursor = 0
	}

	m.cursor = newCursor
}

func (m projectFormModel) View() string {
	var s strings.Builder

	// Fixed title bar - Always rendered first and never included in scrollable content
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

		// Build scrollable content
		var scrollContent strings.Builder
		for _, item := range items {
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

		// Add scrollable content within viewport
		if m.ready {
			m.updateViewportContent(scrollContent.String())
			content.WriteString(m.viewport.View())

			// Add scroll indicators if content exceeds viewport
			if m.contentHeight > m.viewport.Height {
				var indicators strings.Builder
				if m.viewport.YOffset > 0 {
					indicators.WriteString("↑")
				} else {
					indicators.WriteString(" ")
				}
				indicators.WriteString(" • ")
				if m.viewport.YOffset+m.viewport.Height < m.contentHeight {
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
			} else if m.depsError != "" {
				content.WriteString("\n" + errorStyle.Render(m.depsError))
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
		} else if m.validationError != "" {
			content.WriteString(errorStyle.Render(m.validationError))
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
				content.WriteString(selectedItemStyle.UnsetForeground().Render("> [•] None") + "    " + itemStyle.UnsetForeground().Render("  [ ] API Key") + "\n")
			} else {
				content.WriteString(itemStyle.UnsetForeground().Render("  [ ] None") + "    " + selectedItemStyle.UnsetForeground().Render("> [•] API Key") + "\n")
			}
		} else {
			if m.authCursor == 0 {
				content.WriteString(itemStyle.UnsetForeground().Render("  [•] None      [ ] API Key\n"))
			} else {
				content.WriteString(itemStyle.UnsetForeground().Render("  [ ] None      [•] API Key\n"))
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
	if m.step <= 1 {
		help = append(help, "pgup/pgdn scroll")
	}
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
