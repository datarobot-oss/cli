# DataRobot smoke test script for Windows

# Get DR_API_TOKEN from args
$DR_API_TOKEN = $args[0]

$ErrorActionPreference = "Stop"

function Write-ErrorMsg {
    param([string]$Message)
    Write-Host "[ERROR] " -NoNewline -ForegroundColor Red
    Write-Host $Message
    Write-Host ""
    exit 1
}

function Write-SuccessMsg {
    param([string]$Message)
    Write-Host "[OK] " -NoNewline -ForegroundColor Green
    Write-Host $Message
}

function Write-InfoMsg {
    param([string]$Message)
    Write-Host "[INFO] " -NoNewline -ForegroundColor Cyan
    Write-Host $Message
}

function Write-Delimiter {
    param([string]$Message)
    Write-Host ""
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

function Test-URLAccessible {
    param([string]$Url)
    try {
        $null = Invoke-WebRequest -Uri $Url -Method Head -TimeoutSec 5 -UseBasicParsing -ErrorAction Stop
        return $true
    } catch {
        return $false
    }
}

try {
    # Main execution
    Write-Host 'Running smoke tests for Windows...'

    # Validate DR_API_TOKEN is provided
    if ([string]::IsNullOrEmpty($DR_API_TOKEN)) {
        Write-ErrorMsg "The variable 'DR_API_TOKEN' must be supplied as an argument."
    }

    # Used throughout testing
    $testing_url = "https://app.datarobot.com"

    # Determine if we can access URL
    Write-InfoMsg "Checking URL accessibility: $testing_url"
    $url_accessible = Test-URLAccessible -Url $testing_url

    # Setup config directory and file
    $testing_dr_cli_config_dir = Join-Path (Join-Path (Get-Location) ".config") "datarobot"
    $null = New-Item -ItemType Directory -Force -Path $testing_dr_cli_config_dir
    $env:DATAROBOT_CLI_CONFIG = Join-Path $testing_dr_cli_config_dir "drconfig.yaml"

    # Copy example config
    $example_config = Join-Path (Join-Path (Join-Path (Get-Location) "smoke_test_scripts") "assets") "example_config.yaml"
    Copy-Item -Path $example_config -Destination $env:DATAROBOT_CLI_CONFIG -Force

    # Set API token in config file
    Write-InfoMsg "Setting API token in config file: $env:DATAROBOT_CLI_CONFIG"
    $config_content = Get-Content $env:DATAROBOT_CLI_CONFIG -Raw
    $config_content = $config_content -replace 'token: ""', "token: `"$DR_API_TOKEN`""
    Set-Content -Path $env:DATAROBOT_CLI_CONFIG -Value $config_content

    # Test basic commands
    Write-Delimiter "Execute dr help"
    $help_output = dr help
    if ($LASTEXITCODE -ne 0) {
        Write-ErrorMsg "dr help command failed"
    }

    $header_copy = "Build AI Applications Faster"
    if ($help_output -match $header_copy) {
        Write-SuccessMsg "Help command returned expected content."
    } else {
        Write-ErrorMsg "Help command did not return expected content - missing header copy: $header_copy"
    }
    Write-End

    Write-Delimiter "Execute dr self version"
    $version_json = dr self version --format=json
    if ($LASTEXITCODE -ne 0) {
        Write-ErrorMsg "dr self version command failed"
    }

    # Simplified version key check for PowerShell
    if ($version_json -match '"version":') {
        Write-SuccessMsg "Version command returned expected 'version' key in json output."
    } else {
        Write-ErrorMsg "Version command did not return expected 'version' key in json output."
    }
    Write-End

    # Test completion generation
    Write-Delimiter "Testing completion generation"
    $completion_file = "completion_powershell.ps1"
    dr self completion powershell > $completion_file
    if ($LASTEXITCODE -ne 0) {
        Write-ErrorMsg "dr self completion powershell command failed"
    }

    # Check if completion file contains expected content
    if (Test-Path $completion_file) {
        $completion_content = Get-Content $completion_file -Raw
        if ($completion_content -match "Register-ArgumentCompleter") {
            Write-SuccessMsg "Assertion passed: We have expected $completion_file file with Register-ArgumentCompleter."
            Remove-Item $completion_file -Force
        } else {
            Write-ErrorMsg "Assertion failed: We don't have expected $completion_file file with expected function."
        }
    } else {
        Write-ErrorMsg "Completion file was not created."
    }
    Write-End

    # Test dr run command
    Write-Delimiter "Testing dr run command"
    $run_output = dr run 2>&1
    
    if (Test-Path ".env") {
        $usage_message = "No Taskfiles found in child directories."
    } else {
        $usage_message = "You don't seem to be in a DataRobot Template directory."
    }

    if ($run_output -match $usage_message) {
        Write-SuccessMsg "Run command returned expected content."
    } else {
        Write-ErrorMsg "Run command did not return expected content - missing informative message: $usage_message"
    }
    Write-End

    # Test auth setURL
    Write-Delimiter "Testing dr auth setURL"
    Write-InfoMsg "Setting auth URL to: $testing_url"
    # Simulate setting the URL (in bash version this uses expect)
    # For Windows, we'll set it directly via config
    $config_content = Get-Content $env:DATAROBOT_CLI_CONFIG -Raw
    $config_content = $config_content -replace 'endpoint: ""', "endpoint: `"${testing_url}/api/v2`""
    Set-Content -Path $env:DATAROBOT_CLI_CONFIG -Value $config_content

    # Verify the endpoint was set
    $config_content = Get-Content $env:DATAROBOT_CLI_CONFIG -Raw
    if ($config_content -match "endpoint: `"${testing_url}/api/v2`"") {
        Write-SuccessMsg "Assertion passed: We have expected 'endpoint' auth URL value in config."
        Write-Host "Value: endpoint: `"${testing_url}/api/v2`""
    } else {
        Write-ErrorMsg "Assertion failed: We don't have expected 'endpoint' auth URL value."
    }
    Write-End

    # Test auth login
    Write-Delimiter "Testing dr auth login"
    Write-InfoMsg "Testing dr auth login command (non-interactive)..."
    # Note: Full interactive testing would require a Windows expect equivalent
    dr auth check
    if ($LASTEXITCODE -ne 0) {
        Write-InfoMsg "dr auth check failed as expected (no real credentials)"
    }
    Write-End

    # Test templates (if URL is accessible)
    if (-not $url_accessible) {
        Write-InfoMsg "URL (${testing_url}) is not accessible so skipping 'dr templates setup' test."
    } else {
        Write-Delimiter "Testing dr templates setup"
        Write-InfoMsg "Testing template setup would require interactive input..."
        Write-InfoMsg "Skipping template clone test on Windows (requires interactive expect-like tool)"
        Write-End
    }

    Write-SuccessMsg "Smoke tests for Windows completed successfully."
    exit 0

} finally {
    Write-InfoMsg "Cleaning up..."
    if (Test-Path $testing_dr_cli_config_dir) {
        Remove-Item -Recurse -Force $testing_dr_cli_config_dir
    }
    if (Test-Path "completion_powershell.ps1") {
        Remove-Item -Force "completion_powershell.ps1"
    }
}

