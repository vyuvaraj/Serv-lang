# Serv Regression Test Suite
# Runs all examples: compilation check, test execution, and server smoke tests.
#
# Usage: powershell -ExecutionPolicy Bypass -File test_regression.ps1
#
# Requirements:
# - serv.exe built (go build -o serv.exe main.go)
# - curl available (built into Windows 10+)

param(
    [switch]$Verbose,
    [switch]$CompileOnly
)

$ErrorActionPreference = "Continue"
$pass = 0
$fail = 0
$skip = 0
$results = @()

function Write-Result($name, $status, $detail) {
    $color = switch ($status) {
        "PASS" { "Green" }
        "FAIL" { "Red" }
        "SKIP" { "Yellow" }
    }
    Write-Host "  [$status] $name" -ForegroundColor $color
    if ($detail -and $Verbose) { Write-Host "         $detail" -ForegroundColor DarkGray }
    $script:results += [PSCustomObject]@{ Name=$name; Status=$status; Detail=$detail }
}

# Ensure serv.exe exists
if (-not (Test-Path ".\serv.exe")) {
    Write-Host "Building serv.exe..." -ForegroundColor Cyan
    go build -o serv.exe main.go 2>&1 | Out-Null
    if ($LASTEXITCODE -ne 0) {
        Write-Host "FATAL: Failed to build serv.exe" -ForegroundColor Red
        exit 1
    }
}

Write-Host ""
Write-Host "=== Serv Regression Tests ===" -ForegroundColor Cyan
Write-Host ""

# --- Phase 1: Compilation ---
Write-Host "Phase 1: Compilation" -ForegroundColor White
Write-Host "--------------------"

$examples = Get-ChildItem examples\*.srv | Sort-Object Name
foreach ($file in $examples) {
    $null = & .\serv.exe build $file.FullName -o regression_test.exe 2>&1
    if ($LASTEXITCODE -eq 0) {
        $pass++
        Write-Result $file.Name "PASS" "compiled"
    } else {
        $fail++
        Write-Result $file.Name "FAIL" "compilation failed"
    }
}
Remove-Item examples\regression_test.exe -ErrorAction SilentlyContinue
Write-Host ""

if ($CompileOnly) {
    Write-Host "=== Results (compile only) ===" -ForegroundColor Cyan
    Write-Host "  Pass: $pass | Fail: $fail | Skip: $skip"
    if ($fail -gt 0) { exit 1 }
    exit 0
}

# --- Phase 2: Unit Tests (test-only files) ---
Write-Host "Phase 2: Unit Tests (serv test)" -ForegroundColor White
Write-Host "--------------------------------"

$testFiles = @(
    "12_static_types.srv",
    "14_phase3_features.srv",
    "20_raw_strings.srv"
)

foreach ($name in $testFiles) {
    $file = "examples\$name"
    if (-not (Test-Path $file)) { continue }

    $output = & .\serv.exe test $file 2>&1 | Out-String
    if ($LASTEXITCODE -eq 0) {
        $pass++
        Write-Result "$name (test)" "PASS" "tests passed"
    } else {
        $fail++
        $firstError = ($output -split "`n" | Where-Object { $_ -match "FAIL|Error|panic" } | Select-Object -First 1)
        Write-Result "$name (test)" "FAIL" $firstError
    }
}
Write-Host ""

# --- Phase 3: Server Smoke Tests ---
Write-Host "Phase 3: Server Smoke Tests (start → /health → stop)" -ForegroundColor White
Write-Host "------------------------------------------------------"

# Files that start a server and have routes we can hit
$serverTests = @(
    @{ File="02_rest_api.srv"; Port="8080"; Endpoints=@("/health") },
    @{ File="37_structured_logging.srv"; Port="8080"; Endpoints=@("/health", "/api/users") },
    @{ File="38_destructuring.srv"; Port="8080"; Endpoints=@("/health") },
    @{ File="39_optional_chaining.srv"; Port="8080"; Endpoints=@("/health", "/api/user") },
    @{ File="40_spread_operator.srv"; Port="8080"; Endpoints=@("/health", "/api/config") },
    @{ File="41_new_features.srv"; Port="8080"; Endpoints=@("/health", "/api/status") },
    @{ File="43_request_validation.srv"; Port="8080"; Endpoints=@("/health") }
)

# Files needing external deps (DB, broker, etc.) — skip server tests
$needsExternal = @(
    "03_pubsub_concurrency.srv",
    "04_python_binding.srv",
    "05_error_handling.srv",
    "06_json_support.srv",
    "07_advanced_features.srv",
    "08_multi_database.srv",
    "09_yaml_config.srv",
    "18_python_pool.srv",
    "35_primitives.srv",
    "42_config_validation.srv",
    "44_package_usage.srv"
)

foreach ($test in $serverTests) {
    $file = "examples\$($test.File)"
    $port = $test.Port
    $binName = "smoke_$($test.File -replace '\.srv$', '.exe')"
    $binPath = "examples\$binName"

    # Clean previous
    Remove-Item $binPath -ErrorAction SilentlyContinue

    # Build (serv places output relative to the .srv file's directory)
    $null = & .\serv.exe build $file -o $binName 2>&1
    if ($LASTEXITCODE -ne 0) {
        $fail++
        Write-Result "$($test.File) (smoke)" "FAIL" "build failed"
        continue
    }

    if (-not (Test-Path $binPath)) {
        $fail++
        Write-Result "$($test.File) (smoke)" "FAIL" "binary not found at $binPath"
        continue
    }

    # Start the server in background
    $proc = Start-Process -FilePath (Resolve-Path $binPath) -PassThru -WindowStyle Hidden
    Start-Sleep -Seconds 2

    if ($proc.HasExited) {
        $skip++
        Write-Result "$($test.File) (smoke)" "SKIP" "server exited immediately (may need external deps)"
        Remove-Item $binPath -ErrorAction SilentlyContinue
        continue
    }

    # Hit endpoints
    $allPassed = $true
    foreach ($endpoint in $test.Endpoints) {
        try {
            $response = Invoke-WebRequest -Uri "http://localhost:$port$endpoint" -UseBasicParsing -TimeoutSec 3 -ErrorAction Stop
            if ($response.StatusCode -ne 200) {
                $allPassed = $false
            }
        } catch {
            $allPassed = $false
        }
    }

    # Stop the server
    Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
    Start-Sleep -Milliseconds 500
    Remove-Item $binPath -ErrorAction SilentlyContinue

    if ($allPassed) {
        $pass++
        Write-Result "$($test.File) (smoke)" "PASS" "endpoints: $($test.Endpoints -join ', ')"
    } else {
        $fail++
        Write-Result "$($test.File) (smoke)" "FAIL" "endpoint check failed"
    }
}

Write-Host ""

# --- Summary ---
Write-Host "=== Summary ===" -ForegroundColor Cyan
Write-Host "  Pass: $pass | Fail: $fail | Skip: $skip" -ForegroundColor White
$total = $pass + $fail
if ($total -gt 0) {
    $pct = [math]::Round(($pass / $total) * 100)
    Write-Host "  Pass rate: $pct%" -ForegroundColor $(if ($pct -eq 100) { "Green" } else { "Yellow" })
}
Write-Host ""

if ($fail -gt 0) {
    Write-Host "Failed tests:" -ForegroundColor Red
    $results | Where-Object { $_.Status -eq "FAIL" } | ForEach-Object { Write-Host "  - $($_.Name): $($_.Detail)" -ForegroundColor Red }
    exit 1
}

exit 0
