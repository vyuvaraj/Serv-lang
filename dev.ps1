# dev.ps1 — Developer utility script for Serv-lang
# Usage:
#   .\dev.ps1 build          — Build serv.exe + serv-lsp.exe
#   .\dev.ps1 vsix           — Package VS Code extension (.vsix)
#   .\dev.ps1 install        — Build + copy to PATH location
#   .\dev.ps1 test           — Run regression tests
#   .\dev.ps1 test-unit      — Run unit tests (Phase 2)
#   .\dev.ps1 fmt            — Format all .srv files
#   .\dev.ps1 lint           — Lint all examples
#   .\dev.ps1 clean          — Remove build artifacts
#   .\dev.ps1 release [ver]  — Full release build
#   .\dev.ps1 all            — Build + test + fmt check

param(
    [Parameter(Position=0)]
    [string]$Command = "help",
    [string]$Version = "dev"
)

$ErrorActionPreference = "Continue"
$env:GOPROXY = "off"

function Write-Step($msg) { Write-Host "  $msg" -ForegroundColor Cyan }
function Write-Ok($msg) { Write-Host "  $([char]0x2713) $msg" -ForegroundColor Green }
function Write-Err($msg) { Write-Host "  $([char]0x2717) $msg" -ForegroundColor Red }

switch ($Command) {

    "build" {
        Write-Host "Building Serv..." -ForegroundColor White
        Write-Step "serv.exe"
        go build -o serv.exe .
        if ($LASTEXITCODE -ne 0) { Write-Err "Failed"; exit 1 }
        Write-Step "serv-lsp.exe"
        go build -o serv-lsp.exe ./lsp/
        if ($LASTEXITCODE -ne 0) { Write-Err "Failed"; exit 1 }
        Write-Ok "Built: serv.exe + serv-lsp.exe"
    }

    "vsix" {
        Write-Host "Packaging VS Code extension..." -ForegroundColor White
        Push-Location vscode-support\extension
        if (-not (Get-Command vsce -ErrorAction SilentlyContinue)) {
            Write-Err "vsce not found. Install: npm install -g @vscode/vsce"
            Pop-Location; exit 1
        }
        vsce package --allow-missing-repository 2>&1 | Out-Null
        $vsix = Get-ChildItem *.vsix | Sort-Object LastWriteTime -Descending | Select-Object -First 1
        Pop-Location
        if ($vsix) {
            Write-Ok "Created: vscode-support\extension\$($vsix.Name)"
        } else {
            Write-Err "Failed to create .vsix"
        }
    }

    "install" {
        Write-Host "Building and installing..." -ForegroundColor White
        go build -o serv.exe .
        if ($LASTEXITCODE -ne 0) { Write-Err "Build failed"; exit 1 }
        go build -o serv-lsp.exe ./lsp/
        if ($LASTEXITCODE -ne 0) { Write-Err "LSP build failed"; exit 1 }
        $installDir = "C:\software\Serv-lang"
        if (-not (Test-Path $installDir)) {
            New-Item -ItemType Directory -Force -Path $installDir | Out-Null
        }
        Copy-Item -Force serv.exe $installDir\
        Copy-Item -Force serv-lsp.exe $installDir\
        Copy-Item -Recurse -Force runtime $installDir\runtime
        Copy-Item -Recurse -Force stdlib $installDir\stdlib
        Copy-Item -Force go.mod $installDir\
        Copy-Item -Force go.sum $installDir\
        if (Test-Path declarations) { Copy-Item -Recurse -Force declarations $installDir\declarations }
        Write-Ok "Installed to $installDir"
    }

    "test" {
        Write-Host "Running regression tests..." -ForegroundColor White
        & powershell -ExecutionPolicy Bypass -File test_regression.ps1
    }

    "test-unit" {
        Write-Host "Running unit tests..." -ForegroundColor White
        $testFiles = @("test_sample.srv", "examples\50_new_features.srv")
        foreach ($f in $testFiles) {
            if (Test-Path $f) {
                Write-Step $f
                & .\serv.exe test $f 2>&1 | Out-Null
                if ($LASTEXITCODE -eq 0) { Write-Ok "PASS" } else { Write-Err "FAIL" }
            }
        }
    }

    "fmt" {
        Write-Host "Formatting all .srv files..." -ForegroundColor White
        $files = Get-ChildItem -Recurse -Filter "*.srv" | Where-Object { $_.FullName -notlike "*.build*" }
        foreach ($f in $files) {
            & .\serv.exe fmt $f.FullName 2>&1 | Out-Null
        }
        Write-Ok "Formatted $($files.Count) files"
    }

    "lint" {
        Write-Host "Linting examples..." -ForegroundColor White
        $files = Get-ChildItem examples\*.srv
        $pass = 0; $fail = 0
        foreach ($f in $files) {
            & .\serv.exe lint $f.FullName 2>&1 | Out-Null
            if ($LASTEXITCODE -eq 0) { $pass++ } else { $fail++; Write-Err $f.Name }
        }
        Write-Ok "Lint: $pass passed, $fail failed"
    }

    "clean" {
        Write-Host "Cleaning..." -ForegroundColor White
        Remove-Item -Force serv.exe -ErrorAction SilentlyContinue
        Remove-Item -Force serv-lsp.exe -ErrorAction SilentlyContinue
        Remove-Item -Recurse -Force .build -ErrorAction SilentlyContinue
        Remove-Item -Recurse -Force dist -ErrorAction SilentlyContinue
        Remove-Item -Recurse -Force examples\.build -ErrorAction SilentlyContinue
        Remove-Item -Force vscode-support\extension\*.vsix -ErrorAction SilentlyContinue
        Write-Ok "Cleaned"
    }

    "release" {
        Write-Host "Building release $Version..." -ForegroundColor White
        & powershell -ExecutionPolicy Bypass -File release-scripts\build_release.ps1 $Version
    }

    "all" {
        Write-Host "=== Full Check ===" -ForegroundColor White
        Write-Host ""
        go build -o serv.exe .

        if ($LASTEXITCODE -ne 0) { Write-Err "Build failed"; exit 1 }
        go build -o serv-lsp.exe ./lsp/
        if ($LASTEXITCODE -ne 0) { Write-Err "LSP build failed"; exit 1 }
        Write-Ok "Build"
        Write-Host ""
        $testFiles = @("test_sample.srv", "examples\50_new_features.srv")
        foreach ($f in $testFiles) {
            if (Test-Path $f) {
                & .\serv.exe test $f 2>&1 | Out-Null
                if ($LASTEXITCODE -eq 0) { Write-Ok "Test: $f" } else { Write-Err "Test: $f" }
            }
        }
        Write-Host ""
        Write-Host "=== Done ===" -ForegroundColor Green
    }

    default {
        Write-Host "Serv Developer Utilities" -ForegroundColor Cyan
        Write-Host ""
        Write-Host "  .\dev.ps1 build        Build serv.exe + serv-lsp.exe"
        Write-Host "  .\dev.ps1 vsix         Package VS Code extension"
        Write-Host "  .\dev.ps1 install      Build + install to C:\software\Serv-lang"
        Write-Host "  .\dev.ps1 test         Run full regression suite"
        Write-Host "  .\dev.ps1 test-unit    Run unit tests only"
        Write-Host "  .\dev.ps1 fmt          Format all .srv files"
        Write-Host "  .\dev.ps1 lint         Lint all examples"
        Write-Host "  .\dev.ps1 clean        Remove build artifacts"
        Write-Host "  .\dev.ps1 release v1.0 Build release archives"
        Write-Host "  .\dev.ps1 all          Build + test + lint"
        Write-Host ""
    }
}
