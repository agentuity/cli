package util

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/agentuity/go-common/logger"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUrlParse(t *testing.T) {
	testInside = true
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"localhost", "http://localhost:3000", "http://host.docker.internal:3000"},
		{"localhost", "http://localhost:3000/test", "http://host.docker.internal:3000/test"},
		{"localhost", "http://localhost:3123/test", "http://host.docker.internal:3123/test"},
		{"localhost", "https://api.agentuity.dev/test", "http://host.docker.internal:3012/test"},
		{"non-localhost", "https://api.example.com/test", "https://api.example.com/test"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := TransformUrl(test.url)
			if got != test.want {
				t.Errorf("TransformUrl(%q) = %q; want %q", test.url, got, test.want)
			}
		})
	}
}

func TestDoPathHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]string{
			"path": r.URL.Path,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	mockLogger := &mockLogger{}

	tests := []struct {
		name     string
		baseURL  string
		path     string
		expected string
	}{
		{
			name:     "base URL without path, path without leading slash",
			baseURL:  server.URL,
			path:     "api/users",
			expected: "/api/users",
		},
		{
			name:     "base URL without path, path with leading slash",
			baseURL:  server.URL,
			path:     "/api/users",
			expected: "/api/users",
		},
		{
			name:     "base URL with path, path without leading slash",
			baseURL:  server.URL + "/v1",
			path:     "api/users",
			expected: "/v1/api/users",
		},
		{
			name:     "base URL with path, path with leading slash",
			baseURL:  server.URL + "/v1",
			path:     "/api/users",
			expected: "/v1/api/users",
		},
		{
			name:     "empty path",
			baseURL:  server.URL + "/v1",
			path:     "",
			expected: "/v1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := NewAPIClient(context.Background(), mockLogger, test.baseURL, "")

			var response map[string]string

			err := client.Do("GET", test.path, nil, &response)
			if err != nil {
				t.Fatalf("Do() error = %v", err)
			}

			if response["path"] != test.expected {
				t.Errorf("Do() path = %q, want %q", response["path"], test.expected)
			}
		})
	}
}

func TestDoWithPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var requestBody map[string]interface{}
		err := json.NewDecoder(r.Body).Decode(&requestBody)
		if err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		response := map[string]interface{}{
			"received": requestBody,
			"method":   r.Method,
			"headers":  r.Header,
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	mockLogger := &mockLogger{}
	client := NewAPIClient(context.Background(), mockLogger, server.URL, "test-token")

	t.Run("POST with payload", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":  "test",
			"value": 123,
		}

		var response map[string]interface{}
		err := client.Do("POST", "/api/data", payload, &response)
		require.NoError(t, err)

		received, ok := response["received"].(map[string]interface{})
		require.True(t, ok)
		assert.Equal(t, "test", received["name"])
		assert.Equal(t, float64(123), received["value"])

		method, ok := response["method"].(string)
		require.True(t, ok)
		assert.Equal(t, "POST", method)

		headers, ok := response["headers"].(map[string]interface{})
		require.True(t, ok)

		authHeaders, ok := headers["Authorization"].([]interface{})
		require.True(t, ok)
		assert.Equal(t, "Bearer test-token", authHeaders[0])
	})
}

func TestDoWithErrorResponses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/error/") {
			statusCode := 500
			if strings.Contains(r.URL.Path, "/400") {
				statusCode = 400
			} else if strings.Contains(r.URL.Path, "/404") {
				statusCode = 404
			} else if strings.Contains(r.URL.Path, "/422") {
				statusCode = 422
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(statusCode)
				fmt.Fprint(w, `{"success":false,"message":"Validation failed","code":"UPGRADE_REQUIRED"}`)
				return
			} else if strings.Contains(r.URL.Path, "/validation") {
				statusCode = 422
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(statusCode)
				fmt.Fprint(w, `{"success":false,"error":{"issues":[{"code":"invalid_type","message":"Expected string, received number","path":["name"]}]}}`)
				return
			} else if strings.Contains(r.URL.Path, "/message") {
				statusCode = 400
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(statusCode)
				fmt.Fprint(w, `{"success":false,"message":"Bad request message"}`)
				return
			}

			w.WriteHeader(statusCode)
			return
		}

		response := map[string]string{
			"status": "ok",
		}
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	mockLogger := &mockLogger{}
	client := NewAPIClient(context.Background(), mockLogger, server.URL, "")

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "400 error",
			path:           "/error/400",
			expectedStatus: 400,
			expectedError:  "request failed with status (400 Bad Request)",
		},
		{
			name:           "404 error",
			path:           "/error/404",
			expectedStatus: 404,
			expectedError:  "request failed with status (404 Not Found)",
		},
		{
			name:           "500 error",
			path:           "/error/500",
			expectedStatus: 500,
			expectedError:  "request failed with status (500 Internal Server Error)",
		},
		{
			name:           "upgrade required error",
			path:           "/error/422",
			expectedStatus: 422,
			expectedError:  "Validation failed",
		},
		{
			name:           "validation error",
			path:           "/error/validation",
			expectedStatus: 422,
			expectedError:  "Expected string, received number (invalid_type) name",
		},
		{
			name:           "message error",
			path:           "/error/message",
			expectedStatus: 400,
			expectedError:  "Bad request message",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var response map[string]string
			err := client.Do("GET", test.path, nil, &response)

			require.Error(t, err)
			apiErr, ok := err.(*APIError)
			require.True(t, ok)

			assert.Equal(t, test.expectedStatus, apiErr.Status)
			assert.Equal(t, test.expectedError, apiErr.Error())
		})
	}
}

func TestAPIError(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		var apiErr *APIError
		assert.Equal(t, "", apiErr.Error())

		apiErr = &APIError{}
		assert.Equal(t, "", apiErr.Error())
	})

	t.Run("with error", func(t *testing.T) {
		apiErr := NewAPIError(
			"https://api.example.com",
			"GET",
			404,
			"not found",
			fmt.Errorf("resource not found"),
			"trace-123",
		)

		assert.Equal(t, "resource not found", apiErr.Error())
		assert.Equal(t, "https://api.example.com", apiErr.URL)
		assert.Equal(t, "GET", apiErr.Method)
		assert.Equal(t, 404, apiErr.Status)
		assert.Equal(t, "not found", apiErr.Body)
		assert.Equal(t, "trace-123", apiErr.TraceID)
	})
}

func TestUserAgent(t *testing.T) {
	originalVersion := Version
	originalCommit := Commit

	defer func() {
		Version = originalVersion
		Commit = originalCommit
	}()

	t.Run("with version and commit", func(t *testing.T) {
		Version = "1.2.3"
		Commit = "abc123"

		userAgent := UserAgent()
		assert.Contains(t, userAgent, "Agentuity CLI/1.2.3")
		assert.Contains(t, userAgent, "abc123")
	})

	t.Run("with build info", func(t *testing.T) {
		userAgent := UserAgent()
		assert.NotEmpty(t, userAgent)
		assert.Contains(t, userAgent, "Agentuity CLI/")
	})
}

func TestGetURLs(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configFile := tempDir + "/config.json"

	// Save original environment
	originalConfigFile := os.Getenv("AGENTUITY_CONFIG")
	defer os.Setenv("AGENTUITY_CONFIG", originalConfigFile)

	// Set environment to use our test config
	os.Setenv("AGENTUITY_CONFIG", configFile)

	originalTestInside := testInside
	defer func() { testInside = originalTestInside }()

	testInside = false

	tests := []struct {
		name              string
		apiUrl            string
		appUrl            string
		transportUrl      string
		expectedApi       string
		expectedApp       string
		expectedTransport string
	}{
		{
			name:              "production URLs",
			apiUrl:            "https://api.agentuity.com",
			appUrl:            "https://other.example.com",
			transportUrl:      "https://other.transport.com",
			expectedApi:       "https://api.agentuity.com",
			expectedApp:       "https://app.agentuity.com",
			expectedTransport: "https://agentuity.ai",
		},
		{
			name:              "production URLs already correct",
			apiUrl:            "https://api.agentuity.com",
			appUrl:            "https://app.agentuity.com",
			transportUrl:      "https://agentuity.ai",
			expectedApi:       "https://api.agentuity.com",
			expectedApp:       "https://app.agentuity.com",
			expectedTransport: "https://agentuity.ai",
		},
		{
			name:              "dev API with prod app",
			apiUrl:            "https://api.agentuity.dev",
			appUrl:            "https://app.agentuity.com",
			transportUrl:      "https://agentuity.ai",
			expectedApi:       "http://host.docker.internal:3012",
			expectedApp:       "http://host.docker.internal:3000",
			expectedTransport: "http://host.docker.internal:3939",
		},
		{
			name:              "dev URLs already correct",
			apiUrl:            "https://api.agentuity.dev",
			appUrl:            "http://localhost:3000",
			transportUrl:      "http://localhost:3939",
			expectedApi:       "http://host.docker.internal:3012",
			expectedApp:       "http://host.docker.internal:3000",
			expectedTransport: "http://host.docker.internal:3939",
		},
		{
			name:              "custom URLs",
			apiUrl:            "https://custom-api.example.com",
			appUrl:            "https://custom-app.example.com",
			transportUrl:      "https://custom-transport.example.com",
			expectedApi:       "https://custom-api.example.com",
			expectedApp:       "https://custom-app.example.com",
			expectedTransport: "https://custom-transport.example.com",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create config file with test values
			configContent := fmt.Sprintf(`{
				"overrides": {
					"api_url": "%s",
					"app_url": "%s",
					"transport_url": "%s"
				}
			}`, test.apiUrl, test.appUrl, test.transportUrl)

			err := os.WriteFile(configFile, []byte(configContent), 0644)
			require.NoError(t, err)

			// Reset viper config
			viper.Reset()
			viper.SetConfigFile(configFile)
			err = viper.ReadInConfig()
			require.NoError(t, err)

			originalTestInside := testInside
			if test.name == "dev_API_with_prod_app" || test.name == "dev_URLs_already_correct" {
				testInside = true // Simulate container environment
			} else {
				testInside = false
			}
			defer func() { testInside = originalTestInside }()

			mockLogger := &mockLogger{}
			apiUrl, appUrl, transportUrl := GetURLs(mockLogger)

			assert.Equal(t, test.expectedApi, apiUrl)
			assert.Equal(t, test.expectedApp, appUrl)
			assert.Equal(t, test.expectedTransport, transportUrl)
		})
	}

	// Test with container environment
	t.Run("inside container", func(t *testing.T) {
		configContent := `{
			"overrides": {
				"api_url": "https://api.agentuity.dev",
				"app_url": "http://localhost:3000",
				"transport_url": "http://localhost:3939"
			}
		}`

		err := os.WriteFile(configFile, []byte(configContent), 0644)
		require.NoError(t, err)

		viper.Reset()
		viper.SetConfigFile(configFile)
		err = viper.ReadInConfig()
		require.NoError(t, err)

		originalTestInside := testInside
		testInside = true // Simulate container environment
		defer func() { testInside = originalTestInside }()

		mockLogger := &mockLogger{}
		apiUrl, appUrl, transportUrl := GetURLs(mockLogger)

		assert.Equal(t, "http://host.docker.internal:3012", apiUrl)
		assert.Equal(t, "http://host.docker.internal:3000", appUrl)
		assert.Equal(t, "http://host.docker.internal:3939", transportUrl)
	})
}

func TestTryLoggedIn(t *testing.T) {
	tempDir := t.TempDir()
	configFile := tempDir + "/config.json"

	originalConfigFile := os.Getenv("AGENTUITY_CONFIG")
	defer os.Setenv("AGENTUITY_CONFIG", originalConfigFile)

	os.Setenv("AGENTUITY_CONFIG", configFile)

	tests := []struct {
		name           string
		apiKey         string
		userId         string
		expires        int64
		expectLoggedIn bool
	}{
		{
			name:           "logged in",
			apiKey:         "test-api-key",
			userId:         "user-123",
			expires:        time.Now().Add(1 * time.Hour).UnixMilli(),
			expectLoggedIn: true,
		},
		{
			name:           "expired",
			apiKey:         "test-api-key",
			userId:         "user-123",
			expires:        time.Now().Add(-1 * time.Hour).UnixMilli(),
			expectLoggedIn: false,
		},
		{
			name:           "missing api key",
			apiKey:         "",
			userId:         "user-123",
			expires:        time.Now().Add(1 * time.Hour).UnixMilli(),
			expectLoggedIn: false,
		},
		{
			name:           "missing user id",
			apiKey:         "test-api-key",
			userId:         "",
			expires:        time.Now().Add(1 * time.Hour).UnixMilli(),
			expectLoggedIn: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configContent := fmt.Sprintf(`{
				"auth": {
					"api_key": "%s",
					"user_id": "%s",
					"expires": %d
				}
			}`, test.apiKey, test.userId, test.expires)

			err := os.WriteFile(configFile, []byte(configContent), 0644)
			require.NoError(t, err)

			viper.Reset()
			viper.SetConfigFile(configFile)
			err = viper.ReadInConfig()
			require.NoError(t, err)

			apiKey, userId, loggedIn := TryLoggedIn()

			assert.Equal(t, test.expectLoggedIn, loggedIn)
			if test.expectLoggedIn {
				assert.Equal(t, test.apiKey, apiKey)
				assert.Equal(t, test.userId, userId)
			} else {
				if !loggedIn {
					assert.Equal(t, "", apiKey)
					assert.Equal(t, "", userId)
				}
			}
		})
	}
}

type mockLogger struct{}

func (m *mockLogger) Debug(format string, args ...interface{}) {}
func (m *mockLogger) Info(format string, args ...interface{})  {}
func (m *mockLogger) Warn(format string, args ...interface{})  {}
func (m *mockLogger) Error(format string, args ...interface{}) {}
func (m *mockLogger) Fatal(format string, args ...interface{}) {}
func (m *mockLogger) Trace(format string, args ...interface{}) {}
func (m *mockLogger) SetLevel(level string)                    {}
func (m *mockLogger) GetLevel() string                         { return "info" }
func (m *mockLogger) WithField(key string, value interface{}) logger.Logger {
	return m
}
func (m *mockLogger) WithFields(fields map[string]interface{}) logger.Logger {
	return m
}
func (m *mockLogger) WithError(err error) logger.Logger {
	return m
}
func (m *mockLogger) Stack(logger logger.Logger) logger.Logger {
	return m
}
func (m *mockLogger) With(fields map[string]interface{}) logger.Logger {
	return m
}
func (m *mockLogger) WithContext(ctx context.Context) logger.Logger {
	return m
}
func (m *mockLogger) WithPrefix(prefix string) logger.Logger {
	return m
}
