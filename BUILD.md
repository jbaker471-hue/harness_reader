# Building Trivex Harness Auditor v2.8-candidate

The build is fully **offline and reproducible**. Every dependency is vendored in `vendor/`
(`go build` uses it automatically). No CGO, no C compiler, no native DLLs, no Python.

## Toolchain

- Go **1.24.x** (built and tested with go1.24.7). `go.mod` declares `go 1.24.0`, because
  go-pdfium v1.17.3 and wazero v1.11.0 require Go 1.24.

## Commands (PowerShell, from `source\`)

```powershell
$env:GOFLAGS="-mod=vendor"; $env:GOPROXY="off"
go vet ./...
go test -count=1 ./...
$env:GOOS="windows"; $env:GOARCH="amd64"; $env:CGO_ENABLED="0"
go build -trimpath -ldflags="-H=windowsgui -s -w" -o Trivex_Harness_Auditor_v2.8.exe .
```

The same steps are in `build_windows.ps1` and `build.sh`.

Optional local regression on the authorized 9-page drawing:
`$env:TRIVEX_REGRESSION_PDF="C:\path\to\authorized\copy.pdf"; go test -count=1 ./...`

Diagnostics: `TRIVEX_PDF_INTERPRETER=1` forces wazero's interpreter (no in-memory native code).
`TRIVEX_NO_WINDOW=1` runs the backend headless and prints its URL and token (for automated tests).

## Pinned dependencies (vendored)

| Module | Version | Upstream commit | License |
|---|---|---|---|
| github.com/klippa-app/go-pdfium | v1.17.3 | f5dcefb7ac05ad44e4e9013a2114edfa5811b1c6 | MIT |
| — embedded `webassembly/pdfium.wasm` | PDFium chromium/7323 | (from go-pdfium v1.17.3) | BSD-3-Clause (PDFium) + bundled third-party, see THIRD_PARTY_NOTICES |
| github.com/tetratelabs/wazero | v1.11.0 | fe2e75192a79e7ff9f583825ed92a6e1fa3293d8 | Apache-2.0 |
| github.com/jolestar/go-commons-pool/v2 | v2.1.2 | 3f25f8504f93d8cf32aab01b7125c18fca53f646 | Apache-2.0 |
| github.com/google/uuid | v1.6.0 | 0f11ee6918f41a04c201eceeadf612a377bc7fbc | BSD-3-Clause |
| golang.org/x/net | v0.50.0 | ebddb99633e0fc35d135f62e9400678492c1d3be | BSD-3-Clause |
| golang.org/x/sys | v0.41.0 | fc646e489fd944b6f77d327ab77f1a4bab81d5ad | BSD-3-Clause |
| golang.org/x/text | v0.34.0 | 817fba9abd337b4d9097b10c61a540c74feaaeff | BSD-3-Clause |

Provenance: the module sources were taken from the upstream Git repositories (GitHub mirrors of
golang.org/x/*) at the tags above, because the build environment could not reach the Go module
proxy. `go.sum` is therefore not included. Vendor mode does not use it. To re-verify against
the public checksum database on a connected machine, delete `vendor/`, run `go mod tidy` and
`go mod vendor`, and diff the result. The package source must be identical.

## Source map (v2.7 additions)

| File | Role |
|---|---|
| `pdfium_engine.go` | PDFium start-up (empty filesystem, compiler with interpreter fallback); display-space transform from FPDF_PageToDevice; glyph extraction and geometric grouping; Form XObject traversal with composed matrices; picture placement/orientation; vector paths and circles; rendering |
| `extraction_pipeline.go` | `ExtractDocument` (single source of truth), v2.6 in-memory comparison, in-memory session (no disk) |
| `pinout_tables.go` | Header-grounded pinout / wire-list reader; corroboration merge |
| `evidence.go` | Label locations, insert-view candidates, cross-source endpoint evidence, finding categories, interpretation status, per-sheet diagnostics |
| `region.go` | Page-review selection reading (grid, roles, auto rows); picture gallery from rendered page; `AppVersion` |
| `selftest.go` | Reference-library schema validation; built-in orientation self-test |
| `synth_fixtures.go` | Deterministic NON-CUI PDF generator (10 encodings + outlined-text negative) |
| `main.go`, `proc_*.go` | Local server, endpoints, window lifecycle, Edge process-tree termination |
| `assets/ui.html` | Full workspace UI (Page review, interpretation status, categorized alerts, …) |
