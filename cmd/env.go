package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/tui"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	cstr "github.com/agentuity/go-common/string"
	"github.com/agentuity/go-common/sys"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Environment related commands",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var (
	hasTTY          = term.IsTerminal(int(os.Stdout.Fd()))
	looksLikeSecret = regexp.MustCompile(`(?i)KEY|SECRET|TOKEN|PASSWORD|sk_`)
	isAgentuityEnv  = regexp.MustCompile(`(?i)AGENTUITY_`)
)

func loadEnvFile(le []env.EnvLine, forceSecret bool) (map[string]string, map[string]string) {
	envs := make(map[string]string)
	secrets := make(map[string]string)
	for _, ev := range le {
		if isAgentuityEnv.MatchString(ev.Key) {
			continue
		}
		if looksLikeSecret.MatchString(ev.Key) || forceSecret {
			secrets[ev.Key] = ev.Val
		} else {
			envs[ev.Key] = ev.Val
		}
	}
	return envs, secrets
}

func promptForEnv(logger logger.Logger, key string, isSecret bool, localenv map[string]string, osenv map[string]string) string {
	prompt := "Enter the value for " + key
	var help string
	var defaultValue string
	var value string
	if isSecret {
		prompt = "Enter the secret value for " + key
		if val, ok := localenv[key]; ok {
			help = "Press enter to set as " + maxString(cstr.Mask(val), 30) + " from your .env file"
			defaultValue = val
		} else if val, ok := osenv[key]; ok {
			help = "Press enter to set as " + maxString(cstr.Mask(val), 30) + " from your environment"
			defaultValue = val
		} else {
			help = "Your input will be masked"
		}
		value = tui.Password(logger, prompt, help)
	} else {
		value = tui.Input(logger, prompt, help)
	}

	if value == "" && defaultValue != "" {
		value = defaultValue
	}
	return value
}

func loadOSEnv() map[string]string {
	osenv := make(map[string]string)
	for _, line := range os.Environ() {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 && !isAgentuityEnv.MatchString(parts[0]) {
			osenv[parts[0]] = parts[1]
		}
	}
	return osenv
}

var envSetCmd = &cobra.Command{
	Use:     "set [key] [value]",
	Aliases: []string{"add", "put"},
	Short:   "Set environment variables",
	Run: func(cmd *cobra.Command, args []string) {
		context := project.EnsureProject(cmd)
		logger := context.Logger
		dir := context.Dir
		apiUrl := context.APIURL
		apiKey := context.Token
		theproject := context.Project

		forceSecret, _ := cmd.Flags().GetBool("secret")
		noConfirm, _ := cmd.Flags().GetBool("force")

		var askMore bool
		var hasEnvFile bool
		var hasSetFromFile bool
		var setFromEnv bool

		envs := make(map[string]string)
		secrets := make(map[string]string)
		localenv := make(map[string]string)
		osenv := make(map[string]string)

		setFromFile, err := cmd.Flags().GetString("file")
		if setFromFile != "" {
			if sys.Exists(setFromFile) {
				le, _ := env.ParseEnvFile(setFromFile)
				envs, secrets = loadEnvFile(le, forceSecret)
				if len(envs) > 0 || len(secrets) > 0 {
					hasSetFromFile = true
					setFromEnv = true
				}
			} else {
				errsystem.New(errsystem.ErrInvalidCommandFlag, err).ShowErrorAndExit()
			}
		}
		if !hasSetFromFile {
			// load any environment variables from the environment
			osenv = loadOSEnv()

			// load any environment variables from the .env file
			envfile := filepath.Join(dir, ".env")
			if sys.Exists(envfile) {
				le, _ := env.ParseEnvFile(envfile)
				var added bool
				for _, ev := range le {
					if !isAgentuityEnv.MatchString(ev.Key) {
						localenv[ev.Key] = ev.Val
						added = true
					}
				}
				hasEnvFile = added
			}
		}

		if len(args) == 0 && hasEnvFile && len(localenv) > 0 && !hasSetFromFile && !noConfirm {
			var options []tui.Option
			for k := range localenv {
				if !isAgentuityEnv.MatchString(k) {
					options = append(options, tui.Option{ID: k, Text: k, Selected: true})
				}
			}
			results := tui.MultiSelect(logger, "Set environment variables from .env", "", options)
			for _, result := range results {
				val := localenv[result]
				if looksLikeSecret.MatchString(result) || forceSecret {
					secrets[result] = val
				} else {
					envs[result] = val
				}
			}
			setFromEnv = len(results) > 0
		}

	restart:
		var key string
		var value string
		var isSecret bool
		switch len(args) {
		case 1:
			key = args[0]
		case 2:
			key = args[0]
			value = args[1]
		default:
			if noConfirm && len(envs) == 0 && len(secrets) == 0 {
				logger.Fatal("you must provide a key and value or --env-file when specifying --force")
			}
			askMore = !setFromEnv
		}
		if key == "" && !setFromEnv {
			var help string
			if len(envs) > 0 || len(secrets) > 0 {
				help = "Press enter to save..."
			}
			key = tui.Input(logger, "Enter the environment variable name", help)
			if key == "" {
				askMore = false
			}
		}
		isSecret = looksLikeSecret.MatchString(key) || forceSecret
		if key != "" && value == "" && !noConfirm {
			if len(envs) == 0 && len(secrets) == 0 {
				fi, _ := os.Stdin.Stat()
				if fi != nil && fi.Size() > 0 {
					buf, _ := io.ReadAll(os.Stdin)
					if len(buf) > 0 {
						value = strings.TrimRight(string(buf), "\n")
					}
				}
			}
			if value == "" {
				value = promptForEnv(logger, key, isSecret, localenv, osenv)
			}
		}
		if key != "" && value != "" {
			if isSecret {
				secrets[key] = value
				tui.ShowSuccess("%s=%s", key, maxString(cstr.Mask(value), 40))
			} else {
				envs[key] = value
				tui.ShowSuccess("%s=%s", key, maxString(value, 40))
			}
		}
		if askMore {
			args = nil
			goto restart
		}

		action := func() {
			// make sure secrets are not in envs as duplicates since secrets take precedence
			for k := range secrets {
				delete(envs, k)
			}
			// make sure envs are not in secrets as duplicates since envs take precedence
			for k := range envs {
				delete(secrets, k)
			}
			_, err := theproject.SetProjectEnv(logger, apiUrl, apiKey, envs, secrets)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithUserMessage("Failed to save project settings")).ShowErrorAndExit()
			}
		}

		spinner.New().Title("Saving ...").Action(action).Run()

		switch {
		case len(envs) > 0 && len(secrets) > 0:
			tui.ShowSuccess("Environment variables and secrets saved")
		case len(envs) == 1:
			tui.ShowSuccess("Environment variable saved")
		case len(secrets) == 1:
			tui.ShowSuccess("Secret saved")
		case len(envs) > 0:
			tui.ShowSuccess("Environment variables saved")
		case len(secrets) > 0:
			tui.ShowSuccess("Secrets saved")
		}
	},
}

var envGetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get an environment or secret value",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		context := project.EnsureProject(cmd)
		logger := context.Logger
		theproject := context.Project
		apiUrl := context.APIURL
		apiKey := context.Token

		projectData, err := theproject.GetProject(logger, apiUrl, apiKey)
		if err != nil {
			errsystem.New(errsystem.ErrApiRequest, err).ShowErrorAndExit()
		}
		var found bool
		for key, value := range projectData.Env {
			if key == args[0] {
				if !hasTTY {
					fmt.Println(value)
				} else {
					fmt.Println(tui.Title(value))
				}
				found = true
				break
			}
		}
		if !found {
			for key, value := range projectData.Secrets {
				if key == args[0] {
					if !hasTTY {
						fmt.Println(value)
					} else {
						fmt.Println(tui.Muted(value))
					}
					found = true
					break
				}
			}
		}
		if !found {
			tui.ShowWarning("No environment variables or secrets set for this project named %s", args[0])
		}
	},
}

var envListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "show", "print"},
	Args:    cobra.NoArgs,
	Short:   "List all environment variables and secrets",
	Run: func(cmd *cobra.Command, args []string) {
		context := project.EnsureProject(cmd)
		logger := context.Logger
		theproject := context.Project
		apiUrl := context.APIURL
		apiKey := context.Token

		projectData, err := theproject.GetProject(logger, apiUrl, apiKey)
		if err != nil {
			errsystem.New(errsystem.ErrApiRequest, err).ShowErrorAndExit()
		}
		for key, value := range projectData.Env {
			if !hasTTY {
				fmt.Printf("%s=%s\n", key, value)
			} else {
				fmt.Printf("%s=%s\n", tui.Title(key), tui.Body(value))
			}
		}
		for key, value := range projectData.Secrets {
			if !hasTTY {
				fmt.Printf("%s=%s\n", key, value)
			} else {
				fmt.Printf("%s=%s\n", tui.Title(key), tui.Muted(maxString(value, 40)))
			}
		}
		if len(projectData.Env) == 0 && len(projectData.Secrets) == 0 {
			tui.ShowWarning("No environment variables or secrets set for this project")
			fmt.Println()
			fmt.Printf("You can set environment variables with %s", tui.Command("env", "set", "<key>", "<value>"))
			fmt.Println()
		}
	},
}

var envDeleteCmd = &cobra.Command{
	Use:     "delete [key...]",
	Aliases: []string{"rm", "del"},
	Args:    cobra.MinimumNArgs(1),
	Short:   "Delete one or more environment variables and secrets",
	Run: func(cmd *cobra.Command, args []string) {
		context := project.EnsureProject(cmd)
		logger := context.Logger
		theproject := context.Project
		apiUrl := context.APIURL
		apiKey := context.Token

		projectData, err := theproject.GetProject(logger, apiUrl, apiKey)
		if err != nil {
			errsystem.New(errsystem.ErrApiRequest, err).ShowErrorAndExit()
		}
		force, _ := cmd.Flags().GetBool("force")
		var options []huh.Option[string]
		secretKeys := make(map[string]bool)
		envKeys := make(map[string]bool)
		for k := range projectData.Env {
			options = append(options, huh.NewOption(k, k))
			envKeys[k] = true
		}
		for k := range projectData.Secrets {
			options = append(options, huh.NewOption(k, k))
			secretKeys[k] = true
		}
		var secretsToDelete []string
		var envsToDelete []string
		var results []string
		if len(args) > 0 {
			for _, key := range args {
				if secretKeys[key] {
					secretsToDelete = append(secretsToDelete, key)
				} else {
					envsToDelete = append(envsToDelete, key)
				}
			}
		} else {
			var title string
			switch {
			case len(envKeys) > 0 && len(secretKeys) > 0 && !force:
				title = "Pick the environment variables and secrets to delete"
			case len(envKeys) > 1 && !force:
				title = "Pick the environment variables to delete"
			case len(secretKeys) > 1 && !force:
				title = "Pick the secrets to delete"
			default:
				// if just one of each or force is true, delete all of them
				for k := range secretKeys {
					secretsToDelete = append(secretsToDelete, k)
				}
				for k := range envKeys {
					envsToDelete = append(envsToDelete, k)
				}
			}
			// only prompt if there are multiple options
			if title != "" && !force {
				if huh.NewMultiSelect[string]().
					Options(options...).
					Title(title).
					Value(&results).Run() != nil {
					return
				}
				for _, result := range results {
					if secretKeys[result] {
						secretsToDelete = append(secretsToDelete, result)
					} else {
						envsToDelete = append(envsToDelete, result)
					}
				}
			}
		}
		if len(secretsToDelete) > 0 || len(envsToDelete) > 0 {
			if !force {
				var title string
				switch {
				case len(secretsToDelete) > 0 && len(envsToDelete) > 0:
					title = "Are you sure you want to delete these environment variables and secrets?"
				case len(secretsToDelete) > 1:
					title = "Are you sure you want to delete these secrets?"
				case len(secretsToDelete) == 1:
					title = "Are you sure you want to delete this secret?"
				case len(envsToDelete) > 1:
					title = "Are you sure you want to delete these environment variables?"
				case len(envsToDelete) == 1:
					title = "Are you sure you want to delete this environment variable?"
				}
				if !tui.Ask(logger, title, false) {
					tui.ShowWarning("cancelled")
					return
				}
			}
			err := theproject.DeleteProjectEnv(logger, apiUrl, apiKey, envsToDelete, secretsToDelete)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err).ShowErrorAndExit()
			}
			switch {
			case len(envsToDelete) > 0 && len(secretsToDelete) > 0:
				tui.ShowSuccess("Environment variables and secrets deleted")
			case len(envsToDelete) == 1:
				tui.ShowSuccess("Environment variable deleted")
			case len(envsToDelete) > 0:
				tui.ShowSuccess("Environment variables deleted")
			case len(secretsToDelete) == 1:
				tui.ShowSuccess("Secret deleted")
			case len(secretsToDelete) > 0:
				tui.ShowSuccess("Secrets deleted")
			}
		} else if force {
			tui.ShowWarning("No environment variables or secrets to delete")
		}
	},
}

func init() {
	rootCmd.AddCommand(envCmd)

	envSetCmd.Flags().StringP("file", "f", "", "The path to a file containing environment variables to set")
	envSetCmd.Flags().BoolP("secret", "s", false, "Force the value(s) to be treated as a secret")
	envSetCmd.Flags().Bool("force", !hasTTY, "Don't prompt for confirmation")

	envCmd.AddCommand(envSetCmd)
	envCmd.AddCommand(envListCmd)
	envCmd.AddCommand(envGetCmd)
	envCmd.AddCommand(envDeleteCmd)

	envDeleteCmd.Flags().Bool("force", !hasTTY, "Don't prompt for confirmation")
	for _, cmd := range []*cobra.Command{envSetCmd, envListCmd, envGetCmd, envDeleteCmd} {
		cmd.Flags().StringP("dir", "d", ".", "The directory to the project to deploy")
	}
}
