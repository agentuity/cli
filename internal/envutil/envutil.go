package envutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/agentuity/cli/internal/deployer"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	util "github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	cstr "github.com/agentuity/go-common/string"
	"github.com/agentuity/go-common/tui"
	"github.com/charmbracelet/lipgloss"
)

var EnvTemplateFileNames = []string{".env.example", ".env.template"}

var border = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1).BorderForeground(lipgloss.AdaptiveColor{Light: "#999999", Dark: "#999999"})
var redDiff = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#990000", Dark: "#EE0000"})

var LooksLikeSecret = looksLikeSecret
var IsAgentuityEnv = isAgentuityEnv

var looksLikeSecret = regexp.MustCompile(`(?i)(^|_|-)(APIKEY|API_KEY|PRIVATE_KEY|KEY|SECRET|TOKEN|CREDENTIAL|CREDENTIALS|PASSWORD|sk_[a-zA-Z0-9_-]*|BEARER|AUTH|JWT|WEBHOOK)($|_|-)`)
var isAgentuityEnv = regexp.MustCompile(`(?i)AGENTUITY_`)

// ProcessEnvFiles handles .env and template env processing
func ProcessEnvFiles(ctx context.Context, logger logger.Logger, dir string, theproject *project.Project, projectData *project.ProjectData, apiUrl, token string, force bool) (*deployer.EnvFile, *project.ProjectData) {
	envfilename := filepath.Join(dir, ".env")
	var envFile *deployer.EnvFile
	if (tui.HasTTY || force) && util.Exists(envfilename) {
		// attempt to see if we have any template files
		templateEnvs := ReadPossibleEnvTemplateFiles(dir)

		le, err := env.ParseEnvFileWithComments(envfilename)
		if err != nil {
			errsystem.New(errsystem.ErrParseEnvironmentFile, err,
				errsystem.WithContextMessage("Error parsing .env file")).ShowErrorAndExit()
		}
		envFile = &deployer.EnvFile{Filepath: envfilename, Env: le}

		le, err = HandleMissingTemplateEnvs(logger, dir, envfilename, le, templateEnvs, force)
		if err != nil {
			errsystem.New(errsystem.ErrParseEnvironmentFile, err,
				errsystem.WithContextMessage("Error parsing .env file")).ShowErrorAndExit()
		}

		projectData = HandleMissingProjectEnvs(ctx, logger, le, projectData, theproject, apiUrl, token, force)
		envFile.Env = le
		return envFile, projectData
	}
	return envFile, projectData
}

// HandleMissingTemplateEnvs handles missing envs from template files
func HandleMissingTemplateEnvs(logger logger.Logger, dir, envfilename string, le []env.EnvLineComment, templateEnvs map[string][]env.EnvLineComment, force bool) ([]env.EnvLineComment, error) {
	if len(templateEnvs) == 0 {
		return le, nil
	}
	kvmap := make(map[string]env.EnvLineComment)
	for _, ev := range le {
		if isAgentuityEnv.MatchString(ev.Key) {
			continue
		}
		kvmap[ev.Key] = ev
	}
	var osenv map[string]string
	var addtoenvfile []env.EnvLineComment
	// look to see if we have any template environment variables that are not in the .env file
	for filename, evs := range templateEnvs {
		for _, ev := range evs {
			if _, ok := kvmap[ev.Key]; !ok {
				isSecret := looksLikeSecret.MatchString(ev.Key)
				if !isSecret && DescriptionLookingLikeASecret(ev.Comment) {
					isSecret = true
				}
				if !force {
					var content string
					var para []string
					para = append(para, tui.Warning("Missing Environment Variable\n"))
					para = append(para, fmt.Sprintf("The variable %s was found in %s but not in your %s file:\n", tui.Bold(ev.Key), tui.Bold(filename), tui.Bold(".env")))
					if ev.Comment != "" {
						para = append(para, tui.Muted(fmt.Sprintf("# %s", ev.Comment)))
					}
					if isSecret {
						para = append(para, redDiff.Render(fmt.Sprintf("+ %s=%s\n", ev.Key, cstr.Mask(ev.Val))))
					} else {
						para = append(para, redDiff.Render(fmt.Sprintf("+ %s=%s\n", ev.Key, ev.Val)))
					}
					content = lipgloss.JoinVertical(lipgloss.Left, para...)
					fmt.Println(border.Render(content))
					if !tui.Ask(logger, "Would you like to add it to your .env file?", true) {
						fmt.Println()
						tui.ShowWarning("cancelled")
						continue
					}
					if osenv == nil {
						osenv = LoadOSEnv()
					}
					if ev.Val == "" {
						ev.Val = PromptForEnv(logger, ev.Key, isSecret, nil, osenv, ev.Val, ev.Comment)
					}
					ev.Raw = ev.Val
				}
				addtoenvfile = append(addtoenvfile, env.EnvLineComment{
					EnvLine: env.EnvLine{
						Key: ev.Key,
						Val: ev.Val,
						Raw: ev.Raw,
					},
					Comment: ev.Comment,
				})
			}
		}
	}
	if len(addtoenvfile) > 0 {
		var err error
		le, err = AppendToEnvFile(envfilename, addtoenvfile)
		if err != nil {
			return le, err
		}
		if tui.HasTTY {
			tui.ShowSuccess("added %s to your .env file", util.Pluralize(len(addtoenvfile), "environment variable", "environment variables"))
			fmt.Println()
		}
	}
	return le, nil
}

// HandleMissingProjectEnvs handles missing envs in project
func HandleMissingProjectEnvs(ctx context.Context, logger logger.Logger, le []env.EnvLineComment, projectData *project.ProjectData, theproject *project.Project, apiUrl, token string, force bool) *project.ProjectData {

	if projectData == nil {
		projectData = &project.ProjectData{}
	}
	keyvalue := map[string]string{}
	for _, ev := range le {
		if isAgentuityEnv.MatchString(ev.Key) {
			continue
		}
		if projectData.Env != nil && projectData.Env[ev.Key] == ev.Val {
			continue
		}
		if projectData.Secrets != nil && projectData.Secrets[ev.Key] == cstr.Mask(ev.Val) {
			continue
		}
		keyvalue[ev.Key] = ev.Val
	}
	if len(keyvalue) > 0 {
		if !force {
			var title string
			var suffix string
			switch {
			case len(keyvalue) < 3 && len(keyvalue) > 1:
				suffix = "it"
				var colorized []string
				for key := range keyvalue {
					colorized = append(colorized, tui.Bold(key))
				}
				title = fmt.Sprintf("The environment variables %s from %s are not been set in the project.", strings.Join(colorized, ", "), tui.Bold(".env"))
			case len(keyvalue) == 1:
				var key string
				for _key := range keyvalue {
					key = _key
					break
				}
				suffix = "it"
				title = fmt.Sprintf("The environment variable %s from %s has not been set in the project.", tui.Bold(key), tui.Bold(".env"))
			default:
				suffix = "them"
				title = fmt.Sprintf("There are %d environment variables from %s that are not set in the project.", len(keyvalue), tui.Bold(".env"))
			}
			fmt.Println(title)
			force = tui.Ask(logger, "Would you like to set "+suffix+" now?", true)
		}
		if force {
			for key, val := range keyvalue {
				if looksLikeSecret.MatchString(key) {
					if projectData.Secrets == nil {
						projectData.Secrets = make(map[string]string)
					}
					projectData.Secrets[key] = cstr.Mask(val)
				} else {
					if projectData.Env == nil {
						projectData.Env = make(map[string]string)
					}
					projectData.Env[key] = val
				}
			}
			_, err := theproject.SetProjectEnv(ctx, logger, apiUrl, token, projectData.Env, projectData.Secrets)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithUserMessage("Failed to save project settings")).ShowErrorAndExit()
			}
		}
	}
	return projectData
}

// ReadPossibleEnvTemplateFiles reads .env.example and .env.template files
func ReadPossibleEnvTemplateFiles(baseDir string) map[string][]env.EnvLineComment {
	var results map[string][]env.EnvLineComment
	keys := make(map[string]bool)
	for _, file := range EnvTemplateFileNames {
		filename := filepath.Join(baseDir, file)
		if !util.Exists(filename) {
			continue
		}
		if efc, err := env.ParseEnvFileWithComments(filename); err == nil {
			if results == nil {
				results = make(map[string][]env.EnvLineComment)
			}
			for _, ev := range efc {
				if _, ok := keys[ev.Key]; !ok {
					if isAgentuityEnv.MatchString(ev.Key) {
						continue
					}
					keys[ev.Key] = true
					results[file] = append(results[file], ev)
				}
			}
		} else {
			errsystem.New(errsystem.ErrParseEnvironmentFile, err,
				errsystem.WithContextMessage("Error parsing .env file")).ShowErrorAndExit()
		}
	}
	return results
}

// AppendToEnvFile appends envs to the .env file
func AppendToEnvFile(envfile string, envs []env.EnvLineComment) ([]env.EnvLineComment, error) {
	le, err := env.ParseEnvFileWithComments(envfile)
	if err != nil {
		return nil, err
	}
	var buf strings.Builder
	for _, ev := range le {
		if ev.Comment != "" {
			buf.WriteString(fmt.Sprintf("# %s\n", ev.Comment))
		}
		raw := ev.Raw
		if raw == "" {
			raw = ev.Val
		}
		buf.WriteString(fmt.Sprintf("%s=%s\n", ev.Key, raw))
	}
	for _, ev := range envs {
		if ev.Comment != "" {
			buf.WriteString(fmt.Sprintf("# %s\n", ev.Comment))
		}
		raw := ev.Raw
		if raw == "" {
			raw = ev.Val
		}
		buf.WriteString(fmt.Sprintf("%s=%s\n", ev.Key, raw))
		le = append(le, ev)
	}
	if err := os.WriteFile(envfile, []byte(buf.String()), 0600); err != nil {
		return nil, err
	}
	return le, nil
}

// Utility functions needed for env processing

// DescriptionLookingLikeASecret checks if a description looks like a secret
func DescriptionLookingLikeASecret(description string) bool {
	if description == "" {
		return false
	}
	return looksLikeSecret.MatchString(description)
}

// LoadOSEnv loads the OS environment variables
func LoadOSEnv() map[string]string {
	osenv := make(map[string]string)
	for _, line := range os.Environ() {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 && !isAgentuityEnv.MatchString(parts[0]) {
			osenv[parts[0]] = parts[1]
		}
	}
	return osenv
}

func PromptForEnv(logger logger.Logger, key string, isSecret bool, localenv map[string]string, osenv map[string]string, defaultValue string, placeholder string) string {
	prompt := "Enter the value for " + key
	var help string
	var value string
	if isSecret {
		prompt = "Enter the secret value for " + key
		if val, ok := localenv[key]; ok {
			help = "Press enter to set as " + cstr.Mask(util.MaxString(val, 30)) + " from your .env file"
			if defaultValue == "" {
				defaultValue = val
			}
		} else if val, ok := osenv[key]; ok {
			help = "Press enter to set as " + cstr.Mask(util.MaxString(val, 30)) + " from your environment"
			if defaultValue == "" {
				defaultValue = val
			}
		} else {
			help = "Your input will be masked"
		}
		value = tui.Password(logger, prompt, help)
	} else {
		if placeholder == "" {
			value = tui.InputWithPlaceholder(logger, prompt, help, defaultValue)
		} else {
			value = tui.InputWithPlaceholder(logger, prompt, placeholder, defaultValue)
		}
	}

	if value == "" && defaultValue != "" {
		value = defaultValue
	}
	return value
}
