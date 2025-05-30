package templates

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/agentuity/cli/internal/util"
)

func localizePath(path string) string {
	p, err := filepath.Localize(path)
	if err != nil {
		panic("Could not localize path: " + path)
	}
	return p
}

func embeddedPath(path string) string {
	return strings.ReplaceAll(path, "\\", "/")
}

type Step interface {
	Run(ctx TemplateContext) error
}

type CommandStep struct {
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
}

var _ Step = (*CommandStep)(nil)

func (s *CommandStep) Run(ctx TemplateContext) error {
	ctx.Logger.Debug("Running command: %s with args: %s", s.Command, strings.Join(s.Args, " "))

	timeoutCtx, cancel := context.WithTimeout(ctx.Context, 1*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(timeoutCtx, s.Command, s.Args...)
	cmd.Dir = ctx.ProjectDir
	cmd.Stdin = nil

	out, err := cmd.CombinedOutput()
	if err != nil {
		if s.Command == "uv" && len(s.Args) >= 3 && s.Args[0] == "add" {
			exitErr, ok := err.(*exec.ExitError)
			if ok && exitErr.ExitCode() == 130 {
				packages := s.Args[2:]
				ctx.Logger.Debug("Command interrupted with SIGINT (130), trying alternative installation methods for: %v", packages)

				altCmd := exec.CommandContext(ctx.Context, "uv", "pip", "install")
				altCmd.Args = append(altCmd.Args, packages...)
				altCmd.Dir = ctx.ProjectDir
				altOut, altErr := altCmd.CombinedOutput()

				if altErr == nil {
					ctx.Logger.Debug("Successfully installed packages with uv pip: %s", strings.TrimSpace(string(altOut)))
					return nil
				}

				ctx.Logger.Debug("Failed to install with uv pip, trying with pip: %v", altErr)
				pipCmd := exec.CommandContext(ctx.Context, "pip", "install")
				pipCmd.Args = append(pipCmd.Args, packages...)
				pipCmd.Dir = ctx.ProjectDir
				pipOut, pipErr := pipCmd.CombinedOutput()

				if pipErr == nil {
					ctx.Logger.Debug("Successfully installed packages with pip: %s", strings.TrimSpace(string(pipOut)))
					return nil
				}

				return fmt.Errorf("failed to install packages %v with multiple methods: original error: %w, pip error: %v (%s)",
					packages, err, pipErr, string(pipOut))
			}
		}

		return fmt.Errorf("failed to run command: %s with args: %s: %w (%s)", s.Command, strings.Join(s.Args, " "), err, string(out))
	}

	buf := strings.TrimSpace(string(out))
	if buf != "" {
		ctx.Logger.Debug("Command output: %s", buf)
	}
	return nil
}

type DeleteFileActionStep struct {
	Files []string `yaml:"files"`
}

var _ Step = (*DeleteFileActionStep)(nil)

func (s *DeleteFileActionStep) Run(ctx TemplateContext) error {
	for _, file := range s.Files {
		filename := filepath.Join(ctx.ProjectDir, localizePath(file))
		if !util.Exists(filename) {
			ctx.Logger.Debug("file %s does not exist", filename)
			continue
		}
		ctx.Logger.Debug("deleting file: %s", filename)
		if err := os.Remove(filename); err != nil {
			return fmt.Errorf("failed to delete file: %w", err)
		}
	}
	return nil
}

type nameValue struct {
	Name  string
	Value any
}

type ModifyPackageJsonStep struct {
	Script      []nameValue
	Main        string
	Type        string
	Name        string
	Version     string
	Description string
	Keywords    []string
}

var _ Step = (*ModifyPackageJsonStep)(nil)

func (s *ModifyPackageJsonStep) Run(ctx TemplateContext) error {
	packageJsonPath := filepath.Join(ctx.ProjectDir, "package.json")
	if !util.Exists(packageJsonPath) {
		return fmt.Errorf("package.json does not exist")
	}
	pkgjson, err := os.ReadFile(packageJsonPath)
	if err != nil {
		return fmt.Errorf("failed to read package.json: %w", err)
	}
	packageJsonMap, err := util.NewOrderedMapFromJSON(util.PackageJsonKeysOrder, pkgjson)
	if err != nil {
		return fmt.Errorf("failed to unmarshal package.json: %w", err)
	}
	for _, script := range s.Script {
		var scripts map[string]any
		if val, ok := packageJsonMap.Data["scripts"].(map[string]any); ok {
			scripts = val
		} else {
			scripts = make(map[string]any)
			packageJsonMap.Data["scripts"] = scripts
		}
		scripts[script.Name] = script.Value
	}
	if s.Main != "" {
		packageJsonMap.Data["main"] = ctx.Interpolate(s.Main)
	}
	if s.Type != "" {
		packageJsonMap.Data["type"] = ctx.Interpolate(s.Type)
	}
	if s.Name != "" {
		packageJsonMap.Data["name"] = ctx.Interpolate(s.Name)
	}
	if s.Version != "" {
		packageJsonMap.Data["version"] = ctx.Interpolate(s.Version)
	}
	if s.Description != "" {
		packageJsonMap.Data["description"] = ctx.Interpolate(s.Description)
	}
	if len(s.Keywords) > 0 {
		packageJsonMap.Data["keywords"] = s.Keywords
	}
	packageJson, err := packageJsonMap.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal package.json: %w", err)
	}
	ctx.Logger.Debug("Writing package.json: %s", packageJsonPath)
	if err := os.WriteFile(packageJsonPath, packageJson, 0644); err != nil {
		return fmt.Errorf("failed to write package.json: %w", err)
	}
	return nil
}

type ModifyTsConfigStep struct {
	Types           []string
	CompilerOptions []nameValue
}

var _ Step = (*ModifyTsConfigStep)(nil)

func (s *ModifyTsConfigStep) Run(ctx TemplateContext) error {
	tsconfigPath := filepath.Join(ctx.ProjectDir, "tsconfig.json")
	if !util.Exists(tsconfigPath) {
		return fmt.Errorf("tsconfig.json does not exist")
	}
	tsconfig, err := os.ReadFile(tsconfigPath)
	if err != nil {
		return fmt.Errorf("failed to read tsconfig.json: %w", err)
	}
	tsconfigMap, err := util.NewOrderedMapFromJSON(util.PackageJsonKeysOrder, tsconfig)
	if err != nil {
		return fmt.Errorf("failed to unmarshal tsconfig.json: %w", err)
	}
	var compilerOptions map[string]any
	if val, ok := tsconfigMap.Data["compilerOptions"].(map[string]any); ok {
		compilerOptions = val
	} else {
		compilerOptions = make(map[string]any)
	}
	for _, nv := range s.CompilerOptions {
		compilerOptions[nv.Name] = nv.Value
	}
	if len(s.Types) > 0 {
		compilerOptions["types"] = s.Types
	}
	tsconfigMap.Data["compilerOptions"] = compilerOptions
	tsbuf, err := tsconfigMap.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal tsconfig.json: %w", err)
	}
	ctx.Logger.Debug("Writing tsconfig: %s", tsconfigPath)
	if err := os.WriteFile(tsconfigPath, tsbuf, 0644); err != nil {
		return fmt.Errorf("failed to write tsconfig.json: %w", err)
	}
	return nil
}

type AppendFileStep struct {
	Filename string
	Content  string
}

var _ Step = (*AppendFileStep)(nil)

func (s *AppendFileStep) Run(ctx TemplateContext) error {
	filename := filepath.Join(ctx.ProjectDir, localizePath(s.Filename))
	if !util.Exists(filename) {
		return fmt.Errorf("%s does not exist", filename)
	}
	buf, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read gitignore: %w", err)
	}

	newbuf := append(buf, []byte(s.Content)...)

	ctx.Logger.Debug("Writing %s", filename)
	if err := os.WriteFile(filename, newbuf, 0644); err != nil {
		return fmt.Errorf("failed to write gitignore: %w", err)
	}
	return nil
}

type CreateFileAction struct {
	Filename string
	Content  string
	Template string
	From     string
}

var _ Step = (*CreateFileAction)(nil)

func (s *CreateFileAction) Run(ctx TemplateContext) error {
	filename := filepath.Join(ctx.ProjectDir, localizePath(s.Filename))
	dir := filepath.Dir(filename)
	if !util.Exists(dir) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}
	ctx.Logger.Debug("Creating file: %s", filename)
	var output []byte
	if s.Template != "" {
		fr, err := getEmbeddedFile(filepath.Join(ctx.TemplateDir, s.Template))
		if err != nil {
			return fmt.Errorf("failed to get embedded file: %w", err)
		}
		defer fr.Close()
		tbuf, err := io.ReadAll(fr)
		if err != nil {
			return fmt.Errorf("failed to read embedded file: %w", err)
		}
		tmpl := template.New(s.Template)
		tmpl, err = funcTemplates(tmpl, ctx.Template.Language == "python").Parse(string(tbuf))
		if err != nil {
			return fmt.Errorf("failed to parse template: %w", err)
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, ctx); err != nil {
			return fmt.Errorf("failed to execute template: %w", err)
		}
		output = buf.Bytes()
	} else if s.Content != "" {
		output = []byte(s.Content) // just use the content as is
	} else if s.From != "" {
		from, err := getEmbeddedFile(filepath.Join(ctx.TemplateDir, s.From))
		if err != nil {
			return fmt.Errorf("failed to get embedded file: %s", err)
		}
		defer from.Close()
		buf, err := io.ReadAll(from)
		if err != nil {
			return fmt.Errorf("failed to read embedded file: %s", err)
		}
		output = buf
	}
	if len(output) == 0 {
		return fmt.Errorf("no content to write to file: %s", filename)
	}
	if err := os.WriteFile(filename, output, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}

type CopyFileAction struct {
	From string
	To   string
}

var _ Step = (*CopyFileAction)(nil)

func (s *CopyFileAction) Run(ctx TemplateContext) error {
	from, err := getEmbeddedFile(filepath.Join(ctx.TemplateDir, s.From))
	if err != nil {
		return fmt.Errorf("failed to get embedded file: %w", err)
	}
	defer from.Close()
	buf, err := io.ReadAll(from)
	if err != nil {
		return fmt.Errorf("failed to read embedded file: %w", err)
	}
	to := filepath.Join(ctx.ProjectDir, localizePath(s.To))
	dir := filepath.Dir(to)
	if !util.Exists(dir) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}
	ctx.Logger.Debug("Copying file: %s to %s", s.From, s.To)
	if err := os.WriteFile(to, buf, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}

type CopyDirAction struct {
	From   string
	To     string
	Filter string
}

var _ Step = (*CopyDirAction)(nil)

func (s *CopyDirAction) Run(ctx TemplateContext) error {
	filename := embeddedPath(s.From)
	from, err := getEmbeddedDir(filepath.Join(ctx.TemplateDir, filename))
	if err != nil {
		return fmt.Errorf("failed to get embedded file: %w", err)
	}
	dir := filepath.Join(ctx.ProjectDir, localizePath(s.To))
	if !util.Exists(dir) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
		ctx.Logger.Debug("Created directory: %s", dir)
	}
	for _, file := range from {
		if s.Filter != "" {
			matched, err := filepath.Match(s.Filter, file.Name())
			if err != nil {
				return fmt.Errorf("failed to match filter: %w", err)
			}
			if !matched {
				continue
			}
		}
		name := filepath.Join(ctx.TemplateDir, s.From, file.Name())
		r, err := getEmbeddedFile(name)
		if err != nil {
			return fmt.Errorf("failed to get embedded file: %w", err)
		}
		defer r.Close()
		buf, err := io.ReadAll(r)
		if err != nil {
			return fmt.Errorf("failed to read embedded file: %w", err)
		}
		to := filepath.Join(dir, localizePath(file.Name()))
		if err := os.WriteFile(to, buf, 0644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
		ctx.Logger.Debug("Copied file: %s to %s", name, to)
	}
	return nil
}

type CloneRepoAction struct {
	Repo   string
	Branch string
	Todir  string
}

var _ Step = (*CloneRepoAction)(nil)

func (s *CloneRepoAction) Run(ctx TemplateContext) error {
	var url string
	if s.Branch == "" {
		url = fmt.Sprintf("https://agentuity.sh/repo/%s", s.Repo)
	} else {
		url = fmt.Sprintf("https://agentuity.sh/repo/%s/%s", s.Repo, s.Branch)
	}
	req, err := http.NewRequestWithContext(ctx.Context, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", util.UserAgent())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to clone repo %s: %s", s.Repo, resp.Status)
	}

	if !util.Exists(s.Todir) {
		if err := os.MkdirAll(s.Todir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}
	tmpFile, err := os.CreateTemp("", "temp.zip")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	_, err = io.Copy(tmpFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to copy zipball: %w", err)
	}
	tmpFile.Close()

	zipReader, err := zip.OpenReader(tmpFile.Name())
	if err != nil {
		return err
	}
	defer zipReader.Close()

	var trimDir string

	for _, file := range zipReader.File {
		if strings.Contains(file.Name, "..") {
			ctx.Logger.Warn("skipping invalid zip file: %s", file.Name)
			continue
		}
		isDir := file.FileInfo().IsDir()
		if trimDir == "" && isDir {
			trimDir = file.Name
			continue
		}
		if strings.HasPrefix(file.Name, ".git") {
			continue
		}
		destFile := filepath.Join(s.Todir, strings.TrimPrefix(file.Name, trimDir))
		if isDir {
			if err := os.MkdirAll(destFile, 0755); err != nil {
				return fmt.Errorf("failed to create directory: %w", err)
			}
		} else {
			rc, err := file.Open()
			if err != nil {
				return fmt.Errorf("failed to open file: %w", err)
			}
			defer rc.Close()
			out, err := os.Create(destFile)
			if err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
			defer out.Close()
			_, err = io.Copy(out, rc)
			if err != nil {
				return fmt.Errorf("failed to copy file: %w", err)
			}
			rc.Close()
			ctx.Logger.Debug("unzipped file: %s", destFile)
		}
	}

	return nil
}

func anyToStringArray(ctx TemplateContext, val []any) []string {
	var result []string
	for _, v := range val {
		if v, ok := v.(string); ok {
			result = append(result, ctx.Interpolate(v).(string))
		}
	}
	return result
}

func anyToNameValueArray(ctx TemplateContext, val []any) []nameValue {
	var result []nameValue
	for _, v := range val {
		if kv, ok := v.(map[string]any); ok {
			result = append(result, nameValue{
				Name:  ctx.Interpolate(kv["name"]).(string),
				Value: ctx.Interpolate(kv["value"]),
			})
		}
	}
	return result
}

func resolveStep(ctx TemplateContext, step any) (Step, bool) {
	if kv, ok := step.(map[string]any); ok {
		if command, ok := kv["command"].(string); ok {
			var args []string
			if val, ok := kv["args"].([]any); ok {
				args = anyToStringArray(ctx, val)
			}
			return &CommandStep{Command: command, Args: args}, true
		}
		if action, ok := kv["action"].(string); ok {
			switch action {
			case "delete_file":
				var files []string
				if val, ok := kv["files"].([]any); ok {
					files = anyToStringArray(ctx, val)
				}
				return &DeleteFileActionStep{Files: files}, true
			case "modify_package_json":
				var script []nameValue
				if val, ok := kv["script"].([]any); ok {
					script = anyToNameValueArray(ctx, val)
				}
				var main string
				if val, ok := kv["main"].(string); ok {
					main = ctx.Interpolate(val).(string)
				}
				var typ string
				if val, ok := kv["type"].(string); ok {
					typ = ctx.Interpolate(val).(string)
				}
				var name string
				if val, ok := kv["name"].(string); ok {
					name = util.SafeProjectFilename(ctx.Interpolate(val).(string), ctx.Template.Language == "python")
				}
				var version string
				if val, ok := kv["version"].(string); ok {
					version = ctx.Interpolate(val).(string)
				}
				var description string
				if val, ok := kv["description"].(string); ok {
					description = ctx.Interpolate(val).(string)
				}
				var keywords []string
				if val, ok := kv["keywords"].([]any); ok {
					keywords = anyToStringArray(ctx, val)
				}
				return &ModifyPackageJsonStep{Script: script, Main: main, Type: typ, Name: name, Version: version, Description: description, Keywords: keywords}, true
			case "modify_ts_config":
				var types []string
				if val, ok := kv["types"].([]any); ok {
					types = anyToStringArray(ctx, val)
				}
				var compilerOptions []nameValue
				if val, ok := kv["compilerOptions"].([]interface{}); ok {
					for _, v := range val {
						if kv, ok := v.(map[string]any); ok {
							if name, ok := kv["name"].(string); ok {
								compilerOptions = append(compilerOptions, nameValue{Name: name, Value: kv["value"]})
							}
						}
					}
				}
				return &ModifyTsConfigStep{Types: types, CompilerOptions: compilerOptions}, true
			case "append_file":
				var filename string
				var content string
				if val, ok := kv["filename"].(string); ok {
					filename = ctx.Interpolate(val).(string)
				}
				if val, ok := kv["content"].(string); ok {
					content = ctx.Interpolate(val).(string)
				}
				return &AppendFileStep{Filename: filename, Content: content}, true
			case "create_file":
				var filename string
				var content string
				var template string
				var from string
				if val, ok := kv["filename"].(string); ok {
					filename = ctx.Interpolate(val).(string)
				}
				if val, ok := kv["content"].(string); ok {
					content = ctx.Interpolate(val).(string)
				}
				if val, ok := kv["template"].(string); ok {
					template = ctx.Interpolate(val).(string)
				}
				if val, ok := kv["from"].(string); ok {
					from = ctx.Interpolate(val).(string)
				}
				return &CreateFileAction{Filename: filename, Content: content, Template: template, From: from}, true
			case "copy_file":
				var from string
				var to string
				if val, ok := kv["from"].(string); ok {
					from = ctx.Interpolate(val).(string)
				}
				if val, ok := kv["to"].(string); ok {
					to = ctx.Interpolate(val).(string)
				}
				return &CopyFileAction{From: from, To: to}, true
			case "copy_dir":
				var from string
				var to string
				var filter string
				if val, ok := kv["from"].(string); ok {
					from = ctx.Interpolate(val).(string)
				}
				if val, ok := kv["to"].(string); ok {
					to = ctx.Interpolate(val).(string)
				}
				if val, ok := kv["filter"].(string); ok {
					filter = ctx.Interpolate(val).(string)
				}
				return &CopyDirAction{From: from, To: to, Filter: filter}, true
			case "clone_repo":
				var repo string
				if val, ok := kv["repo"].(string); ok {
					repo = ctx.Interpolate(val).(string)
				}
				if repo == "" {
					panic("repo is required")
				}
				todir := ctx.ProjectDir
				if val, ok := kv["to"].(string); ok {
					todir = filepath.Join(ctx.ProjectDir, ctx.Interpolate(val).(string))
				}
				var branch string
				if val, ok := kv["branch"].(string); ok {
					branch = ctx.Interpolate(val).(string)
				}
				return &CloneRepoAction{Repo: repo, Branch: branch, Todir: todir}, true
			default:
				panic(fmt.Sprintf("unknown step action: %s", action))
			}
		}
	}
	return nil, false
}
