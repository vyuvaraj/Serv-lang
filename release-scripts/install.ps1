# Serv Installer for Windows
# Usage: irm https://raw.githubusercontent.com/vyuvaraj/Serv-lang/main/release-scripts/install.ps1 | iex
# Or:    powershell -ExecutionPolicy Bypass -File install.ps1

param(
    [string]$Version = "latest",
    [string]$InstallDir = "$env:USERPROFILE\.serv"
)

$ErrorActionPreference = "Stop"

Write-Host "Installing Serv..." -ForegroundColor Cyan

# Determine download URL
$repo = "vyuvaraj/Serv-lang"
if ($Version -eq "latest") {
    $releaseUrl = "https://api.github.com/repos/$repo/releases/latest"
    try {
        $release = Invoke-RestMethod -Uri $releaseUrl
        $Version = $release.tag_name -replace '^v', ''
    } catch {
        Write-Host "Failed to fetch latest version. Specify version: -Version 1.0.0" -ForegroundColor Red
        exit 1
    }
}

$downloadUrl = "https://github.com/$repo/releases/download/v$Version/serv-windows-amd64.zip"
$zipPath = "$env:TEMP\serv-windows-amd64.zip"

Write-Host "  Downloading Serv v$Version..."
Invoke-WebRequest -Uri $downloadUrl -OutFile $zipPath

# Extract
Write-Host "  Extracting to $InstallDir..."
if (Test-Path $InstallDir) {
    Remove-Item -Recurse -Force $InstallDir
}
Expand-Archive -Path $zipPath -DestinationPath $InstallDir
Remove-Item $zipPath

# Add to PATH (user-level)
$currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($currentPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$InstallDir;$currentPath", "User")
    Write-Host "  Added $InstallDir to PATH" -ForegroundColor Green
}

# Set SERV_HOME
[Environment]::SetEnvironmentVariable("SERV_HOME", $InstallDir, "User")

Write-Host ""
Write-Host "Serv v$Version installed successfully!" -ForegroundColor Green
Write-Host ""
Write-Host "  Location:  $InstallDir" -ForegroundColor White
Write-Host "  SERV_HOME: $InstallDir" -ForegroundColor White
Write-Host ""
Write-Host "  Restart your terminal, then:" -ForegroundColor Yellow
Write-Host "    serv init myapp" -ForegroundColor White
Write-Host "    cd myapp" -ForegroundColor White
Write-Host "    serv run main.srv --watch" -ForegroundColor White
Write-Host ""
Write-Host "  Prerequisite: Go 1.18+ must be installed (https://go.dev/dl/)" -ForegroundColor DarkGray
