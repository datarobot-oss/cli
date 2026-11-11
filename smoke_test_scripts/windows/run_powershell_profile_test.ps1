# DataRobot CLI - PowerShell profile completion test
# Installs completion into the PowerShell profile and checks that a new PowerShell session loads it
# without printing anything. Needs `dr` on PATH and PowerShell 7 (`pwsh`). No API token is needed.
# Usage: .\run_powershell_profile_test.ps1

$ErrorActionPreference = "Stop"

# The installer picks the PowerShell 7 profile when this folder exists.
$profileDir = Join-Path $HOME "Documents\PowerShell"
$profilePath = Join-Path $profileDir "Microsoft.PowerShell_profile.ps1"

New-Item -ItemType Directory -Force -Path $profileDir | Out-Null
Remove-Item -Force -ErrorAction SilentlyContinue $profilePath

function Fail($message) {
    Write-Host "[ERROR] $message" -ForegroundColor Red
    exit 1
}

function Pass($message) {
    Write-Host "[OK] $message" -ForegroundColor Green
}

# Starts a new session, which loads the profile, and returns everything the session printed.
function Get-SessionOutput {
    $previous = $ErrorActionPreference
    $ErrorActionPreference = "Continue"

    $output = (& pwsh -NoLogo -Command "exit 0" 2>&1 | Out-String).Trim()

    $ErrorActionPreference = $previous

    return $output
}

function Install-Completion([switch]$Force) {
    $arguments = @("self", "completion", "install", "powershell", "--yes")
    if ($Force) { $arguments += "--force" }

    & dr @arguments | Out-Host
    if ($LASTEXITCODE -ne 0) { Fail "dr $($arguments -join ' ') exited with $LASTEXITCODE" }
}

Write-Host "== a fresh install writes a command that exists"
Install-Completion

if (-not (Test-Path $profilePath)) { Fail "no profile at $profilePath" }

$content = Get-Content -Raw $profilePath

if ($content -notmatch 'dr self completion powershell') { Fail "the profile lacks the current command: $content" }
if ($content -match 'dr completion powershell') { Fail "the profile holds the legacy command: $content" }

$output = Get-SessionOutput
if ($output) { Fail "a new session printed output: $output" }

Pass "a new session loads the profile without output"

Write-Host "== a profile written by an earlier release"
$legacy = $content -replace 'dr self completion powershell', 'dr completion powershell'
Set-Content -Path $profilePath -Value $legacy -NoNewline

# The control proves the legacy command is what breaks a session.
$output = Get-SessionOutput
if (-not $output) { Fail "the legacy profile should print an error, so this test cannot tell a fix from a no-op" }

Pass "the legacy profile prints an error"

# A profile file counts as installed, so replacing the legacy command takes --force.
Install-Completion -Force

$content = Get-Content -Raw $profilePath

if ($content -match 'dr completion powershell') { Fail "the legacy command is still in the profile: $content" }
if (([regex]::Matches($content, '# dr completion')).Count -ne 1) { Fail "the dr block is not there exactly once: $content" }

$output = Get-SessionOutput
if ($output) { Fail "a new session printed output after the migration: $output" }

Pass "the migration replaces the legacy command in place and a new session is clean"

Write-Host "== a second forced install changes nothing"
$before = $content

Install-Completion -Force

if ((Get-Content -Raw $profilePath) -ne $before) { Fail "a second install changed the profile" }

Pass "the profile is unchanged"

exit 0
