package util

import (
	"archive/zip"
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

func downloadFile(url, filePath string) error {
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

func verifyChecksum(filePath, expectedChecksum string) (bool, error) {
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

func getChecksumFromFile(checksumFilePath, targetFileName string) (string, error) {
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

func getBinaryName() string {
	binaryName := "agentuity"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	return binaryName
}

func isWindowsMsiInstallation() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(exe), "\\program files\\") || 
	       strings.Contains(strings.ToLower(exe), "\\program files (x86)\\")
}

func getReleaseAssetName(version string) string {
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

func getMsiInstallerName(version string) string {
	arch := runtime.GOARCH
	var msiArch string
	if arch == "amd64" {
		msiArch = "x64"
	} else if arch == "386" {
		msiArch = "x86"
	} else {
		msiArch = arch
	}
	
	return fmt.Sprintf("agentuity-%s.msi", msiArch)
}

func isAdmin() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	
	cmd := exec.Command("powershell", "-Command", "([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	
	return strings.TrimSpace(string(output)) == "True"
}

func UpgradeCLI(ctx context.Context, logger logger.Logger, force bool) error {
	if runtime.GOOS == "darwin" {
		exe, err := os.Executable()
		if err == nil && strings.Contains(exe, "/homebrew/Cellar/agentuity/") {
			logger.Info("Detected Homebrew installation, upgrading via brew")
			return upgradeWithHomebrew(logger)
		}
	}
	
	if isWindowsMsiInstallation() {
		logger.Info("Detected Windows MSI installation, upgrading via MSI")
		
		release, err := GetLatestRelease(ctx)
		if err != nil {
			return fmt.Errorf("failed to get latest release: %w", err)
		}
		
		if Version == release && !force {
			tui.ShowSuccess("You are already on the latest version (%s)", release)
			return nil
		}
		
		return upgradeWithWindowsMsi(ctx, logger, release)
	}
	
	release, err := GetLatestRelease(ctx) // Using public function from version.go
	if err != nil {
		return fmt.Errorf("failed to get latest release: %w", err)
	}
	
	if Version == release && !force {
		tui.ShowSuccess("You are already on the latest version (%s)", release)
		return nil
	}
	
	assetName := getReleaseAssetName(release)
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
	
	var downloadErr error
	downloadAction := func() {
		if err := downloadFile(assetURL, assetPath); err != nil {
			downloadErr = fmt.Errorf("failed to download release asset: %w", err)
		}
	}
	tui.ShowSpinner("Downloading release...", downloadAction)
	if downloadErr != nil {
		return downloadErr
	}
	
	var checksumDownloadErr error
	checksumAction := func() {
		if err := downloadFile(checksumURL, checksumPath); err != nil {
			checksumDownloadErr = fmt.Errorf("failed to download checksum file: %w", err)
		}
	}
	tui.ShowSpinner("Downloading checksum...", checksumAction)
	if checksumDownloadErr != nil {
		return checksumDownloadErr
	}
	
	var checksumErr error
	var checksum string
	var valid bool
	verifyAction := func() {
		var err1, err2 error
		checksum, err1 = getChecksumFromFile(checksumPath, assetName)
		if err1 != nil {
			checksumErr = fmt.Errorf("failed to get checksum: %w", err1)
			return
		}
		
		valid, err2 = verifyChecksum(assetPath, checksum)
		if err2 != nil {
			checksumErr = fmt.Errorf("failed to verify checksum: %w", err2)
		}
	}
	tui.ShowSpinner("Verifying checksum...", verifyAction)
	if checksumErr != nil {
		return checksumErr
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
	
	appDir := GetAppSupportDir("agentuity") // Using public function from user.go
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
	
	var backupErr error
	backupAction := func() {
		if err := copyFile(currentExe, backupPath); err != nil {
			backupErr = fmt.Errorf("failed to create backup: %w", err)
		}
	}
	tui.ShowSpinner("Creating backup...", backupAction)
	if backupErr != nil {
		return backupErr
	}
	
	tempDir, err := os.MkdirTemp("", "agentuity-extract")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)
	
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
	
	var replaceErr error
	replaceAction := func() {
		if err := copyFile(binaryPath, currentExe); err != nil {
			replaceErr = fmt.Errorf("failed to replace binary: %w", err)
			logger.Error("Failed to replace binary: %v", err)
			logger.Info("Attempting to restore from backup")
			copyFile(backupPath, currentExe)
			return
		}
		
		if err := os.Chmod(currentExe, fileMode); err != nil {
			logger.Error("Failed to set file permissions: %v", err)
		}
	}
	tui.ShowSpinner("Replacing binary...", replaceAction)
	if replaceErr != nil {
		return replaceErr
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
	var extractErr error
	var binaryPath string
	
	extractAction := func() {
		binaryName := getBinaryName()
		
		if strings.HasSuffix(assetPath, ".zip") {
			reader, err := zip.OpenReader(assetPath)
			if err != nil {
				extractErr = fmt.Errorf("failed to open zip file: %w", err)
				return
			}
			defer reader.Close()
			
			for _, file := range reader.File {
				path := filepath.Join(extractDir, file.Name)
				
				if !strings.HasPrefix(path, filepath.Clean(extractDir)+string(os.PathSeparator)) {
					continue
				}
				
				if file.FileInfo().IsDir() {
					os.MkdirAll(path, file.Mode())
					continue
				}
				
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					extractErr = fmt.Errorf("failed to create directory: %w", err)
					return
				}
				
				outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
				if err != nil {
					extractErr = fmt.Errorf("failed to create file: %w", err)
					return
				}
				
				inFile, err := file.Open()
				if err != nil {
					outFile.Close()
					extractErr = fmt.Errorf("failed to open file in archive: %w", err)
					return
				}
				
				_, err = io.Copy(outFile, inFile)
				outFile.Close()
				inFile.Close()
				if err != nil {
					extractErr = fmt.Errorf("failed to copy file: %w", err)
					return
				}
				
				if filepath.Base(path) == binaryName {
					binaryPath = path
				}
			}
		} else {
			cmd := exec.Command("tar", "-xzf", assetPath, "-C", extractDir)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			
			if err := cmd.Run(); err != nil {
				extractErr = fmt.Errorf("failed to extract archive: %w", err)
				return
			}
			
			err := filepath.Walk(extractDir, func(path string, info os.FileInfo, err error) error {
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
				extractErr = fmt.Errorf("failed to find binary in extracted archive: %w", err)
				return
			}
		}
		
		if binaryPath == "" {
			extractErr = fmt.Errorf("binary not found in extracted archive")
			return
		}
		
		if runtime.GOOS != "windows" {
			os.Chmod(binaryPath, 0755)
		}
	}
	
	tui.ShowSpinner("Extracting release archive...", extractAction)
	
	if extractErr != nil {
		logger.Error("%v", extractErr)
		return ""
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

func upgradeWithWindowsMsi(ctx context.Context, logger logger.Logger, version string) error {
	if !isAdmin() {
		return fmt.Errorf("administrator privileges required to upgrade MSI installation. Please run the command as administrator")
	}
	
	tempDir, err := os.MkdirTemp("", "agentuity-upgrade-msi")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)
	
	installerName := getMsiInstallerName(version)
	installerURL := fmt.Sprintf("https://github.com/agentuity/cli/releases/download/%s/%s", version, installerName)
	installerPath := filepath.Join(tempDir, installerName)
	
	var downloadErr error
	downloadAction := func() {
		if err := downloadFile(installerURL, installerPath); err != nil {
			downloadErr = fmt.Errorf("failed to download MSI installer: %w", err)
		}
	}
	tui.ShowSpinner("Downloading MSI installer...", downloadAction)
	if downloadErr != nil {
		return downloadErr
	}
	
	exe, _ := os.Executable()
	exePath := filepath.Dir(exe)
	productCodePath := filepath.Join(exePath, "product_code.txt")
	
	if _, err := os.Stat(productCodePath); err == nil {
		productCodeBytes, err := os.ReadFile(productCodePath)
		if err == nil {
			productCode := strings.TrimSpace(string(productCodeBytes))
			if productCode != "" {
				logger.Info("Uninstalling existing installation")
				uninstallCmd := exec.Command("msiexec", "/x", productCode, "/qn")
				
				uninstallAction := func() {
					if err := uninstallCmd.Run(); err != nil {
						logger.Warn("Failed to uninstall existing installation: %v", err)
					}
				}
				tui.ShowSpinner("Uninstalling previous version...", uninstallAction)
			}
		}
	}
	
	logger.Info("Installing new version")
	cmd := exec.Command("msiexec", "/i", installerPath, "/qn", "REINSTALLMODE=amus", "REINSTALL=ALL")
	
	var installErr error
	installAction := func() {
		if err := cmd.Run(); err != nil {
			installErr = fmt.Errorf("failed to run MSI installer: %w", err)
		}
	}
	tui.ShowSpinner("Installing upgrade...", installAction)
	if installErr != nil {
		return installErr
	}
	
	tui.ShowSuccess("Successfully upgraded to version %s", version)
	return nil
}
