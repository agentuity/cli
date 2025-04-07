package templates

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Masterminds/semver"
	"github.com/agentuity/cli/internal/util"
	"github.com/spf13/viper"
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
	util.ProcessSetup(c)
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
	util.ProcessSetup(c)
	if err := c.Run(); err != nil {
		return fmt.Errorf("failed to update brew: %w", err)
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
	util.ProcessSetup(c)
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
			util.ProcessSetup(c)
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
		util.ProcessSetup(c)
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
	rules, err := LoadTemplateRuleForIdentifier(ctx.TemplateDir, t.Identifier)
	if err != nil {
		return nil, err
	}
	if !util.Exists(ctx.ProjectDir) {
		if err := os.MkdirAll(ctx.ProjectDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory %s: %w", ctx.ProjectDir, err)
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
	name := filepath.Join(ctx.TemplateDir, "common", "github", t.Identifier+".yaml")
	reader, err := getEmbeddedFile(name)
	if err != nil {
		return fmt.Errorf("failed to load embedded file for %s: %w", name, err)
	}
	defer reader.Close()
	buf, err := io.ReadAll(reader)
	outdir := filepath.Join(ctx.ProjectDir, ".github", "workflows")
	if err := os.MkdirAll(outdir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", outdir, err)
	}
	outfile := filepath.Join(outdir, "agentuity.yaml")
	if err := os.WriteFile(outfile, buf, 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", outfile, err)
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

type Release struct {
	ZipballURL string `json:"zipball_url"`
	TagName    string `json:"tag_name"`
}

const githubTemplatesLatest = "https://api.github.com/repos/agentuity/templates/releases/latest"

func getLatestRelease(ctx context.Context) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", githubTemplatesLatest, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", util.UserAgent())
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("failed to get latest release: %s", resp.Status)
	}

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", "", err
	}

	return release.TagName, release.ZipballURL, nil
}

const markerFileName = ".marker"

func unzip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	var rootDir string

	for _, f := range r.File {
		if strings.Contains(f.Name, "..") {
			return fmt.Errorf("invalid file path: %s", f.Name)
		}

		// find the root directory
		if rootDir == "" && f.FileInfo().IsDir() {
			rootDir = f.Name
			continue
		}

		// skip the .github directory
		if strings.HasPrefix(f.Name, filepath.Join(rootDir, ".github")) {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()

		fpath := filepath.Join(dest, strings.Replace(f.Name, rootDir, "", 1))

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
		} else {
			var fdir string
			if lastIndex := strings.LastIndex(fpath, string(os.PathSeparator)); lastIndex > -1 {
				fdir = fpath[:lastIndex]
			}

			err = os.MkdirAll(fdir, os.ModePerm)
			if err != nil {
				return err
			}
			f, err := os.OpenFile(
				fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
			if err != nil {
				return err
			}
			defer f.Close()

			_, err = io.Copy(f, rc)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// LoadTemplatesFromGithub loads the templates from github and returns the templates
// If the etag is provided, it will be used to check if the templates have changed
// If the templates have not changed, it will return the templates from the local directory
// If the templates have changed, it will download the new templates and return them
func LoadTemplatesFromGithub(ctx context.Context, dir string) (Templates, error) {
	etag := viper.GetString("templates.etag")
	release := viper.GetString("templates.release")
	tagName, zipballURL, err := getLatestRelease(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", zipballURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", util.UserAgent())
	if etag != "" && release == tagName && util.Exists(filepath.Join(dir, markerFileName)) {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotModified:
		return loadTemplateFromDir(dir)
	case http.StatusOK:
		break
	default:
		return nil, fmt.Errorf("failed to load templates from github: %s", resp.Status)
	}

	etag = resp.Header.Get("ETag")
	if etag != "" {
		viper.Set("templates.etag", etag)
		viper.Set("templates.release", tagName)
		viper.WriteConfig()
	}

	tmpfile, err := os.CreateTemp("", "agentuity-templates.zip")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpfile.Name())
	if _, err := io.Copy(tmpfile, resp.Body); err != nil {
		return nil, err
	}

	os.RemoveAll(dir) // clear the directory first

	if err := unzip(tmpfile.Name(), dir); err != nil {
		return nil, err
	}

	// write a marker file to the directory to indicate that the templates have been loaded
	if err := os.WriteFile(filepath.Join(dir, markerFileName), []byte(time.Now().Format(time.RFC3339)), 0600); err != nil {
		return nil, err
	}

	return loadTemplateFromDir(dir)
}

func loadTemplateFromDir(dir string) (Templates, error) {
	reader, err := getEmbeddedFile(filepath.Join(dir, "runtimes.yaml"))
	if err != nil {
		return nil, fmt.Errorf("failed to open embedded runtimes file: %w", err)
	}
	return loadTemplates(reader)
}

// LoadTemplates returns all the templates available
func LoadTemplates(ctx context.Context, dir string, custom bool) (Templates, error) {
	if custom && util.Exists(dir) && util.Exists(filepath.Join(dir, "runtimes.yaml")) {
		return loadTemplateFromDir(dir)
	}
	return LoadTemplatesFromGithub(ctx, dir)
}

func LoadLanguageTemplates(ctx context.Context, dir string, runtime string) (LanguageTemplates, error) {
	filename := filepath.Join(dir, runtime, "templates.yaml")
	reader, err := getEmbeddedFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded file for %s: %w", filename, err)
	}
	if reader == nil {
		return nil, fmt.Errorf("template %s not found", filename)
	}
	templates := make(LanguageTemplates, 0)
	if err := yaml.NewDecoder(reader).Decode(&templates); err != nil {
		return nil, fmt.Errorf("failed to decode templates for %s: %w", filename, err)
	}
	return templates, nil
}

func IsValidRuntimeTemplateName(ctx context.Context, dir string, runtime string, name string) bool {
	templates, err := LoadLanguageTemplates(ctx, dir, runtime)
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
