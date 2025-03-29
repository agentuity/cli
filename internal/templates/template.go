package templates

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
	"gopkg.in/yaml.v3"
)

type TemplateContext struct {
	Context          context.Context
	Logger           logger.Logger
	Name             string
	Description      string
	AgentName        string
	AgentDescription string
	TemplateDir      string
	ProjectDir       string
	Template         *Template
	TemplateName     string
	AgentuityCommand string
}

func funcTemplates(t *template.Template) *template.Template {
	return t.Funcs(template.FuncMap{
		"safe_filename": func(s string) string {
			return util.SafeFilename(s)
		},
	})
}
func (t *TemplateContext) Interpolate(val any) any {
	if val == nil {
		return nil
	}
	if s, ok := val.(string); ok && s != "" && strings.Contains(s, "{{") {
		tmpl := template.New(t.Name)
		if strings.Contains(s, "|") { // slight optimization to avoid loading if not needed
			tmpl = funcTemplates(tmpl)
		}
		tmpl, err := tmpl.Parse(s)
		if err != nil {
			panic(fmt.Sprintf("failed to parse template %s: %s", val, err))
		}
		var out strings.Builder
		if err := tmpl.Execute(&out, t); err != nil {
			panic(fmt.Sprintf("failed to execute template %s: %s", val, err))
		}
		return out.String()
	}
	return val
}

type ProjectTemplate struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	Dependencies []string `yaml:"dependencies"`
	Steps        []any    `yaml:"steps"`
}

type NewProjectSteps struct {
	Steps []any `yaml:"steps"`
}

type NewAgentSteps struct {
	Steps []any `yaml:"steps"`
}

type Bundle struct {
	Enabled bool     `yaml:"enabled"`
	Ignore  []string `yaml:"ignore"`
}

type Resources struct {
	Memory string `yaml:"memory"`
	CPU    string `yaml:"cpu"`
}

type Watch struct {
	Enabled bool     `yaml:"enabled"`
	Files   []string `yaml:"files"`
}

type Development struct {
	Port    int      `yaml:"port"`
	Watch   Watch    `yaml:"watch"`
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
}

type Deployment struct {
	Resources Resources `yaml:"resources"`
	Command   string    `yaml:"command"`
	Args      []string  `yaml:"args"`
}

type TemplateRules struct {
	Identifier      string          `yaml:"identifier"`
	Runtime         string          `yaml:"runtime"`
	Language        string          `yaml:"language"`
	Framework       string          `yaml:"framework"`
	SrcDir          string          `yaml:"src_dir"`
	Filename        string          `yaml:"filename"`
	Bundle          Bundle          `yaml:"bundle"`
	Development     Development     `yaml:"development"`
	Deployment      Deployment      `yaml:"deployment"`
	NewProjectSteps NewProjectSteps `yaml:"new_project"`
	NewAgentSteps   NewAgentSteps   `yaml:"new_agent"`
}

type LanguageTemplates []ProjectTemplate

func LoadTemplateRuleForIdentifier(templateDir, identifier string) (*TemplateRules, error) {
	reader, err := getEmbeddedFile(filepath.Join(templateDir, identifier, "rules.yaml"))
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded file for %s: %s", identifier+"/rules.yaml", err)
	}
	defer reader.Close()
	var rules TemplateRules
	if err := yaml.NewDecoder(reader).Decode(&rules); err != nil {
		return nil, fmt.Errorf("failed to decode rules for %s: %s", identifier+"/rules.yaml", err)
	}
	return &rules, nil
}

func (t *TemplateRules) NewProject(ctx TemplateContext) error {
	templates, err := LoadLanguageTemplates(ctx.Context, ctx.TemplateDir, t.Identifier)
	if err != nil {
		return err
	}
	for _, step := range t.NewProjectSteps.Steps {
		if command, ok := resolveStep(ctx, step); ok {
			if err := command.Run(ctx); err != nil {
				return err
			}
		}
	}
	for _, template := range templates {
		if template.Name == ctx.TemplateName {
			for _, step := range template.Steps {
				if command, ok := resolveStep(ctx, step); ok {
					if err := command.Run(ctx); err != nil {
						return err
					}
				}
			}
			return nil
		}
	}
	return nil
}

func (t *TemplateRules) NewAgent(ctx TemplateContext) error {
	for _, step := range t.NewAgentSteps.Steps {
		if command, ok := resolveStep(ctx, step); ok {
			if err := command.Run(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}
