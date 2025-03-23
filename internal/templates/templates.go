package templates

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/agentuity/cli/internal/util"
	"gopkg.in/yaml.v3"
)

type ErrRequirementsNotMet struct {
	Message string
}

func (e *ErrRequirementsNotMet) Error() string {
	return e.Message
}

type Requirement struct {
	Command    string   `yaml:"command"`
	Args       []string `yaml:"args"`
	Version    string   `yaml:"version"`
	Brew       string   `yaml:"brew"`
	URL        string   `yaml:"url"`
	Selfupdate *struct {
		Command string   `yaml:"command"`
		Args    []string `yaml:"args"`
	} `yaml:"selfupdate"`
}

func (r *Requirement) hasCommand(cmd string) (string, bool) {
	lookup, err := exec.LookPath(cmd)
	if err != nil {
		return "", false
	}
	return lookup, true
}

func (r *Requirement) checkForBrew(brew string, formula string) bool {
	c := exec.Command(brew, "info", formula)
	c.Stdin = nil
	c.Stdout = io.Discard
	c.Stderr = io.Discard
	out, err := c.Output()
	if err != nil {
		return false
	}
	if strings.Contains(string(out), "No available formula") {
		return false
	}
	return true
}

func (r *Requirement) upgradeBrew(brew string, formula string) error {
	c := exec.Command(brew, "update")
	c.Stdin = nil
	c.Stdout = io.Discard
	c.Stderr = io.Discard
	if err := c.Run(); err != nil {
		return fmt.Errorf("failed to update brew: %s", err)
	}
	c = exec.Command(brew, "upgrade", formula)
	c.Stdin = nil
	c.Stdout = io.Discard
	c.Stderr = io.Discard
	return c.Run()
}

func (r *Requirement) installBrew(brew string, formula string) error {
	c := exec.Command(brew, "install", formula)
	c.Stdin = os.Stdin
	c.Stdout = io.Discard
	c.Stderr = io.Discard
	return c.Run()
}

func (r *Requirement) TryInstall(ctx TemplateContext) error {
	if r.Selfupdate != nil {
		if cmd, ok := r.hasCommand(r.Selfupdate.Command); ok {
			ctx.Logger.Debug("self-upgrading %s", r.Command)
			c := exec.Command(cmd, r.Selfupdate.Args...)
			c.Stdin = os.Stdin
			c.Stdout = io.Discard
			c.Stderr = io.Discard
			return c.Run()
		}
	}
	if runtime.GOOS == "darwin" && r.Brew != "" {
		ctx.Logger.Debug("checking for brew")
		if brew, ok := r.hasCommand("brew"); ok {
			ctx.Logger.Debug("brew found: %s", brew)
			if r.checkForBrew(brew, r.Brew) {
				ctx.Logger.Debug("trying to upgrade: %s", r.Brew)
				return r.upgradeBrew(brew, r.Brew)
			}
			ctx.Logger.Debug("trying to install formula: %s", r.Brew)
			return r.installBrew(brew, r.Brew)
		}
	}
	if r.URL != "" {
		return &ErrRequirementsNotMet{fmt.Sprintf("Required dependency %s is missing and cannot automatically be installed. You can find installation instructions from %s", r.Command, r.URL)}
	}
	return &ErrRequirementsNotMet{fmt.Sprintf("Required dependency %s is missing and cannot automatically be installed. Install it manually before continuing", r.Command)}
}

func (r *Requirement) Matches(ctx TemplateContext) bool {
	if r.Version == "" {
		panic(fmt.Sprintf("invalid requirement for command %s: version is required", r.Command))
	}

	if command, ok := r.hasCommand(r.Command); ok {
		ctx.Logger.Debug("checking version of %s", command)
		c := exec.Command(command, r.Args...)
		out, err := c.Output()
		if err != nil {
			return false
		}
		constraint, err := semver.NewConstraint(r.Version)
		if err != nil {
			panic(fmt.Sprintf("invalid requirement for command %s: version %s is invalid: %s", r.Command, r.Version, err))
		}
		expected := strings.TrimSpace(string(out))
		// allows for loose version output from commands
		tok := strings.Split(expected, " ")
		for _, v := range tok {
			ctx.Logger.Trace("checking version token: %s", v)
			version, err := semver.NewVersion(v)
			if err != nil {
				ctx.Logger.Trace("version token [%s] wasn't a valid version identifier", v)
				continue
			}
			ctx.Logger.Debug("checking version of %s. requires: %s, found: %s", command, r.Version, version)
			return constraint.Check(version)
		}
	}
	return false
}

type Template struct {
	Name         string        `yaml:"name"`
	Description  string        `yaml:"description"`
	Identifier   string        `yaml:"identifier"`
	Language     string        `yaml:"language"`
	Requirements []Requirement `yaml:"requirements"`
}

func (t *Template) NewProject(ctx TemplateContext) (*TemplateRules, error) {
	rules, err := LoadTemplateRuleForIdentifier(t.Identifier)
	if err != nil {
		return nil, err
	}
	if !util.Exists(ctx.ProjectDir) {
		if err := os.MkdirAll(ctx.ProjectDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %s", ctx.ProjectDir, err)
		}
		ctx.Logger.Debug("created directory %s", ctx.ProjectDir)
	}
	ctx.Template = t
	return rules, rules.NewProject(ctx)
}

// Matches returns true if the template matches the requirements
func (t *Template) Matches(ctx TemplateContext) bool {
	for _, requirement := range t.Requirements {
		if !requirement.Matches(ctx) {
			return false
		}
	}
	return true
}

// Install installs the requirements for the template
func (t *Template) Install(ctx TemplateContext) error {
	for _, requirement := range t.Requirements {
		if !requirement.Matches(ctx) {
			if err := requirement.TryInstall(ctx); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *Template) AddGitHubAction(ctx TemplateContext) error {
	name := "common/github/" + t.Identifier + ".yaml"
	reader, err := getEmbeddedFile(name)
	if err != nil {
		return fmt.Errorf("failed to load embedded file for %s: %s", name, err)
	}
	if reader == nil {
		return fmt.Errorf("template %s not found", name)
	}
	buf, err := io.ReadAll(reader)
	outdir := filepath.Join(ctx.ProjectDir, ".github", "workflows")
	if err := os.MkdirAll(outdir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %s", outdir, err)
	}
	outfile := filepath.Join(outdir, "agentuity.yaml")
	if err := os.WriteFile(outfile, buf, 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %s", outfile, err)
	}
	return nil
}

type Templates []Template

func loadTemplates(reader io.Reader) (Templates, error) {
	var templates Templates
	if err := yaml.NewDecoder(reader).Decode(&templates); err != nil {
		return nil, err
	}

	return templates, nil
}

// LoadTemplates returns all the templates available
func LoadTemplates() (Templates, error) {
	reader, err := getEmbeddedFile("templates.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to open embedded templates file: %s", err)
	}
	return loadTemplates(reader)
}

func LoadLanguageTemplates(runtime string) (LanguageTemplates, error) {
	reader, err := getEmbeddedFile(runtime + "/templates.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded file for %s: %s", runtime+"/templates.yaml", err)
	}
	if reader == nil {
		return nil, fmt.Errorf("template %s not found", runtime+"/templates.yaml")
	}
	templates := make(LanguageTemplates, 0)
	if err := yaml.NewDecoder(reader).Decode(&templates); err != nil {
		return nil, fmt.Errorf("failed to decode templates for %s: %s", runtime+"/templates.yaml", err)
	}
	return templates, nil
}

func IsValidRuntimeTemplateName(runtime string, name string) bool {
	templates, err := LoadLanguageTemplates(runtime)
	if err != nil {
		return false
	}
	for _, t := range templates {
		if t.Name == name {
			return true
		}
	}
	return false
}
