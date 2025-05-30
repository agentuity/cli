package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/agentuity/cli/internal/envutil"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	cstr "github.com/agentuity/go-common/string"
	"github.com/agentuity/go-common/sys"
	"github.com/agentuity/go-common/tui"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/spf13/cobra"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Environment related commands",
	Long: `Environment related commands for managing environment variables and secrets.

Use the subcommands to set, get, list, and delete environment variables and secrets.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var (
	hasTTY = tui.HasTTY
)

func loadEnvFile(le []env.EnvLineComment, forceSecret bool) (map[string]string, map[string]string) {
	envs := make(map[string]string)
	secrets := make(map[string]string)
	for _, ev := range le {
		if envutil.IsAgentuityEnv.MatchString(ev.Key) {
			continue
		}
		if envutil.LooksLikeSecret.MatchString(ev.Key) || forceSecret || envutil.DescriptionLookingLikeASecret(ev.Comment) {
			secrets[ev.Key] = ev.Val
		} else {
			envs[ev.Key] = ev.Val
		}
	}
	return envs, secrets
}

func loadOSEnv() map[string]string {
	osenv := make(map[string]string)
	for _, line := range os.Environ() {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 && !envutil.IsAgentuityEnv.MatchString(parts[0]) {
			osenv[parts[0]] = parts[1]
		}
	}
	return osenv
}

var envSetCmd = &cobra.Command{
	Use:     "set [key] [value]",
	Aliases: []string{"add", "put"},
	Short:   "Set environment variables",
	Long: `Set environment variables or secrets for your project.

Arguments:
  [key]    The name of the environment variable
  [value]  The value of the environment variable

Flags:
  --file      Path to a file containing environment variables to set
  --secret    Force the value(s) to be treated as a secret
  --force     Don't prompt for confirmation

Examples:
  agentuity env set API_KEY "my-api-key"
  agentuity env set --secret TOKEN "secret-token"
  agentuity env set --file .env`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		logger := env.NewLogger(cmd)
		context := project.EnsureProject(ctx, cmd)
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
				le, _ := env.ParseEnvFileWithComments(setFromFile)
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
					if !envutil.IsAgentuityEnv.MatchString(ev.Key) {
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
				if !envutil.IsAgentuityEnv.MatchString(k) {
					options = append(options, tui.Option{ID: k, Text: k, Selected: true})
				}
			}
			results := tui.MultiSelect(logger, "Set environment variables from .env", "", options)
			for _, result := range results {
				val := localenv[result]
				if envutil.LooksLikeSecret.MatchString(result) || forceSecret {
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
		isSecret = envutil.LooksLikeSecret.MatchString(key) || forceSecret
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
				value = envutil.PromptForEnv(logger, key, isSecret, localenv, osenv, "", "")
			}
		}
		if key != "" && value != "" {
			if isSecret {
				secrets[key] = value
				tui.ShowSuccess("%s=%s", key, util.MaxString(cstr.Mask(value), 40))
			} else {
				envs[key] = value
				tui.ShowSuccess("%s=%s", key, util.MaxString(value, 40))
			}
		}
		if askMore {
			args = nil
			goto restart
		}

		action := func() {
			combined := make(map[string]string)
			// make sure secrets are not in envs as duplicates since secrets take precedence
			for k := range secrets {
				delete(envs, k)
				combined[k] = secrets[k]
			}
			// make sure envs are not in secrets as duplicates since envs take precedence
			for k := range envs {
				delete(secrets, k)
				combined[k] = envs[k]
			}
			_, err := theproject.SetProjectEnv(ctx, logger, apiUrl, apiKey, envs, secrets)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithUserMessage("Failed to save project settings")).ShowErrorAndExit()
			}
			if err := project.SaveEnvValue(ctx, logger, dir, combined); err != nil {
				errsystem.New(errsystem.ErrApiRequest, err, errsystem.WithUserMessage("Failed to save .env file")).ShowErrorAndExit()
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
	Long: `Get the value of an environment variable or secret.

Arguments:
  [key]    The name of the environment variable or secret to get

Examples:
  agentuity env get API_KEY`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		context := project.EnsureProject(ctx, cmd)
		logger := context.Logger
		theproject := context.Project
		apiUrl := context.APIURL
		apiKey := context.Token

		mask, _ := cmd.Flags().GetBool("mask")

		projectData, err := theproject.GetProject(ctx, logger, apiUrl, apiKey, mask, false)
		if err != nil {
			errsystem.New(errsystem.ErrApiRequest, err).ShowErrorAndExit()
		}
		format, _ := cmd.Flags().GetString("format")
		var outkv map[string]string
		if format == "json" {
			outkv = make(map[string]string)
		}
		var found bool
		for key, value := range projectData.Env {
			if key == args[0] {
				if format == "json" {
					outkv[key] = value
					continue
				}
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
					if format == "json" {
						outkv[key] = value
						continue
					}
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
		if format == "json" {
			json.NewEncoder(os.Stdout).Encode(outkv)
			return
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
	Long: `List all environment variables and secrets for your project.

This command displays all environment variables and secrets set for your project.

Examples:
  agentuity env list
  agentuity env ls`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		context := project.EnsureProject(ctx, cmd)
		logger := context.Logger
		theproject := context.Project
		apiUrl := context.APIURL
		apiKey := context.Token

		mask, _ := cmd.Flags().GetBool("mask")
		includeProjectKeys, _ := cmd.Flags().GetBool("include-project-keys")

		projectData, err := theproject.GetProject(ctx, logger, apiUrl, apiKey, mask, includeProjectKeys)
		if err != nil {
			errsystem.New(errsystem.ErrApiRequest, err).ShowErrorAndExit()
		}

		format, _ := cmd.Flags().GetString("format")
		if format == "json" {
			kv := map[string]any{
				"environment": projectData.Env,
				"secrets":     projectData.Secrets,
			}
			json.NewEncoder(os.Stdout).Encode(kv)
			return
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
				fmt.Printf("%s=%s\n", tui.Title(key), tui.Muted(util.MaxString(value, 40)))
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
	Long: `Delete one or more environment variables and secrets from your project.

Arguments:
  [key...]    One or more environment variable or secret names to delete

Flags:
  --force    Don't prompt for confirmation

Examples:
  agentuity env delete API_KEY
  agentuity env delete API_KEY SECRET_TOKEN
  agentuity env delete --force API_KEY`,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		context := project.EnsureProject(ctx, cmd)
		logger := context.Logger
		theproject := context.Project
		apiUrl := context.APIURL
		apiKey := context.Token

		projectData, err := theproject.GetProject(ctx, logger, apiUrl, apiKey, true, false)
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
			err := theproject.DeleteProjectEnv(ctx, logger, apiUrl, apiKey, envsToDelete, secretsToDelete)
			if err != nil {
				errsystem.New(errsystem.ErrApiRequest, err).ShowErrorAndExit()
			}
			project.RemoveEnvValues(ctx, logger, context.Dir, append(envsToDelete, secretsToDelete...)...)
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

	for _, cmd := range []*cobra.Command{envListCmd, envGetCmd} {
		cmd.Flags().String("format", "text", "The format to use for the output. Can be either 'text' or 'json'")
		cmd.Flags().Bool("mask", true, "Mask secrets in the output")
	}

	envListCmd.Flags().Bool("include-project-keys", false, "Include project keys in the output")
}
