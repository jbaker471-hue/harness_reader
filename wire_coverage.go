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
func positionalSchematicRows(p Page, rows []WireRow) []WireRow {
	if !p.PageAccurate || p.Width <= 0 {
		return nil
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
		return nil
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
	if left == nil || right == nil || right.x-left.x < p.Width*.15 {
		return nil
	}
	var out []WireRow
	for i, l := range lines {
		lw, okL := left.lines[i]
		rw, okR := right.lines[i]
		if !okL || !okR || isRead(l.Y) {
			continue
		}
		em := lineEm(l)
		from, to := schematicTag(p, lw.X, l.Y, em), schematicTag(p, rw.X, l.Y, em)
		if from == "" || to == "" || from == to {
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
			Confidence: "DRAFT — READ BY PIN COLUMN POSITION; CHECK SOURCE"})
	}
	if len(out)+len(rows) < 3 {
		return nil
	}
	return out
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
func gaugeLabelAudits(d PDFData, wires []WireRow, connectors []string) []TableAudit {
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
