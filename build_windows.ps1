# Offline, reproducible Windows x64 build of Trivex Harness Auditor v2.8-candidate
$ErrorActionPreference = "Stop"
$env:GOFLAGS = "-mod=vendor"; $env:GOPROXY = "off"
go vet ./...
go test -count=1 ./...
$env:GOOS = "windows"; $env:GOARCH = "amd64"; $env:CGO_ENABLED = "0"
go build -trimpath -ldflags="-H=windowsgui -s -w" -o Trivex_Harness_Auditor_v2.8.exe .
Get-FileHash Trivex_Harness_Auditor_v2.8.exe -Algorithm SHA256
