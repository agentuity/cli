package cmd

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/adhocore/gronx"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/go-common/logger"
	cstr "github.com/agentuity/go-common/string"
	"github.com/charmbracelet/huh"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var ioCmd = &cobra.Command{
	Use:   "io",
	Short: "Input and Output commands",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var ioSourceCmd = &cobra.Command{
	Use:     "source",
	Aliases: []string{"src", "in", "input"},
	Short:   "Source commands",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var ioDestinationCmd = &cobra.Command{
	Use:     "destination",
	Aliases: []string{"dest", "out", "output"},
	Short:   "Destination commands",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var srcOptions = []huh.Option[string]{
	huh.NewOption("Webhook", "webhook"),
	huh.NewOption("Schedule", "cron"),
	huh.NewOption("SMS", "sms"),
	huh.NewOption("Email", "email"),
}

var destOptions = []huh.Option[string]{
	huh.NewOption("Webhook", "webhook"),
	huh.NewOption("SMS", "sms"),
	huh.NewOption("Email", "email"),
}

func validateURL(urlstr string) error {
	if urlstr == "" {
		return fmt.Errorf("URL is required")
	}
	if !strings.HasPrefix(urlstr, "https://") {
		return fmt.Errorf("URL must start with https://")
	}
	_, err := url.Parse(urlstr)
	if err != nil {
		return err
	}
	return nil
}

func minLength(min int) func(string) error {
	return func(s string) error {
		if len(s) < min {
			return fmt.Errorf("must be at least %d characters long", min)
		}
		return nil
	}
}

func configurationCron(logger logger.Logger) map[string]any {
	gron := gronx.New()
	validateCron := func(expr string) error {
		if !gron.IsValid(expr) {
			return fmt.Errorf("invalid cron schedule")
		}
		return nil
	}
	schedule := getInput(logger, "Enter the Schedule Expression (UTC timezone)", "The pattern should be in cron syntax (https://crontab.guru/ is a good resource)", "", false, "0 * * * 1-5", validateCron)
	return map[string]any{"cronExpression": schedule}
}

func configurationWebhook(logger logger.Logger, needsURL bool) map[string]any {
	var url string
	if needsURL {
		url = getInput(logger, "Enter the URL", "", "", false, "", validateURL)
	}
	var authType string
	if huh.NewSelect[string]().
		Title("Select the Authorization Type").
		Options(
			huh.NewOption("None", "none"),
			huh.NewOption("Bearer Token", "bearer"),
			huh.NewOption("HTTP Basic Auth", "basic"),
			huh.NewOption("HTTP Header", "header"),
		).
		Value(&authType).Run() != nil {
		logger.Fatal("failed to select authorization type")
	}
	config := map[string]any{}
	if needsURL {
		config["url"] = url
	}
	switch authType {
	case "none":
	case "bearer":
		token := getInput(logger, "Enter the Authorization Bearer Token", "The input will be masked", "", true, "", minLength(10))
		config["authorization"] = map[string]string{
			"type":  "bearer",
			"token": token,
		}
	case "basic":
		username := getInput(logger, "Enter the HTTP Basic Auth Username", "", "", false, "", minLength(1))
		password := getInput(logger, "Enter the HTTP Basic Auth Password", "", "", true, "", minLength(1))
		config["authorization"] = map[string]string{
			"type":     "basic",
			"username": username,
			"password": password,
		}
	case "header":
		headerName := getInput(logger, "Enter the HTTP Header Name", "", "", false, "", minLength(1))
		headerValue := getInput(logger, "Enter the HTTP Header Value", "", "", false, "", minLength(1))
		config["authorization"] = map[string]string{
			"type":  "header",
			"name":  headerName,
			"value": headerValue,
		}
	}
	return config
}

var ioDestinationCreateCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"new"},
	Short:   "Create a new destination",
	Run: func(cmd *cobra.Command, args []string) {
		context := ensureProject(cmd)
		logger := context.Logger
		theproject := context.Project
		apiUrl := context.APIURL
		apiKey := context.Token
		var destinationType string
		if huh.NewSelect[string]().
			Title("Select a destination type").
			Options(destOptions...).
			Value(&destinationType).
			WithTheme(theme).
			Run() != nil {
			logger.Fatal("failed to select destination type")
		}
		var config map[string]any
		switch destinationType {
		case "webhook":
			config = configurationWebhook(logger, true)
		default:
			logger.Fatal("unsupported destination type: %s", destinationType)
		}
		io, err := theproject.CreateIO(logger, apiUrl, apiKey, "destination", project.IO{
			Direction: "destination",
			Config:    config,
			Type:      destinationType,
		})
		if err != nil {
			logger.Fatal("failed to create destination: %s", err)
		}
		// if we are re-activating an IO, we will get back the same object
		// so we need to check if it already exists in the project
		var found bool
		for _, input := range theproject.Outputs {
			if input.ID == io.ID && input.Type == io.Type {
				found = true
				break
			}
		}
		if !found {
			theproject.Outputs = append(theproject.Outputs, *io)
			if err := theproject.Save(context.Dir); err != nil {
				logger.Fatal("failed to save project: %s", err)
			}
		}
		printSuccess("%s destination created: %s", destinationType, io.ID)
	},
}

var ioSourceCreateCmd = &cobra.Command{
	Use:     "create",
	Aliases: []string{"new"},
	Short:   "Create a new source",
	Run: func(cmd *cobra.Command, args []string) {
		context := ensureProject(cmd)
		logger := context.Logger
		theproject := context.Project
		apiUrl := context.APIURL
		apiKey := context.Token
		var destinationType string
		if huh.NewSelect[string]().
			Title("Select a source type").
			Options(srcOptions...).
			Value(&destinationType).
			WithTheme(theme).
			Run() != nil {
			logger.Fatal("failed to select source type")
		}
		var config map[string]any
		switch destinationType {
		case "cron":
			config = configurationCron(logger)
		case "webhook":
			config = configurationWebhook(logger, false)
		default:
			logger.Fatal("unsupported source type: %s", destinationType)
		}
		io, err := theproject.CreateIO(logger, apiUrl, apiKey, "source", project.IO{
			Direction: "source",
			Config:    config,
			Type:      destinationType,
		})
		if err != nil {
			logger.Fatal("failed to create source: %s", err)
		}
		// if we are re-activating an IO, we will get back the same object
		// so we need to check if it already exists in the project
		var found bool
		for _, input := range theproject.Inputs {
			if input.ID == io.ID && input.Type == io.Type {
				found = true
				break
			}
		}
		if !found {
			theproject.Inputs = append(theproject.Inputs, *io)
			if err := theproject.Save(context.Dir); err != nil {
				logger.Fatal("failed to save project: %s", err)
			}
		}
		printSuccess("%s source created: %s", destinationType, io.ID)
	},
}

var ioSourceDeleteCmd = &cobra.Command{
	Use:     "delete [id]",
	Aliases: []string{"del", "rm"},
	Short:   "Delete a source",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		context := ensureProject(cmd)
		logger := context.Logger
		theproject := context.Project
		apiUrl := context.APIURL
		apiKey := context.Token
		if !ask(logger, "Are you sure you want to delete this source?", false) {
			printWarning("cancelled")
			return
		}
		err := theproject.DeleteIO(logger, apiUrl, apiKey, args[0])
		if err != nil {
			logger.Fatal("failed to delete source: %s", err)
		}
		printSuccess("source deleted: %s", args[0])
	},
}

var ioDestinationDeleteCmd = &cobra.Command{
	Use:     "delete [id]",
	Aliases: []string{"del", "rm"},
	Short:   "Delete a destination",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		context := ensureProject(cmd)
		logger := context.Logger
		theproject := context.Project
		apiUrl := context.APIURL
		apiKey := context.Token
		if !ask(logger, "Are you sure you want to delete this destination?", false) {
			printWarning("cancelled")
			return
		}
		err := theproject.DeleteIO(logger, apiUrl, apiKey, args[0])
		if err != nil {
			logger.Fatal("failed to delete destination: %s", err)
		}
		printSuccess("destination deleted: %s", args[0])
	},
}

func printIO(res []project.IO) {
	for _, item := range res {
		fmt.Printf("%s: %s\n", color.WhiteString("type"), color.GreenString(item.Type))
		fmt.Printf("  %s: %s\n", color.WhiteString("id"), color.BlackString(item.ID))
		fmt.Printf("  %s: %s\n", color.WhiteString("config"), color.BlackString(fmt.Sprintf("%v", cstr.JSONStringify(item.Config))))
		fmt.Println()
	}
}

var ioSourceListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"show", "get"},
	Short:   "List configured sources",
	Run: func(cmd *cobra.Command, args []string) {
		context := ensureProject(cmd)
		logger := context.Logger
		theproject := context.Project
		apiUrl := context.APIURL
		apiKey := context.Token
		res, err := theproject.ListIO(logger, apiUrl, apiKey, "source")
		if err != nil {
			logger.Fatal("failed to list sources: %s", err)
		}
		printIO(res)
	},
}

var ioDestinationListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"show", "get"},
	Short:   "List configured destinations",
	Run: func(cmd *cobra.Command, args []string) {
		context := ensureProject(cmd)
		logger := context.Logger
		theproject := context.Project
		apiUrl := context.APIURL
		apiKey := context.Token
		res, err := theproject.ListIO(logger, apiUrl, apiKey, "destination")
		if err != nil {
			logger.Fatal("failed to list destination: %s", err)
		}
		printIO(res)
	},
}

func init() {
	rootCmd.AddCommand(ioCmd)
	addURLFlags(ioCmd)

	ioCmd.AddCommand(ioSourceCmd)
	ioCmd.AddCommand(ioDestinationCmd)

	ioDestinationCmd.AddCommand(ioDestinationCreateCmd)
	ioDestinationCmd.AddCommand(ioDestinationListCmd)
	ioDestinationCmd.AddCommand(ioDestinationDeleteCmd)

	ioSourceCmd.AddCommand(ioSourceCreateCmd)
	ioSourceCmd.AddCommand(ioSourceListCmd)
	ioSourceCmd.AddCommand(ioSourceDeleteCmd)

	for _, cmd := range []*cobra.Command{
		ioDestinationCreateCmd,
		ioSourceCreateCmd,
		ioSourceDeleteCmd,
		ioSourceListCmd,
		ioDestinationListCmd,
		ioDestinationDeleteCmd,
	} {
		cmd.Flags().StringP("dir", "d", ".", "The directory to the project to deploy")
	}
}
