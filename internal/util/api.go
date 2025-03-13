package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/agentuity/cli/internal/tui"
	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/sys"
	"github.com/spf13/viper"
)

type APIClient struct {
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
}

func (e *APIError) Error() string {
	if e == nil || e.TheError == nil {
		return ""
	}
	return e.TheError.Error()
}

func NewAPIError(url, method string, status int, body string, err error) *APIError {
	return &APIError{
		URL:      url,
		Method:   method,
		Status:   status,
		Body:     body,
		TheError: err,
	}
}

func NewAPIClient(logger logger.Logger, baseURL, token string) *APIClient {
	return &APIClient{
		logger:  logger,
		baseURL: baseURL,
		token:   token,
		client:  http.DefaultClient,
	}
}

type APIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Error   struct {
		Issues []struct {
			Code    string   `json:"code"`
			Message string   `json:"message"`
			Path    []string `json:"path"`
		} `json:"issues"`
	} `json:"error"`
}

func (c *APIClient) Do(method, path string, payload interface{}, response interface{}) error {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return NewAPIError(c.baseURL, method, 0, "", fmt.Errorf("error parsing base url: %w", err))
	}
	u.Path = path

	var body []byte
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return NewAPIError(u.String(), method, 0, "", fmt.Errorf("error marshalling payload: %w", err))
		}
	}
	c.logger.Trace("sending request: %s %s", method, u.String())

	req, err := http.NewRequest(method, u.String(), bytes.NewBuffer(body))
	if err != nil {
		return NewAPIError(u.String(), method, 0, "", fmt.Errorf("error creating request: %w", err))
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return NewAPIError(u.String(), method, 0, "", fmt.Errorf("error sending request: %w", err))
	}
	defer resp.Body.Close()
	c.logger.Debug("response status: %s", resp.Status)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return NewAPIError(u.String(), method, 0, "", fmt.Errorf("error reading response body: %w", err))
	}

	c.logger.Debug("response body: %s, content-type: %s", string(respBody), resp.Header.Get("content-type"))
	if resp.StatusCode > 299 && strings.Contains(resp.Header.Get("content-type"), "application/json") {
		var apiResponse APIResponse
		if err := json.Unmarshal(respBody, &apiResponse); err != nil {
			return NewAPIError(u.String(), method, resp.StatusCode, string(respBody), fmt.Errorf("error unmarshalling response: %w", err))
		}
		if !apiResponse.Success {
			if len(apiResponse.Error.Issues) > 0 {
				var errs []string
				for _, issue := range apiResponse.Error.Issues {
					msg := fmt.Sprintf("%s (%s)", issue.Message, issue.Code)
					if issue.Path != nil {
						msg = msg + " " + strings.Join(issue.Path, ".")
					}
					errs = append(errs, msg)
				}
				return NewAPIError(u.String(), method, resp.StatusCode, string(respBody), fmt.Errorf("%s", strings.Join(errs, ". ")))
			}
			if apiResponse.Message != "" {
				return NewAPIError(u.String(), method, resp.StatusCode, string(respBody), fmt.Errorf("%s", apiResponse.Message))
			}
		}
	}

	if resp.StatusCode > 299 {
		return NewAPIError(u.String(), method, resp.StatusCode, string(respBody), fmt.Errorf("request failed with status (%s)", resp.Status))
	}

	if response != nil {
		if err := json.Unmarshal(respBody, &response); err != nil {
			return NewAPIError(u.String(), method, resp.StatusCode, string(respBody), fmt.Errorf("error JSON decoding response: %w", err))
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

func GetURLs(logger logger.Logger) (string, string) {
	appUrl := viper.GetString("overrides.app_url")
	apiUrl := viper.GetString("overrides.api_url")
	if apiUrl == "https://api.agentuity.com" && appUrl != "https://app.agentuity.com" {
		logger.Debug("switching app url to production since the api url is production")
		appUrl = "https://app.agentuity.com"
	} else if apiUrl == "https://api.agentuity.dev" && appUrl == "https://app.agentuity.com" {
		logger.Debug("switching app url to dev since the api url is dev")
		appUrl = "http://localhost:3000"
	}
	return TransformUrl(apiUrl), TransformUrl(appUrl)
}

func ShowLogin() {
	fmt.Println(tui.Warning("You are not currently logged in or your session has expired."))
	tui.ShowBanner("Login", tui.Text("Use ")+tui.Command("login")+tui.Text(" to login to Agentuity"), false)
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
