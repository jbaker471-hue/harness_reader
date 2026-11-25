# One-click local run for Trivex Harness Auditor on Windows.
#   run_local.ps1               full gate (JS, vet, Go tests, build, smoke test) then start the app
#   run_local.ps1 -SkipChecks   rebuild and start only
#   run_local.ps1 -Shuffle      also run go test -shuffle=on -count=5
#   run_local.ps1 -DrawingsDir <folder>   also run the regression ratchet on your drawings (numbers only;
#                               the baseline lives in your user configuration folder, never the repo)
#       -SaveBaseline           record new drawings and keep improvements (a regression is never saved)
#       -AcceptRegression       with -SaveBaseline, accept a regression on purpose
param([switch]$SkipChecks, [switch]$Shuffle, [string]$DrawingsDir = "", [switch]$SaveBaseline, [switch]$AcceptRegression)
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
if ($DrawingsDir) {
    if (-not (Test-Path -LiteralPath $DrawingsDir)) { throw "Drawings folder not found: $DrawingsDir" }
    $env:GOFLAGS = "-mod=vendor -buildvcs=false"; $env:GOPROXY = "off"; $env:GOTOOLCHAIN = "local"; $env:CGO_ENABLED = "0"
    Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
    $env:TRIVEX_DRAWINGS_DIR = $DrawingsDir
    $ratchetOut = Join-Path $PSScriptRoot "ratchet_report.log"   # *.log is git-ignored; numbers only
    $env:TRIVEX_RATCHET_OUT = $ratchetOut
    if ($SaveBaseline) { $env:TRIVEX_RATCHET_SAVE = "1" }
    if ($AcceptRegression) { $env:TRIVEX_RATCHET_ACCEPT = "1" }
    go test -count=1 -run TestLocalRatchet .
    $ratchetExit = $LASTEXITCODE
    Remove-Item Env:TRIVEX_DRAWINGS_DIR, Env:TRIVEX_RATCHET_OUT, Env:TRIVEX_RATCHET_SAVE, Env:TRIVEX_RATCHET_ACCEPT -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $ratchetOut) { Get-Content -LiteralPath $ratchetOut }
    if ($ratchetExit -ne 0) { throw "Ratchet: a reading got worse. Nothing was started. Fix it, or rerun with -SaveBaseline -AcceptRegression if it is intended." }
}
Start-Process -FilePath (Join-Path $PSScriptRoot "Trivex_Harness_Auditor_v2.8.exe")
Write-Host "Started. Server binds 127.0.0.1 only. Log: $log"
