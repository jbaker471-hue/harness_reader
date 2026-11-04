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

// positionalSchematicRows reads a wiring diagram by pin position, not by its
// colour words. Every conductor carries a printed gauge label ("24 AWG"); the
// wire runs from the pin nearest the label on its left to the pin nearest it
// on its right. A printed line may carry several labels when connectors are
// chained (a main connector to a smaller one, and that one on to another):
// each label is its own wire, between the pins on either side of it.
//
// Each pin belongs to the connector block it sits in. Pins are grouped into
// columns (left- or right-aligned numbers both line up); a column is split
// into blocks where a connector header is printed between two of its pins or
// where its pins stop; each block's connector is read once from the header
// above it: its designator, or else its header part number when exactly one
// connector of the drawing has that part. A block whose header is not found is
// never given the header of the block above it, except when the gap it follows
// is another column's header (a fan-out on the other side of the wires). The
// colour, when printed, is recorded but never decides whether a wire counts.
// Wires already read by the colour-based reader are skipped.
//
// why says, in structure only (no drawing text), why some printed wire labels
// were not read; it is "" when every one was. It goes into the gauge-label
// audit so an unread sheet explains itself in the discrepancy export.
func positionalSchematicRows(p Page, rows []WireRow, partTags map[string]string) (out []WireRow, why string) {
	if !p.PageAccurate || p.Width <= 0 {
		return nil, "page geometry is not accurate enough to read columns"
	}
	readAt := map[int]int{}
	pairRead := map[string]bool{}
	for _, r := range rows {
		readAt[int(math.Round(r.Y))]++
		pairRead[r.Connector+":"+r.Cavity+">"+r.End+":"+r.EndCavity] = true
		pairRead[r.End+":"+r.EndCavity+">"+r.Connector+":"+r.Cavity] = true
	}
	readNear := func(y float64) int {
		k := int(math.Round(y))
		return readAt[k] + readAt[k-1] + readAt[k+1]
	}
	lines := pageLines(p)
	type tok struct {
		w    Word
		line int
		em   float64
		pin  string // the cavity as read (leading zeros and a glued colour removed)
	}
	var toks []tok
	gauges := map[int][]int{} // line -> word indices of its gauge labels, left to right
	words := make([][]Word, len(lines))
	labels := 0
	for i, l := range lines {
		ws := schematicWords(l)
		words[i] = ws
		isGauge := map[int]bool{}
		for j, w := range ws {
			if (j+1 < len(ws) && schematicGauge.MatchString(w.Text) && strings.EqualFold(ws[j+1].Text, "AWG")) || reGaugeLabel.MatchString(w.Text) {
				gauges[i] = append(gauges[i], j)
				isGauge[j] = true
			}
		}
		if len(gauges[i]) == 0 {
			continue
		}
		labels += len(gauges[i])
		em := lineEm(l)
		for j, w := range ws {
			pin, ok := pinToken(w.Text)
			if isGauge[j] || !ok {
				continue
			}
			toks = append(toks, tok{w, i, em, pin})
		}
	}
	if len(gauges) < 3 {
		return nil, ""
	}
	if len(toks) < 6 {
		return nil, fmt.Sprintf("only %d pin numbers found beside the %d printed wire gauge labels", len(toks), labels)
	}
	// Columns: numbers on different lines whose left edges or right edges line up.
	parent := make([]int, len(toks))
	for i := range parent {
		parent[i] = i
	}
	find := func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	for i := range toks {
		for j := i + 1; j < len(toks); j++ {
			a, b := toks[i], toks[j]
			if a.line == b.line {
				continue
			}
			tol := 0.6 * math.Max(a.em, b.em)
			if math.Abs(a.w.X-b.w.X) <= tol || math.Abs(a.w.right()-b.w.right()) <= tol {
				parent[find(i)] = find(j)
			}
		}
	}
	type block struct{ tag, via string }
	blockOf := map[int]*block{}
	cols := map[int][]int{}
	for i := range toks {
		cols[find(i)] = append(cols[find(i)], i)
	}
	blocks, resolved := 0, 0
	isHeaderWord := func(w Word) bool {
		t := strings.ToUpper(strings.TrimSpace(w.Text))
		return !w.Rotated && ((t != "" && reRefDesignator.FindString(t) == t) || reSplitPNHead.MatchString(t))
	}
	for _, idx := range cols {
		sort.Slice(idx, func(a, b int) bool { return toks[idx[a]].w.Y > toks[idx[b]].w.Y })
		gaps := []float64{}
		for k := 1; k < len(idx); k++ {
			gaps = append(gaps, toks[idx[k-1]].w.Y-toks[idx[k]].w.Y)
		}
		pitch := 0.0
		if len(gaps) > 0 {
			s := append([]float64(nil), gaps...)
			sort.Float64s(s)
			pitch = s[len(s)/2]
		}
		colX := 0.0
		for _, t := range idx {
			colX += toks[t].w.X
		}
		colX /= float64(len(idx))
		// a connector header printed between two heights: near this column, or anywhere
		headerBetween := func(hi, lo float64, anywhere bool) bool {
			for _, w := range p.Words {
				if w.Y < hi && w.Y > lo && (anywhere || math.Abs(w.X-colX) <= p.Width*.12) && isHeaderWord(w) {
					return true
				}
			}
			return false
		}
		type seg struct {
			from, to int
			cont     bool // follows a gap that is another column's header
		}
		var segs []seg
		start := 0
		for k := 1; k < len(idx); k++ {
			hi, lo := toks[idx[k-1]].w.Y, toks[idx[k]].w.Y
			near := headerBetween(hi, lo, false)
			if near || (gaps[k-1] > 2.5*pitch && gaps[k-1] > 2.5*toks[idx[k]].em) {
				segs = append(segs, seg{start, k, false})
				start = k
				if !near && headerBetween(hi, lo, true) {
					segs = append(segs, seg{-1, -1, true}) // marks the next segment
				}
			}
		}
		segs = append(segs, seg{start, len(idx), false})
		ceiling := math.Inf(1) // a block's header lies below the block above it
		var prev *block
		cont := false
		for _, sg := range segs {
			if sg.from < 0 {
				cont = true
				continue
			}
			part := idx[sg.from:sg.to]
			pins := map[int]Word{}
			x := 0.0
			for _, t := range part {
				pins[toks[t].line] = toks[t].w
				x += toks[t].w.X
			}
			x /= float64(len(part))
			tag, via := columnConnector(p, x, pins, partTags, ceiling)
			ceiling = toks[part[len(part)-1]].w.Y
			var b *block
			switch {
			case tag != "":
				b = &block{tag, via}
			case cont && prev != nil && prev.tag != "":
				b = prev // the same connector continues past another column's header
			default:
				b = &block{}
			}
			cont = false
			if b != prev {
				blocks++
				if b.tag != "" {
					resolved++
				}
			}
			for _, t := range part {
				blockOf[t] = b
			}
			prev = b
		}
	}
	byLine := map[int][]int{}
	for i, t := range toks {
		byLine[t.line] = append(byLine[t.line], i)
	}
	noPin, noConn, same := 0, 0, 0
	// shapes of what is printed on the side of a label that has no pin (structure only)
	shapes := map[string]int{}
	emptySide := map[string]int{}
	isPinTok := map[int]map[float64]bool{}
	for _, t := range toks {
		if isPinTok[t.line] == nil {
			isPinTok[t.line] = map[float64]bool{}
		}
		isPinTok[t.line][t.w.X] = true
	}
	for i, l := range lines {
		gs := gauges[i]
		if len(gs) == 0 {
			continue
		}
		if len(gs) == 1 && readNear(l.Y) > 0 {
			continue // read by the colour-based reader
		}
		ws := words[i]
		for k, g := range gs {
			gx := ws[g].X
			lo, hi := math.Inf(-1), math.Inf(1)
			if k > 0 {
				lo = ws[gs[k-1]].X
			}
			if k+1 < len(gs) {
				hi = ws[gs[k+1]].X
			}
			li, ri, anyL, anyR := -1, -1, false, false
			for _, t := range byLine[i] {
				x := toks[t].w.X
				resolvedPin := blockOf[t] != nil && blockOf[t].tag != ""
				if x > lo && x < gx {
					anyL = true
					if resolvedPin && (li < 0 || x > toks[li].w.X) {
						li = t
					}
				}
				if x > gx && x < hi {
					anyR = true
					if resolvedPin && (ri < 0 || x < toks[ri].w.X) {
						ri = t
					}
				}
			}
			switch {
			case !anyL || !anyR:
				noPin++
				side, a, b := "left", lo, gx
				if anyL {
					side, a, b = "right", gx, hi
				}
				emptySide[side]++
				for j, w := range ws {
					if j == g || w.X <= a || w.X >= b || strings.EqualFold(w.Text, "AWG") || isPinTok[i][w.X] {
						continue
					}
					isG := false
					for _, gg := range gs {
						isG = isG || gg == j
					}
					if !isG {
						shapes[tokenShape(w.Text)]++
					}
				}
				continue
			case li < 0 || ri < 0:
				noConn++
				continue
			}
			lb, rb := blockOf[li], blockOf[ri]
			if lb.tag == rb.tag {
				same++
				continue
			}
			lw := toks[li].w
			lp, rp := toks[li].pin, toks[ri].pin
			if pairRead[lb.tag+":"+lp+">"+rb.tag+":"+rp] {
				continue
			}
			conf := "DRAFT — READ BY PIN COLUMN POSITION; CHECK SOURCE"
			if lb.via == "part" || rb.via == "part" {
				conf = "DRAFT — READ BY PIN COLUMN POSITION; CONNECTOR MATCHED BY THE HEADER PART NUMBER (DESIGNATOR NOT READ); CHECK SOURCE"
			}
			colour, gauge := "", ws[g].Text
			if m := reGaugeLabel.FindStringSubmatch(gauge); m != nil {
				gauge = m[1]
			}
			for j, w := range ws {
				if w.X == lw.X && j+1 < len(ws) && reColourWord.MatchString(ws[j+1].Text) && !strings.EqualFold(ws[j+1].Text, "AWG") {
					colour = strings.ToUpper(ws[j+1].Text)
				}
			}
			if c := schematicColor(colour); c != "" {
				colour = c
			}
			out = append(out, WireRow{Page: p.Number, Connector: lb.tag, Cavity: lp, End: rb.tag, EndCavity: rp, Color: colour, AWG: gauge, Y: l.Y, Method: "paired-schematic-position", Confidence: conf})
		}
	}
	if len(out)+len(rows) < 3 {
		out = nil
	}
	if labels-len(rows)-len(out) > 0 {
		parts := []string{}
		if noPin > 0 {
			sides := []string{}
			for _, sd := range []string{"left", "right"} {
				if emptySide[sd] > 0 {
					sides = append(sides, fmt.Sprintf("%d on the %s", emptySide[sd], sd))
				}
			}
			msg := fmt.Sprintf("%d label(s) have no pin on one side (%s)", noPin, strings.Join(sides, ", "))
			if len(shapes) > 0 {
				ks := []string{}
				for k := range shapes {
					ks = append(ks, k)
				}
				sort.Slice(ks, func(a, b int) bool {
					return shapes[ks[a]] > shapes[ks[b]] || (shapes[ks[a]] == shapes[ks[b]] && ks[a] < ks[b])
				})
				items := []string{}
				for _, k := range ks {
					items = append(items, fmt.Sprintf("%d %s", shapes[k], k))
				}
				msg += "; that side holds " + strings.Join(items, ", ")
			} else {
				msg += "; that side holds no text on the line"
			}
			parts = append(parts, msg)
		}
		if noConn > 0 {
			parts = append(parts, fmt.Sprintf("%d label(s) have a pin whose connector block header was not matched", noConn))
		}
		if same > 0 {
			parts = append(parts, fmt.Sprintf("%d label(s) have both pins in blocks matched to the same connector", same))
		}
		why = fmt.Sprintf("%d pin blocks found, %d with a connector matched", blocks, resolved)
		if len(parts) > 0 {
			why += "; " + strings.Join(parts, "; ")
		}
	}
	return out, why
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
		total := 0
		for _, l := range gl {
			total += gaugeLabelCount(l)
		}
		a := TableAudit{Page: p.Number, Title: "wire gauge labels", Counted: true, RuledRows: total, RowsRead: len(rows), Missed: []MissedRow{}, Connectors: []string{}}
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
			n, found := gaugeLabelCount(l), 0
			for _, r := range rows {
				if math.Abs(r.Y-l.Y) <= 2 {
					found++
				}
			}
			switch {
			case found == 0:
				a.Missed = append(a.Missed, MissedRow{Text: strings.TrimSpace(l.Text), Reason: "a printed wire line (gauge label) with no wire read", Y: l.Y})
			case found < n:
				a.Missed = append(a.Missed, MissedRow{Text: strings.TrimSpace(l.Text), Reason: fmt.Sprintf("a printed line with %d gauge labels (chained connectors) but only %d wire(s) read", n, found), Y: l.Y})
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
			a.Detail = fmt.Sprintf("sheet %d shows %d printed wire gauge labels but only %d wire(s) were read on it", p.Number, total, len(rows))
			if r := why[p.Number]; r != "" {
				a.Detail += " (column reader: " + r + ")"
			}
			a.Detail += "."
		} else {
			a.Status = "COMPLETE"
			a.Detail = fmt.Sprintf("every one of the %d printed wire gauge labels on sheet %d has a wire read", total, p.Number)
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
// Text at or above ceiling (the lowest pin of the block above in the same
// column) belongs to that block and is not considered.
func columnConnector(p Page, colX float64, rows map[int]Word, partTags map[string]string, ceiling float64) (string, string) {
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
		if w.Rotated || reRefDesignator.FindString(t) != t || t == "" || w.Y >= ceiling {
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
		if !reSplitPNHead.MatchString(ht) || h.Y-top <= 0 || h.Y-top > 12*size || math.Abs(h.X-colX) > reach || h.Y >= ceiling {
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

// gaugeLabelCount is the number of wire gauge labels on one printed line (more
// than one where connectors are chained along the line).
func gaugeLabelCount(l evidenceLine) int {
	n := 0
	for i, w := range l.Words {
		if reGaugeLabel.MatchString(w.Text) {
			n++
			continue
		}
		if i+1 < len(l.Words) && schematicGauge.MatchString(strings.TrimSpace(w.Text)) && strings.EqualFold(strings.TrimSpace(l.Words[i+1].Text), "AWG") && l.Words[i+1].X-w.right() < 2*math.Max(w.Size, 4) {
			n++
		}
	}
	if n == 0 {
		// "24 AWG" printed inside one fragment with other text
		for _, w := range schematicWords(l) {
			if strings.EqualFold(w.Text, "AWG") {
				n++
			}
		}
	}
	if n == 0 {
		n = 1
	}
	return n
}

var (
	rePinNumber    = regexp.MustCompile(`^0*([1-9][0-9]{0,2})$`)
	rePinLetter    = regexp.MustCompile(`^(?:[A-HJ-NP-Z]|[a-hj-np-z]|([A-HJ-NP-Z])[A-HJ-NP-Z])$`)
	rePinColourEnd = regexp.MustCompile(`^0*([1-9][0-9]{0,2})([A-Z]{2,6}(?:/[A-Z]{2,6})?)$`)
	rePinColourBeg = regexp.MustCompile(`^([A-Z]{2,6}(?:/[A-Z]{2,6})?)0*([1-9][0-9]{0,2})$`)
)

// pinToken reads a cavity label as printed beside a connector block: a number
// (leading zeros dropped), a contact letter (A, b, AA; I and O are not used
// as contact letters), or a number glued to its colour ("12WHT", "WHT12").
func pinToken(t string) (string, bool) {
	t = strings.TrimSpace(t)
	if m := rePinNumber.FindStringSubmatch(t); m != nil {
		return m[1], true
	}
	if m := rePinLetter.FindStringSubmatch(t); m != nil {
		if len(t) == 2 && t[0] != t[1] {
			return "", false // two different letters is a word or a colour, not a contact
		}
		return t, true
	}
	u := strings.ToUpper(t)
	if m := rePinColourEnd.FindStringSubmatch(u); m != nil && schematicColor(m[2]) != "" {
		return m[1], true
	}
	if m := rePinColourBeg.FindStringSubmatch(u); m != nil && schematicColor(m[1]) != "" {
		return m[2], true
	}
	return "", false
}

// tokenShape names the form of a printed item without its text, for reasons
// that must not carry drawing content.
func tokenShape(t string) string {
	t = strings.TrimSpace(t)
	alpha, digit, other := 0, 0, 0
	for _, r := range t {
		switch {
		case r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z':
			alpha++
		case r >= '0' && r <= '9':
			digit++
		default:
			other++
		}
	}
	switch {
	case t == "":
		return "empty item"
	case strings.ContainsAny(t, "-–") && digit > 0:
		return "hyphenated code"
	case strings.Contains(t, "/") && digit > 0:
		return "slashed code"
	case alpha == 0 && other == 0:
		return fmt.Sprintf("%d-digit number", digit)
	case digit == 0 && other == 0 && schematicColor(strings.ToUpper(t)) != "":
		return "colour word"
	case digit == 0 && other == 0 && alpha == 1:
		return "single letter I or O"
	case digit == 0 && other == 0:
		return fmt.Sprintf("%d-letter word", alpha)
	case other == 0 && t[0] >= '0' && t[0] <= '9':
		return "digits then letters"
	case other == 0:
		return "letters then digits"
	case alpha == 0 && digit == 0:
		return "symbol"
	}
	return "mixed item with symbols"
}
