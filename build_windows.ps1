# Offline, reproducible Windows x64 build of Trivex Harness Auditor v2.8-candidate
$ErrorActionPreference = "Stop"
$goExe = $env:TRIVEX_GO_EXE
if (-not $goExe) {
    $goCommand = Get-Command go -ErrorAction SilentlyContinue
    if ($goCommand) {
        $goExe = $goCommand.Source
    }
}
if (-not $goExe) {
    $portableGo = Join-Path $env:USERPROFILE "Documents\Codex\_toolchains\go1.24.7\go\bin\go.exe"
    if (Test-Path -LiteralPath $portableGo) {
        $goExe = $portableGo
    }
}
if (-not $goExe -or -not (Test-Path -LiteralPath $goExe)) {
    throw "Go 1.24.7 was not found. Install it or set TRIVEX_GO_EXE to go.exe."
}

$goVersion = & $goExe version
if ($LASTEXITCODE -ne 0 -or $goVersion -notmatch 'go1\.24\.7\s') {
    throw "Expected Go 1.24.7, got: $goVersion"
}

$env:GOFLAGS = "-mod=vendor"
$env:GOPROXY = "off"
$env:GOTOOLCHAIN = "local"
$env:CGO_ENABLED = "0"

node test_ui_regression.js
if ($LASTEXITCODE -ne 0) { throw "JavaScript regression failed" }

& $goExe vet ./...
if ($LASTEXITCODE -ne 0) { throw "Go vet failed" }

& $goExe test -count=1 -timeout=20m ./...
if ($LASTEXITCODE -ne 0) { throw "Go tests failed" }

$env:GOOS = "windows"
$env:GOARCH = "amd64"
& $goExe build -trimpath -ldflags="-H=windowsgui -s -w" -o Trivex_Harness_Auditor_v2.8.exe .
if ($LASTEXITCODE -ne 0) { throw "Windows build failed" }

node scripts/smoke.js .\Trivex_Harness_Auditor_v2.8.exe
if ($LASTEXITCODE -ne 0) { throw "Executable smoke test failed" }
Get-FileHash Trivex_Harness_Auditor_v2.8.exe -Algorithm SHA256
