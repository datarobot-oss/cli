# DataRobot CLI - PowerShell profile completion test
# Installs completion into the PowerShell profile and checks that a new PowerShell session loads it
# without printing anything. Runs for PowerShell 7 (pwsh), for Windows PowerShell 5.1 (powershell.exe)
# and for a machine that has both. Then uninstalls and checks that the profile still loads.
# Needs `dr` on PATH and both shells. No API token is needed.
# Replaces the PowerShell profiles, so it runs only where CI is set (a CI runner or a throwaway machine).
# Usage: .\run_powershell_profile_test.ps1

$ErrorActionPreference = "Stop"

function Fail($message) {
    Write-Host "[ERROR] $message" -ForegroundColor Red
    exit 1
}

function Pass($message) {
    Write-Host "[OK] $message" -ForegroundColor Green
}

if (-not $env:CI) { Fail "this test replaces the PowerShell profiles; run it on a CI runner or a throwaway machine, or set CI=1" }

$documents = Join-Path $HOME "Documents"
$pwshDir = Join-Path $documents "PowerShell"
$windowsPsDir = Join-Path $documents "WindowsPowerShell"
$pwshProfile = Join-Path $pwshDir "Microsoft.PowerShell_profile.ps1"
$windowsPsProfile = Join-Path $windowsPsDir "Microsoft.PowerShell_profile.ps1"

# The legacy command is the one earlier releases wrote into the profile.
$legacyBlock = "`n# dr completion`nif (Get-Command dr -ErrorAction SilentlyContinue) {`n    dr completion powershell | Out-String | Invoke-Expression`n}`n"

# Starts a new session, which loads the profile, and returns everything the session printed.
function Get-SessionOutput([string]$Shell) {
    $previousPreference = $ErrorActionPreference
    $previousModulePath = $env:PSModulePath

    $ErrorActionPreference = "Continue"
    # powershell.exe must rebuild its own module path instead of inheriting the one pwsh 7 set.
    $env:PSModulePath = $null

    $output = (& $Shell -NoLogo -ExecutionPolicy Bypass -Command "exit 0" 2>&1 | Out-String).Trim()

    $env:PSModulePath = $previousModulePath
    $ErrorActionPreference = $previousPreference

    return $output
}

function Assert-CleanSession([string]$Shell, [string]$Context) {
    $output = Get-SessionOutput $Shell
    if ($output) { Fail "$Shell printed output $Context`: $output" }
}

# The control proves the legacy command is what breaks a session, so a pass cannot be a no-op.
function Assert-BrokenSession([string]$Shell) {
    $output = Get-SessionOutput $Shell
    if (-not $output) { Fail "the legacy profile should make $Shell print an error, so this test cannot tell a fix from a no-op" }
}

function Reset-Profiles {
    Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $pwshDir, $windowsPsDir
}

function Invoke-Dr([string[]]$Arguments) {
    & dr @Arguments | Out-Host
    if ($LASTEXITCODE -ne 0) { Fail "dr $($Arguments -join ' ') exited with $LASTEXITCODE" }
}

function Install-Completion([switch]$Force) {
    $arguments = @("self", "completion", "install", "powershell", "--yes")
    if ($Force) { $arguments += "--force" }

    Invoke-Dr $arguments
}

function Uninstall-Completion {
    Invoke-Dr @("self", "completion", "uninstall", "powershell", "--yes")
}

function Count-Blocks([string]$Content) {
    return ([regex]::Matches($Content, '# dr completion')).Count
}

# Runs the scenario for one shell. The installer writes $ProfilePath, which $Shell loads.
function Test-Shell([string]$Name, [string]$ProfileDir, [string]$ProfilePath, [string]$Shell) {
    Write-Host "== $Name`: a fresh install writes a command that exists"

    Reset-Profiles
    New-Item -ItemType Directory -Force -Path $ProfileDir | Out-Null

    Install-Completion

    if (-not (Test-Path $ProfilePath)) { Fail "no profile at $ProfilePath" }

    $content = Get-Content -Raw $ProfilePath

    if ($content -notmatch 'dr self completion powershell') { Fail "the profile lacks the current command: $content" }
    if ($content -match 'dr completion powershell') { Fail "the profile holds the legacy command: $content" }

    Assert-CleanSession $Shell "after a fresh install"

    Pass "$Shell loads the profile without output"

    Write-Host "== $Name`: a profile written by an earlier release"
    $legacy = $content -replace 'dr self completion powershell', 'dr completion powershell'
    Set-Content -Path $ProfilePath -Value $legacy -NoNewline

    Assert-BrokenSession $Shell

    Pass "the legacy profile prints an error in $Shell"

    # A profile file counts as installed, so replacing the legacy command takes --force.
    Install-Completion -Force

    $content = Get-Content -Raw $ProfilePath

    if ($content -match 'dr completion powershell') { Fail "the legacy command is still in the profile: $content" }
    if ((Count-Blocks $content) -ne 1) { Fail "the dr block is not there exactly once: $content" }

    Assert-CleanSession $Shell "after the migration"

    Pass "the migration replaces the legacy command in place and $Shell is clean"

    Write-Host "== $Name`: a second forced install changes nothing"
    $before = $content

    Install-Completion -Force

    if ((Get-Content -Raw $ProfilePath) -ne $before) { Fail "a second install changed the profile" }

    Pass "the profile is unchanged"

    Write-Host "== $Name`: uninstall leaves a profile that loads"
    Uninstall-Completion

    $content = Get-Content -Raw $ProfilePath

    if ($content -match 'completion powershell') { Fail "the profile still holds a completion command: $content" }

    Assert-CleanSession $Shell "after the uninstall"

    Pass "$Shell is clean after the uninstall"
}

Test-Shell "PowerShell 7" $pwshDir $pwshProfile "pwsh"
Test-Shell "Windows PowerShell 5.1" $windowsPsDir $windowsPsProfile "powershell.exe"

# A machine that ran an earlier release under 5.1 and later installed PowerShell 7 has a legacy
# block in both profiles, and the installer writes only the PowerShell 7 profile.
Write-Host "== both shells installed: one install migrates both profiles"

Reset-Profiles
New-Item -ItemType Directory -Force -Path $pwshDir, $windowsPsDir | Out-Null

Set-Content -Path $pwshProfile -Value $legacyBlock -NoNewline
Set-Content -Path $windowsPsProfile -Value $legacyBlock -NoNewline

Assert-BrokenSession "pwsh"
Assert-BrokenSession "powershell.exe"

Install-Completion -Force

foreach ($profilePath in @($pwshProfile, $windowsPsProfile)) {
    $content = Get-Content -Raw $profilePath

    if ($content -match 'dr completion powershell') { Fail "the legacy command is still in $profilePath`: $content" }
}

Assert-CleanSession "pwsh" "after the migration"
Assert-CleanSession "powershell.exe" "after the migration"

Pass "both profiles are migrated and both shells are clean"

Uninstall-Completion

foreach ($profilePath in @($pwshProfile, $windowsPsProfile)) {
    $content = Get-Content -Raw $profilePath

    if ($content -match 'completion powershell') { Fail "the profile still holds a completion command: $profilePath`: $content" }
}

Assert-CleanSession "pwsh" "after the uninstall"
Assert-CleanSession "powershell.exe" "after the uninstall"

Pass "both shells are clean after the uninstall"

exit 0
