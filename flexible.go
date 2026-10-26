package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Fallback recognizers operate on geometry, never on a fixed P1 location or
// one customer's sheet template. Every result remains an unverified candidate.
var (
	reConnectPart   = regexp.MustCompile(`(?i)\b(?:D38999/[0-9A-Z-]{7,22}|MS(?:27[0-9]{3}|34[0-9]{2})[A-Z0-9-]{3,20}|(?:[0-9]{1,2}D)[0-9A-Z-]{6,22})\b`)
	reRefDesignator = regexp.MustCompile(`(?i)\b(?:J|P|X|TB|CN|CON|PL|SK)[-_]?[0-9]{1,4}[A-Z]?\b`)
	reBOMWord       = regexp.MustCompile(`(?i)(?:BILL\s+OF\s+MATERIALS|BOM|PARTS\s+LIST|MATERIAL\s+LIST)`)
	reNumeric       = regexp.MustCompile(`^[0-9]{1,3}$`)
)

type evidenceLine struct {
	Y     float64
	Words []Word
	Text  string
}

func pageLines(p Page) []evidenceLine {
	var ls []evidenceLine
	for _, w := range p.Words {
		if strings.TrimSpace(w.Text) == "" {
			continue
		}
		hit := -1
		for i := range ls {
			if abs(ls[i].Y-w.Y) < 3.6 {
				hit = i
				break
			}
		}
		if hit < 0 {
			ls = append(ls, evidenceLine{Y: w.Y})
			hit = len(ls) - 1
		}
		ls[hit].Words = append(ls[hit].Words, w)
	}
	for i := range ls {
		sort.Slice(ls[i].Words, func(j, k int) bool { return ls[i].Words[j].X < ls[i].Words[k].X })
		chunks := []string{}
		for _, w := range ls[i].Words {
			chunks = append(chunks, strings.TrimSpace(w.Text))
		}
		ls[i].Text = strings.Join(chunks, " ")
	}
	sort.Slice(ls, func(i, j int) bool { return ls[i].Y > ls[j].Y })
	return ls
}
func compactPN(s string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), " ", ""), "\u00a0", ""))
}
func findPartNumbers(p Page) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(s string) {
		s = compactPN(s)
		if reConnectPart.MatchString(s) && len(s) > 7 && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, line := range pageLines(p) {
		for _, s := range reConnectPart.FindAllString(line.Text, -1) {
			add(s)
		}
		for _, s := range reConnectPart.FindAllString(compactPN(line.Text), -1) {
			add(s)
		}
		for _, w := range line.Words {
			for _, s := range reConnectPart.FindAllString(w.Text, -1) {
				add(s)
			}
		}
	}
	return out
}
func findTags(p Page) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, l := range pageLines(p) {
		for _, s := range reRefDesignator.FindAllString(l.Text, -1) {
			s = strings.ToUpper(s)
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
	}
	return out
}
func normalizedHeader(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.Join(strings.Fields(s), "")
	s = strings.Trim(s, ".:/-_#")
	switch {
	case s == "ITEM" || s == "ITEMNO" || s == "ITEM#" || s == "FIND" || s == "FINDNO" || s == "NO":
		return "item"
	case s == "QTY" || s == "QUANTITY" || s == "QTY/HARNESS" || s == "REQD" || s == "QTYREQD":
		return "qty"
	case s == "PARTNUMBER" || s == "PARTNO" || s == "PART#" || s == "P/N" || s == "PN" || s == "MFGPN":
		return "part"
	case s == "DESCRIPTION" || s == "NOMENCLATURE" || s == "DESC":
		return "description"
	case s == "MANUFACTURER" || s == "MFG" || s == "VENDOR" || s == "SOURCE":
		return "vendor"
	case s == "REFDES" || s == "REF.DES" || s == "REFERENCE" || s == "REFERENCE(S)" || s == "DESIGNATOR":
		return "ref"
	}
	return ""
}
func headerCenters(line evidenceLine) map[string]float64 {
	ret := map[string]float64{}
	for i, w := range line.Words {
		for step := 1; step <= 3 && i+step <= len(line.Words); step++ {
			ss := []string{}
			for _, x := range line.Words[i : i+step] {
				ss = append(ss, x.Text)
			}
			if k := normalizedHeader(strings.Join(ss, " ")); k != "" {
				if _, ok := ret[k]; !ok {
					ret[k] = w.X
				}
			}
		}
	}
	return ret
}
func readCell(line evidenceLine, left, right float64) string {
	tok := []string{}
	for _, w := range line.Words {
		if w.X >= left && w.X < right {
			tok = append(tok, w.Text)
		}
	}
	return strings.TrimSpace(strings.Join(tok, " "))
}
func flexibleBOMRows(p Page) []BOMRow {
	lines := pageLines(p)
	header := -1
	cols := map[string]float64{}
	for i, l := range lines {
		c := headerCenters(l)
		_, part := c["part"]
		_, qty := c["qty"]
		_, item := c["item"]
		if part && qty && (item || len(c) >= 4) && len(c) > len(cols) {
			header = i
			cols = c
		}
	}
	if header < 0 {
		return nil
	}
	// Require contextual BOM evidence rather than accepting arbitrary engineering tables.
	context := false
	for i := 0; i < len(lines) && i < header+3; i++ {
		if reBOMWord.MatchString(lines[i].Text) {
			context = true
			break
		}
	}
	if !context && len(cols) < 4 {
		return nil
	}
	type column struct {
		k string
		x float64
	}
	cc := []column{}
	for k, x := range cols {
		cc = append(cc, column{k, x})
	}
	sort.Slice(cc, func(i, j int) bool { return cc[i].x < cc[j].x })
	if len(cc) < 3 {
		return nil
	}
	boundaries := map[string][2]float64{}
	for i, c := range cc {
		lo := 0.0
		hi := p.Width
		if i > 0 {
			lo = (cc[i-1].x + c.x) / 2
		}
		if i+1 < len(cc) {
			hi = (cc[i+1].x + c.x) / 2
		}
		// Text headings typically left-align to their columns; use the heading
		// start minus a small margin rather than midpoint of long columns.
		lo = c.x - 8
		if i > 0 {
			lo = (cc[i-1].x + c.x) / 2
			if cc[i-1].k == "description" {
				lo = c.x - 10
			}
		}
		boundaries[c.k] = [2]float64{lo, hi}
	}
	// Midpoints can amputate the leading item or quantity; fit row anchors
	// using local minimum X for numbered item anchors.
	get := func(l evidenceLine, k string) string {
		b, ok := boundaries[k]
		if !ok {
			return ""
		}
		return readCell(l, b[0], b[1])
	}
	rows := []BOMRow{}
	for i := header + 1; i < len(lines); i++ {
		l := lines[i]
		if l.Y < p.Height*.12 {
			break
		}
		item := strings.TrimSpace(get(l, "item"))
		qty := strings.TrimSpace(get(l, "qty"))
		part := strings.TrimSpace(get(l, "part"))
		vendor := strings.TrimSpace(get(l, "vendor"))
		desc := strings.TrimSpace(get(l, "description"))
		ref := strings.TrimSpace(get(l, "ref"))
		if item == "" && qty == "" && part == "" {
			continue
		}
		if !reNumeric.MatchString(item) { // Multi-line descriptions stay attached to the last item, not new rows.
			if len(rows) > 0 && part != "" {
				rows[len(rows)-1].Part = strings.TrimSpace(rows[len(rows)-1].Part + " " + part)
			}
			if len(rows) > 0 && desc != "" {
				rows[len(rows)-1].Description = strings.TrimSpace(rows[len(rows)-1].Description + " " + desc)
			}
			continue
		}
		if part == "" && desc == "" {
			continue
		}
		if qty == "" {
			qty = "UNRESOLVED"
		}
		status := "DRAFT — CHECK AGAINST SOURCE"
		if part == "" {
			status = "INCOMPLETE PART NUMBER — HOLD"
		}
		rows = append(rows, BOMRow{Item: item, Qty: qty, Part: part, Vendor: vendor, Description: desc, Reference: ref, Page: p.Number, Status: status})
	}
	if len(rows) == 0 {
		return nil
	}
	return rows
}

// Flexible schematic row scanner: locates repeated cavity-number columns
// using left/right signal evidence and nearby connector designators.
// Reject isolated numbers, notes and dimension tables instead of guessing nets.
func flexibleWireRows(p Page) []WireRow {
	lines := pageLines(p)
	type candidate struct {
		line        evidenceLine
		anchor      Word
		other       Word
		left, right string
		bin         int
	}
	bins := map[int][]candidate{}
	for _, l := range lines {
		if l.Y < p.Height*.16 || l.Y > p.Height*.86 {
			continue
		}
		for _, w := range l.Words {
			id, ok := numberWord(w.Text)
			if !ok {
				continue
			}
			_ = id
			if w.X < p.Width*.10 || w.X > p.Width*.7 {
				continue
			}
			left := []Word{}
			right := []Word{}
			var opposite Word
			for _, v := range l.Words {
				if v.X < w.X-11 && !reDigitsOnly.MatchString(v.Text) && len(v.Text) > 1 {
					left = append(left, v)
				}
				if v.X > w.X+p.Width*.19 && len(v.Text) > 1 && !reNumeric.MatchString(v.Text) {
					right = append(right, v)
				}
				if v.X > w.X+p.Width*.19 && v.X < p.Width*.84 && reNumeric.MatchString(v.Text) {
					opposite = v
				}
			}
			if len(left) == 0 || len(right) == 0 {
				continue
			}
			// Deny typical title block/BOM artifacts.
			rowText := strings.ToUpper(l.Text)
			if strings.Contains(rowText, "REV") || strings.Contains(rowText, "TITLE") || strings.Contains(rowText, "SHEET") {
				continue
			}
			b := int(w.X / 14)
			bins[b] = append(bins[b], candidate{line: l, anchor: w, other: opposite, left: readCell(l, 0, w.X-9), right: readCell(l, w.X+p.Width*.19, p.Width), bin: b})
		}
	}
	var best []candidate
	for _, entries := range bins {
		seen := map[int]bool{}
		good := []candidate{}
		for _, e := range entries {
			key := int(e.line.Y / 4)
			if seen[key] {
				continue
			}
			seen[key] = true
			good = append(good, e)
		}
		if len(good) > len(best) {
			best = good
		}
	}
	if len(best) < 3 {
		return nil
	}
	sort.Slice(best, func(i, j int) bool { return best[i].line.Y > best[j].line.Y })
	anchorX := best[0].anchor.X
	// Select the connector reference nearest the cavity column in a heading
	// zone above the first recognized wire row.
	connector := "UNASSIGNED"
	bestDistance := 1e12
	for _, w := range p.Words {
		if w.Y <= best[0].line.Y+4 || w.Y > p.Height*.94 {
			continue
		}
		for _, tag := range reRefDesignator.FindAllString(strings.ToUpper(w.Text), -1) {
			dx := abs(w.X - anchorX)
			dy := abs(w.Y - best[0].line.Y)
			dist := dx + dy*.28
			if dist < bestDistance && dx < p.Width*.28 {
				bestDistance = dist
				connector = tag
			}
		}
	}
	rows := []WireRow{}
	for _, e := range best {
		if abs(e.anchor.X-anchorX) > 15 {
			continue
		}
		cavity, ok := numberWord(e.anchor.Text)
		if !ok {
			continue
		}
		endCavity := ""
		if e.other.Text != "" {
			endCavity, _ = numberWord(e.other.Text)
		}
		endTag := "UNRESOLVED ENDPOINT"
		for _, l := range lines {
			if l.Y <= e.line.Y || l.Y > e.line.Y+p.Height*.19 {
				continue
			}
			for _, w := range l.Words {
				if w.X < e.anchor.X+p.Width*.3 {
					continue
				}
				if reRefDesignator.MatchString(strings.ToUpper(w.Text)) {
					endTag = reRefDesignator.FindString(strings.ToUpper(w.Text))
					break
				}
			}
			if endTag != "UNRESOLVED ENDPOINT" {
				break
			}
		}
		confidence := "CANDIDATE: POSITIONAL ROW; CHECK ENDPOINT"
		if connector == "UNASSIGNED" {
			confidence = "UNASSIGNED CONNECTOR — CHECK SOURCE"
		}
		if endCavity != "" {
			confidence = "CANDIDATE: NUMERIC END CONTACT"
		}
		if connector == "UNASSIGNED" && endCavity != "" {
			confidence = "UNASSIGNED CONNECTOR — CHECK SOURCE"
		}
		rows = append(rows, WireRow{Page: p.Number, Connector: connector, Cavity: cavity, Signal: e.left, End: endTag, EndCavity: endCavity, EndSignal: e.right, Confidence: confidence, Y: e.anchor.Y})
	}
	return rows
}

func flexibleConnectors(d PDFData, boms []BOMRow, wires []WireRow, already []Connector) []Connector {
	seen := map[string]bool{}
	for _, c := range already {
		seen[c.Part+"|"+c.Tag] = true
	}
	out := []Connector{}
	wireTags := map[string]bool{}
	for _, w := range wires {
		if w.Connector != "UNASSIGNED" && w.Connector != "" {
			wireTags[w.Connector] = true
		}
	}
	for _, p := range d.Pages {
		parts := findPartNumbers(p)
		if len(parts) == 0 {
			continue
		}
		tags := findTags(p)
		for _, pn := range parts {
			tag := "UNASSIGNED"
			// Unique designator in a BOM reference or a wiring source is usable
			// only as an unverified association; avoid first-tag guesses.
			for _, b := range boms {
				if b.Page != p.Number || !strings.Contains(compactPN(b.Part), pn) {
					continue
				}
				t := reRefDesignator.FindAllString(strings.ToUpper(b.Reference), -1)
				if len(t) == 1 {
					tag = t[0]
				}
			}
			if tag == "UNASSIGNED" && len(tags) == 1 {
				tag = tags[0]
			}
			if tag == "UNASSIGNED" && len(wireTags) == 1 {
				for t := range wireTags {
					tag = t
				}
			}
			if seen[pn+"|"+tag] {
				continue
			}
			count, layout := getLayout(pn)
			family := "UNVERIFIED CONNECTOR FAMILY"
			if strings.HasPrefix(pn, "D38999/") {
				family = "MIL-DTL-38999"
			}
			if count == 0 {
				layout = "UNRESOLVED INSERT / CONTACT CONFIGURATION — CHECK SOURCE"
			}
			out = append(out, Connector{Tag: tag, Part: pn, CavityCount: count, Family: family, Pages: []int{p.Number}, LayoutStatus: layout})
			seen[pn+"|"+tag] = true
		}
	}
	return out
}

func flexibleWarnings(d PDFData, a Analysis) []Finding {
	found := map[string]bool{}
	for _, w := range a.Wires {
		found[fmt.Sprint(w.Page)] = true
	}
	issues := []Finding{}
	for _, p := range d.Pages {
		if len(p.Words) < 3 {
			continue
		}
		hasPinout := false
		for _, l := range pageLines(p) {
			upper := strings.ToUpper(l.Text)
			if strings.Contains(upper, "PINOUT") || strings.Contains(upper, "WIRE LIST") || strings.Contains(upper, "CONNECTION TABLE") || strings.Contains(upper, "FROM PIN") || strings.Contains(upper, "TO PIN") {
				hasPinout = true
				break
			}
		}
		if hasPinout && !found[strconv.Itoa(p.Number)] {
			issues = append(issues, Finding{Severity: "ERROR", Code: "WIRING_PAGE_UNINTERPRETED", Message: fmt.Sprintf("Sheet %d contains wiring/pinout headings, but no conductor rows were accepted; review the source page", p.Number), Pages: []int{p.Number}})
		}
	}
	return issues
}
