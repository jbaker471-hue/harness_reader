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
- Multi-conductor cable: the conductor count per gauge is read from BOM wire text (twisted pair, N conductor, NC, or the M27500 part-number format) and can be overridden per gauge on the Cut list tab (saved in the project as `cableSpec`). Conductors are grouped into one cut per cable by gauge, connector pair and drawing order, requiring distinct colors; the grouping is inferred and flagged for confirmation. If the BOM gives conflicting counts for a gauge, nothing is assumed. Not yet verified on a real drawing; the BOM wording and the drawing's wire-to-cable link are unconfirmed.
- Wire/cable specification library: `assets/WIRE_SPEC_REFERENCE.csv` (family, slash-sheet, M27500 component-wire, shield and jacket rows, each with source URLs and a status; all rows are "SUPPLIED REFERENCE — UNVERIFIED") and `wire_decode.go` (`DecodeWirePart`, endpoint `/api/wire-decode?part=`). It decodes M27500 (gauge, conductor count, component wire to AS22759 slash sheet, shield, jacket) and AS22759 slash parts, recognizes the other supplied families, reports unknown codes as unresolved (never guessed), and states that twist and shield scope are not encoded in the part number. The Cut list tab shows a decode card with clickable https sources for each wire/cable part in the BOM. Tests: `wire_decode_test.go`, the decode-card test in `test_ui_regression.js`, and `scripts/smoke.js`.
- Cable construction (`DescribeCableConstruction`, returned as `construction` by `/api/wire-decode?part=&description=`): signal conductors, groups (pair/triad/quad, N pairs), twist, shield present, overall vs individual shields, drain; each attribute is DECODED FROM PART NUMBER, STATED IN BOM TEXT or NOT STATED (never defaulted), and conflicts between BOM text and part number are flagged. Shield and drain are never counted as signal conductors. On the Cut list tab, shielded cables get per-end shield termination selectors (connector shell / backshell / ground point / splice / not terminated; saved as `shieldTerm`), drawing notes that mention shields are listed as evidence only, and open terminations block the quote. Covered: single twisted shielded pairs and multi-pair cables with an overall shield; triads/quads parse from BOM text but are untested against real drawings. Do not run `go fmt ./...` repo-wide: it reformats unrelated files.
- Not yet built from the supplied reference spec: coax/triax/hybrid and foil/drain-specific modeling, automatic reading of shield terminations from the drawing, BOM description generation, DLA/SAE slash-sheet construction details (plating, ratings), color-code tables, URL accessibility checks, and a data-entry path for adding standards. None of the supplied definitions have been verified against the source documents.
- Overall harness length: `harness_length.go` reads a stated length (HARNESS / ASSEMBLY / CABLE / OVERALL / FINISHED / REQUIRED / CUT / WIRE LENGTH, or a leading LENGTH, value on the same or next line, ft-in supported, tolerance kept) from the first three sheets and returns only page, fixed label, normalized value and tolerance in `harnessLengths` (no source text). Strip, sleeve, braid and similar lengths and values such as "PER NOTE 3" or ".25" are ignored. The Cut list tab pre-selects a single distinct detected value (for confirmation), asks the user to choose when several differ, and uses it only for conductors with no length of their own (status HARNESS LENGTH, flagged for branched harnesses). Default excess is now 2 in. per cut (editable). Harness length, shield terminations, cable overrides and typed costs are cleared when another drawing is loaded or the drawing is cleared; rates and markup are kept. Verified on synthetic drawings only; the label wording on the real drawing is unconfirmed.
- Connector face view: front and rear are both shown. With no drawn or reviewed layout the view is a plain cavity list (no shell outline, no square grid) and says positions and shell shape are not shown; the round outline is drawn only for views taken from the drawing, whose rear side is a mirror unless the drawing shows it. Cavities are wired (crimp contact + wire), open (sealing plug, plus filler contact under the contact+plug fill policy), conflict or unresolved; open appears only from a verified inventory without a HELD status, or from the typed occupied list against the inventory. A cavity action table lists the action per cavity. Rectangular/D-sub/other shell shapes and keying are not modelled (no source data); a reviewed insert-geometry data path is not built.
- Connector family from the part number: `connector_family.go` (`/api/connector-family?part=`) decodes the family (D38999, MS3xxx, M83723, M83513 micro-D, M24308 and DE/DA/DB/DC-style D-sub, Deutsch DT) and its shell shape, plus shell size, contact count and sex for D38999 series III arrangement 35. The face view draws a shell icon from the family and places cavities only where a layout can be stated: standard two-row D-sub (9/15/25/37, computed from row counts and 2.77 mm pitch, labelled UNVERIFIED), oriented by pin/socket and mirrored for the rear. No other family has cavity positions, so those show a shell icon and a cavity list; unrecognized families and D-sub parts with no stated pin/socket get no positions. Circular insert arrangements (including D38999) need a reviewed geometry source that is not in the repo; family patterns are unverified against the standards.
- Connector documentation library: `assets/CONNECTOR_SPEC_REFERENCE.csv` holds the supplied layout standards (MIL-STD-1560/1669/1651/1554/1698/3067), family, contact (AS39029), tooling (AS81969), backshell (AS85049), fiber, coax, workmanship and manufacturer references with only the URLs that were supplied; every row is UNVERIFIED and rows with no URL say so. `/api/connector-family` returns the family specification and the layout standard(s) for a part, shown as https links on the face view. `assets/CONNECTOR_INSERT_GEOMETRY.csv` is the arrangement-keyed (not part-number-keyed) store for cavity X/Y, view (pin/socket front/rear), keying, source page and status; it ships EMPTY, a bad row rejects its whole arrangement, and a layout whose cavity count disagrees with the part number is not drawn. Not built: reading the supplied standards into that file, keying alternatives, contact-size-to-radius, service class, qualification and revision tracking, QPD records, mating-connector references, document import fields, URL checks.
- Reference-document intake: a .pdf URL is not assumed to be a PDF. `ValidateReferencePDF` (`/api/reference-validate`, UI card on the Connectors tab) accepts a file only if it is not HTML, has the %PDF- signature and opens in the PDF engine; HTML banners, login and access-denied pages come back MANUAL_DOWNLOAD_REQUIRED and other failures REJECTED. It checks only: nothing is stored and no coordinates are read. `scripts/fetch_reference_docs.js <dir outside the repo>` (developer tool, the app itself stays offline) downloads the listed PDFs and writes manifest.json. Last run: 8 Glenair PDFs verified (saved to ..eference_library, not committed); Amphenol 38999 catalog HTTP 403 and the DLA record page are MANUAL_DOWNLOAD_REQUIRED. The Glenair MIL-STD-1560 sheets are drawn diagrams with no numeric X/Y tables, so nothing has been loaded into CONNECTOR_INSERT_GEOMETRY.csv; the Amphenol catalog (numeric coordinate tables) is the needed source.
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
