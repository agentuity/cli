package templates

import (
	"context"
	"fmt"
	"strings"
	"text/template"

	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
	"gopkg.in/yaml.v3"
)

type TemplateContext struct {
	Context     context.Context
	Logger      logger.Logger
	Name        string
	Description string
	ProjectDir  string
	Template    *Template
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

type NewProjectSteps struct {
	InitialAgent InitialAgent `yaml:"initial"`
	Steps        []any        `yaml:"steps"`
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

type Deployment struct {
	Resources Resources `yaml:"resources"`
	Command   string    `yaml:"command"`
	Args      []string  `yaml:"args"`
}

type InitialAgent struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

type TemplateRules struct {
	Identifier      string          `yaml:"identifier"`
	Runtime         string          `yaml:"runtime"`
	Language        string          `yaml:"language"`
	Framework       string          `yaml:"framework"`
	SrcDir          string          `yaml:"src_dir"`
	Filename        string          `yaml:"filename"`
	Bundle          Bundle          `yaml:"bundle"`
	Deployment      Deployment      `yaml:"deployment"`
	NewProjectSteps NewProjectSteps `yaml:"new_project"`
	NewAgentSteps   NewAgentSteps   `yaml:"new_agent"`
}

func LoadTemplateRuleForIdentifier(identifier string) (*TemplateRules, error) {
	reader, err := getEmbeddedFile(identifier + "/rules.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded file for %s: %s", identifier+"/rules.yaml", err)
	}
	var rules TemplateRules
	if err := yaml.NewDecoder(reader).Decode(&rules); err != nil {
		return nil, fmt.Errorf("failed to decode rules for %s: %s", identifier+"/rules.yaml", err)
	}
	return &rules, nil
}

func (t *TemplateRules) NewProject(ctx TemplateContext) error {
	for _, step := range t.NewProjectSteps.Steps {
		if command, ok := resolveStep(ctx, step); ok {
			if err := command.Run(ctx); err != nil {
				return err
			}
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
