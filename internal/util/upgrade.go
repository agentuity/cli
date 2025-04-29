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

func getReleaseAssetName() string {
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

func getMsiInstallerName() string {
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

func isAdmin(ctx context.Context) bool {
	if runtime.GOOS != "windows" {
		return false
	}

	cmd := exec.CommandContext(ctx, "powershell", "-Command", "([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	return strings.TrimSpace(string(output)) == "True"
}

func UpgradeCLI(ctx context.Context, logger logger.Logger, force bool) error {
	if runtime.GOOS == "darwin" {
		exe, err := os.Executable()
		if err == nil {
			if strings.Contains(exe, "/usr/local/Cellar/agentuity/") ||
				strings.Contains(exe, "/opt/homebrew/Cellar/agentuity/") ||
				strings.Contains(exe, "/homebrew/Cellar/agentuity/") {
				logger.Debug("Detected Homebrew installation, upgrading via brew")
				return upgradeWithHomebrew(ctx, logger)
			}

			if strings.Contains(exe, "/usr/local/bin/agentuity") ||
				strings.Contains(exe, "/opt/homebrew/bin/agentuity") {
				logger.Debug("Detected Homebrew symlink, upgrading via brew")
				return upgradeWithHomebrew(ctx, logger)
			}
		}
	}

	if isWindowsMsiInstallation() {
		release, err := GetLatestRelease(ctx)
		if err != nil {
			return fmt.Errorf("failed to get latest release: %w", err)
		}

		if Version == release && !force {
			tui.ShowSuccess("You are already on the latest version (%s)", release)
			return nil
		}

		return upgradeWithWindowsMsi(ctx, release)
	}

	release, err := GetLatestRelease(ctx) // Using public function from version.go
	if err != nil {
		return fmt.Errorf("failed to get latest release: %w", err)
	}

	if Version == release && !force {
		tui.ShowSuccess("You are already on the latest version (%s)", release)
		return nil
	}

	assetName := getReleaseAssetName()
	checksumFileName := "checksums.txt"

	tempDir, err := os.MkdirTemp("", "agentuity-upgrade")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	assetURL := fmt.Sprintf("https://github.com/agentuity/cli/releases/download/v%s/%s", release, assetName)
	checksumURL := fmt.Sprintf("https://github.com/agentuity/cli/releases/download/v%s/%s", release, checksumFileName)

	assetPath := filepath.Join(tempDir, assetName)
	checksumPath := filepath.Join(tempDir, checksumFileName)

	var downloadErr error
	downloadAction := func() {
		if err := downloadFile(assetURL, assetPath); err != nil {
			downloadErr = fmt.Errorf("failed to download release asset: %w", err)
		}
	}
	tui.ShowSpinner(fmt.Sprintf("Downloading %s...", release), downloadAction)
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

	return replaceBinary(ctx, logger, assetPath, release)
}

func replaceBinary(ctx context.Context, logger logger.Logger, assetPath, version string) error {
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

	binaryPath := extractBinary(ctx, logger, assetPath, tempDir)
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

	tui.ShowSuccess("Successfully upgraded to %s", version)
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

func extractBinary(ctx context.Context, logger logger.Logger, assetPath, extractDir string) string {
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
			cmd := exec.CommandContext(ctx, "tar", "-xzf", assetPath, "-C", extractDir)
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

func upgradeWithHomebrew(ctx context.Context, logger logger.Logger) error {
	release, rerr := GetLatestRelease(ctx)
	if rerr != nil {
		return fmt.Errorf("failed to get latest release: %w", rerr)
	}

	exe, _ := os.Executable()
	v, _ := exec.CommandContext(ctx, exe, "version").Output()
	currentVersion := strings.TrimSpace(string(v))

	if currentVersion == release {
		tui.ShowSuccess("You are already on the latest version (%s)", currentVersion)
		return nil
	}

	var newVersion string
	var err error

	action := func() {
		logger.Debug("Updating Homebrew")
		updateCmd := exec.CommandContext(ctx, "brew", "update")
		updateCmd.Stdout = os.Stdout
		updateCmd.Stderr = os.Stderr
		if lerr := updateCmd.Run(); lerr != nil {
			err = fmt.Errorf("failed to update Homebrew: %w", lerr)
			return
		}

		logger.Debug("Upgrading agentuity")
		upgradeCmd := exec.CommandContext(ctx, "brew", "upgrade", "agentuity")
		upgradeCmd.Stdout = os.Stdout
		upgradeCmd.Stderr = os.Stderr
		if lerr := upgradeCmd.Run(); lerr != nil {
			err = fmt.Errorf("failed to upgrade via Homebrew: %w", lerr)
			return
		}

		v, _ = exec.CommandContext(ctx, exe, "version").Output()
		newVersion = strings.TrimSpace(string(v))
	}

	action()

	if err != nil {
		return err
	}

	tui.ShowSuccess("Successfully upgraded to version %s via Homebrew", newVersion)
	return nil
}

func upgradeWithWindowsMsi(ctx context.Context, version string) error {
	if os.Getenv("CI") != "" || os.Getenv("GITHUB_ACTIONS") != "" || os.Getenv("NONINTERACTIVE") != "" {
		tui.ShowWarning("Non-interactive environment detected, skipping automatic MSI installation")

		tempDir, err := os.MkdirTemp("", "agentuity-upgrade-msi")
		if err != nil {
			return fmt.Errorf("failed to create temp directory: %w", err)
		}
		defer os.RemoveAll(tempDir)

		installerName := getMsiInstallerName()
		if strings.HasPrefix(version, "v") {
			version = strings.TrimPrefix(version, "v")
		}
		installerURL := fmt.Sprintf("https://github.com/agentuity/cli/releases/download/v%s/%s", version, installerName)
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

		homePath := os.Getenv("HOME")
		if homePath == "" {
			homePath = os.Getenv("USERPROFILE")
		}
		if homePath == "" {
			return fmt.Errorf("unable to determine home directory")
		}

		destPath := filepath.Join(homePath, installerName)
		if err := copyFile(installerPath, destPath); err != nil {
			return fmt.Errorf("failed to copy MSI to %s: %w", destPath, err)
		}

		tui.ShowSuccess("MSI installer copied to %s", destPath)
		tui.ShowWarning("To install manually, run the MSI installer at: %s", destPath)
		return nil
	}

	if !isAdmin(ctx) {
		tui.ShowWarning("Administrator privileges required to upgrade the CLI on Windows")
		tui.ShowWarning("Please restart the CLI with administrator privileges and try again")
		return fmt.Errorf("administrator privileges required")
	}

	tempDir, err := os.MkdirTemp("", "agentuity-upgrade-msi")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	var uninstallErr error
	uninstallAction := func() {
		uninstallScriptPath := filepath.Join(tempDir, "uninstall.ps1")
		uninstallScript := `
$products = Get-CimInstance -Class Win32_Product | Where-Object { $_.Name -like "*Agentuity*" }
if ($products) {
    foreach ($product in $products) {
        Write-Output "Uninstalling: $($product.Name) ($($product.IdentifyingNumber))"
        $result = $product | Invoke-CimMethod -MethodName Uninstall
        if ($result.ReturnValue -eq 0) {
            Write-Output "Successfully uninstalled $($product.Name)"
        } else {
            Write-Output "Failed to uninstall $($product.Name) with return code $($result.ReturnValue)"
            exit 1
        }
    }
    exit 0
} else {
    Write-Output "No existing Agentuity installation found"
    exit 0
}
`
		if err := os.WriteFile(uninstallScriptPath, []byte(uninstallScript), 0644); err != nil {
			uninstallErr = fmt.Errorf("failed to create uninstall script: %w", err)
			return
		}

		uninstallCmd := exec.CommandContext(ctx, "powershell", "-ExecutionPolicy", "Bypass", "-File", uninstallScriptPath)
		output, err := uninstallCmd.CombinedOutput()
		if err != nil {
			uninstallErr = fmt.Errorf("failed to run uninstall script: %w, output: %s", err, string(output))
			return
		}
		tui.ShowSuccess("Uninstall step completed: %s", strings.TrimSpace(string(output)))
	}
	tui.ShowSpinner("Checking for existing installations...", uninstallAction)
	if uninstallErr != nil {
		tui.ShowWarning("Uninstall failed, continuing with installation: %v", uninstallErr)
	}

	installerName := getMsiInstallerName()
	if strings.HasPrefix(version, "v") {
		version = strings.TrimPrefix(version, "v")
	}
	installerURL := fmt.Sprintf("https://github.com/agentuity/cli/releases/download/v%s/%s", version, installerName)
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

	var installErr error
	installAction := func() {
		installCmd := exec.CommandContext(ctx, "msiexec", "/i", installerPath, "/qn", "/norestart")
		if err := installCmd.Run(); err != nil {
			tui.ShowWarning("Direct install failed, trying with update approach: %v", err)

			updateCmd := exec.CommandContext(ctx, "msiexec", "/update", installerPath, "/qn")
			if err := updateCmd.Run(); err != nil {
				tui.ShowWarning("Update approach failed, trying install with reinstall: %v", err)

				reinstallCmd := exec.CommandContext(ctx, "msiexec", "/i", installerPath, "/qn", "REINSTALLMODE=amus", "REINSTALL=ALL")
				if err := reinstallCmd.Run(); err != nil {
					installErr = fmt.Errorf("failed to run MSI installer: %w", err)
				}
			}
		}
	}
	tui.ShowSpinner("Installing upgrade...", installAction)
	if installErr != nil {
		tui.ShowWarning("Automatic installation failed: %v", installErr)

		homePath := os.Getenv("HOME")
		if homePath == "" {
			homePath = os.Getenv("USERPROFILE")
		}
		if homePath == "" {
			return fmt.Errorf("unable to determine home directory: %w", installErr)
		}

		destPath := filepath.Join(homePath, installerName)
		if err := copyFile(installerPath, destPath); err != nil {
			return fmt.Errorf("failed to copy MSI to %s: %w", destPath, err)
		}

		tui.ShowSuccess("MSI installer copied to %s", destPath)
		tui.ShowWarning("To install manually, run the MSI installer at: %s", destPath)
		return nil
	}

	tui.ShowSuccess("Successfully upgraded to version %s", version)
	return nil
}
