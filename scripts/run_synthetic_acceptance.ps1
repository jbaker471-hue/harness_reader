$ErrorActionPreference = "Stop"
$env:GOFLAGS = "-mod=vendor -buildvcs=false"
$env:GOPROXY = "off"
$env:GOTOOLCHAIN = "local"
$env:CGO_ENABLED = "0"
$go = "C:\Users\Jordan\Documents\Codex\_toolchains\go1.24.7\go\bin\go.exe"
& $go test -run TestSyntheticAcceptancePack -count=1 -v .
exit $LASTEXITCODE
