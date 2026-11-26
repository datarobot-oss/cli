# Smoke test for `dr build-studio` on Windows.
#
# 1. `dr build-studio --help` prints the command's own help.
# 2. `dr build-studio --yes --port <free port>` installs the xp and opencode
#    plugins from the public plugin registry into a sandbox, and xp answers
#    HTTP on that port. Skipped when the registry or DataRobot isn't reachable.
#
# The live start needs DataRobot credentials, because xp requires them:
# DATAROBOT_ENDPOINT (defaults to https://app.datarobot.com/api/v2) and
# DATAROBOT_API_TOKEN, or DR_API_TOKEN as in the rest of the smoke suite.
# Plugins install into a temporary XDG_CONFIG_HOME, so the test never touches
# your own plugins or config.
#
#   $env:DR_API_TOKEN = "<token>"; .\smoke_test_scripts\windows\run_build_studio_test.ps1

$ErrorActionPreference = "Stop"

$RegistryUrl = "https://cli.datarobot.com/plugins/index.json"
$StartTimeoutSec = 240

function Write-ErrorMsg {
    param([string]$Message)
    Write-Host "[ERROR] " -NoNewline -ForegroundColor Red
    Write-Host $Message
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

# Returns a TCP port that is free right now, by letting the OS pick one.
function Get-FreePort {
    $listener = [System.Net.Sockets.TcpListener]::new([System.Net.IPAddress]::Loopback, 0)
    $listener.Start()
    try {
        return $listener.LocalEndpoint.Port
    } finally {
        $listener.Stop()
    }
}

# Returns the HTTP status code of a GET to $Url, or 0 when nothing answers.
function Get-HttpStatus {
    param([string]$Url)
    try {
        $response = Invoke-WebRequest -Uri $Url -TimeoutSec 10 -UseBasicParsing -ErrorAction Stop
        return [int]$response.StatusCode
    } catch {
        if ($_.Exception.Response) {
            return [int]$_.Exception.Response.StatusCode
        }
        return 0
    }
}

function Show-Log {
    param([string]$Title, [string]$Path)
    Write-Host "----- $Title -----"
    if (Test-Path $Path) {
        Get-Content $Path | ForEach-Object { Write-Host $_ }
    }
    Write-Host "----- end $Title -----"
}

$failed = $false

# ── 1. Help ──────────────────────────────────────────────────────────────────
Write-InfoMsg "Running: dr build-studio --help"
$helpText = (dr build-studio --help 2>&1 | Out-String)
if ($LASTEXITCODE -ne 0) {
    Write-Host $helpText
    Write-ErrorMsg "dr build-studio --help exited with code $LASTEXITCODE"
    exit 1
}
foreach ($expected in @("Open Build Studio", "--yes")) {
    if (-not $helpText.Contains($expected)) {
        Write-Host $helpText
        Write-ErrorMsg "dr build-studio --help does not contain '$expected'"
        exit 1
    }
}
Write-SuccessMsg "dr build-studio --help prints the command's help"

# ── 2. Live start ────────────────────────────────────────────────────────────
$endpoint = if ($env:DATAROBOT_ENDPOINT) { $env:DATAROBOT_ENDPOINT } else { "https://app.datarobot.com/api/v2" }
$token = if ($env:DATAROBOT_API_TOKEN) { $env:DATAROBOT_API_TOKEN } else { $env:DR_API_TOKEN }

if ([string]::IsNullOrEmpty($token)) {
    Write-InfoMsg "Skipping the live start: neither DATAROBOT_API_TOKEN nor DR_API_TOKEN is set."
    exit 0
}

# Any HTTP answer, even an error status, means the host is reachable. In CI
# an unreachable host fails the test instead of skipping it, so the step
# can't pass without the live start having run.
$endpointHost = ([System.Uri]$endpoint).GetLeftPart([System.UriPartial]::Authority)
foreach ($probe in @($RegistryUrl, $endpointHost)) {
    if ((Get-HttpStatus -Url $probe) -eq 0) {
        if ($env:CI) {
            Write-ErrorMsg "$probe is not reachable, so the live start can't run."
            exit 1
        }
        Write-InfoMsg "Skipping the live start: $probe is not reachable."
        exit 0
    }
}

$sandbox = Join-Path ([System.IO.Path]::GetTempPath()) ("dr-build-studio-" + [System.Guid]::NewGuid().ToString("N"))
$null = New-Item -ItemType Directory -Force -Path $sandbox
$stdoutLog = Join-Path $sandbox "stdout.log"
$stderrLog = Join-Path $sandbox "stderr.log"
$stdinFile = Join-Path $sandbox "stdin.txt"
$null = New-Item -ItemType File -Force -Path $stdinFile

$port = Get-FreePort
$url = "http://127.0.0.1:$port/"

# Child processes inherit these. Saved so a local run gets its shell back.
$saved = @{
    XDG_CONFIG_HOME     = $env:XDG_CONFIG_HOME
    DATAROBOT_ENDPOINT  = $env:DATAROBOT_ENDPOINT
    DATAROBOT_API_TOKEN = $env:DATAROBOT_API_TOKEN
}
$env:XDG_CONFIG_HOME = $sandbox
$env:DATAROBOT_ENDPOINT = $endpoint
$env:DATAROBOT_API_TOKEN = $token

$proc = $null
try {
    Write-InfoMsg "Running: dr build-studio --yes --port $port --plain-output (sandbox $sandbox)"
    $dr = (Get-Command dr -CommandType Application | Select-Object -First 1).Source
    $proc = Start-Process -FilePath $dr `
        -ArgumentList @("build-studio", "--yes", "--port", "$port", "--plain-output") `
        -RedirectStandardInput $stdinFile `
        -RedirectStandardOutput $stdoutLog `
        -RedirectStandardError $stderrLog `
        -NoNewWindow -PassThru
    # Caching the handle now keeps ExitCode readable after dr exits.
    $null = $proc.Handle

    $deadline = (Get-Date).AddSeconds($StartTimeoutSec)
    $status = 0
    while ((Get-Date) -lt $deadline) {
        $status = Get-HttpStatus -Url $url
        if ($status -eq 200) {
            break
        }
        if ($proc.HasExited) {
            break
        }
        Start-Sleep -Seconds 3
    }

    if ($status -eq 200) {
        Write-SuccessMsg "Build Studio answered HTTP 200 on $url"
    } elseif ($proc.HasExited) {
        Write-ErrorMsg "dr build-studio exited with code $($proc.ExitCode) before answering on $url"
        $failed = $true
    } else {
        Write-ErrorMsg "Build Studio did not answer HTTP 200 on $url within $StartTimeoutSec s (last status: $status)"
        $failed = $true
    }

    $pluginsDir = Join-Path (Join-Path $sandbox "datarobot") "plugins"
    foreach ($name in @("xp", "opencode")) {
        if (Test-Path (Join-Path $pluginsDir $name)) {
            Write-SuccessMsg "Plugin '$name' is installed in the sandbox"
        } else {
            Write-ErrorMsg "Plugin '$name' is not installed in $pluginsDir"
            $failed = $true
        }
    }
} catch {
    Write-ErrorMsg "Live start failed: $_"
    $failed = $true
} finally {
    # dr -> powershell -> xp server -> dr opencode -> powershell -> opencode:
    # stop the whole tree, not just dr. Plugin processes left behind by a dr
    # that already exited run from the sandbox, so stop those by path.
    if ($proc -and -not $proc.HasExited) {
        & taskkill.exe /PID $proc.Id /T /F 2>&1 | Out-Null
        $null = $proc.WaitForExit(15000)
    }
    Get-Process -ErrorAction SilentlyContinue |
        Where-Object { $_.Path -and $_.Path.StartsWith($sandbox, [System.StringComparison]::OrdinalIgnoreCase) } |
        ForEach-Object { & taskkill.exe /PID $_.Id /T /F 2>&1 | Out-Null }

    if ($failed) {
        Show-Log -Title "dr build-studio stdout" -Path $stdoutLog
        Show-Log -Title "dr build-studio stderr" -Path $stderrLog
    }

    foreach ($key in $saved.Keys) {
        if ($null -eq $saved[$key]) {
            Remove-Item "Env:\$key" -ErrorAction SilentlyContinue
        } else {
            Set-Item "Env:\$key" $saved[$key]
        }
    }

    Remove-Item -Recurse -Force $sandbox -ErrorAction SilentlyContinue
}

if ($failed) {
    exit 1
}

Write-SuccessMsg "dr build-studio smoke test passed"
