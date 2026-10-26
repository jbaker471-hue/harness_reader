#!/bin/sh
# Offline, reproducible Windows x64 cross-build from Linux/macOS
set -e
export GOFLAGS=-mod=vendor GOPROXY=off
go vet ./...
go test -count=1 ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-H=windowsgui -s -w" -o Trivex_Harness_Auditor_v2.8.exe .
sha256sum Trivex_Harness_Auditor_v2.8.exe
