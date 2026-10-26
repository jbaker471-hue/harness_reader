# Trivex Harness Auditor v2.8

Offline Windows x64 harness drawing review application. Download the Windows ZIP from GitHub Releases, extract it, and run `Trivex_Harness_Auditor_v2.8.exe`. No Go, Python, installer, or source upload is needed. Microsoft Edge is used for the application window.

All PDF interpretation runs locally in the executable through embedded PDFium WebAssembly. The server listens on 127.0.0.1 and requires a session token. No remote OCR, telemetry, or drawing uploads are implemented.

## Candidate status

Engineering results are DRAFT. Review source evidence before production. Catalog counts and assumed cavity geometry do not establish unused-cavity quantities. Tooling data includes chart-extracted, unverified routes; confirm the current manufacturer chart and positioner data plate. Customer drawing acceptance must occur locally on the owner's PC.

Connector cards provide a front/rear mirrored face view, case-sensitive cavity identifiers, near/far-end occupancy and unresolved/conflict indicators. The build plan compares contact/sealing-plug needs against the BOM and lists frame/positioner routes with AWG compatibility. Select the fill policy required by your drawing and process.

## Automated release

Each push to main runs JavaScript regression, Go vet and synthetic Go tests on Windows 2022 with Go 1.24.7 and vendored dependencies. The workflow builds a standalone Windows GUI executable, exercises its local APIs and shutdown signal, and publishes a prerelease only if those checks pass. Downloads include SHA256SUMS and the tested commit.

The executable smoke test uses headless test mode. It verifies the actual GUI-subsystem binary but does not certify interactive Edge window behavior or customer drawing recognition. Those checks remain pending local user feedback.

## Development

See BUILD.md. Keep customer drawings, extracted content, screenshots and production metadata out of this repository. Only synthetic fixtures belong in CI.
