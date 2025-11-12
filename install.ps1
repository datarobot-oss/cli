# DataRobot CLI installation script for Windows
#
# Usage:
#   Install latest version:
#     irm https://raw.githubusercontent.com/datarobot-oss/cli/main/install.ps1 | iex
#
#   Install specific version:
#     $env:VERSION = "v0.1.0"; irm https://raw.githubusercontent.com/datarobot-oss/cli/main/install.ps1 | iex
#
#   Custom install directory:
#     $env:INSTALL_DIR = "C:\custom\path"; irm https://raw.githubusercontent.com/datarobot-oss/cli/main/install.ps1 | iex
#
#   Combine options:
#     $env:VERSION = "v0.1.0"; $env:INSTALL_DIR = "C:\tools"; irm https://raw.githubusercontent.com/datarobot-oss/cli/main/install.ps1 | iex

param(
    [string]$Version = "latest"
)

$ErrorActionPreference = 'Stop'

# Configuration
$REPO = "datarobot-oss/cli"
$BINARY_NAME = "dr"
$INSTALL_DIR_EXPLICITLY_SET = [bool]$env:INSTALL_DIR
$INSTALL_DIR = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { "$env:LOCALAPPDATA\Programs\$BINARY_NAME" }

# Use version from environment variable if set, otherwise use "latest"
$Version = if ($env:VERSION) { $env:VERSION } else { "latest" }

# ASCII Art Banner
$banner = @"
    ____        __        ____        __          __
   / __ \____ _/ /_____ _/ __ \____  / /_  ____  / /_
  / / / / __ `/ __/ __ `/ /_/ / __ \/ __ \/ __ \/ __/
 / /_/ / /_/ / /_/ /_/ / _, _/ /_/ / /_/ / /_/ / /_
/_____/\__,_/\__/\__,_/_/ |_|\____/_.___/\____/\__/

"@

# Helper functions
function Write-Info {
    param([string]$Message)
    Write-Host "==> " -ForegroundColor Green -NoNewline
    Write-Host $Message -ForegroundColor White
}

function Write-Step {
    param([string]$Message)
    Write-Host "  → " -ForegroundColor Blue -NoNewline
    Write-Host $Message
}

function Write-Warn {
    param([string]$Message)
    Write-Host "Warning: " -ForegroundColor Yellow -NoNewline
    Write-Host $Message
}

function Write-ErrorMsg {
    param([string]$Message)
    Write-Host "Error: " -ForegroundColor Red -NoNewline
    Write-Host $Message
    exit 1
}

function Write-Success {
    param([string]$Message)
    Write-Host "   ✓ " -ForegroundColor Green -NoNewline
    Write-Host $Message
}

# Detect architecture
function Get-Architecture {
    $arch = [System.Environment]::Is64BitOperatingSystem
    if ($arch) {
        return "x86_64"
    } else {
        Write-ErrorMsg "32-bit Windows is not supported"
    }
}

# Check requirements
function Test-Requirements {
    # PowerShell version check
    if ($PSVersionTable.PSVersion.Major -lt 5) {
        Write-ErrorMsg "PowerShell 5.0 or higher is required. You have version $($PSVersionTable.PSVersion)"
    }
}

# Get the latest release version or validate specified version
function Get-LatestVersion {
    param([string]$RequestedVersion)

    if ($RequestedVersion -eq "latest") {
        Write-Step "Fetching latest version..."
        try {
            # Prepare headers with authentication if GITHUB_TOKEN is available
            $headers = @{}
            if ($env:GITHUB_TOKEN) {
                $headers["Authorization"] = "token $env:GITHUB_TOKEN"
            }

            $response = Invoke-RestMethod -Uri "https://api.github.com/repos/$REPO/releases/latest" -Method Get -Headers $headers
            $version = $response.tag_name
            Write-Host "   Version: " -NoNewline
            Write-Host $version -ForegroundColor White
            return $version
        } catch {
            Write-ErrorMsg "Failed to fetch the latest version from GitHub: $_"
        }
    } else {
        Write-Step "Using specified version: $RequestedVersion"
        # Ensure version starts with 'v'
        if ($RequestedVersion -notmatch '^v') {
            $RequestedVersion = "v$RequestedVersion"
        }
        Write-Host "   Version: " -NoNewline
        Write-Host $RequestedVersion -ForegroundColor White
        return $RequestedVersion
    }
}

# Compare versions (returns -1 if v1 < v2, 0 if equal, 1 if v1 > v2)
function Compare-Versions {
    param(
        [string]$Version1,
        [string]$Version2
    )

    # Remove 'v' prefix
    $v1 = $Version1 -replace '^v', ''
    $v2 = $Version2 -replace '^v', ''

    try {
        $ver1 = [version]$v1
        $ver2 = [version]$v2

        if ($ver1 -lt $ver2) { return -1 }
        elseif ($ver1 -eq $ver2) { return 0 }
        else { return 1 }
    } catch {
        # Fallback to string comparison
        if ($v1 -eq $v2) { return 0 }
        return -1
    }
}

# Check if binary is already installed
function Test-ExistingInstallation {
    param(
        [string]$BinaryPath,
        [string]$TargetVersion
    )

    if (Test-Path $BinaryPath) {
        try {
            $currentVersionOutput = & $BinaryPath version 2>$null | Select-Object -First 1

            # Extract version number (e.g., "v1.2.3" from "dr version v1.2.3")
            if ($currentVersionOutput -match '(v?\d+\.\d+\.\d+)') {
                $currentVersion = $matches[1]
            } else {
                Write-Warn "Unable to determine current version"
                Write-Step "Proceeding with installation of $TargetVersion"
                return $false
            }

            # Normalize versions (ensure both have 'v' prefix)
            if ($currentVersion -notmatch '^v') {
                $currentVersion = "v$currentVersion"
            }

            Write-Step "Current installation: $currentVersion"
            Write-Step "Target version: $TargetVersion"

            # Check if versions are the same
            $comparison = Compare-Versions -Version1 $currentVersion -Version2 $TargetVersion

            if ($comparison -eq 0) {
                Write-Info "DataRobot CLI $TargetVersion is already installed"
                Write-Step "Installation location: $BinaryPath"

                # Check if in PATH
                $inPath = $env:Path -split ';' | Where-Object { $_ -eq $INSTALL_DIR }
                if (-not $inPath) {
                    Write-Warn "$INSTALL_DIR is not in your PATH"
                    Show-PathInstructions
                }

                Write-Host ""
                Write-Info "Already up to date!"
                return $true
            }
            elseif ($comparison -lt 0) {
                # Update available
                Write-Info "Update available: $currentVersion → $TargetVersion"
                Write-Host ""
                $response = Read-Host "Would you like to upgrade to $TargetVersion? [Y/n]"

                if ($response -match '^[Nn](o)?$') {
                    Write-Info "Installation cancelled"
                    exit 0
                } else {
                    Write-Host ""
                    Write-Info "Upgrading DataRobot CLI..."
                    return $false
                }
            }
            else {
                # Downgrade
                Write-Warn "Target version ($TargetVersion) is older than current version ($currentVersion)"
                Write-Host ""
                $response = Read-Host "Would you like to downgrade to $TargetVersion? [y/N]"

                if ($response -match '^[Yy](es)?$') {
                    Write-Host ""
                    Write-Info "Downgrading DataRobot CLI..."
                    return $false
                } else {
                    Write-Info "Installation cancelled"
                    exit 0
                }
            }
        } catch {
            # If version check fails, proceed with installation
            Write-Warn "Version check failed, proceeding with installation"
            return $false
        }
    }
    return $false
}

# Download and install the binary
function Install-Binary {
    param(
        [string]$Version,
        [string]$Architecture
    )

    # Construct download URL
    $archiveName = "${BINARY_NAME}_${Version}_Windows_${Architecture}.zip"
    $downloadUrl = "https://github.com/$REPO/releases/download/$Version/$archiveName"

    Write-Step "Downloading from GitHub..."
    Write-Host "   $downloadUrl" -ForegroundColor DarkGray

    # Create temporary directory
    $tempDir = New-Item -ItemType Directory -Path (Join-Path $env:TEMP ([System.IO.Path]::GetRandomFileName()))

    try {
        # Download archive
        $archivePath = Join-Path $tempDir $archiveName
        try {
            $ProgressPreference = 'SilentlyContinue'
            Invoke-WebRequest -Uri $downloadUrl -OutFile $archivePath -UseBasicParsing
            $ProgressPreference = 'Continue'
        } catch {
            Write-ErrorMsg "Failed to download binary. Please check the version exists: https://github.com/$REPO/releases"
        }

        # Extract archive
        Write-Step "Extracting binary..."
        try {
            Expand-Archive -Path $archivePath -DestinationPath $tempDir -Force
        } catch {
            Write-ErrorMsg "Failed to extract archive: $_"
        }

        # Create install directory if it doesn't exist
        if (-not (Test-Path $INSTALL_DIR)) {
            Write-Step "Creating install directory: $INSTALL_DIR"
            try {
                New-Item -ItemType Directory -Path $INSTALL_DIR -Force | Out-Null
            } catch {
                Write-ErrorMsg "Failed to create install directory: $_"
            }
        }

        # Install binary
        $binaryPath = Join-Path $INSTALL_DIR "$BINARY_NAME.exe"
        Write-Step "Installing binary to $binaryPath..."
        try {
            Copy-Item -Path (Join-Path $tempDir "$BINARY_NAME.exe") -Destination $binaryPath -Force
        } catch {
            Write-ErrorMsg "Failed to install binary. Do you have write permissions to $INSTALL_DIR?"
        }

    } finally {
        # Clean up
        Remove-Item -Path $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# Show PATH configuration instructions
function Show-PathInstructions {
    Write-Host ""
    Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Yellow
    Write-Host "Next step: Add to PATH" -ForegroundColor White
    Write-Host ""
    Write-Host "The installation directory is not in your PATH."
    Write-Host "Run this command to add it (requires reopening terminal):"
    Write-Host ""
    Write-Host '  $path = [Environment]::GetEnvironmentVariable("Path", "User")' -ForegroundColor Blue
    Write-Host "  `$newPath = `"`$path;$INSTALL_DIR`"" -ForegroundColor Blue
    Write-Host '  [Environment]::SetEnvironmentVariable("Path", $newPath, "User")' -ForegroundColor Blue
    Write-Host ""
    Write-Host "Or use the full path: " -NoNewline
    Write-Host "$INSTALL_DIR\$BINARY_NAME.exe" -ForegroundColor Blue
    Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Yellow
}

# Add install directory to PATH
function Add-ToPath {
    $currentPath = [Environment]::GetEnvironmentVariable("Path", "User")

    if ($currentPath -notlike "*$INSTALL_DIR*") {
        Write-Warn "$INSTALL_DIR is not in your PATH"
        Write-Host ""

        # Ask user if they want to add to PATH automatically
        $response = Read-Host "Would you like to add $INSTALL_DIR to your PATH automatically? [y/N]"

        if ($response -match '^[Yy](es)?$') {
            Write-Host ""
            Write-Step "Adding $INSTALL_DIR to your PATH..."

            try {
                $newPath = if ($currentPath) { "$currentPath;$INSTALL_DIR" } else { $INSTALL_DIR }
                [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
                $env:Path = "$env:Path;$INSTALL_DIR"
                Write-Success "PATH updated successfully"
                Write-Warn "You may need to restart your terminal for PATH changes to take effect"
                return $true
            } catch {
                Write-Warn "Failed to update PATH automatically"
                Show-PathInstructions
                return $false
            }
        } else {
            Write-Host ""
            Show-PathInstructions
            return $false
        }
    } else {
        Write-Step "Installation directory is already in PATH"
        return $true
    }
}

# Verify installation
function Test-Installation {
    param([string]$BinaryPath)

    if (Test-Path $BinaryPath) {
        Write-Step "Verifying installation..."
        try {
            $version = & $BinaryPath --version 2>$null | Select-Object -First 1
            Write-Success $version
        } catch {
            Write-Success "Binary installed at: $BinaryPath"
        }
    } else {
        Write-ErrorMsg "Binary not found at $BinaryPath"
    }
}

# Prompt user to install shell completions
function Invoke-CompletionPrompt {
    param([string]$BinaryPath)

    Write-Host ""
    $response = Read-Host "Would you like to install shell completions for PowerShell? [Y/n]"

    if ($response -match '^[Nn](o)?$') {
        Write-Host ""
        Write-Info "Skipping completion installation"
        Write-Step "You can install completions later with:"
        Write-Host "  $BINARY_NAME completion install --yes" -ForegroundColor Blue
        return $false
    }

    return $true
}

# Install PowerShell completions using the built-in command
function Install-Completions {
    param([string]$BinaryPath)

    Write-Host ""
    Write-Info "Installing shell completions..."

    try {
        # Note: PowerShell doesn't have the interactive install command yet
        # Fall back to the manual method
        $completionScript = & $BinaryPath self completion powershell 2>$null | Out-String

        if (-not $completionScript) {
            Write-Warn "Failed to generate completion script"
            Write-Step "Install manually with: $BINARY_NAME completion powershell --help"
            return $false
        }

        # Check if profile exists
        if (-not (Test-Path $PROFILE)) {
            New-Item -Path $PROFILE -ItemType File -Force | Out-Null
        }

        # Check if completion is already in profile
        $profileContent = Get-Content $PROFILE -Raw -ErrorAction SilentlyContinue
        if ($profileContent -and $profileContent -match "$BINARY_NAME completion") {
            Write-Success "Shell completions already installed"
            return $true
        }

        # Add completion to profile
        $completionLine = "`n# $BINARY_NAME completion`n& '$BinaryPath' self completion powershell | Out-String | Invoke-Expression"
        Add-Content -Path $PROFILE -Value $completionLine

        Write-Success "Shell completions installed successfully"
        Write-Step "Restart PowerShell or run: . `$PROFILE"
        return $true
    } catch {
        Write-Warn "Failed to install completions: $_"
        Write-Step "Install manually with: $BINARY_NAME completion powershell --help"
        return $false
    }
}

# Main installation flow
function Install-DataRobotCLI {
    Write-Host $banner -ForegroundColor Cyan
    Write-Info "Installing DataRobot CLI"
    Write-Host ""

    Test-Requirements

    $architecture = Get-Architecture
    Write-Step "Detected architecture: $architecture"

    $version = Get-LatestVersion -RequestedVersion $Version

    $binaryPath = Join-Path $INSTALL_DIR "$BINARY_NAME.exe"
    $alreadyInstalled = Test-ExistingInstallation -BinaryPath $binaryPath -TargetVersion $version

    if ($alreadyInstalled) {
        return
    }

    Write-Host ""
    Write-Info "Downloading and installing..."
    Install-Binary -Version $version -Architecture $architecture

    Write-Host ""
    Write-Info "Installation complete!"
    Test-Installation -BinaryPath $binaryPath

    $inPath = Add-ToPath

    # Prompt for completion installation
    if (Invoke-CompletionPrompt -BinaryPath $binaryPath) {
        Install-Completions -BinaryPath $binaryPath
    }

    Write-Host ""
    if ($inPath) {
        Write-Info "Get started by running: " -NoNewline
        Write-Host "$BINARY_NAME --help" -ForegroundColor White
    } else {
        Write-Info "Get started by running: " -NoNewline
        Write-Host "$binaryPath --help" -ForegroundColor White
    }
    Write-Host ""
}

# Run installation
try {
    Install-DataRobotCLI
} catch {
    Write-Host ""
    Write-ErrorMsg "Installation failed: $_"
}
