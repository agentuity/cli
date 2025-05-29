package envutil

import (
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
