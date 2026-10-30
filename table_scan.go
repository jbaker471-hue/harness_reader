package main

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Table-oriented fallback for CAD/ECAD wire lists. A conventional schematic
// uses free-floating numbers in columns; other exporters use an explicit FROM,
// PIN, SIGNAL, TO, PIN table. It is deliberately NOT used on pages containing
// a recognized BOM, and emits only DRAFT candidates.
var (
	tablePin        = regexp.MustCompile(`(?i)^(?:[A-Z]{0,2}[0-9]{1,4}[A-Z]?|[A-Z]{1,2})$`)
	tableEnd        = regexp.MustCompile(`(?i)^(?:P|J|CN|CON|X|TB|PL|SK)[-_]?[0-9]{1,4}[A-Z]?$`)
	tableBOMContext = regexp.MustCompile(`(?i)\b(?:BILL\s+OF\s+MATERIALS|PARTS\s+LIST|BOM\b|MATERIAL\s+LIST)\b`)
	tableHeadSource = regexp.MustCompile(`(?i)\b(?:FROM|SOURCE|ORIGIN|START)\b`)
	tableHeadDest   = regexp.MustCompile(`(?i)\b(?:TO|DESTINATION|DEST|END)\b`)
	tableHeadCavity = regexp.MustCompile(`(?i)\b(?:PIN|CAVITY|CONTACT|TERMINAL|POS|POSITION)\b`)
	tableBOMRow     = regexp.MustCompile(`(?i)^\s*([0-9]{1,3})\s+(A\s*/\s*R|[0-9]{1,5})\s+([A-Z0-9][A-Z0-9./_-]{3,})(?:\s+(.*))?$`)
	tableBOMHead    = regexp.MustCompile(`(?i)(?:\b(?:ITEM|FIND)\b.*\b(?:QTY|QUANTITY)\b.*\b(?:PART\s*(?:NUMBER|NO|#)|P\s*/\s*N)\b)`)
)

func lineTokens(l evidenceLine) []string {
	var tokens []string
	for _, w := range l.Words {
		// Some CAD exporters output one text object containing an entire row;
		// retaining text boundaries here lets table headings and row parsing work.
		tokens = append(tokens, strings.Fields(strings.ReplaceAll(w.Text, "|", " "))...)
	}
	return tokens
}
func normalizedTag(s string) string { return strings.ToUpper(strings.Trim(s, " :;()[]")) }
func containsTableHeader(s string) bool {
	return tableHeadSource.MatchString(s) && tableHeadDest.MatchString(s) && tableHeadCavity.MatchString(s)
}
func findStructuredWireRows(p Page) []WireRow {
	lines := pageLines(p)
	// Require table-specific headings. These conditions avoid misinterpreting
	// measurements, revision history or BOM item quantities as cavity IDs.
	headings := []int{}
	for i, l := range lines {
		if containsTableHeader(l.Text) {
			headings = append(headings, i)
		}
	}
	if len(headings) == 0 {
		return nil
	}
	out := []WireRow{}
	seen := map[string]bool{}
	for _, h := range headings {
		// A heading may split across neighboring Y coordinates: scan within a
		// bounded table region without crossing the next heading.
		for i := h + 1; i < len(lines) && i < h+80; i++ {
			l := lines[i]
			if l.Y < p.Height*.11 {
				break
			}
			if i > h+1 && containsTableHeader(l.Text) {
				break
			}
			if tableBOMContext.MatchString(l.Text) {
				break
			}
			t := lineTokens(l)
			if len(t) < 4 {
				continue
			}
			// Common explicit netlist layouts: FROM PIN SIGNAL TO PIN,
			// FROM:PIN SIGNAL TO:PIN, and source/destination swapped ordering.
			tag, from, to, endTag, signal := "", "", "", "", ""
			if len(t) >= 5 && tableEnd.MatchString(normalizedTag(t[0])) && tablePin.MatchString(t[1]) && tableEnd.MatchString(normalizedTag(t[len(t)-2])) && tablePin.MatchString(t[len(t)-1]) {
				tag = normalizedTag(t[0])
				from = t[1]
				endTag = normalizedTag(t[len(t)-2])
				to = t[len(t)-1]
				signal = strings.Join(t[2:len(t)-2], " ")
			} else if len(t) >= 4 && tableEnd.MatchString(normalizedTag(t[0])) && tablePin.MatchString(t[1]) && tableEnd.MatchString(normalizedTag(t[len(t)-1])) {
				// Source pin signal destination tag, without an end cavity.
				tag = normalizedTag(t[0])
				from = t[1]
				endTag = normalizedTag(t[len(t)-1])
				signal = strings.Join(t[2:len(t)-1], " ")
			}
			if tag == "" || from == "" || signal == "" {
				continue
			}
			if strings.Contains(strings.ToUpper(signal), "SHEET") || strings.Contains(strings.ToUpper(signal), "REVISION") {
				continue
			}
			// Contextual table evidence is mandatory; no drawing release implied.
			key := tag + ":" + from + "|" + strconv.Itoa(p.Number)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, WireRow{Page: p.Number, Connector: tag, Cavity: strings.ToUpper(from), Signal: signal, End: endTag, EndCavity: strings.ToUpper(to), Confidence: "DRAFT — HEADER-GROUNDED NETLIST ROW", Y: l.Y})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Y > out[j].Y })
	return out
}

// Compact textual BOMs often do not have separately positioned cell objects.
// Parse these as draft rows only after a BOM title and column evidence.
func findTextBOMRows(p Page) []BOMRow {
	lines := pageLines(p)
	title := false
	head := -1
	for i, l := range lines {
		if tableBOMContext.MatchString(l.Text) {
			title = true
		}
		if tableBOMHead.MatchString(l.Text) && title {
			head = i
			break
		}
	}
	if head < 0 {
		return nil
	}
	rows := []BOMRow{}
	for i := head + 1; i < len(lines) && i < head+100; i++ {
		s := strings.TrimSpace(lines[i].Text)
		if lines[i].Y < p.Height*.12 {
			break
		}
		m := tableBOMRow.FindStringSubmatch(s)
		if len(m) == 0 {
			continue
		}
		item, _ := strconv.Atoi(m[1])
		if item <= 0 || item > 250 {
			continue
		}
		qty := strings.ReplaceAll(strings.ToUpper(m[2]), " ", "")
		part := strings.TrimSpace(m[3])
		if len(part) < 4 {
			continue
		}
		desc := strings.TrimSpace(m[4])
		if strings.Contains(strings.ToUpper(desc), "TITLE BLOCK") {
			continue
		}
		rows = append(rows, BOMRow{Item: strconv.Itoa(item), Qty: qty, Part: part, Description: desc, Page: p.Number, Status: "DRAFT — TEXT-TABLE FALLBACK; CHECK SOURCE"})
	}
	return rows
}

// A source drawing can specify a connector in an ECAD parts table using a
// manufacturer-specific part not covered by a military-number regex. Only
// BOM rows explicitly describing connector hardware qualify; a generic part
// number alone is never sufficient to infer a connector or contact tool.
func connectorsFromDescribedParts(boms []BOMRow, already []Connector) []Connector {
	used := map[string]bool{}
	recognizedParts := map[string]bool{}
	for _, c := range already {
		part := strings.ToUpper(strings.TrimSpace(c.Part))
		used[part+"|"+strings.ToUpper(strings.TrimSpace(c.Tag))] = true
		if part != "" {
			recognizedParts[part] = true
		}
	}
	out := []Connector{}
	context := regexp.MustCompile(`(?i)\b(?:CONNECTOR|PLUG|RECEPTACLE|CONTACT HOUSING|D-SUB|D SUB|MATING HOUSING)\b`)
	valid := regexp.MustCompile(`(?i)^[A-Z0-9][A-Z0-9./_-]{3,40}$`)
	for _, b := range boms {
		if !context.MatchString(b.Description) {
			continue
		}
		part := strings.ToUpper(strings.TrimSpace(b.Part))
		if !valid.MatchString(part) || strings.EqualFold(part, "VARIOUS") {
			continue
		}
		// This is a fallback for parts the primary BOM recognizer did not
		// cover. A reference such as "P3 GSE" is shortened to "P3" below,
		// so comparing only exact part/tag pairs can manufacture a duplicate
		// connector from a part that was already recognized.
		if recognizedParts[part] {
			continue
		}
		refs := reRefDesignator.FindAllString(strings.ToUpper(b.Reference), -1)
		if len(refs) != 1 {
			// A BOM COTS item described as a connector or D-sub cable is not
			// by itself a discrete harness connector. Require a specific
			// designator; otherwise leave it in the BOM only.
			continue
		}
		tag := refs[0]
		key := part + "|" + tag
		if used[key] {
			continue
		}
		used[key] = true
		n, s := getLayout(part)
		if n == 0 {
			s = "CONNECTOR DESCRIPTION IDENTIFIED; INSERT/CONTACT TOOLING UNRESOLVED"
		}
		out = append(out, Connector{Tag: tag, Part: part, CavityCount: n, Family: "MANUFACTURER CONNECTOR — REVIEW", Pages: []int{b.Page}, LayoutStatus: s})
	}
	return out
}
