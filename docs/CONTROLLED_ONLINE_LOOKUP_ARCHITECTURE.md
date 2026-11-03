# Controlled online catalog lookup architecture

Status: design baseline for review. This document is not a CUI compliance certification and does not replace the contract, System Security Plan, organizational security policy, export-control review, or information-owner authorization.

## Objective

Permit public manufacturer and distributor research without allowing a harness drawing, drawing-derived context, CUI, proprietary information, or project metadata to reach an unapproved external service.

## Operating modes

### Sealed workspace (default)

- Intended for CUI, proprietary, export-controlled, or classification-uncertain drawings.
- The drawing, rendered pages, extracted text, notes, BOM, designators, quantities, hashes, filenames, project records, and evidence remain local.
- The UI Content Security Policy allows connections only to the loopback backend.
- External lookup is blocked in the application policy and no external provider adapter is enabled.
- A part number detected in a controlled drawing is not presumed public or releasable.

### Public/releasable lookup workspace

- Must be a separate workspace with no source drawing attached.
- Accepts one human-reviewed exact part number at a time.
- Requires an authorized user to attest that the exact value is releasable.
- Produces a preview of the complete outbound payload before transmission.
- Sends no filename, drawing number, sheet, page region, designator, quantity, note, customer, program, project hash, surrounding text, or relationship data.
- Records an immutable request/response audit receipt locally.

## Recommended boundary

Use a separate catalog broker in an approved connected environment:

1. The desktop application creates a release record containing only the exact part number, requested public fields, provider name, request ID, user identity, approval time, and policy version.
2. A release gate validates the schema, rejects extra fields, enforces length and character rules, and records the approved request.
3. The broker sends the part number only to allowlisted HTTPS manufacturer or distributor APIs through the organization's approved proxy.
4. The broker normalizes public results and records provider URL, retrieval time, response hash, source revision/date, and licensing/redistribution restrictions.
5. Returned data enters the COTS registry as `EXTERNAL CANDIDATE — REVIEW REQUIRED`; it never overwrites drawing evidence or becomes an approved alternate automatically.

The broker should be a separate component and security boundary, not a general-purpose web client embedded in the drawing process.

## Provider requirements

- Contractually and organizationally approved for the intended data type.
- Fixed hostname allowlist; no arbitrary URLs, redirects to unapproved hosts, or user-supplied endpoints.
- TLS validation with normal certificate and revocation checking.
- API credentials stored outside project files and logs, preferably in an approved secrets service or Windows credential facility.
- Documented retention, telemetry, training-use, subprocessors, hosting region, and incident-reporting terms.
- Rate limits, timeouts, response-size limits, content-type validation, and malicious-content scanning.
- Public manufacturer sources take precedence over distributor enrichment. Search-engine snippets are discovery aids only.

## Required controls

- Deny by default and fail closed.
- Exact outbound schema allowlist; reject unknown fields.
- Separate authorization from drawing review duties where organizational policy requires it.
- Append-only audit events for mode changes, release approval, request, response, rejection, provider and result hash.
- No cookies, browser history, analytics SDK, crash uploader, automatic update telemetry, or third-party scripts in the sealed application.
- No automatic lookup triggered by PDF parsing.
- No bulk lookup from a controlled BOM. Each released identifier needs an explicit approval record unless an approved organizational release list exists.
- Cache public results by normalized part number and provider, with source and retrieval date. A cache hit must not imply current lifecycle status.
- Returned alternates remain separate candidates with qualification basis, restrictions, and reviewer approval.
- Security testing must verify that filenames, hashes, notes, designators, quantities, nearby text, and page coordinates cannot appear in outbound payloads or logs.

## Deployment paths

1. **Maximum isolation:** sealed desktop application plus a physically or logically separate connected lookup workstation; transfer only approved release manifests through the organization's authorized process.
2. **Controlled enterprise connection:** sealed application and broker on a managed CUI-capable network, with proxy enforcement, endpoint protection, identity, audit collection, configuration management, and an approved interconnection documented in the System Security Plan.
3. **Public-only edition:** a separately packaged edition that cannot open controlled projects and only performs catalog research. Do not rely on a UI toggle alone to change the security boundary.

## Release phases

1. Current: local registries, sealed default, part-number-only payload preview and release audit queue; no network provider.
2. Provider pilot: one official manufacturer API through a test broker using synthetic/public part numbers only.
3. Security review: schema-fuzzing, negative leakage tests, proxy and DNS allowlist tests, log inspection, credential review and incident handling.
4. Controlled deployment: approve the provider and environment, update the System Security Plan, document responsibilities, sign the application, and enable the adapter by managed configuration.
5. Expansion: add approved providers individually; do not add general web browsing to the drawing workspace.

## Windows distribution

- Sign every executable and installer with one consistent verified publisher identity.
- Use SHA-256 Authenticode and an RFC 3161 timestamp.
- Verify signatures and hashes in CI and publish a signed manifest.
- Prefer Microsoft Artifact Signing for direct distribution or MSIX/Microsoft Store distribution where appropriate.
- Keep the sealed and connected editions separately identifiable, signed, tested, and configured.

