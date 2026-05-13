@echo off
REM DataRobot CLI - Shell Detection Test for cmd.exe
REM Run this from cmd.exe to verify shell detection identifies "cmd" correctly
REM Usage: run_shell_detection_test_cmd.bat

echo.
echo ==================== Shell Detection Test (cmd.exe) ====================
echo.

REM Run dr with debug to capture shell detection
echo [INFO] Running: dr --debug self version
dr --debug self version 2>&1 | findstr /C:"Shell" > shell_detection_output.txt

REM Check if shell was detected as "cmd"
findstr /C:"name=cmd" shell_detection_output.txt >nul 2>&1
if %ERRORLEVEL% EQU 0 (
    echo [OK] Shell detection correctly identified cmd.exe
    type shell_detection_output.txt
    del shell_detection_output.txt
    echo.
    echo ============================== END ==============================
    exit /b 0
) else (
    echo [ERROR] Shell detection did not identify cmd.exe
    echo [ERROR] Expected 'name=cmd' in debug output
    echo [ERROR] Actual output:
    type shell_detection_output.txt
    del shell_detection_output.txt
    echo.
    echo ============================== END ==============================
    exit /b 1
)
