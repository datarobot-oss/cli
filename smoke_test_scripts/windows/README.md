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

## Files

- `run_smoke_test.ps1` - Main smoke test suite (includes shell detection)
- `run_shell_detection_test_powershell.ps1` - Standalone PowerShell detection test
- `run_shell_detection_test_cmd.bat` - Standalone cmd.exe detection test

## How Shell Detection Works on Windows

1. The CLI calls `os.Getppid()` to get the parent process ID
2. Runs `tasklist /FI "PID eq <ppid>" /NH /FO CSV` to query the parent process
3. Parses the CSV output to extract the process name (e.g., "powershell.exe", "cmd.exe")
4. Strips `.exe` and lowercases the name
5. Logs the result in debug mode: `Shell name=<shell>`

This approach works reliably because it inspects the actual running process, not environment variables (which can be misleading - e.g., `$env:PSModulePath` exists system-wide even when running from cmd.exe).
