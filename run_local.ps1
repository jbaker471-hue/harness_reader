# One-click local run for Trivex Harness Auditor on Windows.
#   run_local.ps1               full gate (JS, vet, Go tests, build, smoke test) then start the app
#   run_local.ps1 -SkipChecks   rebuild and start only
#   run_local.ps1 -Shuffle      also run go test -shuffle=on -count=5
param([switch]$SkipChecks, [switch]$Shuffle)
$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
$log = Join-Path $PSScriptRoot "local_run.log"   # *.log is git-ignored
if ($SkipChecks) {
    $env:GOFLAGS = "-mod=vendor -buildvcs=false"; $env:GOPROXY = "off"; $env:GOTOOLCHAIN = "local"; $env:CGO_ENABLED = "0"
    $env:GOOS = "windows"; $env:GOARCH = "amd64"
    go build -trimpath -ldflags="-H=windowsgui -s -w" -o Trivex_Harness_Auditor_v2.8.exe .
    if ($LASTEXITCODE -ne 0) { throw "Build failed" }
} else {
    & "$PSScriptRoot\build_windows.ps1" 2>&1 | Tee-Object -FilePath $log
    if ($LASTEXITCODE -ne 0) { throw "Checks failed; see $log" }
}
if ($Shuffle) {
    $env:GOFLAGS = "-mod=vendor -buildvcs=false"; $env:GOPROXY = "off"; $env:GOTOOLCHAIN = "local"; $env:CGO_ENABLED = "0"
    Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
    go test -shuffle=on -count=5 -timeout=20m ./...
    if ($LASTEXITCODE -ne 0) { throw "Shuffled tests failed" }
}
Start-Process -FilePath (Join-Path $PSScriptRoot "Trivex_Harness_Auditor_v2.8.exe")
Write-Host "Started. Server binds 127.0.0.1 only. Log: $log"
