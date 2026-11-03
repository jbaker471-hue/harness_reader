# Trivex Harness Auditor repository guidance

## Purpose and boundaries

- This is an offline Windows x64 harness-drawing review application.
- Customer drawings, extracted customer content, screenshots, and production metadata must never be committed or used in automated tests.
- Use only the synthetic fixtures under `testdata/` for development and automated verification.
- Engineering output remains draft until a human verifies it against the source drawing and current manufacturer data.

## Toolchain

- Use exactly Go 1.24.7 with vendored modules.
- Keep `GOFLAGS=-mod=vendor`, `GOPROXY=off`, `GOTOOLCHAIN=local`, and `CGO_ENABLED=0` during verification.
- On this workstation, the verified portable compiler is:
  `C:\Users\Jordan\Documents\Codex\_toolchains\go1.24.7\go\bin\go.exe`
- `build_windows.ps1` discovers that portable compiler automatically when `go` is not on `PATH`. `TRIVEX_GO_EXE` may override the compiler path.

## Required verification

Run `powershell -ExecutionPolicy Bypass -File .\build_windows.ps1` before claiming a change is complete. It must run:

1. JavaScript UI regression checks.
2. `go vet ./...`.
3. The complete Go test suite.
4. The Windows GUI build.
5. The standalone executable API and shutdown smoke test.

For parser, extraction, lifecycle, concurrency, or stateful UI changes, also run:

```powershell
go test -shuffle=on -count=5 -timeout=20m ./...
```

## Bug-finding standard

A green happy-path test is not enough. For every reported defect or risky change:

- Reproduce the behavior before changing code when possible.
- Test empty, malformed, boundary, repeated-action, and stale-state cases adjacent to the reported path.
- Check startup, shutdown, unauthorized requests, unexpected origins, and missing drawing state when relevant.
- Exercise the built executable, not only package-level tests.
- Inspect browser-visible errors and backend responses separately so test-driver failures are not reported as app defects.
- Add or strengthen a deterministic regression test for every confirmed bug.
- Report exactly what was run, what was not run, and any remaining manual-only acceptance work.

Use `QA_CHECKLIST.md` for exploratory coverage that is not fully represented by the automated suite.

## Important files

- `assets/ui.html`: complete local browser UI.
- `main.go`: local HTTP server, security checks, endpoints, and lifecycle.
- `pdfium_engine.go` and `extraction_pipeline.go`: PDF rendering and extraction.
- `test_ui_regression.js`: JavaScript behavior checks.
- `scripts/smoke.js`: built-executable API, synthetic fixture, and shutdown checks.
- `BUILD.md`: build rationale and dependency provenance.

