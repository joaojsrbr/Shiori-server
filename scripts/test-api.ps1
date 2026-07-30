<#
.SYNOPSIS
Script de teste rápido para a API do Shiori Server.

.DESCRIPTION
Envia requisições HTTP para os principais endpoints do servidor (Liveness, Capabilities e Jobs)
e exibe as respostas em formato JSON.

.EXAMPLE
.\test-api.ps1
#>

$ErrorActionPreference = "Stop"
$BaseUrl = "http://127.0.0.1:8080"

Write-Host "=========================================" -ForegroundColor Cyan
Write-Host "1. Testando Health Check (/health/ready)" -ForegroundColor Cyan
try {
    $health = Invoke-RestMethod -Uri "$BaseUrl/health/ready" -Method Get
    $health | ConvertTo-Json | Write-Host -ForegroundColor Green
} catch {
    Write-Warning "Falha ao acessar /health/ready. O servidor está rodando?"
    Write-Warning $_.Exception.Message
    exit
}

Write-Host "`n=========================================" -ForegroundColor Cyan
Write-Host "2. Testando Capabilities (/api/v1/capabilities)" -ForegroundColor Cyan
try {
    $caps = Invoke-RestMethod -Uri "$BaseUrl/api/v1/capabilities" -Method Get
    $caps | ConvertTo-Json -Depth 5 | Write-Host -ForegroundColor Green
} catch {
    Write-Warning "Falha ao acessar /capabilities."
}

Write-Host "`n=========================================" -ForegroundColor Cyan
Write-Host "3. Disparando Job de Extração (/api/v1/jobs/extract)" -ForegroundColor Cyan
try {
    $body = @{
        url = "https://lycantoons.com/series/defensor-da-dungeon"
        target = "media"
    } | ConvertTo-Json

    $jobResponse = Invoke-RestMethod -Uri "$BaseUrl/api/v1/jobs/extract" -Method Post -Body $body -ContentType "application/json"
    $jobResponse | ConvertTo-Json | Write-Host -ForegroundColor Green
    Write-Host "Job enfileirado com sucesso! Verifique os logs do servidor para acompanhar o Worker." -ForegroundColor Yellow
} catch {
    Write-Warning "Falha ao disparar o Job de extração."
    Write-Warning $_.Exception.Message
}
