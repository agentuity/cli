package cmd

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/adhocore/gronx"
	"github.com/agentuity/cli/internal/project"
	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/logger"
	cstr "github.com/agentuity/go-common/string"
	"github.com/charmbracelet/huh"
	"github.com/fatih/color"
	"github.com/pkg/browser"
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

var sourceOptions = []huh.Option[string]{
	huh.NewOption("Webhook", "webhook"),
	huh.NewOption("Schedule", "cron"),
	huh.NewOption("SMS", "sms"),
	huh.NewOption("Email", "email"),
}

var destinationOptions = []huh.Option[string]{
	huh.NewOption("Webhook", "webhook"),
	huh.NewOption("SMS", "sms"),
	huh.NewOption("Email", "email"),
	huh.NewOption("Slack", "slack"),
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

func validatePhoneNumber(phoneNumber string) error {
	if phoneNumber == "" {
		return fmt.Errorf("phone number is required")
	}
	if !strings.HasPrefix(phoneNumber, "+1") {
		return fmt.Errorf("phone number must start with +1")
	}
	if len(phoneNumber) != 10 {
		return fmt.Errorf("phone number must be 10 digits")
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

func configureCron(logger logger.Logger) map[string]any {
	gron := gronx.New()
	validateCron := func(expr string) error {
		if !gron.IsValid(expr) {
			return fmt.Errorf("invalid cron schedule")
		}
		return nil
	}
	schedule := getInput(logger, "Enter the Schedule Expression (UTC timezone)", "The pattern should be in cron syntax (https://crontab.guru/ is a good resource)", "", false, "0 * * * 1-5", validateCron)
	var data string
	if huh.NewText().
		Title("If you would like to send a specific payload when triggered, enter it here").
		Value(&data).Run() != nil {
		logger.Fatal("failed to enter payload")
	}
	contentType := "text/plain"
	if data != "" && strings.HasPrefix(data, "{") && strings.HasSuffix(data, "}") {
		contentType = "application/json"
	}
	contentType = getInput(logger, "Enter the Content Type", "The content type of the payload", "", false, contentType, nil)
	if data != "" && contentType == "application/json" {
	}
	if contentType == "application/json" {
		if data == "" {
			contentType = "text/plain"
		} else {
			if !json.Valid([]byte(data)) {
				logger.Fatal("You specified JSON content type but the input was invalid JSON")
			}
		}
	}
	return map[string]any{"cronExpression": schedule, "payload": base64.StdEncoding.EncodeToString([]byte(data)), "contentType": contentType}
}

type ExistingPhoneResponse struct {
	Success bool `json:"success"`
	Data    []struct {
		PhoneNumber   string `json:"phoneNumber"`
		PhoneNumberId string `json:"phoneNumberId"`
	} `json:"data"`
}

type PhoneAvailableResponse struct {
	Success bool `json:"success"`
	Data    []struct {
		PhoneNumber string `json:"phoneNumber"`
	} `json:"data"`
}

type PhoneBuyResponse struct {
	Success bool `json:"success"`
	Data    struct {
		PhoneNumberId string `json:"phoneNumberId"`
	} `json:"data"`
}

type SlackIntegrationResponse struct {
	Success bool `json:"success"`
	Data    []struct {
		TeamId   string `json:"teamId"`
		TeamName string `json:"teamName"`
	} `json:"data"`
}

type SlackOAuthResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Url string `json:"url"`
	} `json:"data"`
}

func configureSlack(logger logger.Logger, apiClient *util.APIClient, projectId string, isSource bool) map[string]any {
	var slackResponse SlackIntegrationResponse
	err := apiClient.Do("GET", fmt.Sprintf("/cli/integration/%s/%s", "slack", projectId), nil, &slackResponse)
	if err != nil {
		logger.Fatal("failed to get Slack integration: %s", err)
	}
	if !slackResponse.Success {
		logger.Fatal("failed to get Slack integration")
	}

	if len(slackResponse.Data) == 0 {
		var oauthResponse SlackOAuthResponse
		err = apiClient.Do("GET", fmt.Sprintf("/oauth/slack/link/%s", projectId), nil, &oauthResponse)
		if err != nil {
			logger.Fatal("failed to get Slack OAuth URL: %s", err)
		}
		if !oauthResponse.Success {
			logger.Fatal("failed to get Slack OAuth URL")
		}

		if err := browser.OpenURL(oauthResponse.Data.Url); err != nil {
			log.Fatalf("failed to open browser: %s", err)
		}
		printSuccess("Slack OAuth URL opened in browser, complete the installation and try again")
		os.Exit(0)
	}
	var teamId string = slackResponse.Data[0].TeamId

	if isSource {
		return map[string]any{}
	}

	channelId := getInput(logger, "Enter the Slack Channel ID", "", "", false, "", nil)

	return map[string]any{
		"channelId": channelId,
		"teamId":    teamId,
	}
}

func configureSMS(logger logger.Logger, apiClient *util.APIClient, projectId string, requireTo bool) map[string]any {
	var phoneNumberId string
	var existingPhoneResponse ExistingPhoneResponse
	err := apiClient.Do("GET", "/phone", nil, &existingPhoneResponse)
	if err != nil {
		logger.Fatal("failed to get existing phone numbers: %s", err)
	}

	if !existingPhoneResponse.Success {
		logger.Fatal("failed to get existing phone numbers: %s", err)
	}

	if len(existingPhoneResponse.Data) > 0 {
		var useExistingPhone bool
		if huh.NewConfirm().
			Title("Do you want to use an existing phone number?").
			Value(&useExistingPhone).Run() != nil {
			logger.Fatal("failed to confirm")
		}
		if useExistingPhone {
			phoneOptions := make([]huh.Option[string], len(existingPhoneResponse.Data))
			for i, p := range existingPhoneResponse.Data {
				phoneOptions[i] = huh.NewOption(p.PhoneNumber, p.PhoneNumberId)
			}
			if huh.NewSelect[string]().
				Title("Select a phone number").
				Options(phoneOptions...).
				Value(&phoneNumberId).Run() != nil {
				logger.Fatal("failed to select phone number")
			}
		}
	}

	var phoneAvailableResponse PhoneAvailableResponse
	if err := apiClient.Do("GET", "/phone/available", nil, &phoneAvailableResponse); err != nil {
		logger.Fatal("failed to get available phone available: %s", err)
	}
	if !phoneAvailableResponse.Success {
		logger.Fatal("failed to get phone available")
	}

	phoneOptions := make([]huh.Option[string], len(phoneAvailableResponse.Data))
	for i, p := range phoneAvailableResponse.Data {
		phoneOptions[i] = huh.NewOption(p.PhoneNumber, p.PhoneNumber)
	}

	if phoneNumberId == "" {
		var phoneNumber string
		if huh.NewSelect[string]().
			Title("Purchase a phone number").
			Options(phoneOptions...).
			Value(&phoneNumber).Run() != nil {
			logger.Fatal("failed to select phone number")
		}
		var phoneBuyResponse PhoneBuyResponse
		err = apiClient.Do("POST", "/phone/buy", map[string]string{
			"phoneNumber": phoneNumber,
			"projectId":   projectId,
		}, &phoneBuyResponse)
		if err != nil {
			logger.Fatal("failed to buy phone number: %s", err)
		}
		if !phoneBuyResponse.Success {
			logger.Fatal("failed to buy phone number")
		}
		logger.Info("phone number purchased: %s", phoneBuyResponse.Data.PhoneNumberId)
		phoneNumberId = phoneBuyResponse.Data.PhoneNumberId
	}

	var toNumber string
	if requireTo {
		toNumber = getInput(logger, "Enter the phone number to send SMS to:", "", "", false, "", validatePhoneNumber)
	}

	config := map[string]any{
		"to":            toNumber,
		"phoneNumberId": phoneNumberId,
	}

	return config
}

func configureWebhook(logger logger.Logger, needsURL bool) map[string]any {
	var url string
	if needsURL {
		url = getInput(logger, "Enter the URL to the Webhook", "", "", false, "", validateURL)
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
		config := map[string]any{}
		if huh.NewSelect[string]().
			Title("Select a destination type").
			Options(destinationOptions...).
			Value(&destinationType).
			WithTheme(theme).
			Run() != nil {
			logger.Fatal("failed to select destination type")
		}
		apiClient := util.NewAPIClient(logger, apiUrl, apiKey)
		switch destinationType {
		case "sms":
			config = configureSMS(logger, apiClient, theproject.ProjectId, true)
		case "webhook":
			config = configureWebhook(logger, true)
		case "slack":
			config = configureSlack(logger, apiClient, theproject.ProjectId, false)
		default:
			logger.Fatal("invalid destination type: %s. currently only sms, webhook, and slack are supported", destinationType)
		}
		io, err := theproject.CreateIO(logger, apiUrl, apiKey, "destination", project.IO{
			Direction: "destination",
			Config:    config,
			Type:      destinationType,
		})
		if err != nil {
			logger.Fatal("failed to create destination: %s", err)
		}
		theproject.Outputs = append(theproject.Outputs, *io)
		if err := theproject.Save(context.Dir); err != nil {
			logger.Fatal("failed to save project: %s", err)
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
		apiClient := util.NewAPIClient(logger, apiUrl, apiKey)
		var sourceType string
		if huh.NewSelect[string]().
			Title("Select a source type").
			Options(sourceOptions...).
			Value(&sourceType).
			WithTheme(theme).
			Run() != nil {
			logger.Fatal("failed to select source type")
		}
		var config map[string]any
		switch sourceType {
		case "cron":
			config = configureCron(logger)
		case "sms":
			config = configureSMS(logger, apiClient, theproject.ProjectId, false)
		case "webhook":
			config = configureWebhook(logger, false)
		case "slack":
			config = configureSlack(logger, apiClient, theproject.ProjectId, true)
		default:
			logger.Fatal("invalid source type: %s. currently only cron, sms, webhook, and slack are supported", sourceType)
		}
		io, err := theproject.CreateIO(logger, apiUrl, apiKey, "source", project.IO{
			Direction: "source",
			Config:    config,
			Type:      sourceType,
		})
		if err != nil {
			logger.Fatal("failed to create source: %s", err)
		}
		theproject.Inputs = append(theproject.Inputs, *io)
		if err := theproject.Save(context.Dir); err != nil {
			logger.Fatal("failed to save project: %s", err)
		}
		printSuccess("%s source created: %s", sourceType, io.ID)
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
		cmd.Flags().StringP("dir", "d", ".", "The root of the project directory")
	}
}
