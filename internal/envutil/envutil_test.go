package envutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLooksLikeSecret(t *testing.T) {
	tests := []struct {
		input   string
		matches bool
	}{
		{"API_KEY", true},
		{"SECRET_TOKEN", true},
		{"MY_PASSWORD", true},
		{"CREDENTIALS", true},
		{"sk_test_123", true},
		{"ACCESS_TOKEN", true},
		{"DATABASE_URL", false},
		{"USERNAME", false},
		{"EMAIL", false},
		{"AGENTUITY_SECRET", true}, // should match because of SECRET
		{"SOME_RANDOM_VAR", false},
		{"PRIVATE_KEY", true},
		{"MY_APIKEY", true},
		{"MY_API_KEY", true},
		{"MY_API-KEY", true},
		{"MONKEY", false},

		{"api_key", true},
		{"secret_token", true},
		{"my_password", true},
		{"credentials", true},
		{"sk_test_123", true},
		{"access_token", true},
		{"database_url", false},
		{"username", false},
		{"email", false},
		{"agentuity_secret", true},
		{"some_random_var", false},
	}

	for _, tt := range tests {
		if LooksLikeSecret.MatchString(tt.input) != tt.matches {
			t.Errorf("LooksLikeSecret.MatchString(%q) = %v, want %v", tt.input, LooksLikeSecret.MatchString(tt.input), tt.matches)
		}
	}
}

func TestIsAgentuityEnv(t *testing.T) {
	tests := []struct {
		input   string
		matches bool
	}{
		{"AGENTUITY_API_KEY", true},
		{"AGENTUITY_SECRET", true},
		{"AGENTUITY_TOKEN", true},
		{"AGENTUITY_SOMETHING", true},
		{"SOME_AGENTUITY_VAR", true},
		{"API_KEY", false},
		{"SECRET", false},
		{"DATABASE_URL", false},

		{"agentuity_api_key", true},
		{"agentuity_secret", true},
		{"agentuity_token", true},
		{"agentuity_something", true},
		{"some_agentuity_var", true},
		{"api_key", false},
		{"secret", false},
		{"database_url", false},
	}

	for _, tt := range tests {
		if IsAgentuityEnv.MatchString(tt.input) != tt.matches {
			t.Errorf("IsAgentuityEnv.MatchString(%q) = %v, want %v", tt.input, IsAgentuityEnv.MatchString(tt.input), tt.matches)
		}
	}
}

func TestShouldSyncToProduction(t *testing.T) {
	tests := []struct {
		name       string
		isLocalDev bool
		shouldSync bool
	}{
		{
			name:       "production mode should sync",
			isLocalDev: false,
			shouldSync: true,
		},
		{
			name:       "local development mode should not sync",
			isLocalDev: true,
			shouldSync: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShouldSyncToProduction(tt.isLocalDev)
			if result != tt.shouldSync {
				t.Errorf("ShouldSyncToProduction(%v) = %v, want %v", tt.isLocalDev, result, tt.shouldSync)
			}
		})
	}
}

func TestDetermineEnvFilename(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	tests := []struct {
		name            string
		isLocalDev      bool
		existingFiles   []string
		expectedFile    string
		shouldCreateDev bool
	}{
		{
			name:          "production mode uses .env",
			isLocalDev:    false,
			existingFiles: []string{".env"},
			expectedFile:  ".env",
		},
		{
			name:          "local dev mode uses .env.development when it exists",
			isLocalDev:    true,
			existingFiles: []string{".env", ".env.development"},
			expectedFile:  ".env.development",
		},
		{
			name:            "local dev mode creates .env.development when it doesn't exist",
			isLocalDev:      true,
			existingFiles:   []string{".env"},
			expectedFile:    ".env.development",
			shouldCreateDev: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a subdirectory for this test
			testDir := filepath.Join(tempDir, tt.name)
			err := os.MkdirAll(testDir, 0755)
			if err != nil {
				t.Fatalf("Failed to create test directory: %v", err)
			}

			// Create existing files
			for _, filename := range tt.existingFiles {
				filePath := filepath.Join(testDir, filename)
				err := os.WriteFile(filePath, []byte("TEST_VAR=test_value\n"), 0644)
				if err != nil {
					t.Fatalf("Failed to create test file %s: %v", filename, err)
				}
			}

			result, err := DetermineEnvFilename(testDir, tt.isLocalDev)
			if err != nil {
				t.Fatalf("DetermineEnvFilename() error = %v", err)
			}

			expectedPath := filepath.Join(testDir, tt.expectedFile)
			if result != expectedPath {
				t.Errorf("DetermineEnvFilename() = %v, want %v", result, expectedPath)
			}

			// Check if .env.development was created when expected
			if tt.shouldCreateDev {
				devFile := filepath.Join(testDir, ".env.development")
				if _, err := os.Stat(devFile); os.IsNotExist(err) {
					t.Errorf("Expected .env.development to be created, but it doesn't exist")
				} else {
					// Check content
					content, err := os.ReadFile(devFile)
					if err != nil {
						t.Fatalf("Failed to read created .env.development: %v", err)
					}
					expectedContent := "# This file is used to store development environment variables\n"
					if string(content) != expectedContent {
						t.Errorf("Created .env.development content = %q, want %q", string(content), expectedContent)
					}
				}
			}
		})
	}
}
