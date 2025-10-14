package util

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"regexp"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/sys"
	"github.com/agentuity/go-common/tui"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	Version = "dev"
	Commit  = "unknown"
	retry   = 5
)

type APIClient struct {
	ctx     context.Context
	baseURL string
	token   string
	client  *http.Client
	logger  logger.Logger
}

type APIError struct {
	URL      string
	Method   string
	Status   int
	Body     string
	TheError error
	TraceID  string
}

func (e *APIError) Error() string {
	if e == nil || e.TheError == nil {
		return ""
	}
	return e.TheError.Error()
}

func NewAPIError(url, method string, status int, body string, err error, traceID string) *APIError {
	return &APIError{
		URL:      url,
		Method:   method,
		Status:   status,
		Body:     body,
		TheError: err,
		TraceID:  traceID,
	}
}

func NewAPIClient(ctx context.Context, logger logger.Logger, baseURL, token string) *APIClient {
	return &APIClient{
		ctx:     ctx,
		logger:  logger,
		baseURL: baseURL,
		token:   token,
		client:  http.DefaultClient,
	}
}

type APIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
	Error   struct {
		Issues []struct {
			Code    string   `json:"code"`
			Message string   `json:"message"`
			Path    []string `json:"path"`
		} `json:"issues"`
	} `json:"error"`
}

func UserAgent() string {
	gitSHA := Commit
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				gitSHA = setting.Value
			}
		}
	}
	return "Agentuity CLI/" + Version + " (" + gitSHA + ")"
}

func shouldRetry(resp *http.Response, err error) bool {
	if err != nil {
		if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) {
			return true
		} else if msg := err.Error(); strings.Contains(msg, "EOF") {
			return true
		}
	}
	if resp != nil {
		switch resp.StatusCode {
		case http.StatusRequestTimeout, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout, http.StatusTooManyRequests:
			return true
		}
	}
	return false
}

func (c *APIClient) Do(method, pathParam string, payload interface{}, response interface{}) error {
	var traceID string

	u, err := url.Parse(c.baseURL)
	if err != nil {
		return NewAPIError(c.baseURL, method, 0, "", fmt.Errorf("error parsing base url: %w", err), traceID)
	}

	i := strings.Index(pathParam, "?")
	if i != -1 {
		u.RawQuery = pathParam[i+1:]
		pathParam = pathParam[:i]
	}

	basePath := u.Path
	if pathParam == "" {
		u.Path = basePath
	} else if basePath == "" || basePath == "/" {
		u.Path = pathParam
	} else {
		u.Path = path.Join(basePath, pathParam)
	}
	var body []byte
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return NewAPIError(u.String(), method, 0, "", fmt.Errorf("error marshalling payload: %w", err), traceID)
		}
	}
	c.logger.Trace("sending request: %s %s", method, u.String())

	req, err := http.NewRequestWithContext(c.ctx, method, u.String(), bytes.NewBuffer(body))
	if err != nil {
		return NewAPIError(u.String(), method, 0, "", fmt.Errorf("error creating request: %w", err), traceID)
	}

	req.Header.Set("User-Agent", UserAgent())
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	var resp *http.Response
	for i := 0; i < retry; i++ {
		isLast := i == retry-1
		var err error
		resp, err = c.client.Do(req)
		if shouldRetry(resp, err) && !isLast {
			c.logger.Trace("client returned retryable error, retrying...")
			// exponential backoff
			v := 150 * math.Pow(2, float64(i))
			time.Sleep(time.Duration(v) * time.Millisecond)
			continue
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			return NewAPIError(u.String(), method, 0, "", fmt.Errorf("error sending request: %w", err), traceID)
		}
		break
	}
	defer resp.Body.Close()
	c.logger.Debug("response status: %s", resp.Status)

	if resp.Header != nil {
		traceID = resp.Header.Get("traceparent")
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return NewAPIError(u.String(), method, 0, "", fmt.Errorf("error reading response body: %w", err), traceID)
	}

	c.logger.Debug("response body: %s, content-type: %s, user-agent: %s", string(respBody), resp.Header.Get("content-type"), UserAgent())
	if resp.StatusCode > 299 && strings.Contains(resp.Header.Get("content-type"), "application/json") {
		var apiResponse APIResponse
		if err := json.Unmarshal(respBody, &apiResponse); err != nil {
			return NewAPIError(u.String(), method, resp.StatusCode, string(respBody), fmt.Errorf("error unmarshalling response: %w", err), traceID)
		}
		if !apiResponse.Success {
			if apiResponse.Code == "UPGRADE_REQUIRED" {
				return NewAPIError(u.String(), method, http.StatusUnprocessableEntity, apiResponse.Message, fmt.Errorf("%s", apiResponse.Message), traceID)
			}
			if len(apiResponse.Error.Issues) > 0 {
				var errs []string
				for _, issue := range apiResponse.Error.Issues {
					msg := fmt.Sprintf("%s (%s)", issue.Message, issue.Code)
					if issue.Path != nil {
						msg = msg + " " + strings.Join(issue.Path, ".")
					}
					errs = append(errs, msg)
				}
				return NewAPIError(u.String(), method, resp.StatusCode, string(respBody), fmt.Errorf("%s", strings.Join(errs, ". ")), traceID)
			}
			if apiResponse.Message != "" {
				return NewAPIError(u.String(), method, resp.StatusCode, string(respBody), fmt.Errorf("%s", apiResponse.Message), traceID)
			}
		}
	}

	if resp.StatusCode > 299 {
		return NewAPIError(u.String(), method, resp.StatusCode, string(respBody), fmt.Errorf("request failed with status (%s)", resp.Status), traceID)
	}

	if response != nil {
		if err := json.Unmarshal(respBody, &response); err != nil {
			return NewAPIError(u.String(), method, resp.StatusCode, string(respBody), fmt.Errorf("error JSON decoding response: %w", err), traceID)
		}
	}
	return nil
}

var testInside bool

func TransformUrl(urlString string) string {
	// NOTE: these urls are special cases for local development inside a container
	if strings.Contains(urlString, "api.agentuity.dev") || strings.Contains(urlString, "localhost:") || strings.Contains(urlString, "127.0.0.1:") {
		if sys.IsRunningInsideDocker() || testInside {
			if strings.HasPrefix(urlString, "https://api.agentuity.dev") {
				u, _ := url.Parse(urlString)
				u.Scheme = "http"
				u.Host = "host.docker.agentuity.io:3012"
				return u.String()
			}
			port := regexp.MustCompile(`:(\d+)`)
			host := "host.docker.agentuity.io:3000"
			if port.MatchString(urlString) {
				host = "host.docker.agentuity.io:" + port.FindStringSubmatch(urlString)[1]
			}
			u, _ := url.Parse(urlString)
			u.Scheme = "http"
			u.Host = host
			return u.String()
		}
	}
	return urlString
}

type CLIUrls struct {
	API       string
	App       string
	Transport string
	Gravity   string
}

func GetURLs(logger logger.Logger) CLIUrls {
	appUrl := viper.GetString("overrides.app_url")
	apiUrl := viper.GetString("overrides.api_url")
	transportUrl := viper.GetString("overrides.transport_url")
	gravityUrl := viper.GetString("overrides.gravity_url")
	if apiUrl == "https://api.agentuity.com" && appUrl != "https://app.agentuity.com" {
		logger.Debug("switching app url to production since the api url is production")
		appUrl = "https://app.agentuity.com"
	} else if apiUrl == "https://api.agentuity.io" && appUrl == "https://app.agentuity.com" {
		logger.Debug("switching app url to dev since the api url is dev")
		appUrl = "https://app.agentuity.io"
	}
	if gravityUrl == "" {
		gravityUrl = "grpc://gravity.agentuity.com"
	}
	if apiUrl == "https://api.agentuity.com" && transportUrl != "https://agentuity.ai" {
		logger.Debug("switching transport url to production since the api url is production")
		transportUrl = "https://agentuity.ai"
	} else if apiUrl == "https://api.agentuity.io" && transportUrl == "https://agentuity.ai" {
		logger.Debug("switching transport url to dev since the api url is dev")
		transportUrl = "https://ai.agentuity.io"
	}
	if apiUrl == "https://api.agentuity.io" {
		logger.Debug("switching gravity url to dev since the api url is dev")
		gravityUrl = "grpc://gravity.agentuity.io:8443"
	}
	return CLIUrls{
		API:       TransformUrl(apiUrl),
		App:       TransformUrl(appUrl),
		Transport: TransformUrl(transportUrl),
		Gravity:   TransformUrl(gravityUrl),
	}
}

func run(ctx context.Context, c *cobra.Command, command string, args ...string) {
	exe, err := os.Executable()
	if err != nil {
		tui.ShowError("Failed to get executable path: %s", err)
		os.Exit(1)
	}
	cmdargs := append([]string{command}, args...)
	if c.Flags().Changed("api-key") {
		cmdargs = append(cmdargs, []string{"--api-key", c.Flag("api-key").Value.String()}...)
	}
	if c.Flags().Changed("api-url") {
		cmdargs = append(cmdargs, []string{"--api-url", c.Flag("api-url").Value.String()}...)
	}
	if c.Flags().Changed("app-url") {
		cmdargs = append(cmdargs, []string{"--app-url", c.Flag("app-url").Value.String()}...)
	}
	if c.Flags().Changed("transport-url") {
		cmdargs = append(cmdargs, []string{"--transport-url", c.Flag("transport-url").Value.String()}...)
	}
	if c.Flags().Changed("log-level") {
		cmdargs = append(cmdargs, []string{"--log-level", c.Flag("log-level").Value.String()}...)
	}
	if c.Flags().Changed("config") {
		cmdargs = append(cmdargs, []string{"--config", c.Flag("config").Value.String()}...)
	}
	cmd := exec.CommandContext(ctx, exe, cmdargs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	err = cmd.Run()
	if err != nil {
		tui.ShowError("Failed to run command: %s", err)
		os.Exit(1)
	}
}

func ShowLogin(ctx context.Context, logger logger.Logger, cmd *cobra.Command) {
	if hasLoggedInBefore() {
		fmt.Println(tui.Warning("You are not currently logged in or your session has expired."))
		if tui.HasTTY {
			run(ctx, cmd, "auth", "login")
		} else {
			fmt.Println(tui.Warning("Use " + tui.Command("agentuity login") + " to login to Agentuity"))
			os.Exit(1)
		}
	} else {
		if tui.HasTTY {
			// we can't assume they don't have an account so we have to ask
			choice := tui.Select(logger, "Authentication Required", "You must login or create an account to continue:", []tui.Option{
				{
					ID:   "login",
					Text: tui.PadRight("Login", 15, " ") + tui.Muted(" Login to your existing account"),
				},
				{
					ID:   "signup",
					Text: tui.PadRight("Signup", 15, " ") + tui.Muted(" Signup for a free account"),
				},
			})
			if choice == "login" {
				run(ctx, cmd, "auth", "login")
			} else {
				run(ctx, cmd, "auth", "signup")
			}
		} else {
			fmt.Println(tui.Warning("Use " + tui.Command("agentuity auth signup") + " to create an account or " + tui.Command("agentuity login") + " to login to Agentuity"))
			os.Exit(1)
		}
	}
}

func TryLoggedIn() (string, string, bool) {
	apikey := viper.GetString("auth.api_key")
	if apikey == "" {
		return "", "", false
	}
	userId := viper.GetString("auth.user_id")
	if userId == "" {
		return "", "", false
	}
	expires := viper.GetInt64("auth.expires")
	if expires < time.Now().UnixMilli() {
		return "", "", false
	}
	return apikey, userId, true
}

func EnsureLoggedIn(ctx context.Context, logger logger.Logger, cmd *cobra.Command) (string, string) {
	apikey := viper.GetString("auth.api_key")
	if apikey == "" {
		ShowLogin(ctx, logger, cmd)
		os.Exit(1)
	}
	userId := viper.GetString("auth.user_id")
	if userId == "" {
		ShowLogin(ctx, logger, cmd)
		os.Exit(1)
	}
	expires := viper.GetInt64("auth.expires")
	if expires < time.Now().UnixMilli() {
		ShowLogin(ctx, logger, cmd)
		os.Exit(1)
	}
	return apikey, userId
}

func EnsureLoggedInWithOnlyAPIKey(ctx context.Context, logger logger.Logger, cmd *cobra.Command) string {
	apikey := viper.GetString("auth.api_key")
	if apikey == "" {
		ShowLogin(ctx, logger, cmd)
		os.Exit(1)
	}
	return apikey
}

func hasLoggedInBefore() bool {
	return viper.GetInt64("auth.expires") > 0 || viper.GetString("templates.etag") != "" || viper.GetInt64("preferences.last_update_check") != 0
}
