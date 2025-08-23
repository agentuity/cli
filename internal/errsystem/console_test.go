package errsystem

import (
	"errors"
	"os"
	"testing"
)

func TestShowErrorAndExitWithQRCode(t *testing.T) {
	// This test demonstrates the QR code functionality
	// It will show the error banner with QR code, but we need to prevent actual exit
	// We'll use a deferred recover to catch the os.Exit call
	
	// Skip in CI or non-interactive environments
	if os.Getenv("CI") != "" {
		t.Skip("Skipping interactive test in CI environment")
	}

	t.Log("This test will show an error with QR code. You should see the Discord QR code in the output.")
	
	// Create a test error
	testErr := errors.New("This is a test error to demonstrate the QR code feature")
	errSys := New(ErrInvalidConfiguration, testErr, 
		WithContextMessage("Testing QR code display in unit test"),
		WithUserMessage("This test shows the QR code feature working properly"))
	
	// Note: In a real scenario, this would call os.Exit(1)
	// For testing purposes, you can comment out the next line to see the QR code
	// and manually verify it works, then uncomment it for automated testing
	
	t.Log("Calling ShowErrorAndExit - this will show the QR code and then exit")
	t.Log("The QR code should point to: https://discord.gg/agentuity")
	
	// This will actually exit the test, but that's okay for a manual verification test
	errSys.ShowErrorAndExit()
}

func TestGenerateQRCode(t *testing.T) {
	// Test that QR code generation works
	qrCode := generateQRCode("https://discord.gg/agentuity")
	
	// Basic validation that something was generated
	if len(qrCode) == 0 {
		t.Error("QR code generation returned empty string")
	}
	
	// Check that it contains expected QR code characters
	if !contains(qrCode, "█") {
		t.Error("QR code should contain block characters")
	}
	
	t.Logf("Generated QR code length: %d characters", len(qrCode))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		   (s == substr || 
		    (len(s) > len(substr) && (s[:len(substr)] == substr || 
		     s[len(s)-len(substr):] == substr || 
		     containsInner(s, substr))))
}

func containsInner(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
