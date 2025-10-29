# DataRobot smoke test script for Windows

$ErrorActionPreference = "Stop"

function Write-ErrorMsg {
    param([string]$Message)
    Write-Host "Error: " -ForegroundColor Red -NoNewline
    Write-Host $Message
    exit 1
}

# Main smoke test flow
function Smoke-Test {
    Write-Host 'Running smoke tests for Windows.'

    $path = [Environment]::GetEnvironmentVariable("Path", "User")

    Write-Host "path: " -NoNewline
    Write-Host $path

    $INSTALL_DIR = "$env:LOCALAPPDATA\Programs\dr"

    Write-Host "INSTALL_DIR: " -NoNewline
    Write-Host $INSTALL_DIR

    Write-Host "=========== DEBUGGING ==========="

    Write-Host pwd
    Write-Host ls

    Write-Host cd C:\
    Write-Host ls

    Write-Host cd Users
    Write-Host ls

    Write-Host "================"

    cd "$env:LOCALAPPDATA\Programs"
    Write-Host ls

    cd $INSTALL_DIR
    Write-Host ls

    Get-Command dr

    Write-Host 'Smoke tests for Windows completed.'
}

# Run tests
try {
    Smoke-Test
} catch {
    Write-ErrorMsg "Smoke tests for Windows failed: $_"
}
