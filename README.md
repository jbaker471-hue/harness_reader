# Trivex Harness Auditor — Windows v2.8

Offline Windows x64 harness drawing and connector tooling analysis app.

## Build and download v2.8

The GitHub Actions Windows pipeline is configured for Go 1.24.7 and runs UI regression tests, `go vet`, Go unit tests and a Windows GUI build. It publishes `Trivex_Harness_Auditor_v2.8.exe` as a downloadable workflow artifact when successful.

The full source package has **not yet been uploaded**. To start the build, upload the single file `Trivex_v2.8_GitHub_Windows_Build.zip` to the root of this repository using **Add file → Upload files → Commit changes**. The build workflow automatically extracts the ZIP and uses its vendored dependencies. You can then download the executable under **Actions → Trivex Auditor v2.8 Windows Build → latest successful run → Artifacts**.

The package may also be extracted into the repository root by a developer. Keep the `vendor/`, `assets/`, and `testdata/` folders intact.

## Data handling

**Do not upload proprietary customer drawings, controlled unclassified information (CUI), customer-derived extracts, or drawing screenshots.** Only approved application source and synthetic fixtures belong here. The app is designed to process customer drawings locally without cloud OCR or telemetry.

## Release verification

A successful unit test and build are not the same as user acceptance. Windows application start, browser UI behavior, PDF recognition, connector cavity mapping and output validation require separate checks. Results based on controlled drawings must be validated locally by an authorized user.
