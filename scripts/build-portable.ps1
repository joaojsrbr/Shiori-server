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

Write-Host "Running static checks and tests..." -ForegroundColor Cyan
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

# Remove artifacts produced by the former dual-build workflow.
foreach ($LegacyArtifact in @("dist\shiori-server-debug.exe", "dist\shiori-server-release.exe")) {
    if (Test-Path -LiteralPath $LegacyArtifact) {
        Remove-Item -LiteralPath $LegacyArtifact -Force
    }
}

Write-Host "Building dist\shiori-server.exe (Windows console application)..." -ForegroundColor Cyan

$env:CGO_ENABLED = "0"
$env:GOOS = "windows"
$env:GOARCH = "amd64"

# The default Go Windows target is the console subsystem. Do not pass
# '-H windowsgui': double-clicking the executable must keep its CMD visible.
$LdFlagsRelease = "$LdFlags -s -w"
go build -trimpath -buildvcs=false -ldflags="$LdFlagsRelease" -o dist\shiori-server.exe .\cmd\api

Write-Host "Build finished successfully: dist\shiori-server.exe" -ForegroundColor Green
