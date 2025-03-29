package util

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/agentuity/go-common/logger"
	"github.com/agentuity/go-common/tui"
)

func DownloadFile(url, filePath string) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	out, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to save file: %w", err)
	}

	return nil
}

func VerifyChecksum(filePath, expectedChecksum string) (bool, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return false, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false, fmt.Errorf("failed to calculate checksum: %w", err)
	}

	actualChecksum := hex.EncodeToString(hash.Sum(nil))
	return actualChecksum == expectedChecksum, nil
}

func GetChecksumFromFile(checksumFilePath, targetFileName string) (string, error) {
	data, err := os.ReadFile(checksumFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to read checksum file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) == 2 && strings.Contains(parts[1], targetFileName) {
			return parts[0], nil
		}
	}

	return "", fmt.Errorf("checksum not found for %s", targetFileName)
}

func GetBinaryName() string {
	binaryName := "agentuity"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	return binaryName
}

func GetReleaseAssetName(version string) string {
	goos := runtime.GOOS
	arch := runtime.GOARCH
	
	var archName string
	if arch == "amd64" {
		archName = "x86_64"
	} else if arch == "386" {
		archName = "i386"
	} else {
		archName = arch
	}
	
	extension := "tar.gz"
	if goos == "windows" {
		extension = "zip"
	}
	
	return fmt.Sprintf("agentuity_%s_%s.%s", strings.Title(goos), archName, extension)
}

func UpgradeCLI(ctx context.Context, logger logger.Logger, force bool) error {
	if runtime.GOOS == "darwin" {
		exe, err := os.Executable()
		if err == nil && strings.Contains(exe, "/homebrew/Cellar/agentuity/") {
			logger.Info("Detected Homebrew installation, upgrading via brew")
			return upgradeWithHomebrew(logger)
		}
	}
	
	release, err := GetLatestRelease(ctx)
	if err != nil {
		return fmt.Errorf("failed to get latest release: %w", err)
	}
	
	if Version == release && !force {
		tui.ShowSuccess("You are already on the latest version (%s)", release)
		return nil
	}
	
	assetName := GetReleaseAssetName(release)
	checksumFileName := "checksums.txt"
	
	tempDir, err := os.MkdirTemp("", "agentuity-upgrade")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)
	
	assetURL := fmt.Sprintf("https://github.com/agentuity/cli/releases/download/%s/%s", release, assetName)
	checksumURL := fmt.Sprintf("https://github.com/agentuity/cli/releases/download/%s/%s", release, checksumFileName)
	
	assetPath := filepath.Join(tempDir, assetName)
	checksumPath := filepath.Join(tempDir, checksumFileName)
	
	logger.Info("Downloading %s", assetName)
	if err := DownloadFile(assetURL, assetPath); err != nil {
		return fmt.Errorf("failed to download release asset: %w", err)
	}
	
	logger.Info("Downloading %s", checksumFileName)
	if err := DownloadFile(checksumURL, checksumPath); err != nil {
		return fmt.Errorf("failed to download checksum file: %w", err)
	}
	
	logger.Info("Verifying checksum")
	checksum, err := GetChecksumFromFile(checksumPath, assetName)
	if err != nil {
		return fmt.Errorf("failed to get checksum: %w", err)
	}
	
	valid, err := VerifyChecksum(assetPath, checksum)
	if err != nil {
		return fmt.Errorf("failed to verify checksum: %w", err)
	}
	
	if !valid {
		return fmt.Errorf("checksum verification failed")
	}
	
	return replaceBinary(logger, assetPath, release)
}

func replaceBinary(logger logger.Logger, assetPath, version string) error {
	currentExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get current executable path: %w", err)
	}
	
	appDir := GetAppSupportDir("agentuity")
	if appDir == "" {
		return fmt.Errorf("failed to get app support directory")
	}
	
	backupDir := filepath.Join(appDir, "backup")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}
	
	backupPath := filepath.Join(backupDir, fmt.Sprintf("agentuity_%s", Version))
	if runtime.GOOS == "windows" {
		backupPath += ".exe"
	}
	
	logger.Info("Creating backup at %s", backupPath)
	if err := copyFile(currentExe, backupPath); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}
	
	tempDir, err := os.MkdirTemp("", "agentuity-extract")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)
	
	logger.Info("Extracting new binary")
	binaryPath := extractBinary(logger, assetPath, tempDir)
	if binaryPath == "" {
		return fmt.Errorf("failed to extract binary")
	}
	
	if err := checkWritePermission(currentExe); err != nil {
		return fmt.Errorf("insufficient permissions to update binary: %w", err)
	}
	
	info, err := os.Stat(currentExe)
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}
	fileMode := info.Mode()
	
	logger.Info("Replacing binary at %s", currentExe)
	if err := copyFile(binaryPath, currentExe); err != nil {
		logger.Error("Failed to replace binary: %v", err)
		logger.Info("Attempting to restore from backup")
		copyFile(backupPath, currentExe)
		return fmt.Errorf("failed to replace binary: %w", err)
	}
	
	if err := os.Chmod(currentExe, fileMode); err != nil {
		logger.Error("Failed to set file permissions: %v", err)
	}
	
	tui.ShowSuccess("Successfully upgraded to version %s", version)
	return nil
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	
	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()
	
	_, err = io.Copy(destFile, sourceFile)
	if err != nil {
		return err
	}
	
	return destFile.Sync()
}

func checkWritePermission(filePath string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	
	file, err := os.OpenFile(filePath, os.O_WRONLY, info.Mode())
	if err != nil {
		return err
	}
	file.Close()
	
	return nil
}

func extractBinary(logger logger.Logger, assetPath, extractDir string) string {
	var cmd *exec.Cmd
	binaryName := GetBinaryName()
	
	if strings.HasSuffix(assetPath, ".zip") {
		cmd = exec.Command("unzip", "-o", assetPath, "-d", extractDir)
	} else {
		cmd = exec.Command("tar", "-xzf", assetPath, "-C", extractDir)
	}
	
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	logger.Info("Running: %s", cmd.String())
	err := cmd.Run()
	if err != nil {
		logger.Error("Failed to extract archive: %v", err)
		return ""
	}
	
	var binaryPath string
	err = filepath.Walk(extractDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Base(path) == binaryName {
			binaryPath = path
			return filepath.SkipDir
		}
		return nil
	})
	
	if err != nil {
		logger.Error("Failed to find binary in extracted archive: %v", err)
		return ""
	}
	
	if binaryPath == "" {
		logger.Error("Binary not found in extracted archive")
		return ""
	}
	
	if runtime.GOOS != "windows" {
		os.Chmod(binaryPath, 0755)
	}
	
	return binaryPath
}

func upgradeWithHomebrew(logger logger.Logger) error {
	logger.Info("Updating Homebrew")
	updateCmd := exec.Command("brew", "update")
	updateCmd.Stdout = os.Stdout
	updateCmd.Stderr = os.Stderr
	if err := updateCmd.Run(); err != nil {
		return fmt.Errorf("failed to update Homebrew: %w", err)
	}
	
	logger.Info("Upgrading agentuity")
	upgradeCmd := exec.Command("brew", "upgrade", "agentuity")
	upgradeCmd.Stdout = os.Stdout
	upgradeCmd.Stderr = os.Stderr
	if err := upgradeCmd.Run(); err != nil {
		return fmt.Errorf("failed to upgrade via Homebrew: %w", err)
	}
	
	exe, _ := os.Executable()
	v, _ := exec.Command(exe, "version").Output()
	version := strings.TrimSpace(string(v))
	
	tui.ShowSuccess("Successfully upgraded to version %s via Homebrew", version)
	return nil
}
