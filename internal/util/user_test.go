package util

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetAppSupportDir(t *testing.T) {
	originalHome := os.Getenv("HOME")
	originalAppData := os.Getenv("APPDATA")
	originalXdgData := os.Getenv("XDG_DATA_HOME")

	defer func() {
		os.Setenv("HOME", originalHome)
		os.Setenv("APPDATA", originalAppData)
		os.Setenv("XDG_DATA_HOME", originalXdgData)
	}()

	testHome := "/test/home"
	testAppData := "/test/appdata"
	testXdgData := "/test/xdg"
	appName := "testapp"

	tests := []struct {
		name     string
		goos     string
		home     string
		appData  string
		xdgData  string
		expected string
	}{
		{
			name:     "darwin",
			goos:     "darwin",
			home:     testHome,
			expected: filepath.Join(testHome, "Library", "Application Support", appName),
		},
		{
			name:     "windows with APPDATA",
			goos:     "windows",
			home:     testHome,
			appData:  testAppData,
			expected: filepath.Join(testAppData, appName),
		},
		{
			name:     "windows without APPDATA",
			goos:     "windows",
			home:     testHome,
			expected: filepath.Join(testHome, "AppData", "Roaming", appName),
		},
		{
			name:     "linux with XDG_DATA_HOME",
			goos:     "linux",
			home:     testHome,
			xdgData:  testXdgData,
			expected: filepath.Join(testXdgData, appName),
		},
		{
			name:     "linux without XDG_DATA_HOME",
			goos:     "linux",
			home:     testHome,
			expected: filepath.Join(testHome, ".local", "share", appName),
		},
		{
			name:     "unsupported OS",
			goos:     "unsupported",
			home:     testHome,
			expected: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.goos != runtime.GOOS && test.goos != "unsupported" {
				t.Skip("Skipping test for different OS")
			}

			os.Setenv("HOME", test.home)
			if test.appData != "" {
				os.Setenv("APPDATA", test.appData)
			} else {
				os.Unsetenv("APPDATA")
			}
			if test.xdgData != "" {
				os.Setenv("XDG_DATA_HOME", test.xdgData)
			} else {
				os.Unsetenv("XDG_DATA_HOME")
			}

			result := GetAppSupportDir(appName)

			if test.goos == runtime.GOOS {
				if test.goos == "unsupported" {
					assert.Equal(t, "", result)
				} else {
					assert.NotEqual(t, "", result)
					assert.Contains(t, result, appName)
				}
			}
		})
	}
}
