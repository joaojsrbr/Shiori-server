<#
.SYNOPSIS
Smoke test for the Shiori Portable Executable.

.DESCRIPTION
This script copies the generated shiori-server.exe to an isolated temporary directory,
starts it, verifies the endpoints (/health/live, /health/ready, /api/v1/capabilities),
checks if the SQLite database was successfully created, and cleans up.
#>

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$BackDir = Split-Path -Parent $ScriptDir
$ExePath = Join-Path -Path $BackDir -ChildPath "dist\shiori-server.exe"

if (!(Test-Path $ExePath)) {
    Write-Error "Executable not found at $ExePath. Run build-portable.ps1 first."
    exit 1
}

$TempDir = Join-Path -Path $env:TEMP -ChildPath "shiori_smoke_test_$(Get-Random)"
New-Item -ItemType Directory -Path $TempDir | Out-Null
Write-Host "Created isolated environment at: $TempDir" -ForegroundColor Cyan

$TestExe = Join-Path -Path $TempDir -ChildPath "shiori-server.exe"
Copy-Item -Path $ExePath -Destination $TestExe

Write-Host "Testing version command..." -ForegroundColor Cyan
$VersionOutput = & $TestExe version
Write-Host $VersionOutput
if ($VersionOutput -notmatch "shiori-server") {
    Write-Error "Version command failed."
}

Write-Host "Starting server..." -ForegroundColor Cyan
$ServerProcess = Start-Process -FilePath $TestExe -ArgumentList "serve --profile portable --port 8080 --log-level info --log-format text" -WorkingDirectory $TempDir -PassThru -WindowStyle Hidden

# Give it a few seconds to start and run migrations
Start-Sleep -Seconds 3

try {
    if ($ServerProcess.HasExited) {
        Write-Error "Server process exited prematurely."
    }

    Write-Host "Testing /health/live..." -ForegroundColor Cyan
    $Live = Invoke-RestMethod -Uri "http://127.0.0.1:8080/health/live"
    if ($Live.status -ne "alive") { Write-Error "Live check failed." }

    Write-Host "Testing /health/ready..." -ForegroundColor Cyan
    $Ready = Invoke-RestMethod -Uri "http://127.0.0.1:8080/health/ready"
    if ($Ready.status -ne "ready") { Write-Error "Ready check failed." }

    Write-Host "Testing /api/v1/capabilities..." -ForegroundColor Cyan
    $Caps = Invoke-RestMethod -Uri "http://127.0.0.1:8080/api/v1/capabilities"
    if ($Caps.profile -ne "portable") { Write-Error "Profile should be portable." }

    Write-Host "Testing that debug endpoint is disabled in info mode..." -ForegroundColor Cyan
    try {
        Invoke-WebRequest -Uri "http://127.0.0.1:8080/api/v1/debug/extract" -Method Post -ContentType "application/json" -Body '{}' -ErrorAction Stop | Out-Null
        Write-Error "Debug endpoint must not be registered in info mode."
    } catch {
        if ($_.Exception.Response.StatusCode.value__ -ne 404) {
            throw
        }
    }
    
    # Check if DB was created
    $DbPath = Join-Path -Path $TempDir -ChildPath "data\shiori.db"
    if (!(Test-Path $DbPath)) {
        Write-Error "shiori.db was not created at expected location: $DbPath"
    } else {
        Write-Host "Verified shiori.db creation: $DbPath" -ForegroundColor Green
    }

    Write-Host "Restarting in debug mode to verify optional route mounting..." -ForegroundColor Cyan
    Stop-Process -Id $ServerProcess.Id -Force -ErrorAction SilentlyContinue
    Wait-Process -Id $ServerProcess.Id -ErrorAction SilentlyContinue

    $DebugProcess = Start-Process -FilePath $TestExe -ArgumentList "serve --profile portable --port 18080 --log-level debug --log-format text" -WorkingDirectory $TempDir -PassThru -WindowStyle Hidden
    Start-Sleep -Seconds 3
    try {
        if ($DebugProcess.HasExited) {
            Write-Error "Server process exited prematurely in debug mode."
        }

        $DebugStatus = 0
        try {
            Invoke-WebRequest -Uri "http://127.0.0.1:18080/api/v1/debug/extract" -Method Post -ContentType "application/json" -Body '{}' -ErrorAction Stop | Out-Null
        } catch {
            $DebugStatus = $_.Exception.Response.StatusCode.value__
        }
        if ($DebugStatus -ne 400) {
            Write-Error "Debug endpoint should be registered and return 400 for an invalid payload; got $DebugStatus."
        }
    } finally {
        Stop-Process -Id $DebugProcess.Id -Force -ErrorAction SilentlyContinue
    }
    
    Write-Host "Smoke test PASSED." -ForegroundColor Green
} finally {
    Write-Host "Stopping server..." -ForegroundColor Cyan
    Stop-Process -Id $ServerProcess.Id -Force -ErrorAction SilentlyContinue
    
    Start-Sleep -Seconds 2
    Write-Host "Cleaning up $TempDir..." -ForegroundColor Cyan
    Remove-Item -Path $TempDir -Recurse -Force -ErrorAction SilentlyContinue
}
