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

func NewAPIClient(logger logger.Logger, baseURL, token string) *APIClient {
	return &APIClient{
		logger:  logger,
		baseURL: baseURL,
		token:   token,
		client:  http.DefaultClient,
	}
}

func (c *APIClient) Do(method, path string, payload interface{}, response interface{}) error {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Errorf("error parsing base url: %w", err)
	}
	u.Path = path

	var body []byte
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("error marshalling payload: %w", err)
		}
	}

	c.logger.Debug("request: %s %s", method, u.String())

	req, err := http.NewRequest(method, u.String(), bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()
	c.logger.Debug("response status: %s", resp.Status)

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %w", err)
	}
	c.logger.Debug("response body: %s", string(respBody))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && response == nil {
		return fmt.Errorf("request failed with status (%s)", resp.Status)
	}

	if response != nil {
		if err := json.NewDecoder(bytes.NewReader(respBody)).Decode(response); err != nil {
			return fmt.Errorf("error decoding response: %w", err)
		}
	}
	return nil
}

func transformUrl(url string) string {
	// NOTE: these urls are special cases for local development inside a container
	if strings.Contains(url, "api.agentuity.dev") || strings.Contains(url, "localhost:") {
		if sys.Exists("/.dockerenv") || sys.Exists("/proc/1/cgroup") {
			if strings.HasPrefix(url, "https://api.agentuity.dev") {
				return "http://host.docker.internal:3012"
			}
			port := regexp.MustCompile(`:(\d+)`)
			if port.MatchString(url) {
				return "http://host.docker.internal:" + port.FindString(url)
			}
			return "http://host.docker.internal:3000"
		}
	}
	return url
}

func GetURLs(logger logger.Logger) (string, string) {
	appUrl := viper.GetString("overrides.app_url")
	apiUrl := viper.GetString("overrides.api_url")
	if apiUrl == "https://api.agentuity.com" && appUrl != "https://app.agentuity.com" {
		logger.Debug("switching app url to production since the api url is production")
		appUrl = "https://app.agentuity.com"
	} else if apiUrl == "https://api.agentuity.div" && appUrl == "https://app.agentuity.com" {
		logger.Debug("switching app url to dev since the api url is dev")
		appUrl = "http://localhost:3000"
	}
	return transformUrl(apiUrl), transformUrl(appUrl)
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
