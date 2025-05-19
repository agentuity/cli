package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/agentuity/cli/internal/apikey"
	"github.com/agentuity/cli/internal/errsystem"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	cstr "github.com/agentuity/go-common/string"
	"github.com/agentuity/go-common/tui"
	"github.com/spf13/cobra"
)

var apikeyCmd = &cobra.Command{
	Use:     "apikey",
	Aliases: []string{"apikeys"},
	Args:    cobra.NoArgs,
	Short:   "Manage API keys",
	Long: `Manage API keys.

Use the subcommands to set, get, list, and delete apikeys.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

type APIKeyList []apikey.APIKey

func (list APIKeyList) SortedIterator() func(yield func(key string, value apikey.APIKey) bool) {
	return func(yield func(key string, value apikey.APIKey) bool) {
		var names []string
		kv := make(map[string]apikey.APIKey)
		for _, item := range list {
			names = append(names, item.Name)
			kv[item.Name] = item
		}
		sort.Strings(names)
		for _, key := range names {
			if !yield(key, kv[key]) {
				return
			}
		}
	}
}

func renderAPIKey(apikey apikey.APIKey, mask bool) {
	val := apikey.Value
	if mask {
		val = cstr.Mask(val)
	}
	fmt.Println(tui.Title(apikey.Name) + " " + tui.Muted("("+apikey.ID+")"))
	if apikey.ProjectId == "" {
		fmt.Println(tui.Text("Org scoped to " + apikey.Org.Name + " (" + apikey.Org.ID + ")"))
	} else {
		fmt.Println(tui.Text("Project scoped to " + apikey.Project.Name + " (" + apikey.Project.ID + ")"))
	}
	fmt.Println(tui.Muted(val))
	fmt.Println(tui.Text("Expires at " + apikey.ExpiresAt))
}

var apikeyListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Args:    cobra.NoArgs,
	Short:   "List API keys",
	Long: `List all API keys.
	
This command displays all apikeys set for your org or project.

Examples:
  agentuity apikey list
  agentuity apikey ls --org-id <orgId>
  agentuity apikey ls --project-id <projectId>
  agentuity apikey ls --mask`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		apiUrl, _, _ := util.GetURLs(logger)
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		format, _ := cmd.Flags().GetString("format")
		apiKey, _ := util.EnsureLoggedIn(ctx, logger, cmd)
		orgId, _ := cmd.Flags().GetString("org-id")
		projectId, _ := cmd.Flags().GetString("project-id")
		apikeys, err := apikey.List(ctx, logger, apiUrl, apiKey, orgId, projectId)
		if err != nil {
			errsystem.New(errsystem.ErrFetchApiKeys, err,
				errsystem.WithContextMessage("Failed to fetch API keys")).ShowErrorAndExit()
		}
		if len(apikeys) == 0 {
			if format == "text" {
				tui.ShowWarning("No API keys found")
			} else if format == "json" {
				json.NewEncoder(os.Stdout).Encode(apikeys)
			}
			return
		}
		mask, _ := cmd.Flags().GetBool("mask")
		byOrgs := make(map[string]APIKeyList)
		orgNames := make(map[string]string)
		var orgs []string
		for _, apikey := range apikeys {
			byOrgs[apikey.OrgId] = append(byOrgs[apikey.OrgId], apikey)
			if _, ok := orgNames[apikey.Org.ID]; !ok {
				orgNames[apikey.Org.ID] = apikey.Org.Name
				orgs = append(orgs, apikey.OrgId)
			}
		}
		sort.Slice(orgs, func(i, j int) bool {
			return orgNames[orgs[i]] < orgNames[orgs[j]]
		})
		switch format {
		case "text":
			for _, orgId := range orgs {
				apikeys := byOrgs[orgId]
				if len(apikeys) == 0 {
					continue
				}
				fmt.Println()
				fmt.Println(tui.Bold(orgNames[orgId]) + " " + tui.Muted("("+orgId+")"))
				fmt.Println()

				var i int

				for _, apikey := range apikeys.SortedIterator() {
					renderAPIKey(apikey, mask)
					if i < len(apikeys)-1 {
						fmt.Println()
					}
					i++
				}
			}
		case "json":
			json.NewEncoder(os.Stdout).Encode(apikeys)
		default:
			logger.Fatal("invalid format: %s", format)
		}
	},
}

var apikeyCreateCmd = &cobra.Command{
	Use:     "create [name]",
	Aliases: []string{"new"},
	Args:    cobra.MaximumNArgs(1),
	Short:   "Create an API key",
	Long: `Create an API key.
	
This command creates an API key for your org or project.

Examples:
  agentuity apikey create <name> --expires-at <expiresAt>
  agentuity apikey create <name> --expires-at <expiresAt> --org-id <orgId>
  agentuity apikey create <name> --expires-at <expiresAt> --project-id <projectId>`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		apiUrl, _, _ := util.GetURLs(logger)
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		apiKey, _ := util.EnsureLoggedIn(ctx, logger, cmd)
		orgId, _ := cmd.Flags().GetString("org-id")
		projectId, _ := cmd.Flags().GetString("project-id")
		if orgId == "" && projectId == "" {
			orgId = promptForOrganization(ctx, logger, cmd, apiUrl, apiKey)
		}
		var name string
		if len(args) > 0 {
			name = args[0]
		}
		if name == "" {
			name = tui.InputWithValidation(logger, "API Key Name", "The name to describe the API key", 100, func(name string) error {
				if name == "" {
					return errors.New("name is required")
				}
				return nil
			})
		}
		expiresAt, _ := cmd.Flags().GetString("expires-at")
		if expiresAt == "" {
			expiresAt = time.Now().AddDate(0, 0, 365*10).Format(time.RFC3339)
		} else {
			d, err := time.Parse(time.RFC3339, expiresAt)
			if err == nil {
				expiresAt = d.Format(time.RFC3339)
			} else {
				dur, err := time.ParseDuration(expiresAt)
				if err == nil {
					expiresAt = time.Now().Add(dur).Format(time.RFC3339)
				} else {
					logger.Fatal("invalid expires at: %s (must be a date in RFC3339 format or a relative duration)", err)
				}
			}
		}
		apikey, err := apikey.Create(ctx, logger, apiUrl, apiKey, orgId, projectId, name, expiresAt)
		if err != nil {
			errsystem.New(errsystem.ErrCreateApiKey, err,
				errsystem.WithContextMessage("Failed to create API key")).ShowErrorAndExit()
		}
		format, _ := cmd.Flags().GetString("format")
		switch format {
		case "text":
			tui.ShowSuccess("API key created: %s", apikey.Value)
		case "json":
			json.NewEncoder(os.Stdout).Encode(apikey)
		default:
			logger.Fatal("invalid format: %s", format)
		}
	},
}

var apikeyDeleteCmd = &cobra.Command{
	Use:     "delete [id]",
	Aliases: []string{"del", "rm"},
	Args:    cobra.MaximumNArgs(1),
	Short:   "Delete an API key",
	Long:    `Delete an API key.`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		apiUrl, _, _ := util.GetURLs(logger)
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		apiKey, _ := util.EnsureLoggedIn(ctx, logger, cmd)
		var id string
		if len(args) > 0 {
			id = args[0]
		}
		if id == "" {
			apikeys, err := apikey.List(ctx, logger, apiUrl, apiKey, "", "")
			if err != nil {
				errsystem.New(errsystem.ErrFetchApiKeys, err,
					errsystem.WithContextMessage("Failed to fetch API keys")).ShowErrorAndExit()
			}
			if len(apikeys) == 0 {
				tui.ShowWarning("No API keys found")
				return
			}
			items := make([]tui.Option, len(apikeys))
			for i, apikey := range apikeys {
				items[i] = tui.Option{
					Text: apikey.Name,
					ID:   apikey.ID,
				}
			}
			id = tui.Select(logger, "Select an API key", "Select an API Key to delete", items)
		}
		err := apikey.Delete(ctx, logger, apiUrl, apiKey, id)
		if err != nil {
			errsystem.New(errsystem.ErrDeleteApiKey, err,
				errsystem.WithContextMessage("Failed to delete API key")).ShowErrorAndExit()
		}
		tui.ShowSuccess("API key deleted")
	},
}

var apikeyGetCmd = &cobra.Command{
	Use:   "get [id]",
	Args:  cobra.ExactArgs(1),
	Short: "Get an API key",
	Long: `Get an API key.
	
Examples:
  agentuity apikey get <id>
  agentuity apikey get <id> --mask`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := env.NewLogger(cmd)
		apiUrl, _, _ := util.GetURLs(logger)
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		apiKey, _ := util.EnsureLoggedIn(ctx, logger, cmd)
		id := args[0]
		apikey, err := apikey.Get(ctx, logger, apiUrl, apiKey, id)
		if err != nil {
			errsystem.New(errsystem.ErrFetchApiKeys, err,
				errsystem.WithContextMessage("Failed to fetch API keys")).ShowErrorAndExit()
		}
		if apikey == nil {
			tui.ShowWarning("No API key found for id: %s", id)
			return
		}
		mask, _ := cmd.Flags().GetBool("mask")
		format, _ := cmd.Flags().GetString("format")
		switch format {
		case "text":
			renderAPIKey(*apikey, mask)
		case "json":
			json.NewEncoder(os.Stdout).Encode(apikey)
		default:
			logger.Fatal("invalid format: %s", format)
		}
	},
}

func init() {
	rootCmd.AddCommand(apikeyCmd)
	apikeyCmd.AddCommand(apikeyListCmd)
	apikeyCmd.AddCommand(apikeyCreateCmd)
	apikeyCmd.AddCommand(apikeyDeleteCmd)
	apikeyCmd.AddCommand(apikeyGetCmd)
	apikeyListCmd.Flags().StringP("org-id", "o", "", "The organization ID to filter by")
	apikeyListCmd.Flags().StringP("project-id", "p", "", "The project ID to filter by")
	apikeyListCmd.Flags().BoolP("mask", "m", false, "Mask the API key value")
	apikeyCreateCmd.Flags().StringP("org-id", "o", "", "The organization ID to associate the API key with")
	apikeyCreateCmd.Flags().StringP("project-id", "p", "", "The project ID to associate the API key with")
	apikeyCreateCmd.Flags().String("expires-at", "", "The expiration date of the API key")
	apikeyGetCmd.Flags().BoolP("mask", "m", false, "Mask the API key value")
	for _, cmd := range []*cobra.Command{apikeyListCmd, apikeyCreateCmd, apikeyGetCmd} {
		cmd.Flags().String("format", "text", "The format to output the API key in")
	}
}
