<#
.SYNOPSIS
Builds the Shiori backend as a single portable executable for Windows.

.DESCRIPTION
This script compiles the Shiori backend setting CGO_ENABLED=0, targeting Windows amd64.
It automatically discovers the current Git commit, dirty state, and build date, injecting
them into the binary via -ldflags. The final artifact is placed at \dist\shiori-server.exe.
#>

$ErrorActionPreference = "Stop"

# Navigate to backend root (script is in scripts/)
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$BackDir = Split-Path -Parent $ScriptDir
Set-Location -Path $BackDir

Write-Host "Checking environment and format..." -ForegroundColor Cyan
go mod tidy
gofmt -w .
go vet ./...
go test ./...

Write-Host "Extracting build metadata..." -ForegroundColor Cyan

# Fetch version info
$Version = "1.0.0" # Can be loaded from a file or tag
$Commit = "unknown"
$Dirty = "false"
$BuildDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")

try {
    $Commit = (git rev-parse HEAD).Trim()
    $Status = (git status --porcelain)
    if ($Status) {
        $Dirty = "true"
    }
} catch {
    Write-Warning "Git not found or not in a git repository. Using 'unknown' for commit."
}

Write-Host "Version: $Version"
Write-Host "Commit: $Commit"
Write-Host "Dirty: $Dirty"
Write-Host "Date: $BuildDate"

$PackagePath = "github.com/joaojsr/shiori-server/internal/buildinfo"
$LdFlags = "-X ${PackagePath}.Version=$Version -X ${PackagePath}.Commit=$Commit -X ${PackagePath}.BuildDate=$BuildDate -X ${PackagePath}.Dirty=$Dirty"

# Ensure dist exists
if (!(Test-Path "dist")) {
    New-Item -ItemType Directory -Path "dist" | Out-Null
}

Write-Host "Building shiori-server-debug.exe..." -ForegroundColor Cyan

$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "amd64"

# Debug build (no optimization flags, keep terminal)
go build -trimpath -buildvcs=false -ldflags="$LdFlags" -o dist\shiori-server-debug.exe .\cmd\api

Write-Host "Building shiori-server-release.exe..." -ForegroundColor Cyan

# Release build (strips debug symbols for smaller size, keeps terminal)
$LdFlagsRelease = "$LdFlags -s -w"
go build -trimpath -buildvcs=false -ldflags="$LdFlagsRelease" -o dist\shiori-server-release.exe .\cmd\api

Write-Host "Build finished successfully: dist\shiori-server-debug.exe and dist\shiori-server-release.exe" -ForegroundColor Green
