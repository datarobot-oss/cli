# Windows Smoke Tests

This directory contains Windows-specific smoke tests for the DataRobot CLI.

## Running All Smoke Tests

From the project root on Windows:

```powershell
task smoke-test-windows
```

This requires the `DR_API_TOKEN` environment variable to be set.

## Shell Detection Tests

The CLI detects which shell it's running from (PowerShell, cmd.exe, pwsh) by inspecting the parent process using `tasklist`. These standalone tests verify shell detection works correctly:

### PowerShell Detection Test

```powershell
.\smoke_test_scripts\windows\run_shell_detection_test_powershell.ps1
```

Expected output: `Shell detection correctly identified PowerShell`

### cmd.exe Detection Test

```cmd
.\smoke_test_scripts\windows\run_shell_detection_test_cmd.bat
```

Expected output: `Shell detection correctly identified cmd.exe`

## PowerShell Profile Completion Test

`run_powershell_profile_test.ps1` installs completion into the PowerShell profile and checks that a new session loads it without printing anything. It runs for PowerShell 7 (`pwsh`), Windows PowerShell 5.1 (`powershell.exe`), and a machine that has both, then uninstalls, with `dr` and with `uninstall.ps1`, and checks that the profile still loads. It needs `dr`, `pwsh`, and `powershell.exe` on `PATH`, and it replaces the PowerShell profiles, so it runs only where `CI` is set:

```powershell
$env:CI = "1"; .\smoke_test_scripts\windows\run_powershell_profile_test.ps1
```

## Build Studio Test

`run_build_studio_test.ps1` checks that `dr build-studio --help` prints the command's help, then runs `dr build-studio --yes --port <free port>`: it installs the `xp` and `opencode` plugins from the public plugin registry into a temporary `XDG_CONFIG_HOME` and waits up to 4 minutes for Build Studio to answer HTTP 200 on that port. It stops the whole process tree and removes the sandbox afterwards, so your own plugins and config are untouched. `xp` requires DataRobot credentials: set `DATAROBOT_API_TOKEN` (or `DR_API_TOKEN`), and `DATAROBOT_ENDPOINT` for an instance other than `https://app.datarobot.com/api/v2`. Without a token the live part is skipped. When the registry or DataRobot is unreachable, it is skipped too, except under CI (`CI` set), where that fails the test:

```powershell
$env:DR_API_TOKEN = "<token>"; .\smoke_test_scripts\windows\run_build_studio_test.ps1
```

## Files

- `run_smoke_test.ps1` - Main smoke test suite (includes shell detection)
- `run_powershell_profile_test.ps1` - Standalone PowerShell profile completion test
- `run_build_studio_test.ps1` - `dr build-studio` help and live start
- `run_shell_detection_test_powershell.ps1` - Standalone PowerShell detection test
- `run_shell_detection_test_cmd.bat` - Standalone cmd.exe detection test

## How Shell Detection Works on Windows

1. The CLI calls `os.Getppid()` to get the parent process ID
2. Runs `tasklist /FI "PID eq <ppid>" /NH /FO CSV` to query the parent process
3. Parses the CSV output to extract the process name (e.g., "powershell.exe", "cmd.exe")
4. Strips `.exe` and lowercases the name
5. Logs the result in debug mode: `Shell name=<shell>`

This approach works reliably because it inspects the actual running process, not environment variables (which can be misleading - e.g., `$env:PSModulePath` exists system-wide even when running from cmd.exe).
