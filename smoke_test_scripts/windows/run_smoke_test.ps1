# DataRobot smoke test script for Windows

$ErrorActionPreference = "Stop"

function Write-ErrorMsg {
    param([string]$Message)
    Write-Host "Error: " -ForegroundColor Red -NoNewline
    Write-Host $Message
    exit 1
}

function Write-Delimiter {
    param([string]$Message)
    Write-Host
    Write-Host ("=" * 20) -NoNewline
    Write-Host " " -NoNewline
    Write-Host $Message -NoNewline
    Write-Host " " -NoNewline
    Write-Host ("=" * 20)
}

function Write-End {
    Write-Host ("=" * 20) -NoNewline
    Write-Host " END " -NoNewline
    Write-Host ("=" * 20)
}

# Main smoke test flow
function Smoke-Test {
    Write-Host 'Running smoke tests for Windows...'

    Write-Delimiter "Execute dr help"
    dr help
    Write-End

    Write-Delimiter "Execute dr version"
    dr self version
    Write-End

    Write-Host 'Smoke tests for Windows completed.'
}

# Run tests
try {
    Smoke-Test
} catch {
    Write-ErrorMsg "Smoke tests for Windows failed: $_"
}
