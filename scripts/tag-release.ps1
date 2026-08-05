Param(
    [switch]$Push
)

$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$backDir = Split-Path -Parent $scriptDir
Set-Location $backDir

$allTags = git tag -l "v*"
$latestTag = $null

if ($allTags) {
    $latestTag = $allTags | Sort-Object { 
        $v = $_ -replace '^v',''
        try { [version]$v } catch { [version]"0.0.0" }
    } | Select-Object -Last 1
}

if (-not $latestTag) {
    $newTag = "v1.0.0"
} else {
    $versionStr = $latestTag -replace '^v',''
    $parts = $versionStr.Split('.')
    $major = [int]$parts[0]
    $minor = [int]$parts[1]
    $patch = [int]$parts[2]
    $newPatch = $patch + 1
    $newTag = "v$major.$minor.$newPatch"
}

Write-Host "Última tag encontrada: $(if ($latestTag) { $latestTag } else { 'Nenhuma' })" -ForegroundColor Cyan
Write-Host "Criando nova tag: $newTag" -ForegroundColor Green

git tag -a $newTag -m "Release $newTag"
Write-Host "Tag $newTag criada com sucesso localmente." -ForegroundColor Green

if ($Push) {
    Write-Host "Enviando $newTag para origin..." -ForegroundColor Yellow
    git push origin $newTag
    Write-Host "Tag $newTag enviada para o GitHub com sucesso!" -ForegroundColor Green
} else {
    Write-Host "`nPara enviar a tag para o GitHub e disparar o autobuild de release, execute:" -ForegroundColor Yellow
    Write-Host "  git push origin $newTag`n" -ForegroundColor White
}
