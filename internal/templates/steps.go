package templates

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/agentuity/cli/internal/util"
)

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
	cmd := exec.CommandContext(ctx.Context, s.Command, s.Args...)
	cmd.Dir = ctx.ProjectDir
	cmd.Stdin = nil
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to run command: %s with args: %s: %s (%s)", s.Command, strings.Join(s.Args, " "), err, string(out))
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
		filename := filepath.Join(ctx.ProjectDir, file)
		if !util.Exists(filename) {
			ctx.Logger.Debug("file %s does not exist", filename)
			continue
		}
		ctx.Logger.Debug("deleting file: %s", filename)
		if err := os.Remove(filename); err != nil {
			return fmt.Errorf("failed to delete file: %s", err)
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
		return fmt.Errorf("failed to read package.json: %s", err)
	}
	packageJsonMap, err := util.NewOrderedMapFromJSON(util.PackageJsonKeysOrder, pkgjson)
	if err != nil {
		return fmt.Errorf("failed to unmarshal package.json: %s", err)
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
		return fmt.Errorf("failed to marshal package.json: %s", err)
	}
	ctx.Logger.Debug("Writing package.json: %s", packageJsonPath)
	if err := os.WriteFile(packageJsonPath, packageJson, 0644); err != nil {
		return fmt.Errorf("failed to write package.json: %s", err)
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
		return fmt.Errorf("failed to read tsconfig.json: %s", err)
	}
	tsconfigMap, err := util.NewOrderedMapFromJSON(util.PackageJsonKeysOrder, tsconfig)
	if err != nil {
		return fmt.Errorf("failed to unmarshal tsconfig.json: %s", err)
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
		return fmt.Errorf("failed to marshal tsconfig.json: %s", err)
	}
	ctx.Logger.Debug("Writing tsconfig: %s", tsconfigPath)
	if err := os.WriteFile(tsconfigPath, tsbuf, 0644); err != nil {
		return fmt.Errorf("failed to write tsconfig.json: %s", err)
	}
	return nil
}

type AppendFileStep struct {
	Filename string
	Content  string
}

var _ Step = (*AppendFileStep)(nil)

func (s *AppendFileStep) Run(ctx TemplateContext) error {
	filename := filepath.Join(ctx.ProjectDir, s.Filename)
	if !util.Exists(filename) {
		return fmt.Errorf("%s does not exist", filename)
	}
	buf, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read gitignore: %s", err)
	}

	newbuf := append(buf, []byte(s.Content)...)

	ctx.Logger.Debug("Writing %s", filename)
	if err := os.WriteFile(filename, newbuf, 0644); err != nil {
		return fmt.Errorf("failed to write gitignore: %s", err)
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
	filename := filepath.Join(ctx.ProjectDir, s.Filename)
	dir := filepath.Dir(filename)
	if !util.Exists(dir) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %s", err)
		}
	}
	ctx.Logger.Debug("Creating file: %s", filename)
	var output []byte
	if s.Template != "" {
		tmpl, err := template.New(s.Filename).Parse(s.Template)
		if err != nil {
			return fmt.Errorf("failed to parse template: %s", err)
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, ctx); err != nil {
			return fmt.Errorf("failed to execute template: %s", err)
		}
		output = buf.Bytes()
	} else if s.Content != "" {
		output = []byte(s.Content) // just use the content as is
	} else if s.From != "" {
		from, err := getEmbeddedFile(s.From)
		if err != nil {
			return fmt.Errorf("failed to get embedded file: %s", err)
		}
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
		return fmt.Errorf("failed to write file: %s", err)
	}
	return nil
}

type CopyFileAction struct {
	From string
	To   string
}

var _ Step = (*CopyFileAction)(nil)

func (s *CopyFileAction) Run(ctx TemplateContext) error {
	from, err := getEmbeddedFile(s.From)
	if err != nil {
		return fmt.Errorf("failed to get embedded file: %s", err)
	}
	buf, err := io.ReadAll(from)
	if err != nil {
		return fmt.Errorf("failed to read embedded file: %s", err)
	}
	to := filepath.Join(ctx.ProjectDir, s.To)
	dir := filepath.Dir(to)
	if !util.Exists(dir) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %s", err)
		}
	}
	ctx.Logger.Debug("Copying file: %s to %s", s.From, s.To)
	if err := os.WriteFile(to, buf, 0644); err != nil {
		return fmt.Errorf("failed to write file: %s", err)
	}
	return nil
}

type CopyDirAction struct {
	From string
	To   string
}

var _ Step = (*CopyDirAction)(nil)

func (s *CopyDirAction) Run(ctx TemplateContext) error {
	from, err := getEmbeddedDir(s.From)
	if err != nil {
		return fmt.Errorf("failed to get embedded file: %s", err)
	}
	dir := filepath.Join(ctx.ProjectDir, s.To)
	if !util.Exists(dir) {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %s", err)
		}
		ctx.Logger.Debug("Created directory: %s", dir)
	}
	for _, file := range from {
		name := filepath.Join(s.From, file.Name())
		r, err := getEmbeddedFile(name)
		if err != nil {
			return fmt.Errorf("failed to get embedded file: %s", err)
		}
		buf, err := io.ReadAll(r)
		if err != nil {
			return fmt.Errorf("failed to read embedded file: %s", err)
		}
		to := filepath.Join(dir, file.Name())
		if err := os.WriteFile(to, buf, 0644); err != nil {
			return fmt.Errorf("failed to write file: %s", err)
		}
		ctx.Logger.Debug("Copied file: %s to %s", name, to)
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
					name = util.SafeFilename(ctx.Interpolate(val).(string))
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
				if val, ok := kv["from"].(string); ok {
					from = ctx.Interpolate(val).(string)
				}
				if val, ok := kv["to"].(string); ok {
					to = ctx.Interpolate(val).(string)
				}
				return &CopyDirAction{From: from, To: to}, true
			default:
				panic(fmt.Sprintf("unknown step action: %s", action))
			}
		}
	}
	return nil, false
}
