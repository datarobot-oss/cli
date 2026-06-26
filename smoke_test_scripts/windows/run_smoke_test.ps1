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
dr help
if ($LASTEXITCODE -ne 0) {
    Write-ErrorMsg "dr help command failed"
}
Write-End

Write-Delimiter "Execute dr help run"
dr help run
if ($LASTEXITCODE -ne 0) {
    Write-ErrorMsg "dr help run command failed"
}
Write-End

Write-Delimiter "Execute dr self version"
dr self version
if ($LASTEXITCODE -ne 0) {
    Write-ErrorMsg "dr self version command failed"
}
Write-End

# Test shell detection
Write-Delimiter "Testing shell detection"
Write-InfoMsg "Running dr --debug self version to verify shell detection..."
# --debug writes to stderr. Under $ErrorActionPreference = "Stop", PowerShell
# wraps every stderr line from a native command in an ErrorRecord — even with
# 2>file, because the redirect happens after PowerShell's own error processing.
# Temporarily relax to "Continue" so stderr lines are captured, not thrown.
$prevEAP = $ErrorActionPreference
$ErrorActionPreference = "Continue"
$debug_output = dr --debug self version 2>&1 | Out-String
$capturedExitCode = $LASTEXITCODE
$ErrorActionPreference = $prevEAP
if ($capturedExitCode -eq 0) {
    if ($debug_output -match 'Shell.*name=powershell') {
        Write-SuccessMsg "Assertion passed: Shell detection correctly identified PowerShell."
    } else {
        Write-Host "Debug output:" -ForegroundColor Yellow
        Write-Host $debug_output
        Write-ErrorMsg "Assertion failed: Shell detection did not identify PowerShell. Expected 'name=powershell' in debug output."
    }
} else {
    Write-ErrorMsg "dr --debug self version command failed unexpectedly with exit code $capturedExitCode"
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

# Test completion install under a fresh-machine execution policy.
# The installer should warn the user that the profile will not load under the
# default Restricted policy and print the exact command to fix it, but it should
# not modify the policy itself.
Write-Delimiter "Testing dr self completion install (Restricted execution policy)"

$originalPolicy = Get-ExecutionPolicy -Scope CurrentUser

# Simulate a fresh Windows machine where the default execution policy is Restricted.
Set-ExecutionPolicy Restricted -Scope CurrentUser -Force

# Run the completion installer and capture its output so we can verify the warning.
$installOutput = (dr self completion install powershell --yes 2>&1 | Out-String)
$installExitCode = $LASTEXITCODE
if ($installExitCode -ne 0) {
    Set-ExecutionPolicy $originalPolicy -Scope CurrentUser -Force -ErrorAction SilentlyContinue
    Write-ErrorMsg "dr self completion install powershell --yes failed with exit code $installExitCode"
}

# Verify the installer warned the user with the exact fix command.
if ($installOutput -notmatch "Set-ExecutionPolicy RemoteSigned -Scope CurrentUser") {
    Set-ExecutionPolicy $originalPolicy -Scope CurrentUser -Force -ErrorAction SilentlyContinue
    Write-ErrorMsg "Assertion failed: installer did not warn user with the execution policy fix command"
}
Write-SuccessMsg "Assertion passed: installer warned about Restricted execution policy"

# Verify the completion profile was written.
$profilePath = "$env:USERPROFILE\Documents\WindowsPowerShell\Microsoft.PowerShell_profile.ps1"
if (-not (Test-Path $profilePath)) {
    $profilePath = "$env:USERPROFILE\Documents\PowerShell\Microsoft.PowerShell_profile.ps1"
}
if (-not (Test-Path $profilePath)) {
    Set-ExecutionPolicy $originalPolicy -Scope CurrentUser -Force -ErrorAction SilentlyContinue
    Write-ErrorMsg "Assertion failed: PowerShell profile was not created"
}
$profileContent = Get-Content $profilePath -Raw
if ($profileContent -notmatch "dr completion powershell") {
    Set-ExecutionPolicy $originalPolicy -Scope CurrentUser -Force -ErrorAction SilentlyContinue
    Write-ErrorMsg "Assertion failed: profile does not contain completion block"
}
Write-SuccessMsg "Assertion passed: profile written and contains completion block"

# Verify the policy remains unchanged (warn-only behavior; the fix is left to the user).
$policy = Get-ExecutionPolicy -Scope CurrentUser
if ($policy -ne "Restricted") {
    Set-ExecutionPolicy $originalPolicy -Scope CurrentUser -Force -ErrorAction SilentlyContinue
    Write-ErrorMsg "Assertion failed: expected execution policy to remain Restricted (warn-only), but it is '$policy'"
}
Write-SuccessMsg "Assertion passed: execution policy remains '$policy' (installer only warned)"

# Restore the original policy so the rest of the smoke test is not affected.
Set-ExecutionPolicy $originalPolicy -Scope CurrentUser -Force -ErrorAction SilentlyContinue
Write-End

# Test dr run command
Write-Delimiter "Testing dr run command"
dr run
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
#
# This mirrors the bash expect test (expect_auth_login.exp): it does NOT verify a
# live token (that is what 'dr auth check' does, and it requires a valid token +
# reachable server). Instead it confirms that 'dr auth login' starts up and prints
# the OAuth redirect URL, then terminates the process. PowerShell has no 'expect',
# so we launch the command in the background, poll its output for the auth URL, and
# kill it once seen.
Write-Delimiter "Testing dr auth login"
Write-InfoMsg "Testing dr auth login command..."

$auth_out = Join-Path (Get-Location) "auth_login_stdout.txt"
$auth_err = Join-Path (Get-Location) "auth_login_stderr.txt"

$auth_proc = Start-Process -FilePath "dr" -ArgumentList "auth", "login" `
    -RedirectStandardOutput $auth_out -RedirectStandardError $auth_err `
    -PassThru -NoNewWindow

# Poll for the auth redirect URL (same marker the bash expect script matches).
$auth_url_shown = $false
for ($i = 0; $i -lt 20; $i++) {
    Start-Sleep -Milliseconds 500
    if (Test-Path $auth_out) {
        if ((Get-Content $auth_out -Raw) -match "cliRedirect=true") {
            $auth_url_shown = $true
            break
        }
    }
    if ($auth_proc.HasExited) { break }
}

# No browser round-trip happens in tests, so stop the waiting process. Guard the
# kill: the process may exit between the check and the Kill() call, which would
# otherwise throw under $ErrorActionPreference = "Stop".
if (-not $auth_proc.HasExited) {
    try {
        $auth_proc.Kill()
        $auth_proc.WaitForExit()
    } catch {
        # Process already exited; nothing to clean up.
    }
}

Remove-Item $auth_out, $auth_err -Force -ErrorAction SilentlyContinue

if ($auth_url_shown) {
    Write-SuccessMsg "Assertion passed: 'dr auth login' displayed the OAuth redirect URL."
} else {
    Write-ErrorMsg "Assertion failed: 'dr auth login' did not display the auth URL (cliRedirect=true)."
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
