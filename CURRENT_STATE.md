# Trivex Harness Auditor: current state

Last verified: 2026-10-29 on Windows 11 x64, Go 1.24.7 (portable toolchain), synthetic fixtures only.

## Source state

- Branch `main` at `1c5fc5e` (PR #12), identical to `origin/main`.
- All 10 `feature/*` and `fix/*` branches (including `fix/full-height-schematic-columns`, commits `7388a77` and `43da8ee`) were squash-merged into main: each is N commits ahead and N behind. Nothing unmerged was found by commit count; content equivalence was not diffed.
- Uncommitted working tree: 23 modified files (required-tooling logic in `accessory_check.go`, `tables_generic.go`, `table_scan.go`, `adapter.go`, `assets/ui.html`, new `required_tooling_test.go`, plus `.gitignore`, `BUILD.md`, `build.sh`, `build_windows.ps1`, `scripts/smoke.js`). Not on any branch or PR yet.
- App version string: v2.8 (candidate).

## Verified (this session, with the uncommitted changes applied)

`build_windows.ps1` exit 0:

- JavaScript UI regression: pass.
- `go vet ./...`: pass.
- `go test -count=1 ./...`: pass.
- Windows GUI build: pass. SHA-256 `D7F9FA5EBB0C01CDB8E4DD3294455D2518349156058032B6E868B2FB5AE999F4`.
- Executable smoke test (offline PDF engine, token protection, synthetic PDF API, build plan, shutdown): pass.
- `go test -shuffle=on -count=5 ./...`: pass.

## UI changes (uncommitted)

- Sidebar grouped by workflow: Inspect the drawing / Quote / Build / Project; sidebar now scrolls instead of overlapping the version footer.
- Each tab now sets its own plain-language description (previously the intake text stayed on every tab).
- Intake has a "Where this job stands" card row (weaknesses, connectors, conductors, BOM, release checks) with jump links.
- Regression coverage added at the end of `test_ui_regression.js`.

## Cut list and quote (uncommitted)

- `WireRow.Length` is now read from LENGTH / LEN / WIRE LENGTH / CUT LENGTH / LENGTH (IN|FT|MM|CM) headings by both the generic table reader and the pinout reader; "length" is also a valid learned-heading role and a Page review column role. Covered by `wire_length_test.go` (including a synthetic PDF with one deliberately blank length).
- UI: **Cut list** tab (merges the same wire seen from both ends, flags no/conflicting/unreadable lengths and unit assumptions, adds an optional service-loop allowance, totals footage per AWG/color, CSV export) and **Quote summary** tab (wire + BOM parts + labor, scrap and markup, every unpriced input listed as a blocker; tooling is counted, not priced). Pure logic is in the `CUTLIST` block of `assets/ui.html` and tested in `test_ui_regression.js`.
- Quote inputs are saved in the project file (`quote`). Labor hours per harness are taken from the last Labor-tab calculation unless typed over.
- Limits: costs are typed by the user (no price database); BOM quantities that are not plain numbers (A/R) are never priced; wire/cable BOM lines are skipped in favor of cut-list footage; splice/branch nets are flagged, not split into cuts; the lengths shown to the user are those stated on the drawing, in the unit chosen when none is stated.

## Environment notes

- The repo folder is owned by the `CodexSandboxOnline` user, so git refuses to run as `Jordan`, and `go build` fails with "error obtaining VCS status". Workaround without changing global config, set before building:
  `GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=safe.directory GIT_CONFIG_VALUE_0=C:/Users/Jordan/Documents/Codex/harness_reader`
  (or run `git config --global --add safe.directory ...` once).
- `gh` CLI is not installed on this workstation, so GitHub Actions history was not inspected.

## Not verified

- GitHub Actions status of `windows-build.yml` (earlier failures were reported; cause unknown).
- Interactive Edge window behavior and the face view rendered in a browser.
- NO_CAVITY_ROWS, BOM_NOT_FOUND and incomplete conductor-row extraction on the customer drawing. These need the authorized local drawing; no synthetic reproducer exists yet.
- Whether the uncommitted tooling changes are complete or intended.

## Next priority

1. Review and commit the uncommitted required-tooling work on a branch and open a PR.
2. Check the Actions run history for `windows-build.yml` (install `gh` or use the web UI).
3. Build a synthetic fixture that reproduces NO_CAVITY_ROWS or BOM_NOT_FOUND, then fix the parser against it.
