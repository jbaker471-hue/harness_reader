# Known issues

Severity: HIGH blocks a trustworthy release, MEDIUM limits accuracy or usability, LOW is cosmetic or process.
Status as of 2026-11-13, branch `work/continue` (from `e6fa0a1`). Items marked OWNER need a decision or a local Windows check that cannot be done remotely.

| ID | Sev | Issue | Status |
|----|-----|-------|--------|
| KI-01 | HIGH | The newest work (`feature/elimination-and-observed-times`, 70 commits ahead of `main`, plus open PR #13) was never merged and the elimination commits never ran in CI. `main` and the published `v2.8.0-candidate.*` releases do not contain it. | Open. A PR from `work/continue` runs the Windows workflow on it. OWNER: decide merge order and whether to close PR #13 in favour of it. |
| KI-02 | HIGH | `CURRENT_STATE.md` had grown to 24 MB through repeated text-encoding corruption in a public repo. | Fixed on `work/continue` (now ~50 KB); guarded by `repo_hygiene_test.go`. The 24 MB blob remains in git history of the unmerged branch. OWNER: history rewrite is not done without authorization. |
| KI-03 | HIGH | Public repository. `CURRENT_STATE.md` says older tests and a UI comment use designators and note wording that match a real customer drawing. Not yet reviewed against the no-customer-content rule. Candidates: `replica_test.go`, `replica_fixture_test.go`, `wire_trace_test.go`, `contact_resolve_test.go`, `assets/ui.html` comments. | Open. OWNER: confirm what is non-CUI. Repository visibility is not changed without owner authorization. |
| KI-04 | MEDIUM | CI (`windows-build.yml`) runs only on pull requests and pushes to `main`. A branch without a PR gets no validation, which is how KI-01 happened. | Open. |
| KI-05 | MEDIUM | Connector catalog layouts, arrangement register and family patterns are marked UNREVIEWED against the standards (PR #13 notes). Tooling compatibility must not be relied on without verified documents. | Open. |
| KI-06 | MEDIUM | Branched harnesses (per-wire lengths from layout dimensions) are not read; their build-time estimates run low because many connectors have no known position count (CURRENT_STATE.md, 2026-11-05). | Open. |
| KI-07 | MEDIUM | Several field drawings read incompletely (two with 0 of 80 and 0 of 44 wires, blocks with no printed name). Diagnosing needs the local tools `local_diag_test.go` / `local_score_test.go` on the owner's PC. | OWNER-run. |
| KI-08 | LOW | `gofmt -l` lists about 18 files as unformatted. Not mass-reformatted, to keep diffs reviewable. | Open. |
| KI-09 | LOW | `AGENTS.md` names a workstation-specific Go path. `build_windows.ps1` finds `go` on PATH first, so this is only documentation. | Open. |
| KI-10 | LOW | Interactive Edge window behaviour, close-window shutdown and customer-drawing recognition are not covered by automation (the smoke test is headless). | OWNER acceptance, see `TESTING.md`. |
