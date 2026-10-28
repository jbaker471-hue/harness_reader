# Reference sources used by Trivex Harness Auditor

Drawing analysis runs locally. No customer file or extracted value is submitted to these sites. The embedded tables are a bounded offline snapshot; links below are optional manual references. Drawing requirements and the contract determine which specifications apply.

## Active offline lookups

- `CONNECTOR_LAYOUT_REFERENCE.csv`: nine reviewed Series III 35-arrangement cavity-ID inventories, A35 through J35 excluding the unused shell letter I. Derived factual cavity identifiers and counts from the user-supplied Glenair MIL-DTL-38999 cylindrical connectors catalog, PDF pages 108-110 (D-2 through D-4). Limited to complete D38999/20, /24 and /26 part-number matches. Inventory identifiers do not establish physical front/rear coordinates or complete wire-path recognition.
- `DMC_EXPANDED_REFERENCE.csv`: 294 supplied contact/tooling records. Used by the contact-tool tables, required-tooling calculations and build plans.
- `MANUFACTURER_CONTACT_REFERENCE.csv`: 125 supplied manufacturer records. Used by specialty/manufacturer tooling requirements with source restrictions retained.
- `SPECIALTY_CONTACTS_REFERENCE.csv`: 37 supplied records including contacts with multiple required termination operations. Used by specialty and required-tooling calculations.
- `ACCESSORY_REFERENCE.csv`: 12 existing accessory rules. Used for backshell/band/plug comparisons. Applicability and actual drawing treatment require review.

The three supplied tooling CSVs were compared row for row with the user's retained copies. Existing source/verification fields remain intact. Manufacturer and specialty requirements are not flattened into a single crimp operation.

Unused candidate IDs are the sourced cavity inventory minus unique occupied IDs from both ends of recognized wire paths. This gives a per-connector candidate list, not proof that all sheets were recognized. Conflicting or out-of-inventory evidence must be resolved before preparation. A circuit called SPARE remains occupied when a physical wire path exists.

## Additional official documents located

These sources were located and reviewed for relevance. Their full requirements have not been encoded into automatic rules. New rules require an exact source section, applicability limits and tests.

| Source | Intended use | Official location |
| --- | --- | --- |
| MIL-STD-1560 | Authoritative insert-arrangement scope and diagrams | https://quicksearch.dla.mil/qsDocDetails.aspx?ident_number=36980 |
| MIL-HDBK-536 | Connector contact and fiber-terminus selection cross-reference | https://quicksearch.dla.mil/qsDocDetails.aspx?ident_number=284910 |
| Glenair standard contact arrangements | Independent check of standard cavity labels, counts and contact sizes | https://www.glenair.com/space-mechanisms/blind-mate/pdf/mil-std-1560/standard-contact-arrangements.pdf |
| Amphenol 38999 catalog | Part decoding, insert/contact references, assembly and sealing information | https://www.amphenol-aerospace.com/resources/literature/view/38999-catalog.pdf |
| DMC document library | Tooling guide, M39029 contact wall chart, AS7928 terminal wall charts | https://dmctools.com/resources/document-library |
| DMC contact search | Exact contact-to-tool reference checks | https://dmctools.com/toolsearch |
| Glenair Band-Master ATS manuals | Band/tool family compatibility, operation and calibration references | https://www.glenair.com/bandmaster/ |
| TE screened backshell adapters | AS85049 backshell/band families and manufacturer drawings | https://www.te.com/en/products/connectors/connector-accessories/connector-backshells/screened-backshells-adapters.html |
| NASA-STD-8739.4 | Cable and harness workmanship reference when invoked by the applicable requirements | https://standards.nasa.gov/standard/NASA/NASA-STD-87394 |

Reviewed 2026-10-28. This index does not certify that every linked revision governs a particular build. Paid or controlled standards are not reproduced in the repository.
