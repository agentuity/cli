<#
.SYNOPSIS
    Installer script for Agentuity CLI on Windows.
.DESCRIPTION
    Downloads and installs the latest version of the Agentuity CLI for Windows.
    This script can be invoked directly with:
    iwr https://get.agentuity.sh/install.ps1 -useb | iex
.PARAMETER Version
    Specific version to install. If not specified, installs the latest version.
.PARAMETER InstallDir
    Custom installation directory. If not specified, uses Program Files.
.PARAMETER NoPrompt
    Skip confirmation prompts. Useful for automated installations.
.EXAMPLE
    .\install.ps1
    Installs the latest version of Agentuity CLI.
.EXAMPLE
    .\install.ps1 -Version 1.2.3
    Installs version 1.2.3 of Agentuity CLI.
.EXAMPLE
    .\install.ps1 -InstallDir "C:\Tools"
    Installs the latest version of Agentuity CLI to C:\Tools.
.NOTES
    Author: Agentuity, Inc.
    Website: https://agentuity.dev
#>

[CmdletBinding()]
param (
    [string]$Version = "latest",
    [string]$InstallDir = "",
    [switch]$NoPrompt = $false
)

# Script version
$ScriptVersion = "1.0.0"

#region Helper Functions

function Write-ColorOutput {
    param (
        [Parameter(Mandatory = $true)]
        [string]$Message,
        
        [Parameter(Mandatory = $false)]
        [string]$ForegroundColor = "White"
    )
    
    $originalColor = $host.UI.RawUI.ForegroundColor
    $host.UI.RawUI.ForegroundColor = $ForegroundColor
    Write-Output $Message
    $host.UI.RawUI.ForegroundColor = $originalColor
}

function Write-Step {
    param (
        [Parameter(Mandatory = $true)]
        [string]$Message
    )
    
    Write-ColorOutput "==> $Message" -ForegroundColor Cyan
}

function Write-Success {
    param (
        [Parameter(Mandatory = $true)]
        [string]$Message
    )
    
    Write-ColorOutput $Message -ForegroundColor Green
}

function Write-Warning {
    param (
        [Parameter(Mandatory = $true)]
        [string]$Message
    )
    
    Write-ColorOutput "Warning: $Message" -ForegroundColor Yellow
}

function Write-Error {
    param (
        [Parameter(Mandatory = $true)]
        [string]$Message,
        
        [Parameter(Mandatory = $false)]
        [switch]$Exit = $false
    )
    
    Write-ColorOutput "Error: $Message" -ForegroundColor Red
    
    if ($Exit) {
        exit 1
    }
}

function Write-Url {
    param (
        [Parameter(Mandatory = $true)]
        [string]$Url
    )
    
    Write-ColorOutput $Url -ForegroundColor Blue
}

function Test-Administrator {
    $currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
    return $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Request-AdminPrivileges {
    param (
        [Parameter(Mandatory = $true)]
        [string]$ScriptPath
    )
    
    Write-Step "Requesting administrator privileges..."
    
    $arguments = "-NoProfile -ExecutionPolicy Bypass -File `"$ScriptPath`""
    if ($Version -ne "latest") {
        $arguments += " -Version `"$Version`""
    }
    if ($InstallDir -ne "") {
        $arguments += " -InstallDir `"$InstallDir`""
    }
    if ($NoPrompt) {
        $arguments += " -NoPrompt"
    }
    
    try {
        Start-Process -FilePath PowerShell.exe -ArgumentList $arguments -Verb RunAs -Wait
        exit 0
    }
    catch {
        Write-Error "Failed to restart with administrator privileges: $_" -Exit
    }
}

function Get-UserConfirmation {
    param (
        [Parameter(Mandatory = $true)]
        [string]$Message,
        
        [Parameter(Mandatory = $false)]
        [bool]$DefaultToYes = $true
    )
    
    if ($NoPrompt) {
        return $true
    }
    
    $choices = if ($DefaultToYes) { "&Yes (default)|&No" } else { "&Yes|&No (default)" }
    $defaultChoice = if ($DefaultToYes) { 0 } else { 1 }
    
    $decision = $Host.UI.PromptForChoice("", $Message, $choices.Split('|'), $defaultChoice)
    
    return $decision -eq 0
}

function Get-LatestReleaseVersion {
    Write-Step "Fetching latest release information..."
    
    try {
        # Set TLS 1.2 for compatibility with GitHub
        [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
        
        $userAgent = "AgentuityInstaller/PowerShell/$ScriptVersion ($env:OS; $env:PROCESSOR_ARCHITECTURE)"
        $headers = @{
            "Accept" = "application/vnd.github+json"
            "X-GitHub-Api-Version" = "2022-11-28"
            "User-Agent" = $userAgent
        }
        
        # Add GitHub token if available (for CI environments)
        if ($env:GITHUB_TOKEN) {
            Write-Step "Using authenticated GitHub API request"
            $headers["Authorization"] = "token $env:GITHUB_TOKEN"
        }
        
        # For CI testing, if a specific version is set in the environment, use it
        if ($env:AGENTUITY_TEST_VERSION) {
            Write-Step "Using test version from environment: $env:AGENTUITY_TEST_VERSION"
            return $env:AGENTUITY_TEST_VERSION
        }
        
        $releaseUrl = "https://api.github.com/repos/agentuity/cli/releases/latest"
        $response = Invoke-RestMethod -Uri $releaseUrl -Headers $headers -Method Get
        
        if ($null -eq $response.tag_name) {
            Write-Error "Failed to parse version from GitHub API response" -Exit
        }
        
        return $response.tag_name
    }
    catch {
        # In CI environment, if API call fails, use a fallback version for testing
        if ($env:CI -eq "true") {
            $fallbackVersion = "0.0.74"
            Write-Warning "GitHub API request failed in CI environment. Using fallback version: $fallbackVersion"
            return $fallbackVersion
        }
        
        Write-Error "Failed to fetch latest release information: $_" -Exit
    }
}

function Get-Architecture {
    $arch = $env:PROCESSOR_ARCHITECTURE
    
    if ($arch -eq "AMD64") {
        return "x64"
    }
    elseif ($arch -eq "ARM64") {
        return "arm64"
    }
    elseif ($arch -eq "X86") {
        return "x86"
    }
    else {
        Write-Warning "Unknown architecture: $arch. Defaulting to x86."
        return "x86"
    }
}

function Get-DefaultInstallDir {
    if ([string]::IsNullOrEmpty($InstallDir)) {
        if (Test-Administrator) {
            return [System.IO.Path]::Combine($env:ProgramFiles, "Agentuity")
        }
        else {
            return [System.IO.Path]::Combine($env:LOCALAPPDATA, "Agentuity")
        }
    }
    else {
        return $InstallDir
    }
}

function Test-PathInEnvironment {
    param (
        [Parameter(Mandatory = $true)]
        [string]$PathToCheck
    )
    
    $envPaths = $env:PATH -split ';'
    return $envPaths -contains $PathToCheck
}

function Add-ToPath {
    param (
        [Parameter(Mandatory = $true)]
        [string]$PathToAdd
    )
    
    if (Test-PathInEnvironment -PathToCheck $PathToAdd) {
        Write-Step "$PathToAdd is already in PATH"
        return
    }
    
    Write-Step "Adding $PathToAdd to PATH environment variable..."
    
    try {
        if (Test-Administrator) {
            # System-wide PATH update (requires admin)
            $systemPath = [Environment]::GetEnvironmentVariable("PATH", [EnvironmentVariableTarget]::Machine)
            $newSystemPath = "$systemPath;$PathToAdd"
            [Environment]::SetEnvironmentVariable("PATH", $newSystemPath, [EnvironmentVariableTarget]::Machine)
            Write-Success "Added to system PATH"
            
            # Also update current session
            $env:PATH = "$env:PATH;$PathToAdd"
        }
        else {
            # User PATH update (doesn't require admin)
            $userPath = [Environment]::GetEnvironmentVariable("PATH", [EnvironmentVariableTarget]::User)
            $newUserPath = "$userPath;$PathToAdd"
            [Environment]::SetEnvironmentVariable("PATH", $newUserPath, [EnvironmentVariableTarget]::User)
            Write-Success "Added to user PATH"
            
            # Also update current session
            $env:PATH = "$env:PATH;$PathToAdd"
        }
    }
    catch {
        Write-Error "Failed to update PATH: $_"
        Write-Warning "You may need to manually add $PathToAdd to your PATH environment variable."
    }
}

function Get-FileHash256 {
    param (
        [Parameter(Mandatory = $true)]
        [string]$FilePath
    )
    
    try {
        $hash = Get-FileHash -Path $FilePath -Algorithm SHA256
        return $hash.Hash.ToLower()
    }
    catch {
        Write-Warning "Failed to compute file hash: $_"
        return $null
    }
}

function Install-MSI {
    param (
        [Parameter(Mandatory = $true)]
        [string]$MsiPath,
        
        [Parameter(Mandatory = $false)]
        [string]$LogPath = "$env:TEMP\agentuity_install.log"
    )
    
    Write-Step "Installing Agentuity CLI..."
    
    try {
        $arguments = "/i `"$MsiPath`" /qn /norestart /log `"$LogPath`" ALLUSERS=1"
        $process = Start-Process -FilePath "msiexec.exe" -ArgumentList $arguments -Wait -PassThru
        
        if ($process.ExitCode -ne 0) {
            Write-Error "Installation failed with exit code $($process.ExitCode). Check the log at $LogPath for details."
            
            # Provide more specific guidance based on common MSI error codes
            switch ($process.ExitCode) {
                1601 { Write-Warning "The Windows Installer service could not be accessed." }
                1602 { Write-Warning "User cancelled installation." }
                1603 { Write-Warning "Fatal error during installation. Check the log for details." }
                1618 { Write-Warning "Another installation is already in progress." }
                1619 { Write-Warning "Installation package could not be opened." }
                1620 { Write-Warning "Installation package could not be opened." }
                1622 { Write-Warning "Error opening installation log file." }
                1623 { Write-Warning "Language of this installation package is not supported by your system." }
                1625 { Write-Warning "This installation is forbidden by system policy." }
                1638 { Write-Warning "Another version of this product is already installed." }
                1639 { Write-Warning "Invalid command line argument." }
                default { Write-Warning "Check the log at $LogPath for details." }
            }
            
            return $false
        }
        
        return $true
    }
    catch {
        Write-Error "Failed to start installation: $_"
        return $false
    }
}

function Test-Installation {
    param (
        [Parameter(Mandatory = $true)]
        [string]$InstallPath
    )
    
    $exePath = Join-Path -Path $InstallPath -ChildPath "agentuity.exe"
    
    if (-not (Test-Path -Path $exePath)) {
        Write-Warning "Agentuity executable not found at $exePath"
        return $false
    }
    
    try {
        $output = & $exePath version 2>&1
        if ($LASTEXITCODE -ne 0) {
            Write-Warning "Agentuity CLI verification failed with exit code $LASTEXITCODE"
            return $false
        }
        
        Write-Success "Agentuity CLI verified: $output"
        return $true
    }
    catch {
        Write-Warning "Failed to verify Agentuity CLI: $_"
        return $false
    }
}

function Set-PowerShellCompletion {
    param (
        [Parameter(Mandatory = $true)]
        [string]$ExePath
    )
    
    Write-Step "Setting up PowerShell completion..."
    
    try {
        # Create PowerShell profile directory if it doesn't exist
        $profileDir = Split-Path -Parent $PROFILE
        if (-not (Test-Path -Path $profileDir)) {
            New-Item -Path $profileDir -ItemType Directory -Force | Out-Null
        }
        
        # Create completion directory
        $completionDir = Join-Path -Path $profileDir -ChildPath "Completion"
        if (-not (Test-Path -Path $completionDir)) {
            New-Item -Path $completionDir -ItemType Directory -Force | Out-Null
        }
        
        # Generate completion script
        $completionPath = Join-Path -Path $completionDir -ChildPath "agentuity.ps1"
        & $ExePath completion powershell | Out-File -FilePath $completionPath -Encoding utf8 -Force
        
        # Check if the profile exists, create if not
        if (-not (Test-Path -Path $PROFILE)) {
            New-Item -Path $PROFILE -ItemType File -Force | Out-Null
        }
        
        # Add completion to profile if not already there
        $profileContent = Get-Content -Path $PROFILE -Raw -ErrorAction SilentlyContinue
        $completionLine = ". '$completionPath'"
        
        if (-not $profileContent -or -not $profileContent.Contains($completionLine)) {
            Add-Content -Path $PROFILE -Value "`n# Agentuity CLI completion`n$completionLine" -Force
            Write-Success "PowerShell completion installed to $completionPath and added to your profile"
        }
        else {
            Write-Success "PowerShell completion already configured in your profile"
        }
    }
    catch {
        Write-Warning "Failed to set up PowerShell completion: $_"
        Write-Warning "You can manually set up completion by running:"
        Write-Warning "  $ExePath completion powershell > $HOME\Documents\WindowsPowerShell\Completion\agentuity.ps1"
        Write-Warning "  Add '. `"$HOME\Documents\WindowsPowerShell\Completion\agentuity.ps1`"' to your PowerShell profile"
    }
}

#endregion

#region Main Script

# Check PowerShell version
if ($PSVersionTable.PSVersion.Major -lt 5) {
    Write-Warning "PowerShell 5.0 or later is recommended. You are running version $($PSVersionTable.PSVersion)."
    
    if (-not (Get-UserConfirmation -Message "Continue with PowerShell $($PSVersionTable.PSVersion)?" -DefaultToYes $false)) {
        Write-Step "Installation cancelled. Please upgrade PowerShell and try again."
        exit 0
    }
}

# Check if running as administrator for system-wide installation
if (-not (Test-Administrator) -and [string]::IsNullOrEmpty($InstallDir)) {
    $message = "You are not running as Administrator. The installer can:"
    $message += "`n1. Install for current user only (recommended)"
    $message += "`n2. Restart with administrator privileges for system-wide installation"
    
    Write-ColorOutput $message -ForegroundColor Yellow
    $choice = Read-Host "Enter choice (1 or 2)"
    
    if ($choice -eq "2") {
        Request-AdminPrivileges -ScriptPath $MyInvocation.MyCommand.Definition
    }
    else {
        Write-Step "Continuing with user installation..."
    }
}

# Determine version to install
if ($Version -eq "latest") {
    $Version = Get-LatestReleaseVersion
}

# Remove 'v' prefix if present
$Version = $Version.TrimStart('v')

# Determine architecture
$arch = Get-Architecture

# Determine installation directory
$installDir = Get-DefaultInstallDir

# Create installation directory if it doesn't exist
if (-not (Test-Path -Path $installDir)) {
    Write-Step "Creating installation directory: $installDir"
    try {
        New-Item -Path $installDir -ItemType Directory -Force | Out-Null
    }
    catch {
        Write-Error "Failed to create installation directory: $_" -Exit
    }
}

# Determine download filename based on architecture
if ($arch -eq "x64") {
    $downloadFilename = "agentuity-x64.msi"
}
elseif ($arch -eq "arm64") {
    $downloadFilename = "agentuity-arm64.msi"
}
else {
    $downloadFilename = "agentuity-x86.msi"
}

# Construct download URLs
$downloadUrl = "https://github.com/agentuity/cli/releases/download/v${Version}/${downloadFilename}"
$checksumsUrl = "https://github.com/agentuity/cli/releases/download/v${Version}/checksums.txt"

# Create temporary directory
$tempDir = Join-Path -Path $env:TEMP -ChildPath "agentuity_install_$([Guid]::NewGuid().ToString())"
New-Item -Path $tempDir -ItemType Directory -Force | Out-Null

try {
    # Download MSI installer
    $msiPath = Join-Path -Path $tempDir -ChildPath $downloadFilename
    Write-Step "Downloading Agentuity CLI v${Version} for Windows/$arch..."
    
    try {
        [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
        $webClient = New-Object System.Net.WebClient
        $webClient.DownloadFile($downloadUrl, $msiPath)
    }
    catch {
        Write-Error "Failed to download from $downloadUrl`: $_" -Exit
    }
    
    # Download and verify checksums
    $checksumsPath = Join-Path -Path $tempDir -ChildPath "checksums.txt"
    Write-Step "Downloading checksums for verification..."
    
    try {
        $webClient.DownloadFile($checksumsUrl, $checksumsPath)
        
        # Verify checksum
        Write-Step "Verifying checksum..."
        $computedChecksum = Get-FileHash256 -FilePath $msiPath
        $expectedChecksum = (Get-Content -Path $checksumsPath | Where-Object { $_ -match $downloadFilename } | Select-Object -First 1) -split '\s+' | Select-Object -First 1
        
        if ([string]::IsNullOrEmpty($expectedChecksum)) {
            Write-Warning "Checksum for $downloadFilename not found in checksums.txt. Skipping verification."
        }
        elseif ($computedChecksum -ne $expectedChecksum) {
            Write-Error "Checksum verification failed. Expected: $expectedChecksum, Got: $computedChecksum" -Exit
        }
        else {
            Write-Success "Checksum verification passed!"
        }
    }
    catch {
        Write-Warning "Failed to verify checksum: $_"
        
        if (-not (Get-UserConfirmation -Message "Continue without checksum verification?" -DefaultToYes $false)) {
            Write-Step "Installation cancelled."
            exit 0
        }
    }
    
    # Confirm installation
    if (-not $NoPrompt) {
        $confirmMessage = "Ready to install Agentuity CLI v${Version} to $installDir. Continue?"
        if (-not (Get-UserConfirmation -Message $confirmMessage -DefaultToYes $true)) {
            Write-Step "Installation cancelled."
            exit 0
        }
    }
    
    # Install MSI
    $installSuccess = Install-MSI -MsiPath $msiPath
    
    if (-not $installSuccess) {
        Write-Warning "MSI installation may have failed. Attempting to verify installation..."
    }
    
    # Verify installation
    $programFilesPath = [System.IO.Path]::Combine($env:ProgramFiles, "Agentuity")
    $localAppDataPath = [System.IO.Path]::Combine($env:LOCALAPPDATA, "Agentuity")
    
    $installPaths = @($programFilesPath, $localAppDataPath, $installDir)
    $installVerified = $false
    
    foreach ($path in $installPaths) {
        if (Test-Installation -InstallPath $path) {
            $installVerified = $true
            $exePath = Join-Path -Path $path -ChildPath "agentuity.exe"
            
            # Add to PATH if not already there
            Add-ToPath -PathToAdd $path
            
            # Set up PowerShell completion
            Set-PowerShellCompletion -ExePath $exePath
            
            break
        }
    }
    
    if (-not $installVerified) {
        Write-Error "Failed to verify installation. The MSI may have installed to a different location or failed silently."
        Write-Warning "Please check the installation log at $env:TEMP\agentuity_install.log for details."
        exit 1
    }
    
    # Success message
    Write-Success "Installation complete! Run 'agentuity --help' to get started."
    Write-Step "For more information, visit: $(Write-Url "https://agentuity.dev")"
}
finally {
    # Clean up temporary directory
    if (Test-Path -Path $tempDir) {
        Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

#endregion
