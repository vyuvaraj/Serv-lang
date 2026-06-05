# build_release.ps1 - Builds fat release archives for all platforms
# Usage: powershell -File release-scripts\build_release.ps1 v1.0.0
# Run from the Serv-lang root directory.
#
# Each archive includes: serv binary, serv-lsp, runtime/, stdlib/, declarations/, go.mod, go.sum

param(
    [string]$Version = "dev"
)

$ErrorActionPreference = "Stop"
$srcDir = Get-Location
$distDir = Join-Path $srcDir "dist"

# Platforms to build
$platforms = @(
    @{ GOOS="windows"; GOARCH="amd64"; Ext=".exe" },
    @{ GOOS="linux";   GOARCH="amd64"; Ext="" },
    @{ GOOS="linux";   GOARCH="arm64"; Ext="" },
    @{ GOOS="darwin";  GOARCH="amd64"; Ext="" },
    @{ GOOS="darwin";  GOARCH="arm64"; Ext="" }
)

# Clean dist
Write-Host "Building Serv $Version for all platforms..." -ForegroundColor Cyan
Write-Host ""
if (Test-Path $distDir) { Remove-Item -Recurse -Force $distDir }
New-Item -ItemType Directory -Force -Path $distDir | Out-Null

foreach ($platform in $platforms) {
    $goos = $platform.GOOS
    $goarch = $platform.GOARCH
    $ext = $platform.Ext
    $archiveName = "serv-$goos-$goarch"
    $stageDir = Join-Path $distDir "stage-$goos-$goarch"

    Write-Host "  Building $goos/$goarch..." -ForegroundColor Green

    # Create staging directory
    New-Item -ItemType Directory -Force -Path $stageDir | Out-Null

    # Build compiler
    $env:GOOS = $goos
    $env:GOARCH = $goarch
    & go build -ldflags="-s -w" -o (Join-Path $stageDir "serv$ext") main.go
    if ($LASTEXITCODE -ne 0) { Write-Host "  FAILED: serv" -ForegroundColor Red; continue }

    # Build LSP
    & go build -ldflags="-s -w" -o (Join-Path $stageDir "serv-lsp$ext") ./lsp/
    if ($LASTEXITCODE -ne 0) { Write-Host "  FAILED: serv-lsp" -ForegroundColor Red; continue }

    # Reset env
    $env:GOOS = ""
    $env:GOARCH = ""

    # Copy runtime, stdlib, declarations, go.mod, go.sum
    Copy-Item -Recurse -Force "runtime" (Join-Path $stageDir "runtime")
    Copy-Item -Recurse -Force "stdlib" (Join-Path $stageDir "stdlib")
    if (Test-Path "declarations") {
        Copy-Item -Recurse -Force "declarations" (Join-Path $stageDir "declarations")
    }
    Copy-Item -Force "go.mod" (Join-Path $stageDir "go.mod")
    Copy-Item -Force "go.sum" (Join-Path $stageDir "go.sum")

    # Create archive
    if ($goos -eq "windows") {
        $zipPath = Join-Path $distDir "$archiveName.zip"
        Compress-Archive -Path "$stageDir\*" -DestinationPath $zipPath -Force
        Write-Host "    -> $archiveName.zip" -ForegroundColor DarkGray
    } else {
        $tarPath = Join-Path $distDir "$archiveName.tar.gz"
        & tar -czf $tarPath -C $stageDir .
        Write-Host "    -> $archiveName.tar.gz" -ForegroundColor DarkGray
    }

    # Cleanup staging
    Remove-Item -Recurse -Force $stageDir
}

Write-Host ""
Write-Host "Release archives:" -ForegroundColor Cyan
Get-ChildItem $distDir -Filter "serv-*" | ForEach-Object {
    $size = [math]::Round($_.Length / 1MB, 1)
    Write-Host "  $($_.Name)  ($size MB)" -ForegroundColor White
}

Write-Host ""
Write-Host "SHA256 hashes:" -ForegroundColor Cyan
Get-ChildItem $distDir -Filter "serv-*" | ForEach-Object {
    $hash = (Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLower()
    Write-Host "  $($_.Name): $hash" -ForegroundColor DarkGray
}

Write-Host ""
Write-Host "Next steps:" -ForegroundColor Yellow
Write-Host "  1. git tag v$Version && git push --tags"
Write-Host "  2. Create GitHub Release with tag v$Version"
Write-Host "  3. Upload archives from dist/"
Write-Host "  4. Copy SHA256 hashes into homebrew/serv.rb and scoop/serv.json"
Write-Host ""
