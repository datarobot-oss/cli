# DataRobot CLI - Shell Detection Test for PowerShell
# Run this from PowerShell to verify shell detection identifies "powershell" correctly
# Usage: .\run_shell_detection_test_powershell.ps1

$ErrorActionPreference = "Stop"

Write-Host ""
Write-Host "==================== Shell Detection Test (PowerShell) ===================="
Write-Host ""

Write-Host "[INFO] Running: dr --debug self version"
$debug_output = & { dr --debug self version 2>&1 } | Out-String

# Check if shell was detected as "powershell"
if ($debug_output -match 'Shell.*name=powershell') {
    Write-Host "[OK] " -NoNewline -ForegroundColor Green
    Write-Host "Shell detection correctly identified PowerShell"
    
    # Extract and display the shell detection line
    $shell_line = $debug_output -split "`n" | Where-Object { $_ -match 'Shell.*name=' } | Select-Object -First 1
    Write-Host "     $shell_line"
    
    Write-Host ""
    Write-Host "============================== END =============================="
    exit 0
} else {
    Write-Host "[ERROR] " -NoNewline -ForegroundColor Red
    Write-Host "Shell detection did not identify PowerShell"
    Write-Host "[ERROR] Expected 'name=powershell' in debug output"
    Write-Host "[ERROR] Actual debug output:"
    Write-Host $debug_output
    Write-Host ""
    Write-Host "============================== END =============================="
    exit 1
}
