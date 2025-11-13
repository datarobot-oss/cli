# DataRobot CLI uninstallation script for Windows
#
# Usage:
#   irm https://raw.githubusercontent.com/datarobot-oss/cli/main/uninstall.ps1 | iex
#
#   Or with custom install directory:
#     $env:INSTALL_DIR = "C:\custom\path"; irm https://raw.githubusercontent.com/datarobot-oss/cli/main/uninstall.ps1 | iex

$ErrorActionPreference = 'Stop'

# Configuration
$BINARY_NAME = "dr"
$INSTALL_DIR = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { "$env:LOCALAPPDATA\Programs\$BINARY_NAME" }

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

# Check if binary exists
function Test-Installation {
    $binaryPath = Join-Path $INSTALL_DIR "$BINARY_NAME.exe"

    if (-not (Test-Path $binaryPath)) {
        Write-ErrorMsg "DataRobot CLI is not installed at $binaryPath"
    }

    try {
        $version = & $binaryPath --version 2>$null | Select-Object -First 1
        Write-Step "Found: $version"
    } catch {
        Write-Step "Found: DataRobot CLI"
    }
    Write-Step "Location: $binaryPath"
}

# Remove binary
function Remove-Binary {
    $binaryPath = Join-Path $INSTALL_DIR "$BINARY_NAME.exe"

    Write-Step "Removing binary from $binaryPath..."
    try {
        Remove-Item -Path $binaryPath -Force -ErrorAction Stop
        Write-Success "Binary removed"

        # Remove directory if empty
        $files = Get-ChildItem -Path $INSTALL_DIR -ErrorAction SilentlyContinue
        if (-not $files) {
            Remove-Item -Path $INSTALL_DIR -Force -ErrorAction SilentlyContinue
            Write-Step "Removed empty directory: $INSTALL_DIR"
        }
    } catch {
        Write-ErrorMsg "Failed to remove binary. Do you have write permissions to $INSTALL_DIR?"
    }
}

# Remove PATH entries
function Remove-FromPath {
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    $machinePath = [Environment]::GetEnvironmentVariable("Path", "Machine")
    $modified = $false

    # Check User PATH
    if ($userPath -like "*$INSTALL_DIR*") {
        Write-Step "Found PATH reference in User environment variables"
        try {
            $newPath = ($userPath -split ';' | Where-Object { $_ -ne $INSTALL_DIR }) -join ';'
            [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
            $env:Path = $newPath
            Write-Success "Removed from User PATH"
            $modified = $true
        } catch {
            Write-Warn "Failed to remove from User PATH: $_"
        }
    }

    # Check Machine PATH (requires admin)
    if ($machinePath -like "*$INSTALL_DIR*") {
        Write-Step "Found PATH reference in System environment variables"
        Write-Warn "System PATH modification requires administrator privileges"

        $response = Read-Host "Would you like to try removing it from System PATH? (requires admin) [y/N]"
        if ($response -match '^[Yy](es)?$') {
            try {
                $newPath = ($machinePath -split ';' | Where-Object { $_ -ne $INSTALL_DIR }) -join ';'
                [Environment]::SetEnvironmentVariable("Path", $newPath, "Machine")
                Write-Success "Removed from System PATH"
                $modified = $true
            } catch {
                Write-Warn "Failed to remove from System PATH. Try running as administrator."
            }
        }
    }

    if ($modified) {
        Write-Host ""
        Write-Warn "PATH was modified. Restart your terminal for changes to take effect."
    } elseif (-not ($userPath -like "*$INSTALL_DIR*") -and -not ($machinePath -like "*$INSTALL_DIR*")) {
        Write-Step "No PATH entries found"
    }
}

# Remove shell completions
function Remove-Completions {
    $removed = $false

    # PowerShell completion - check profile
    $profilePath = $PROFILE
    if (Test-Path $profilePath) {
        $content = Get-Content $profilePath -Raw

        # Check if profile has dr completion reference
        if ($content -match "$BINARY_NAME completion powershell|$BINARY_NAME\.ps1") {
            Write-Step "Found completion reference in PowerShell profile"

            try {
                # Create backup
                $backupPath = "$profilePath.bak.$(Get-Date -Format 'yyyyMMdd_HHmmss')"
                Copy-Item $profilePath $backupPath

                # Remove lines containing dr completion
                $newContent = ($content -split "`n" | Where-Object {
                    $_ -notmatch "$BINARY_NAME completion powershell" -and
                    $_ -notmatch "$BINARY_NAME\.ps1" -and
                    $_ -notmatch "Added by DataRobot CLI installer"
                }) -join "`n"

                Set-Content -Path $profilePath -Value $newContent
                Write-Success "Removed completion from PowerShell profile"
                $removed = $true
            } catch {
                Write-Warn "Failed to remove completion from profile: $_"
            }
        }
    }

    # Remove standalone completion script if it exists
    $completionScript = Join-Path (Split-Path $profilePath) "$BINARY_NAME.ps1"
    if (Test-Path $completionScript) {
        try {
            Remove-Item $completionScript -Force
            Write-Success "Removed completion script: $completionScript"
            $removed = $true
        } catch {
            Write-Warn "Failed to remove completion script: $_"
        }
    }

    if (-not $removed) {
        Write-Step "No shell completions found"
    }
}

# Confirm uninstallation
function Confirm-Uninstall {
    Write-Host ""
    $response = Read-Host "Are you sure you want to uninstall DataRobot CLI? [y/N]"

    if ($response -notmatch '^[Yy](es)?$') {
        Write-Info "Uninstallation cancelled"
        exit 0
    }
}

# Main uninstallation flow
function Uninstall-DataRobotCLI {
    Write-Host $banner -ForegroundColor Cyan
    Write-Info "Uninstalling DataRobot CLI"
    Write-Host ""

    Test-Installation
    Write-Host ""

    Confirm-Uninstall
    Write-Host ""

    Write-Info "Removing DataRobot CLI..."
    Remove-Binary

    Write-Host ""
    Write-Info "Checking PATH environment variables..."
    Remove-FromPath

    Write-Host ""
    Write-Info "Removing shell completions..."
    Remove-Completions

    Write-Host ""
    Write-Info "Uninstallation complete!"
    Write-Step "DataRobot CLI has been removed from your system"
    Write-Host ""
}

# Run uninstallation
try {
    Uninstall-DataRobotCLI
} catch {
    Write-Host ""
    Write-ErrorMsg "Uninstallation failed: $_"
}
