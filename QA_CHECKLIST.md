# Trivex Harness Auditor exploratory QA checklist

Use only synthetic fixtures from `testdata/`. Record the tested commit, executable hash, Windows version, Edge version, and whether each result was automated or observed manually.

## Startup and lifecycle

- Launch with no other Trivex instance running.
- Launch while another instance is active and confirm the behavior is understandable.
- Close through **Exit application**, the Edge window close button, and process termination.
- Confirm the backend stops after the application window closes.
- Reopen immediately after shutdown and confirm a fresh session works.

## Drawing intake and recovery

- Start with no drawing and exercise every available action.
- Load each synthetic PDF in `testdata/`.
- Try a non-PDF, an empty file, a truncated PDF, and a file over the configured size limit.
- Load two different drawings sequentially and confirm no stale state crosses between them.
- Clear a drawing, cancel the confirmation, then clear it for real.
- Save and reopen a project; test a missing source file, a wrong source file, and a matching reattachment.

## Interpretation and page review

- Run the built-in orientation self-test.
- Review upright, 90/180/270-degree, mirrored, sideways, nested-form, glyph-order, and outlined-text fixtures.
- Check first, middle, last, zero, negative, and out-of-range page selections.
- Exercise image, raw-image, region, and reanalysis paths before and after clearing a drawing.
- Confirm diagnostic summaries do not disclose filenames, drawing text, part numbers, or checksums.

## Engineering state

- Verify connector identifiers remain distinct from part numbers and contact sex.
- Check case-sensitive cavity identifiers and front/rear mirroring.
- Exercise empty, duplicate, conflicting, and unresolved conductor endpoints.
- Test zero, one, and large build quantities and both fill policies.
- Verify BOM, kitting, tooling, labor, and build-plan totals after repeated edits and tab changes.
- Confirm invalid labor values, unsupported construction estimates, and incomplete cavity evidence stay blocked.
- Verify a banding backshell selects a tool only from an exact recognized band P/N.
- Verify M85049/20 and M85049/26 crimp-ring families never inherit band-tool requirements from loose BOM descriptions; release must remain blocked until the qualified crimp-ring tool/die and process are resolved.
- Verify clamp/strain-relief backshells call for a correctly sized small flat-blade driver and exact screw/torque review.
- Verify Tinel-Lock/TXR items call for the TE AD-5000 process, while factory-preterminated braid does not create a false field-crimp requirement.

## Local security and robustness

- Reject missing or incorrect session tokens.
- Reject non-loopback origins and wrong HTTP methods.
- Reject malformed, oversized, and trailing JSON bodies.
- Repeat parsing and plan generation to look for stale state or increasing memory use.
- Confirm the app makes no remote requests and writes no drawing content to disk.

## Usability

- Visit every primary navigation tab and inspect the browser console.
- Test keyboard-only navigation, focus order, zoom, narrow window width, and high-DPI scaling.
- Confirm errors are visible, specific, recoverable, and do not leave controls stuck.
- Verify confirmation dialogs clearly state what will be lost or changed.
