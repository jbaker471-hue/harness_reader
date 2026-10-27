package main

import (
	"embed"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

//go:embed assets/*
var assets embed.FS

type Connector struct {
	Tag          string `json:"tag"`
	Part         string `json:"part"`
	CavityCount  int    `json:"cavityCount"`
	Family       string `json:"family"`
	Pages        []int  `json:"pages"`
	LayoutStatus string `json:"layoutStatus"`
}
type WireRow struct {
	Page       int     `json:"page"`
	Connector  string  `json:"connector"`
	Cavity     string  `json:"cavity"`
	Signal     string  `json:"signal"`
	End        string  `json:"end"`
	EndCavity  string  `json:"endCavity"`
	EndSignal  string  `json:"endSignal"`
	Confidence string  `json:"confidence"`
	Y          float64 `json:"y"`
	// v2.7 additions (all optional)
	Wire          string `json:"wire,omitempty"`
	Color         string `json:"color,omitempty"`
	AWG           string `json:"awg,omitempty"`
	Method        string `json:"method,omitempty"`
	Corroboration int    `json:"corroboration,omitempty"`
}
type BOMRow struct {
	PartReference string `json:"partReference,omitempty"`
	Item        string `json:"item"`
	Qty         string `json:"qty"`
	Part        string `json:"part"`
	Vendor      string `json:"vendor"`
	Description string `json:"description"`
	Reference   string `json:"reference"`
	Page        int    `json:"page"`
	Status      string `json:"status"`
}
type Finding struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Pages    []int  `json:"pages"`
	Category string `json:"category"`
}
type ConnectorAudit struct {
	Tag      string   `json:"tag"`
	Expected int      `json:"expected"`
	Occupied []string `json:"occupied"`
	Unused   []string `json:"unused"`
	State    string   `json:"state"`
}
type Analysis struct {
	Pages          int              `json:"pages"`
	Words          int              `json:"words"`
	Connectors     []Connector      `json:"connectors"`
	Wires          []WireRow        `json:"wires"`
	BOM            []BOMRow         `json:"bom"`
	Findings       []Finding        `json:"findings"`
	Audits         []ConnectorAudit `json:"audits"`
	PageSummary    []PageSummary    `json:"pageSummary"`
	ToolingMatches []ToolMatch      `json:"toolingMatches"`
	Unused         []UnusedCavity   `json:"unused"`
}
type PageSummary struct {
	Page       int    `json:"page"`
	Words      int    `json:"words"`
	CavityRows int    `json:"cavityRows"`
	BOMRows    int    `json:"bomRows"`
	Category   string `json:"category"`
}
type ToolMatch struct {
	Part       string `json:"part"`
	Database   string `json:"database"`
	Status     string `json:"status"`
	Candidates int    `json:"candidates"`
}

var rePN = regexp.MustCompile(`(?i)\bD38999/[0-9A-Z-]{7,22}\b`)
var reTag = regexp.MustCompile(`\b(?:[A-Z]{1,5}[-_]?)?[JP][0-9]{1,3}\b`)
var reDigitsOnly = regexp.MustCompile(`^0*([0-9]{1,3})$`)
var reBranch = regexp.MustCompile(`(?i)^(?:USB[ _]?(?:DN)?[0-9]+|DB9\s+(?:COM[0-9]+|MISC)|ETH(?:\s+PORT)?[0-9]+|AUDIO\s+(?:IN|OUT)|PORT[0-9]+)$`)
var reContact = regexp.MustCompile(`(?i)M39029/[0-9]{1,3}-[0-9]{1,4}`)
var reContactNum = regexp.MustCompile(`(?i)(?:[0-9]{4,6}-[0-9]{3,5}|[A-Z][0-9]{4}-[0-9]{3,5})`)

func compact(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }
func numberWord(s string) (string, bool) {
	if !reDigitsOnly.MatchString(strings.TrimSpace(s)) {
		return "", false
	}
	i, e := strconv.Atoi(strings.TrimSpace(s))
	return strconv.Itoa(i), e == nil && i > 0
}
func filterWords(ws []Word, f func(Word) bool) []Word {
	o := []Word{}
	for _, w := range ws {
		if f(w) {
			o = append(o, w)
		}
	}
	return o
}
func nearY(w Word, y float64, delta float64) bool { return abs(w.Y-y) <= delta }
func matchingText(words []Word, xmin, xmax, y, dy float64) string {
	var picked []Word
	for _, w := range words {
		if w.X >= xmin && w.X < xmax && nearY(w, y, dy) {
			picked = append(picked, w)
		}
	}
	sort.Slice(picked, func(i, j int) bool { return picked[i].X < picked[j].X })
	ss := []string{}
	for _, w := range picked {
		ss = append(ss, w.Text)
	}
	return strings.TrimSpace(strings.Join(ss, " "))
}
// reLayoutCode decodes shell size letter + insert arrangement of a MIL-DTL-38999
// Series III/IV part. Contact code: P/S standard, H/J 1500-cycle, A/B less contacts.
var reLayoutCode = regexp.MustCompile(`D38999/\d{2}[A-Z]([A-HJ])([0-9]{2})[PSHJAB]`)

// getLayout returns the catalog position count. The 35-arrangement counts
// (A35 6 … J35 128) agree with the Amphenol 38999 insert availability chart
// and the Glenair MIL-STD-1560 insert arrangement reference.
func getLayout(part string) (int, string) {
	s := strings.ToUpper(part)
	m := reLayoutCode.FindStringSubmatch(s)
	if len(m) == 0 {
		return 0, "UNKNOWN"
	}
	arr := m[1] + m[2]
	counts := map[string]int{"J35": 128, "G35": 79, "H35": 100, "F35": 66, "E35": 55, "D35": 37, "C35": 22, "B35": 13, "A35": 6}
	if v, ok := counts[arr]; ok {
		return v, "CATALOG COUNT; CAVITY MAP ASSUMED NUMERIC"
	}
	return 0, "UNRESOLVED INSERT ARRANGEMENT"
}
func findConnectorWords(p Page) []struct{ Tag, Part string } {
	var pairs []struct{ Tag, Part string }
	for _, w := range p.Words {
		for _, pn := range rePN.FindAllString(w.Text, -1) {
			tag := ""
			best := 1e12
			for _, v := range p.Words {
				if reTag.MatchString(strings.TrimSpace(v.Text)) && abs(v.Y-w.Y) < 11 {
					dist := abs(v.X - w.X)
					if dist < best {
						best = dist
						tag = reTag.FindString(v.Text)
					}
				}
			}
			pairs = append(pairs, struct{ Tag, Part string }{tag, strings.ToUpper(pn)})
		}
	}
	return pairs
}
func bomRows(p Page) []BOMRow {
	hit := false
	for _, w := range p.Words {
		if strings.Contains(strings.ToLower(w.Text), "bill of materials") {
			hit = true
		}
	}
	if !hit {
		return nil
	}
	header := float64(0)
	xItem, xQty, xPN, xVendor, xDesc, xRef := 58.0, 114.0, 177.0, 290.0, 441.0, 632.0
	for _, w := range p.Words {
		if strings.TrimSpace(w.Text) == "Item" && w.Y > header {
			header = w.Y
			xItem = w.X
		}
		if w.Text == "Qty" {
			xQty = w.X
		}
		if strings.Contains(w.Text, "Part Number") {
			xPN = w.X
		}
		if w.Text == "Vendor" {
			xVendor = w.X
		}
		if w.Text == "Description" {
			xDesc = w.X
		}
		if strings.Contains(w.Text, "Ref.") {
			xRef = w.X
		}
	}
	if header == 0 {
		return nil
	}
	anchors := []Word{}
	for _, w := range p.Words {
		_, ok := numberWord(w.Text)
		if ok && w.Y < header-6 && abs(w.X-xItem) < 32 && w.Y > p.Height*0.14 {
			anchors = append(anchors, w)
		}
	}
	sort.Slice(anchors, func(i, j int) bool { return anchors[i].Y > anchors[j].Y })
	rows := []BOMRow{}
	for i, a := range anchors {
		item, _ := numberWord(a.Text)
		if n, _ := strconv.Atoi(item); n > 100 {
			continue
		}
		upper := header - 4.0
		if i > 0 {
			upper = (anchors[i-1].Y + a.Y) / 2
		}
		lower := p.Height * 0.15
		if i+1 < len(anchors) {
			lower = (a.Y + anchors[i+1].Y) / 2
		}
		between := filterWords(p.Words, func(w Word) bool { return w.Y > lower && w.Y <= upper })
		qty := matchingText(between, xQty-8, xPN-19, a.Y, 4.8)
		// Preserve glyph-fragmented part numbers and multi-line A/R alternatives.
		partWords := filterWords(between, func(w Word) bool { return w.X >= xPN-20 && w.X < xVendor-17 })
		byY := map[int][]Word{}
		for _, w := range partWords {
			k := int(w.Y/3 + 0.5)
			byY[k] = append(byY[k], w)
		}
		keys := []int{}
		for k := range byY {
			keys = append(keys, k)
		}
		sort.Sort(sort.Reverse(sort.IntSlice(keys)))
		pnLines := []string{}
		for _, k := range keys {
			ww := byY[k]
			sort.Slice(ww, func(i, j int) bool { return ww[i].X < ww[j].X })
			t := []string{}
			for _, w := range ww {
				t = append(t, strings.Trim(w.Text, ","))
			}
			s := strings.Join(t, "-")
			if s != "" {
				pnLines = append(pnLines, s)
			}
		}
		pn := strings.Join(pnLines, ", ")
		vendorWords := filterWords(between, func(w Word) bool { return w.X >= xVendor-12 && w.X < xDesc-91 && abs(w.Y-a.Y) < 6 })
		sort.Slice(vendorWords, func(i, j int) bool { return vendorWords[i].X < vendorWords[j].X })
		vendTokens := []string{}
		for _, w := range vendorWords {
			vendTokens = append(vendTokens, w.Text)
		}
		vendor := strings.Join(vendTokens, "-")
		if vendor == "Alpha-Wire" {
			vendor = "Alpha Wire"
		}
		if vendor == "L-COM" {
		} else {
			vendor = strings.ReplaceAll(vendor, "Amphenol-", "Amphenol ")
		}
		descWords := filterWords(between, func(w Word) bool { return w.X >= xDesc-85 && w.X < xRef-25 && w.Y > lower && w.Y <= upper })
		sort.Slice(descWords, func(i, j int) bool {
			if abs(descWords[i].Y-descWords[j].Y) > 3 {
				return descWords[i].Y > descWords[j].Y
			}
			return descWords[i].X < descWords[j].X
		})
		d := []string{}
		for _, w := range descWords {
			if w.Text == "1" && w.X < 430 {
				continue
			}
			d = append(d, w.Text)
		}
		ref := matchingText(between, xRef-25, p.Width, a.Y, 7)
		status := "DRAFT — CHECK AGAINST PDF"
		if pn == "" {
			status = "INCOMPLETE PART NUMBER — HOLD"
		}
		rows = append(rows, BOMRow{Item:item, Qty:qty, Part:pn, Vendor:vendor, Description:strings.Join(d, " "), Reference:ref, Page:p.Number, Status:status})
	}
	return rows
}

// Parse numeric cavity rows using positional evidence: a cavity column, a signal on
// its left, and a second endpoint on its right. Never take a bare number from notes.
func wireRows(p Page) []WireRow {
	if len(p.Words) < 10 {
		return nil
	}
	topTags := []Word{}
	for _, w := range p.Words {
		if (compact(w.Text) == "P1" || strings.Contains(compact(w.Text), "CABLE P1")) && w.Y > p.Height*.65 {
			topTags = append(topTags, w)
		}
	}
	tag := ""
	if len(topTags) > 0 {
		tag = "P1"
	} else {
		for _, w := range p.Words {
			if w.Y > p.Height*.72 && w.X > p.Width*.13 && w.X < p.Width*.40 {
				if s := reTag.FindString(strings.ToUpper(w.Text)); s != "" {
					tag = s
					break
				}
			}
		}
	}
	if tag == "" {
		return nil
	}
	headers := []Word{}
	for _, w := range p.Words {
		if w.X > p.Width*.63 && reBranch.MatchString(strings.TrimSpace(w.Text)) {
			headers = append(headers, w)
		}
	}
	sort.Slice(headers, func(i, j int) bool { return headers[i].Y > headers[j].Y })
	// Choose a cavity column among clusters with aligned left-hand signals and right-hand endpoints.
	bins := map[int]int{}
	nums := []Word{}
	for _, w := range p.Words {
		_, ok := numberWord(w.Text)
		if !ok || w.Y < p.Height*.16 || w.Y > p.Height*.91 || w.X < p.Width*.11 || w.X > p.Width*.49 {
			continue
		}
		left := filterWords(p.Words, func(v Word) bool {
			return v.X < w.X-13 && v.Y > p.Height*.16 && nearY(v, w.Y, 4.1) && len(strings.TrimSpace(v.Text)) > 1 && !reDigitsOnly.MatchString(v.Text)
		})
		right := filterWords(p.Words, func(v Word) bool {
			return v.X > p.Width*.69 && nearY(v, w.Y, 5.5) && len(strings.TrimSpace(v.Text)) > 1
		})
		if len(left) == 0 || len(right) == 0 {
			continue
		}
		nums = append(nums, w)
		bins[int(w.X/12)]++
	}
	if len(nums) < 2 {
		return nil
	}
	bestBin, bestCount := -1, 0
	for b, c := range bins {
		if c > bestCount {
			bestCount = c
			bestBin = b
		}
	}
	if bestCount < 2 {
		return nil
	}
	anchorX := float64(bestBin*12 + 6)
	out := []WireRow{}
	used := map[string]bool{}
	for _, w := range nums {
		if abs(w.X-anchorX) > 14 {
			continue
		}
		id, _ := numberWord(w.Text)
		key := fmt.Sprintf("%v/%v", id, int(w.Y))
		if used[key] {
			continue
		}
		used[key] = true
		sig := matchingText(p.Words, 0, w.X-8, w.Y, 5.1)
		otherId := ""
		for _, v := range p.Words {
			if v.X > p.Width*.60 && v.X < p.Width*.81 && nearY(v, w.Y, 4.8) {
				if v2, yes := numberWord(v.Text); yes {
					otherId = v2
					break
				}
			}
		}
		right := matchingText(p.Words, p.Width*.83, p.Width, w.Y, 5.0)
		if right == "" {
			right = matchingText(p.Words, p.Width*.72, p.Width, w.Y, 5)
		}
		branch := ""
		for _, h := range headers {
			if h.Y > w.Y+5 {
				branch = h.Text
			} else {
				break
			}
		}
		if branch == "" {
			branch = "UNRESOLVED ENDPOINT"
		}
		// Signal and other contact can be user-verified in UI.
		conf := "CANDIDATE: BOTH ENDPOINTS"
		if otherId == "" {
			conf = "CANDIDATE: END CONTACT NOT NUMERIC"
		}
		out = append(out, WireRow{Page: p.Number, Connector: tag, Cavity: id, Signal: sig, End: branch, EndCavity: otherId, EndSignal: right, Confidence: conf, Y: w.Y, Method: "legacy-template"})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Y > out[j].Y })
	return out
}
func connectorFromBOM(rows []BOMRow) []Connector {
	var out []Connector
	seen := map[string]bool{}
	for _, r := range rows {
		for _, pn := range rePN.FindAllString(strings.ToUpper(r.Part), -1) {
			tag := strings.TrimSpace(r.Reference)
			if tag == "" || strings.Contains(tag, ",") {
				tag = "UNASSIGNED"
			}
			key := tag + "|" + pn
			if seen[key] {
				continue
			}
			seen[key] = true
			n, s := getLayout(pn)
			out = append(out, Connector{tag, pn, n, "MIL-DTL-38999", []int{r.Page}, s})
		}
	}
	return out
}
func uniqInts(in []int) []int {
	m := map[int]bool{}
	o := []int{}
	for _, n := range in {
		if !m[n] {
			m[n] = true
			o = append(o, n)
		}
	}
	sort.Ints(o)
	return o
}
func AnalyzeDocument(b []byte) (Analysis, error) {
	x, err := ExtractDocument(b)
	if err != nil {
		return Analysis{}, err
	}
	a, _, _, err := AnalyzeExtraction(x)
	return a, err
}

// AnalyzeExtraction runs the recognizers and the cross-source evidence layer.
func AnalyzeExtraction(x *DocExtraction) (Analysis, []EndpointEvidence, []InsertDiagram, error) {
	a, err := AnalyzeData(x.Data)
	if err != nil {
		return a, nil, nil, err
	}
	ev, diagrams, extra := BuildEvidence(x, &a)
	extra = append(extra, interpretationFindings(x, &a)...)
	for i := range extra {
		if extra[i].Pages == nil {
			extra[i].Pages = []int{}
		}
		if extra[i].Category == "" {
			extra[i].Category = categoryFor(extra[i].Code)
		}
	}
	// Interpretation status first, then the rest.
	a.Findings = append(extra, a.Findings...)
	return a, ev, diagrams, nil
}

// AnalyzeData runs every recognizer over already-extracted pages.
func AnalyzeData(d PDFData) (Analysis, error) {
	var a Analysis
	a.Pages = len(d.Pages)
	a.Connectors = []Connector{}
	a.Wires = []WireRow{}
	a.BOM = []BOMRow{}
	a.Findings = []Finding{}
	a.Audits = []ConnectorAudit{}
	a.PageSummary = []PageSummary{}
	var tables []genericTable
	for _, p := range d.Pages {
		a.Words += len(p.Words)
		gts := genericTables(p)
		tables = append(tables, gts...)
		bb := bomRows(p)
		// Retry with geometry-based columns rather than a fixed BOM template.
		if len(bb) == 0 {
			bb = flexibleBOMRows(p)
		}
		if len(bb) == 0 {
			bb = findTextBOMRows(p)
		}
		// General table reader: headings above OR below the rows, two-line
		// headings, FIND NO./CAGE/IDENTIFYING NO. vocabulary.
		var gb []BOMRow
		for _, t := range gts {
			if t.Kind == "bom" {
				gb = append(gb, t.bomRows()...)
			}
		}
		if len(gb) > len(bb) {
			bb = gb
		}
		w := pairedSchematicRows(p)
		if len(w) == 0 {
			w = wireRows(p)
		}
		// Retry when a drawing uses another connector designator, or shifts
		// the cavity column. These remain source-review candidates.
		if len(w) == 0 && len(bb) == 0 {
			w = flexibleWireRows(p)
		}
		if len(w) == 0 && len(bb) == 0 {
			w = findStructuredWireRows(p)
		}
		if len(bb) > 0 {
			// Keep only the strict paired-endpoint schematic recognizer on mixed
			// pages. Legacy positional guesses can mistake BOM quantities for pins.
			kept := w[:0]
			for _, r := range w {if r.Method == "paired-schematic" {kept = append(kept, r)}}
			w = kept
		}
		// Header-grounded pinout/wire-list tables are distinct from BOM
		// tables (their heading has no QTY/PART column), so they are read on
		// every sheet, including a sheet that also carries the BOM.
		have := map[string]bool{}
		for _, r := range w {
			have[r.Connector+":"+r.Cavity] = true
		}
		var tr []WireRow
		for _, t := range gts {
			if t.Kind == "wire" {
				tr = append(tr, t.wireRows()...)
			}
		}
		extra := []WireRow{}
		trWire := map[string]bool{}
		for _, r := range tr {
			if r.Wire != "" {
				trWire[strings.ToUpper(r.Wire)] = true
			}
		}
		for _, r := range pinoutTableRows(p) {
			// The single-line reader cannot see grouped/stacked headings; when
			// the table reader understood the sheet, its unassigned or
			// same-conductor rows are misreadings.
			if len(tr) > 0 && (r.Connector == "UNASSIGNED" || (r.Wire != "" && trWire[strings.ToUpper(r.Wire)])) {
				continue
			}
			extra = append(extra, r)
		}
		extra = append(extra, tr...)
		// Heading-grounded rows outrank positional guesses on the same sheet:
		// drop guessed rows that are unassigned or collide with grounded rows.
		if len(extra) > 0 {
			grounded := map[string]bool{}
			for _, r := range extra {
				grounded[r.Connector+":"+r.Cavity] = true
			}
			kept := w[:0]
			for _, r := range w {
				positional := r.Method == "" || r.Method == "legacy-template"
				if positional && !strings.HasPrefix(r.Confidence, "DRAFT — HEADER-GROUNDED") && (r.Connector == "UNASSIGNED" || grounded[r.Connector+":"+r.Cavity]) {
					continue
				}
				kept = append(kept, r)
			}
			w = kept
			have = map[string]bool{}
			for _, r := range w {
				have[r.Connector+":"+r.Cavity] = true
			}
		}
		for _, r := range extra {
			k := r.Connector + ":" + r.Cavity
			if r.Connector == "UNASSIGNED" || !have[k] {
				w = append(w, r)
				have[k] = true
			}
		}
		cat := "NOTES / OTHER"
		if len(bb) > 0 && len(w) > 0 {
			cat = "BOM + WIRING"
		} else if len(bb) > 0 {
			cat = "BOM"
		} else if len(w) > 0 {
			cat = "WIRING"
		}
		a.PageSummary = append(a.PageSummary, PageSummary{p.Number, len(p.Words), len(w), len(bb), cat})
		a.BOM = append(a.BOM, bb...)
		a.Wires = append(a.Wires, w...)
	}
	// Rows that document a spare / not-connected cavity are not conductors.
	var spare []UnusedCavity
	a.Wires, spare = splitUnusedRows(a.Wires)
	a.Unused = append(spare, explicitUnusedFromNotes(d)...)
	for _, t := range tables {
		a.Unused = append(a.Unused, t.unusedRows()...)
	}
	a.Unused = dedupeUnused(a.Unused)
	a.Wires = mergeWireRows(a.Wires)
	a.Connectors = connectorFromBOM(a.BOM)
	if a.Connectors == nil {
		a.Connectors = []Connector{}
	}
	seenCon := map[string]bool{}
	for _, c := range a.Connectors {
		seenCon[c.Tag] = true
	}
	// Non-military connectors may be identified explicitly by the BOM.
	// They remain unresolved for cavity and crimp tooling unless a source
	// catalog explicitly supports the exact configuration.
	for _, c := range connectorsFromDescribedParts(a.BOM, a.Connectors) {
		duplicate := false
		for _, existing := range a.Connectors {
			if existing.Part == c.Part && existing.Tag == c.Tag {
				duplicate = true
				break
			}
		}
		if !duplicate {
			a.Connectors = append(a.Connectors, c)
			if c.Tag != "UNASSIGNED" {
				seenCon[c.Tag] = true
			}
		}
	}
	// Recognize any additional D38999 part numbers with a unique nearby designator.
	for _, p := range d.Pages {
		for _, pair := range findConnectorWords(p) {
			if pair.Tag == "" || seenCon[pair.Tag] {
				continue
			}
			n, s := getLayout(pair.Part)
			a.Connectors = append(a.Connectors, Connector{pair.Tag, pair.Part, n, "MIL-DTL-38999", []int{p.Number}, s})
			seenCon[pair.Tag] = true
		}
	}
	// Connector part references may occur in a non-D38999 parts list or in
	// a different location from the older fixed P1/SA drawing template.
	for _, c := range flexibleConnectors(d, a.BOM, a.Wires, a.Connectors) {
		if c.Tag != "UNASSIGNED" && seenCon[c.Tag] {
			continue
		}
		duplicate := false
		for _, x := range a.Connectors {
			if x.Part == c.Part {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		a.Connectors = append(a.Connectors, c)
		if c.Tag != "UNASSIGNED" {
			seenCon[c.Tag] = true
		}
	}
	// Accessories (sealing plugs, caps, contacts, backshells…) are never connectors.
	descOf := map[string]string{}
	for _, b := range a.BOM {
		descOf[strings.ToUpper(b.Part)] = b.Description
	}
	kept := a.Connectors[:0]
	for _, c := range a.Connectors {
		if c.Part != "NOT RESOLVED" && isAccessoryPart(c.Part, descOf[strings.ToUpper(c.Part)]) {
			continue
		}
		kept = append(kept, c)
	}
	a.Connectors = kept
	// Parts-list rows described as connectors (without a REF DES column).
	for _, b := range a.BOM {
		// Only ASME noun-first nomenclature ("CONNECTOR, PLUG") or a MIL
		// connector part family; a COTS line such as "CONNECTOR HOUSING" or a
		// cable assembly stays a BOM item (v2.6 behaviour).
		nounFirst := reNounFirstConnector.MatchString(b.Description)
		milFamily := reMilConnectorFamily.MatchString(b.Part)
		if isAccessoryPart(b.Part, b.Description) || !(nounFirst || milFamily) {
			continue
		}
		dup := false
		for _, c := range a.Connectors {
			if strings.EqualFold(c.Part, b.Part) {
				dup = true
			}
		}
		if !dup && rePartLike.MatchString(b.Part) {
			n, st := getLayout(b.Part)
			fam := "MANUFACTURER CONNECTOR — REVIEW"
			if strings.HasPrefix(strings.ToUpper(b.Part), "D38999/") {
				fam = "MIL-DTL-38999"
			}
			a.Connectors = append(a.Connectors, Connector{Tag: "UNASSIGNED", Part: strings.ToUpper(b.Part), CavityCount: n, Family: fam, Pages: []int{b.Page}, LayoutStatus: st})
		}
	}
	// Connector tables (REF DES -> part or FIND NO.) assign designators.
	for tag, part := range connectorTableMap(tables, a.BOM) {
		if isAccessoryPart(part, descOf[strings.ToUpper(part)]) {
			continue
		}
		done := false
		for i := range a.Connectors {
			c := &a.Connectors[i]
			if c.Tag == tag && (c.Part == "NOT RESOLVED" || c.Part == "") {
				c.Part = part
				c.CavityCount, c.LayoutStatus = getLayout(part)
				done = true
			} else if c.Tag == tag {
				done = true
			}
		}
		if done {
			continue
		}
		for i := range a.Connectors {
			c := &a.Connectors[i]
			if c.Tag == "UNASSIGNED" && strings.EqualFold(c.Part, part) {
				c.Tag, done = tag, true
				break
			}
		}
		if !done {
			n, st := getLayout(part)
			a.Connectors = append(a.Connectors, Connector{Tag: tag, Part: part, CavityCount: n, Family: "FROM CONNECTOR TABLE — REVIEW", Pages: []int{}, LayoutStatus: st})
		}
		seenCon[tag] = true
	}
	// An unassigned BOM part may also be found later beside an actual drawing
	// designator. Keep the evidence-backed assignment; never show the same
	// physical part twice merely because the BOM omitted its reference field.
	namedParts := map[string]bool{}
	for _, c := range a.Connectors {
		if c.Tag != "UNASSIGNED" && c.Tag != "" {
			namedParts[c.Part] = true
		}
	}
	deduped := make([]Connector, 0, len(a.Connectors))
	for _, c := range a.Connectors {
		if c.Tag == "UNASSIGNED" && namedParts[c.Part] {
			continue
		}
		deduped = append(deduped, c)
	}
	a.Connectors = deduped
	for _, w := range a.Wires {
		if w.Connector == "" || w.Connector == "UNASSIGNED" {
			continue // reported by ENDPOINT_UNRESOLVED, not as a phantom connector
		}
		if !seenCon[w.Connector] {
			seenCon[w.Connector] = true
			a.Connectors = append(a.Connectors, Connector{w.Connector, "NOT RESOLVED", 0, "UNKNOWN", []int{w.Page}, "UNRESOLVED"})
		}
	}
	if len(a.Connectors) == 0 {
		a.Findings = append(a.Findings, Finding{Severity: "ERROR", Code: "NO_CONNECTORS", Message: "No connector records identified. Text extraction may have succeeded, but connector interpretation has not. Check the per-page coverage and supported identifier patterns; no cavity release is permitted."})
	}
	if len(a.Wires) == 0 {
		a.Findings = append(a.Findings, Finding{Severity: "ERROR", Code: "NO_CAVITY_ROWS", Message: "No conductor rows extracted. This is an engineering-recognition failure, not a successful empty pinout. No unused-cavity list is permitted."})
	}
	if len(a.BOM) == 0 {
		a.Findings = append(a.Findings, Finding{Severity: "WARNING", Code: "BOM_NOT_FOUND", Message: "No BOM table recognized. This is a recognition gap, not proof that the drawing has no BOM. Select the parts list in page review or reconcile manually."})
	}
	if a.Words == 0 {
		a.Findings = append(a.Findings, Finding{Severity: "ERROR", Code: "NO_NATIVE_TEXT", Message: "No machine-readable text extracted (scanned page, or lettering drawn as graphics). Use page review."})
	}
	a.Findings = append(a.Findings, flexibleWarnings(d, a)...)

	for _, c := range a.Connectors {
		nums := map[int]int{}
		issues := []int{}
		for _, w := range a.Wires {
			if w.Connector != c.Tag {
				continue
			}
			i, e := strconv.Atoi(w.Cavity)
			if e != nil {
				continue
			}
			nums[i]++
			if c.CavityCount > 0 && i > c.CavityCount {
				issues = append(issues, w.Page)
			}
		}
		occupied := []string{}
		unused := []string{}
		for i := range nums {
			occupied = append(occupied, strconv.Itoa(i))
		}
		sort.Slice(occupied, func(i, j int) bool {
			x, _ := strconv.Atoi(occupied[i])
			y, _ := strconv.Atoi(occupied[j])
			return x < y
		})
		state := "DRAFT — VERIFY SOURCE"
		if c.CavityCount > 0 {
			for i := 1; i <= c.CavityCount; i++ {
				if nums[i] == 0 {
					unused = append(unused, strconv.Itoa(i))
				}
			}
		} else {
			state = "BLOCKED — UNKNOWN INSERT LAYOUT"
		}
		if len(issues) > 0 {
			ids := []string{}
			for i := range nums {
				if i > c.CavityCount {
					ids = append(ids, strconv.Itoa(i))
				}
			}
			sortCavities(ids)
			a.Findings = append(a.Findings, Finding{Severity: "ERROR", Code: "CAVITY_OUT_OF_RANGE", Message: fmt.Sprintf("%s (%s) is a %d-position insert but drawing assigns cavity numbers %s", c.Tag, c.Part, c.CavityCount, strings.Join(ids, ", ")), Pages: uniqInts(issues)})
			state = "BLOCKED — DRAWING CONFLICT"
		}
		dup := []string{}
		for i, n := range nums {
			if n > 1 {
				dup = append(dup, strconv.Itoa(i))
			}
		}
		if len(dup) > 0 {
			sortCavities(dup)
			a.Findings = append(a.Findings, Finding{Severity: "ERROR", Code: "DUPLICATE_CAVITY", Message: fmt.Sprintf("%s cavity IDs assigned more than once with DIFFERENT destinations: %s", c.Tag, strings.Join(dup, ", "))})
			state = "BLOCKED — DUPLICATES"
		}
		a.Audits = append(a.Audits, ConnectorAudit{c.Tag, c.CavityCount, occupied, unused, state})
	}
	// Even with a complete-looking list, the app cannot establish missing pages automatically.
	a.Findings = append(a.Findings, Finding{Severity: "REVIEW", Code: "INDEPENDENT_SOURCE_CHECK", Message: "Cavity counts and netlist are candidates; verify every relevant sheet and contact against the actual PDF before production use."})
	a.ToolingMatches = toolMatches(d)
	if a.ToolingMatches == nil {
		a.ToolingMatches = []ToolMatch{}
	}
	// Every collection is an array in JSON, including empty/unknown results.
	// This is required for the UI to distinguish an incomplete scan from an internal error.
	if a.Connectors == nil {
		a.Connectors = []Connector{}
	}
	if a.Wires == nil {
		a.Wires = []WireRow{}
	}
	if a.BOM == nil {
		a.BOM = []BOMRow{}
	}
	if a.Findings == nil {
		a.Findings = []Finding{}
	}
	if a.Audits == nil {
		a.Audits = []ConnectorAudit{}
	}
	if a.PageSummary == nil {
		a.PageSummary = []PageSummary{}
	}
	for i := range a.Findings {
		if a.Findings[i].Pages == nil {
			a.Findings[i].Pages = []int{}
		}
		if a.Findings[i].Category == "" {
			a.Findings[i].Category = categoryFor(a.Findings[i].Code)
		}
	}
	for i := range a.Connectors {
		if a.Connectors[i].Pages == nil {
			a.Connectors[i].Pages = []int{}
		}
	}
	for i := range a.Audits {
		if a.Audits[i].Occupied == nil {
			a.Audits[i].Occupied = []string{}
		}
		if a.Audits[i].Unused == nil {
			a.Audits[i].Unused = []string{}
		}
	}
	return a, nil
}
func toolMatches(d PDFData) []ToolMatch {
	refs := map[string]int{}
	for _, p := range d.Pages {
		for _, w := range p.Words {
			for _, v := range reContact.FindAllString(strings.ToUpper(w.Text), -1) {
				refs[v]++
			}
		}
	}
	out := []ToolMatch{}
	for k := range refs {
		cnt := 0
		which := "DMC wall chart"
		f, _ := assets.Open("assets/DMC_EXPANDED_REFERENCE.csv")
		if f != nil {
			r := csv.NewReader(f)
			r.FieldsPerRecord = -1
			for {
				record, err := r.Read()
				if err == io.EOF {
					break
				}
				if err != nil {
					break
				}
				for _, cell := range record {
					if compact(cell) == k {
						cnt++
						break
					}
				}
			}
			f.Close()
		}
		s := "UNRESOLVED — REVIEW CONTACT SPEC"
		if cnt > 0 {
			s = "CANDIDATE TOOL REFERENCE — VERIFY WIRE/INSERT"
		}
		out = append(out, ToolMatch{k, which, s, cnt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Part < out[j].Part })
	return out
}

var errNoFile = errors.New("no file selected")

var (
	reNounFirstConnector = regexp.MustCompile(`(?i)^\s*(?:CONNECTOR|RECEPTACLE|PLUG)\s*,`)
	reMilConnectorFamily = regexp.MustCompile(`(?i)^(?:D38999/(?:2[0-9]|1[0-9])|MS3(?:10[0-8]|11[0-9]|12[0-9]|4[5-9][0-9])|M83723/|M24308/|M83513/|M28840/|MS2747[0-9]|MS2750[0-9])`)
)
