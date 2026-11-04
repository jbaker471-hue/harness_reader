package main

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// Wire coverage checks that need nothing but the wiring sheet itself.
//
// 1. positionalSchematicRows: on a wiring diagram whose rows were read, a line
//    with a pin in the left connector column and a pin in the right connector
//    column is a wire even when its colour word is not one the reader knows.
// 2. gaugeLabelAudits: every wire line on a wiring diagram carries a printed
//    gauge label ("24 AWG"). A sheet whose gauge labels outnumber the wires read
//    on it has unread wires — including a sheet where no wire was read at all —
//    and process of elimination must not be released.

var (
	reGaugeLabel = regexp.MustCompile(`(?i)^\s*#?([0-9]{1,2})\s*AWG\s*$`)
	reColourWord = regexp.MustCompile(`^[A-Za-z]{2,6}(?:/[A-Za-z]{2,6})?$`)
)

// positionalSchematicRows reads a wiring diagram by its columns, not its
// colour words: a conductor is a printed line with a pin in the column beside
// the left connector block and a pin in the column beside the right one. The
// colour, when printed, is recorded but never decides whether a wire counts.
// Lines already read by the colour-based reader are skipped.
//
// why says, in structure only (no drawing text), why no wire was read by
// column; it is "" when wires were read. It goes into the gauge-label audit so
// an unread sheet explains itself in the discrepancy export.
func positionalSchematicRows(p Page, rows []WireRow, partTags map[string]string) (out []WireRow, why string) {
	if !p.PageAccurate || p.Width <= 0 {
		return nil, "page geometry is not accurate enough to read columns"
	}
	read := map[int]bool{}
	for _, r := range rows {
		read[int(math.Round(r.Y))] = true
	}
	isRead := func(y float64) bool {
		k := int(math.Round(y))
		return read[k] || read[k-1] || read[k+1]
	}
	lines := pageLines(p)
	type tok struct {
		w    Word
		line int
		em   float64
	}
	var digits []tok
	var gaugeX []float64
	words := make([][]Word, len(lines))
	for i, l := range lines {
		ws := schematicWords(l)
		words[i] = ws
		em := lineEm(l)
		for j, w := range ws {
			if j+1 < len(ws) && strings.EqualFold(ws[j+1].Text, "AWG") {
				gaugeX = append(gaugeX, w.X) // the gauge number of a wire label, not a pin
				continue
			}
			if reDigitPin.MatchString(w.Text) {
				digits = append(digits, tok{w, i, em})
			}
		}
	}
	if len(digits) < 6 {
		return nil, fmt.Sprintf("only %d pin numbers found on the sheet", len(digits))
	}
	mid := p.Width / 2
	if len(gaugeX) >= 3 {
		sort.Float64s(gaugeX)
		mid = gaugeX[len(gaugeX)/2]
	}
	// Cluster digit tokens into vertical columns.
	sort.Slice(digits, func(i, j int) bool { return digits[i].w.X < digits[j].w.X })
	type column struct {
		x     float64
		lines map[int]Word
	}
	var cols []column
	for _, d := range digits {
		if n := len(cols); n > 0 && d.w.X-cols[n-1].x <= 1.2*d.em {
			cols[n-1].lines[d.line] = d.w
			continue
		}
		cols = append(cols, column{x: d.w.X, lines: map[int]Word{d.line: d.w}})
	}
	// The pin columns are the innermost well-populated columns on each side
	// of the wire run (signal names and face-view labels lie further out).
	var left, right *column
	for i := range cols {
		c := &cols[i]
		if len(c.lines) < 3 {
			continue
		}
		if c.x < mid && (left == nil || c.x > left.x) {
			left = c
		}
		if c.x > mid && (right == nil || c.x < right.x) {
			right = c
		}
	}
	populated := 0
	for _, c := range cols {
		if len(c.lines) >= 3 {
			populated++
		}
	}
	switch {
	case left == nil && right == nil:
		return nil, fmt.Sprintf("no pin column found on either side of the wire labels (%d number columns, %d with 3 or more pins)", len(cols), populated)
	case left == nil:
		return nil, fmt.Sprintf("no pin column found left of the wire labels (%d number columns, %d with 3 or more pins)", len(cols), populated)
	case right == nil:
		return nil, fmt.Sprintf("no pin column found right of the wire labels (%d number columns, %d with 3 or more pins)", len(cols), populated)
	case right.x-left.x < p.Width*.15:
		return nil, fmt.Sprintf("the two pin columns are only %.0f%% of the sheet width apart", 100*(right.x-left.x)/p.Width)
	}
	// Which connector each column belongs to is decided once, from the header
	// above the column: its designator, or else its part number when exactly
	// one connector of the drawing has that part.
	from, fromVia := columnConnector(p, left.x, left.lines, partTags)
	to, toVia := columnConnector(p, right.x, right.lines, partTags)
	switch {
	case from == "" && to == "":
		return nil, fmt.Sprintf("pin columns found (%d and %d pins) but no connector designator or header part number was matched above either column", len(left.lines), len(right.lines))
	case from == "":
		return nil, fmt.Sprintf("pin columns found (%d and %d pins); the right column is %s (by %s) but no connector was matched above the left column", len(left.lines), len(right.lines), to, toVia)
	case to == "":
		return nil, fmt.Sprintf("pin columns found (%d and %d pins); the left column is %s (by %s) but no connector was matched above the right column", len(left.lines), len(right.lines), from, fromVia)
	case from == to:
		return nil, fmt.Sprintf("both pin columns matched the same connector %s (left by %s, right by %s)", from, fromVia, toVia)
	}
	conf := "DRAFT — READ BY PIN COLUMN POSITION; CHECK SOURCE"
	if fromVia == "part" || toVia == "part" {
		conf = "DRAFT — READ BY PIN COLUMN POSITION; CONNECTOR MATCHED BY THE HEADER PART NUMBER (DESIGNATOR NOT READ); CHECK SOURCE"
	}
	paired := 0
	for i, l := range lines {
		lw, okL := left.lines[i]
		rw, okR := right.lines[i]
		if !okL || !okR {
			continue
		}
		paired++
		if isRead(l.Y) {
			continue
		}
		colour, gauge := "", ""
		ws := words[i]
		for j, w := range ws {
			if w.X == lw.X && j+1 < len(ws) && reColourWord.MatchString(ws[j+1].Text) {
				colour = strings.ToUpper(ws[j+1].Text)
			}
			if j+1 < len(ws) && schematicGauge.MatchString(w.Text) && strings.EqualFold(ws[j+1].Text, "AWG") {
				gauge = w.Text
			}
			if m := reGaugeLabel.FindStringSubmatch(w.Text); m != nil {
				gauge = m[1]
			}
		}
		if c := schematicColor(colour); c != "" {
			colour = c
		}
		out = append(out, WireRow{Page: p.Number, Connector: from, Cavity: lw.Text, End: to, EndCavity: rw.Text, Color: colour, AWG: gauge, Y: l.Y, Method: "paired-schematic-position",
			Confidence: conf})
	}
	if len(out)+len(rows) < 3 {
		return nil, fmt.Sprintf("columns %s and %s found, but only %d line(s) have a pin in both columns", from, to, paired)
	}
	return out, ""
}

var reDigitPin = regexp.MustCompile(`^[1-9][0-9]{0,2}$`)

// gaugeLines returns the lines of a page that carry a stand-alone wire gauge label.
func gaugeLines(p Page) []evidenceLine {
	var out []evidenceLine
	for _, l := range pageLines(p) {
		hit := false
		for i, w := range l.Words {
			if reGaugeLabel.MatchString(w.Text) {
				hit = true
			}
			if i+1 < len(l.Words) && schematicGauge.MatchString(strings.TrimSpace(w.Text)) && strings.EqualFold(strings.TrimSpace(l.Words[i+1].Text), "AWG") && l.Words[i+1].X-w.right() < 2*math.Max(w.Size, 4) {
				hit = true
			}
		}
		if hit {
			out = append(out, l)
		}
	}
	return out
}

// gaugeLabelAudits compares the printed gauge labels of each wiring sheet with
// the wires read on it. connectors are every connector of the drawing: a
// sheet with no wire read cannot say which connectors it belongs to.
func gaugeLabelAudits(d PDFData, wires []WireRow, connectors []string, why map[int]string) []TableAudit {
	out := []TableAudit{}
	byPage := map[int][]WireRow{}
	all := append([]string(nil), connectors...)
	for _, w := range wires {
		byPage[w.Page] = append(byPage[w.Page], w)
		for _, t := range []string{w.Connector, w.End} {
			if t != "" && t != "UNASSIGNED" && t != "UNRESOLVED ENDPOINT" {
				all = append(all, t)
			}
		}
	}
	connectors = uniqueStrings(all)
	for _, p := range d.Pages {
		gl := gaugeLines(p)
		if len(gl) < 3 {
			continue // not a wiring diagram (a parts list says "24 AWG" inside longer text)
		}
		rows := byPage[p.Number]
		a := TableAudit{Page: p.Number, Title: "wire gauge labels", Counted: true, RuledRows: len(gl), RowsRead: len(rows), Missed: []MissedRow{}, Connectors: []string{}}
		seen := map[string]bool{}
		for _, r := range rows {
			for _, t := range []string{r.Connector, r.End} {
				if t != "" && t != "UNASSIGNED" && t != "UNRESOLVED ENDPOINT" && !seen[t] {
					seen[t] = true
					a.Connectors = append(a.Connectors, t)
				}
			}
		}
		for _, l := range gl {
			found := false
			for _, r := range rows {
				if math.Abs(r.Y-l.Y) <= 2 {
					found = true
					break
				}
			}
			if !found {
				a.Missed = append(a.Missed, MissedRow{Text: strings.TrimSpace(l.Text), Reason: "a printed wire line (gauge label) with no wire read", Y: l.Y})
			}
		}
		if len(a.Missed) > 0 {
			// The unread wires could belong to any connector.
			for _, c := range connectors {
				if !seen[c] {
					seen[c] = true
					a.Connectors = append(a.Connectors, c)
				}
			}
			a.Status = "INCOMPLETE"
			a.Detail = fmt.Sprintf("sheet %d shows %d printed wire gauge labels but only %d wire(s) were read on it", p.Number, len(gl), len(rows))
			if r := why[p.Number]; r != "" {
				a.Detail += " (column reader: " + r + ")"
			}
			a.Detail += "."
		} else {
			a.Status = "COMPLETE"
			a.Detail = fmt.Sprintf("every one of the %d printed wire gauge labels on sheet %d has a wire read", len(gl), p.Number)
		}
		sort.Strings(a.Connectors)
		out = append(out, a)
	}
	return out
}

// headerPartNumbers reads connector headers printed on wiring sheets, where
// the part number is split across lines under the designator:
//
//	RIG-P5
//	D38999/
//	26FJ35PA
//
// It returns designator -> joined part number. Only a designator directly
// above the split part number (same column) is used.
var (
	reSplitPNHead = regexp.MustCompile(`^(D38999/|MS[0-9]{4,5}/?|M83723/|M24308/)$`)
	reSplitPNTail = regexp.MustCompile(`^[0-9]{2}[A-Z]{1,2}[0-9]{1,3}(?:-[0-9]{1,3})?[A-Z]{1,2}$`)
)

func headerPartNumbers(p Page) map[string]string {
	out := map[string]string{}
	for _, h := range p.Words {
		ht := strings.TrimSpace(strings.ToUpper(h.Text))
		if !reSplitPNHead.MatchString(ht) {
			continue
		}
		em := math.Max(h.Size, 4)
		var tail *Word
		for i := range p.Words {
			w := p.Words[i]
			d := h.Y - w.Y
			if d > 0.3*em && d < 2*em && math.Abs(w.X-h.X) <= 1.5*em && reSplitPNTail.MatchString(strings.TrimSpace(strings.ToUpper(w.Text))) {
				tail = &p.Words[i]
			}
		}
		if tail == nil {
			continue
		}
		tag := ""
		best := math.Inf(1)
		for _, w := range p.Words {
			d := w.Y - h.Y
			t := strings.ToUpper(strings.TrimSpace(w.Text))
			if d > 0.3*em && d < 2.5*em && math.Abs(w.X-h.X) <= 3*em && reRefDesignator.MatchString(t) && reRefDesignator.FindString(t) == t && d < best {
				tag, best = t, d
			}
		}
		if tag != "" {
			out[tag] = ht + strings.TrimSpace(strings.ToUpper(tail.Text))
		}
	}
	return out
}

func seenWireTag(ws []WireRow, tag string) bool {
	for _, w := range ws {
		if w.Connector == tag || w.End == tag {
			return true
		}
	}
	return false
}

// reGluedPart matches "part numbers" made of a note number and a word ("7DELETED").
var reGluedPart = regexp.MustCompile(`^[0-9]{1,3}[A-Z]{4,}$`)

// columnConnector finds the connector a pin column belongs to from the header
// printed above the column's first pin. via is "designator" or "part".
func columnConnector(p Page, colX float64, rows map[int]Word, partTags map[string]string) (string, string) {
	top, size := math.Inf(-1), 6.0
	for _, w := range rows {
		if w.Y > top {
			top, size = w.Y, math.Max(w.Size, 4)
		}
	}
	reach := p.Width * .12
	best, bestScore, ambiguous := "", math.Inf(1), false
	for _, w := range p.Words {
		t := strings.ToUpper(strings.TrimSpace(w.Text))
		if w.Rotated || reRefDesignator.FindString(t) != t || t == "" {
			continue
		}
		dy, dx := w.Y-top, math.Abs(w.X-colX)
		if dy <= 0.3*size || dy > 12*size || dx > reach {
			continue
		}
		s := dy + 0.3*dx
		switch {
		case s < bestScore-size:
			best, bestScore, ambiguous = t, s, false
		case math.Abs(s-bestScore) <= size && t != best:
			ambiguous = true
		}
	}
	if best != "" && !ambiguous {
		return best, "designator"
	}
	// the part number printed in the same header ("D38999/" over its tail)
	for _, h := range p.Words {
		ht := strings.ToUpper(strings.TrimSpace(h.Text))
		if !reSplitPNHead.MatchString(ht) || h.Y-top <= 0 || h.Y-top > 12*size || math.Abs(h.X-colX) > reach {
			continue
		}
		em := math.Max(h.Size, 4)
		for _, w := range p.Words {
			d := h.Y - w.Y
			tt := strings.ToUpper(strings.TrimSpace(w.Text))
			if d > 0.3*em && d < 2*em && math.Abs(w.X-h.X) <= 1.5*em && reSplitPNTail.MatchString(tt) {
				if tag := partTags[ht+tt]; tag != "" {
					return tag, "part"
				}
			}
		}
	}
	return "", ""
}

// headerPartTags maps a connector part number to the one designator the
// drawing's headers give it. A part labelled with two different designators
// maps to none, and is returned as a conflict that holds both connectors.
func headerPartTags(d PDFData) (map[string]string, []TableAudit) {
	type seen struct {
		tag  string
		page int
	}
	first := map[string]seen{}
	out, bad := map[string]string{}, map[string]bool{}
	var conflicts []TableAudit
	for _, p := range d.Pages {
		for tag, part := range headerPartNumbers(p) {
			if prev, ok := first[part]; ok && prev.tag != tag {
				if !bad[part] {
					conflicts = append(conflicts, TableAudit{Page: p.Number, Title: "connector header", Status: "INCOMPLETE", Connectors: []string{prev.tag, tag},
						Missed: []MissedRow{{Text: part + " labelled " + tag, Reason: "the same connector part is labelled " + prev.tag + " on sheet " + itoa(prev.page)}},
						Detail: fmt.Sprintf("connector part %s is labelled %s on sheet %d but %s on sheet %d; resolve which designator is correct", part, prev.tag, prev.page, tag, p.Number)})
				}
				bad[part] = true
				continue
			}
			if _, ok := first[part]; !ok {
				first[part] = seen{tag, p.Number}
			}
			out[part] = tag
		}
	}
	for part := range bad {
		delete(out, part)
	}
	return out, conflicts
}

var reInsertCore = regexp.MustCompile(`D38999/\d{2}[A-Z]([A-HJ][0-9]{2}[PSHJAB])`)

// normPart drops spaces and dashes and reads M38999/MD38999 as D38999 so formatting differences
// between the header and the parts list are not reported as a conflict.
func normPart(s string) string {
	s = strings.ToUpper(strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '‐' || r == '–' {
			return -1
		}
		return r
	}, s))
	switch {
	case strings.HasPrefix(s, "MD38999"):
		s = s[1:]
	case strings.HasPrefix(s, "M38999"):
		s = "D" + s[1:]
	}
	return s
}

// headerPartFinding compares the part printed in a wiring sheet header with
// the part the connector already has. Formatting differences (spaces, dashes,
// M38999 vs D38999, a suffix on one side) are not reported. Any other
// difference is a drawing conflict (ERROR): a different slash sheet, finish,
// key or contact type is a different connector to order and build, even when
// the insert is the same. The message says whether the insert and contact type
// agree (cavities the same), and whether the header part is another
// connector's part (the reader may have paired it with the wrong designator).
func headerPartFinding(conns []Connector, c Connector, header string, page int) (Finding, bool) {
	if normPart(c.Part) == normPart(header) || strings.HasPrefix(normPart(c.Part), normPart(header)) || strings.HasPrefix(normPart(header), normPart(c.Part)) {
		return Finding{}, false
	}
	f := Finding{Severity: "ERROR", Category: CatConflict, Code: "CONNECTOR_PART_MISMATCH", Pages: []int{page}, Connector: c.Tag, Part: c.Part, Expected: "Parts list: " + c.Part, Observed: "Wiring sheet header: " + header}
	f.Message = fmt.Sprintf("%s: the wiring sheet header gives %s but the parts list gives %s.", c.Tag, header, c.Part)
	h, l := reInsertCore.FindStringSubmatch(strings.ToUpper(header)), reInsertCore.FindStringSubmatch(strings.ToUpper(c.Part))
	switch {
	case h != nil && l != nil && h[1] == l[1]:
		f.Message += fmt.Sprintf(" Both have insert %s, so the cavity layout is the same; the connector to order and build is not.", h[1])
	case h != nil && l != nil && h[1][:3] == l[1][:3] && isPinSocketPair(h[1][3], l[1][3]):
		f.Message += fmt.Sprintf(" The two are mating halves (insert %s with %s contacts vs %s with %s contacts): the header may show the connector this one mates with, not this connector.", h[1][:3], contactWord(h[1][3]), l[1][:3], contactWord(l[1][3]))
	case h != nil && l != nil:
		f.Message += fmt.Sprintf(" The inserts differ (%s vs %s), so the cavity layout or contact type is not the same.", h[1], l[1])
	}
	for _, o := range conns {
		if o.Tag == c.Tag || normPart(o.Part) != normPart(header) {
			continue
		}
		if o.Tag == "" || o.Tag == "UNASSIGNED" || o.Tag == "UNRESOLVED ENDPOINT" {
			f.Message += " The parts list also lists the header part, for a connector whose designator was not read: check that this header belongs to " + c.Tag + "."
		} else {
			f.Message += fmt.Sprintf(" The header part is also the parts-list part of %s: check that this header belongs to %s.", o.Tag, c.Tag)
		}
		break
	}
	f.Message += " The parts-list part is kept until engineering resolves it."
	return f, true
}

func isPinSocketPair(a, b byte) bool {
	return (a == 'P' && b == 'S') || (a == 'S' && b == 'P')
}

func contactWord(b byte) string {
	switch b {
	case 'P':
		return "pin"
	case 'S':
		return "socket"
	}
	return string(b)
}
