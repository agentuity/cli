package util

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime/debug"
	"strings"
	"time"

	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/sys"
	"github.com/agentuity/go-common/tui"
	"github.com/spf13/viper"
)

var (
	Version = "dev"
	Commit  = "unknown"
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

func (c *APIClient) Do(method, path string, payload interface{}, response interface{}) error {
	var traceID string

	u, err := url.Parse(c.baseURL)
	if err != nil {
		return NewAPIError(c.baseURL, method, 0, "", fmt.Errorf("error parsing base url: %w", err), traceID)
	}
	u.Path = path

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

	resp, err := c.client.Do(req)
	if err != nil {
		return NewAPIError(u.String(), method, 0, "", fmt.Errorf("error sending request: %w", err), traceID)
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
	if strings.Contains(urlString, "api.agentuity.dev") || strings.Contains(urlString, "localhost:") {
		if sys.Exists("/.dockerenv") || sys.Exists("/proc/1/cgroup") || testInside {
			if strings.HasPrefix(urlString, "https://api.agentuity.dev") {
				u, _ := url.Parse(urlString)
				u.Scheme = "http"
				u.Host = "host.docker.internal:3012"
				return u.String()
			}
			port := regexp.MustCompile(`:(\d+)`)
			host := "host.docker.internal:3000"
			if port.MatchString(urlString) {
				host = "host.docker.internal:" + port.FindStringSubmatch(urlString)[1]
			}
			u, _ := url.Parse(urlString)
			u.Scheme = "http"
			u.Host = host
			return u.String()
		}
	}
	return urlString
}

func GetURLs(logger logger.Logger) (string, string, string) {
	appUrl := viper.GetString("overrides.app_url")
	apiUrl := viper.GetString("overrides.api_url")
	transportUrl := viper.GetString("overrides.transport_url")
	if apiUrl == "https://api.agentuity.com" && appUrl != "https://app.agentuity.com" {
		logger.Debug("switching app url to production since the api url is production")
		appUrl = "https://app.agentuity.com"
	} else if apiUrl == "https://api.agentuity.dev" && appUrl == "https://app.agentuity.com" {
		logger.Debug("switching app url to dev since the api url is dev")
		appUrl = "http://localhost:3000"
	}
	if apiUrl == "https://api.agentuity.com" && transportUrl != "https://agentuity.api" {
		logger.Debug("switching transport url to production since the api url is production")
		transportUrl = "https://agentuity.ai"
	} else if apiUrl == "https://api.agentuity.dev" && transportUrl == "https://agentuity.ai" {
		logger.Debug("switching transport url to dev since the api url is dev")
		transportUrl = "http://localhost:3939"
	}
	return TransformUrl(apiUrl), TransformUrl(appUrl), TransformUrl(transportUrl)
}

func ShowLogin() {
	fmt.Println(tui.Warning("You are not currently logged in or your session has expired."))
	tui.ShowBanner("Login", tui.Text("Use ")+tui.Command("login")+tui.Text(" to login to Agentuity"), false)
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

func EnsureLoggedIn() (string, string) {
	apikey := viper.GetString("auth.api_key")
	if apikey == "" {
		ShowLogin()
		os.Exit(1)
	}
	userId := viper.GetString("auth.user_id")
	if userId == "" {
		ShowLogin()
		os.Exit(1)
	}
	expires := viper.GetInt64("auth.expires")
	if expires < time.Now().UnixMilli() {
		ShowLogin()
		os.Exit(1)
	}
	return apikey, userId
}

func EnsureLoggedInWithOnlyAPIKey() string {
	apikey := viper.GetString("auth.api_key")
	if apikey == "" {
		ShowLogin()
		os.Exit(1)
	}
	return apikey
}
