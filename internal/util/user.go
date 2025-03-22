package util

import (
	"os"
	"path/filepath"
	"runtime"
)

// GetAppSupportDir returns the path to the application support directory for the current user.
// It supports Darwin, Windows, and Linux.
// Returns an empty string if the directory cannot be determined.
func GetAppSupportDir(appName string) string {
	var dir string
	// Get the user's home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	// Determine the OS and set the appropriate path
	switch v := runtime.GOOS; v {
	case "darwin":
		dir = filepath.Join(homeDir, "Library", "Application Support", appName)
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(homeDir, "AppData", "Roaming")
		}
		dir = filepath.Join(appData, appName)
	case "linux":
		configDir := os.Getenv("XDG_DATA_HOME")
		if configDir == "" {
			configDir = filepath.Join(homeDir, ".local", "share")
		}
		dir = filepath.Join(configDir, appName)
	default:
		return ""
	}

	return dir
}
