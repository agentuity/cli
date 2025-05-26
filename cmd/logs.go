package cmd

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/agentuity/cli/internal/util"
	"github.com/agentuity/go-common/env"
	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/tui"
	"github.com/spf13/cobra"
)

// parseFlexibleDuration supports Go durations and 'd' for days (e.g., 1d, 2h, 30m)
func parseFlexibleDuration(s string) (time.Duration, error) {
	// Support 'm' for minutes, 'h' for hours, 'd' for days
	re := regexp.MustCompile(`(?i)^(\d+)(m|h|d)$`)
	matches := re.FindStringSubmatch(s)
	if len(matches) == 3 {
		num, err := strconv.Atoi(matches[1])
		if err != nil {
			return 0, err
		}

		switch strings.ToLower(matches[2]) {
		case "m":
			return time.Duration(num) * time.Minute, nil
		case "h":
			return time.Duration(num) * time.Hour, nil
		case "d":
			return time.Duration(num) * 24 * time.Hour, nil
		}
	}
	return time.ParseDuration(s)
}

type Log struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	Link      string    `json:"link"`
	Severity  string    `json:"severity"`
	Timestamp time.Time `json:"timestamp"`
}

type LogsResponse struct {
	Success bool  `json:"success"`
	Data    []Log `json:"data"`
}

func printLogs(ctx context.Context, logger logger.Logger, cmd *cobra.Command, query url.Values, tail bool, hideDate bool, hideTime bool) {

	apiUrl, _, _ := util.GetURLs(logger)
	apiKey, _ := util.EnsureLoggedIn(ctx, logger, cmd)
	client := util.NewAPIClient(ctx, logger, apiUrl, apiKey)

	startDate := query.Get("startDate")
	if startDate == "" {
		startDate = time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	}

	dateFormat := time.DateTime
	if hideDate && !hideTime {
		dateFormat = time.TimeOnly
	} else if !hideDate && hideTime {
		dateFormat = time.DateOnly
	} else if hideDate && hideTime {
		dateFormat = ""
	}

	prevIds := []string{}
	fetcher := func() bool {
		query.Set("startDate", startDate)
		var response LogsResponse
		err := client.Do("GET",
			fmt.Sprintf("/cli/logs?%s", query.Encode()),
			nil,
			&response,
		)
		if err != nil {
			logger.Error("failed to get logs: %s", err)
			return false
		}

		// Check if any log in response.Data has an ID in prevIds
		found := false
		for _, log := range response.Data {
			if slices.Contains(prevIds, log.ID) {
				found = true
				break
			}
		}
		// If none of the IDs in the response are in prevIds, clear prevIds
		if !found {
			prevIds = prevIds[:0]
		}

		for _, log := range response.Data {
			if slices.Contains(prevIds, log.ID) {
				continue
			}
			prevIds = append(prevIds, log.ID)

			timeString := ""
			if dateFormat != "" {
				timeString = log.Timestamp.Format(dateFormat)
			}
			fmt.Printf("%s %s %s\n",

				tui.Bold(fmt.Sprintf("%-7s", "["+log.Severity+"]")),
				tui.Title(timeString),
				tui.Body(log.Body),
			)
			startDate = log.Timestamp.Format(time.RFC3339)
		}
		return true
	}

	func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if success := fetcher(); !success {
					return
				}
				if !tail {
					return
				}
				time.Sleep(5 * time.Second)
			}
		}
	}()
}

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "View logs for agents, deployments, and more.",
	Run: func(cmd *cobra.Command, args []string) {

		logger := env.NewLogger(cmd)

		query := url.Values{}

		if agent, _ := cmd.Flags().GetString("agent"); agent != "" {
			query.Add("agent", agent)
		}

		if deployment, _ := cmd.Flags().GetString("deployment"); deployment != "" {
			query.Add("deployment", deployment)
		}

		if env, _ := cmd.Flags().GetString("env"); env != "" {
			query.Add("env", env)
		}

		if message, _ := cmd.Flags().GetString("message"); message != "" {
			query.Add("message", message)
		}

		if project, _ := cmd.Flags().GetString("project"); project != "" {
			query.Add("project", project)
		}

		if org, _ := cmd.Flags().GetString("organization"); org != "" {
			query.Add("organization", org)
		}

		if session, _ := cmd.Flags().GetString("session"); session != "" {
			query.Add("session", session)
		}

		if severity, _ := cmd.Flags().GetString("severity"); severity != "" {
			query.Add("severity", severity)
		}

		if since, _ := cmd.Flags().GetString("since"); since != "" {
			startDate, err := parseFlexibleDuration(since)
			if err != nil {
				logger.Fatal("failed to parse since: %s", err)
			}
			query.Add("startDate", time.Now().Add(-1*startDate).Format(time.RFC3339))
		} else {
			query.Add("startDate", time.Now().Add(-1*time.Hour).Format(time.RFC3339))
		}

		tail, _ := cmd.Flags().GetBool("tail")
		hideDate, _ := cmd.Flags().GetBool("hideDate")
		hideTime, _ := cmd.Flags().GetBool("hideTime")
		printLogs(cmd.Context(), logger, cmd, query, tail, hideDate, hideTime)
	},
}

func init() {
	rootCmd.AddCommand(logsCmd)

	logsCmd.Flags().StringP("agent", "a", "", "Filter logs by agent ID or name")
	logsCmd.Flags().StringP("deployment", "d", "", "Filter logs by deployment ID")
	logsCmd.Flags().StringP("env", "e", "local", "Filter logs by environment")
	logsCmd.Flags().StringP("project", "p", "", "Filter logs by project ID or name")
	logsCmd.Flags().StringP("organization", "o", "", "Filter logs by organization ID or name")
	logsCmd.Flags().StringP("session", "S", "", "Filter logs by session ID")
	logsCmd.Flags().StringP("severity", "v", "", "Filter logs by severity (info, warn, error, etc.)")
	logsCmd.Flags().StringP("since", "s", "1h", "Show logs since a specific time (e.g., 1h, 30m, 1d)")

	logsCmd.Flags().BoolP("hideDate", "D", false, "Hide the date from the logs")
	logsCmd.Flags().BoolP("hideTime", "T", false, "Hide the time from the logs")
	logsCmd.Flags().BoolP("tail", "t", false, "Continuously stream new logs (similar to tail -f)")
}
