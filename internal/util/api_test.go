package util

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentuity/go-common/logger"
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

type mockLogger struct{}

func (m *mockLogger) Debug(format string, args ...interface{})  {}
func (m *mockLogger) Info(format string, args ...interface{})   {}
func (m *mockLogger) Warn(format string, args ...interface{})   {}
func (m *mockLogger) Error(format string, args ...interface{})  {}
func (m *mockLogger) Fatal(format string, args ...interface{})  {}
func (m *mockLogger) Trace(format string, args ...interface{})  {}
func (m *mockLogger) SetLevel(level string)                     {}
func (m *mockLogger) GetLevel() string                          { return "info" }
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
