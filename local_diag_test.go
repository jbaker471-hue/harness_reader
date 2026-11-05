package main

// LOCAL STRUCTURE DIAGNOSTIC — never part of an automated run. Skipped unless
// TRIVEX_DIAG_DIR names a folder of drawings on this PC. It writes a report to
// TRIVEX_DIAG_OUT on the PC. The report holds ONLY counts, shape classes and
// relative positions: no text, designators, part numbers, file names or
// checksums. Drawings are numbered in an order that does not reveal names.
// The user reviews the report on the PC before anyone else reads it.

import (
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// shapeClass is a one-word class for a printed item, never its text.
func shapeClass(t string) string {
	t = strings.TrimSpace(t)
	u := strings.ToUpper(t)
	switch {
	case t == "":
		return "empty"
	case strings.EqualFold(t, "AWG"):
		return "AWG"
	case reGaugeLabel.MatchString(t):
		return "GAUGE+AWG"
	case reDigitPin.MatchString(t):
		return "num"
	case schematicColor(u) != "":
		return "COLOUR"
	case reRefDesignator.FindString(u) == u:
		return "DESIGNATOR"
	case reSplitPNHead.MatchString(u):
		return "PARTHEAD"
	case reSplitPNTail.MatchString(u):
		return "PARTTAIL"
	}
	return tokenShape(t)
}

func TestLocalStructureDiagnostic(t *testing.T) {
	dir, outPath := os.Getenv("TRIVEX_DIAG_DIR"), os.Getenv("TRIVEX_DIAG_OUT")
	if dir == "" || outPath == "" {
		t.Skip("local diagnostic: set TRIVEX_DIAG_DIR and TRIVEX_DIAG_OUT")
	}
	var files []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && p != dir {
			return filepath.SkipDir // this folder only
		}
		if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".pdf") {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	if len(files) > 25 {
		files = files[:25]
	}
	out := &strings.Builder{}
	pf := func(f string, v ...any) { fmt.Fprintf(out, f+"\n", v...) }
	pf("STRUCTURE REPORT: counts, shape classes and relative positions only. No drawing text.")
	pf("drawings found: %d", len(files))
	for di, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			pf("\n### drawing %d: could not be read", di+1)
			continue
		}
		x, err := ExtractDocument(b)
		if err != nil {
			pf("\n### drawing %d: text extraction failed", di+1)
			continue
		}
		v, err := viewFromExtraction(x)
		if err != nil {
			pf("\n### drawing %d: analysis failed", di+1)
			continue
		}
		pf("\n### drawing %d: %d sheets, engine %s", di+1, len(x.Data.Pages), x.Engine)
		withLen := 0
		for _, w := range v.Wires {
			if strings.TrimSpace(w.Length) != "" {
				withLen++
			}
		}
		knownPos := 0
		for _, c := range v.Connectors {
			if c.Cavities > 0 {
				knownPos++
			}
		}
		lab := 0
		for _, pg := range x.Data.Pages {
			for _, l := range gaugeLines(pg) {
				lab += gaugeLabelCount(l)
			}
		}
		pf("WIRES: %d gauge labels on the wiring sheets, %d wire rows read", lab, len(v.Wires))
		{
			got, _ := readAnswers(v)
			keys := []string{}
			for k := range got {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for ci, k := range keys {
				a := got[k]
				per := map[string]int{}
				for _, w := range v.Wires {
					if strings.EqualFold(w.Connector, k) {
						per[w.Cavity]++
					}
					if strings.EqualFold(w.End, k) {
						per[w.EndCavity]++
					}
				}
				double := 0
				for _, n := range per {
					if n > 1 {
						double++
					}
				}
				pf("ASSIGN: connector %d: positions %d, wired %d, not used %d, pins with 2+ wires %d", ci+1, a.Positions, a.Wired, a.Unused, double)
			}
			done, total := 0, 0
			for _, t := range v.TableAudits {
				if t.Title == "wire gauge labels" {
					total++
					if t.Status == "COMPLETE" {
						done++
					}
				}
			}
			pf("ASSIGN: wiring sheets with every gauge label read %d of %d", done, total)
		}
		pf("RESULT: connectors %d (positions known %d) · wire rows %d (with a length %d) · parts-list rows %d · findings %d", len(v.Connectors), knownPos, len(v.Wires), withLen, len(v.Boms), len(v.Issues))
		methods := map[string]int{}
		for _, w := range v.Wires {
			methods[w.Method]++
		}
		pf("wire rows by reader: %v", methods)
		codes := map[string]int{}
		for _, i := range v.Issues {
			codes[i.Code]++
		}
		pf("finding codes: %v", codes)
		for i, p := range x.Data.Pages {
			pl := x.Layouts[i]
			single, rot, multi := 0, 0, 0
			sizes := map[int]int{}
			for _, w := range p.Words {
				n := len([]rune(strings.TrimSpace(w.Text)))
				if n == 1 {
					single++
				}
				if len(strings.Fields(w.Text)) > 1 {
					multi++
				}
				if w.Rotated {
					rot++
				}
				sizes[int(math.Round(w.Size))]++
			}
			pct := func(n int) float64 {
				if len(p.Words) == 0 {
					return 0
				}
				return 100 * float64(n) / float64(len(p.Words))
			}
			pf("\n  sheet %d: %.0fx%.0f rot=%d accurate=%v · fragments %d (single-char %.0f%%, multi-word %.0f%%, rotated %.0f%%) · font sizes %v · horizontal rules %d", p.Number, p.Width, p.Height, pl.Rotation, p.PageAccurate, len(p.Words), pct(single), pct(multi), pct(rot), sizes, len(p.HRules))
			shapes := map[string]int{}
			for _, w := range p.Words {
				for _, f := range strings.Fields(w.Text) {
					shapes[shapeClass(f)]++
				}
			}
			pf("    item shapes on the sheet: %v", shapes)
			gl := gaugeLines(p)
			labels := 0
			for _, l := range gl {
				labels += gaugeLabelCount(l)
			}
			awgWords := 0
			for _, w := range p.Words {
				if strings.Contains(strings.ToUpper(w.Text), "AWG") {
					awgWords++
				}
			}
			pf("    fragments containing AWG: %d · lines with a gauge label: %d (labels %d)", awgWords, len(gl), labels)
			colour := pairedSchematicRows(p)
			pos, why := positionalSchematicRows(p, colour, nil)
			tw, twhy := tracedWireRows(p, append(append([]WireRow{}, colour...), pos...), nil)
			if labels > 0 && len(colour)+len(pos)+len(tw) < labels {
				pf("    name check: %s", nameMatchStats(x.Data.Pages, i))
				pf("    part check: %s", partMatchStats(x.Data, p))
				for _, d := range nameDebug(p) {
					pf("    names: %s", d)
				}
				for _, d := range endPatterns(p) {
					pf("    ends: %s", d)
				}
				for _, d := range farEndItems(p, 3) {
					pf("    far end: %s", d)
				}
				for _, d := range traceDebug(p, 3) {
					pf("    trace: %s", d)
				}
			}
			pf("    colour reader rows %d · column reader rows %d · traced rows %d · drawn segments %d · column reader says: %q · tracer says: %q", len(colour), len(pos), len(tw), len(p.Segs), why, twhy)
			for _, tb := range genericTables(p) {
				roles := []string{}
				for _, c := range tb.Cols {
					roles = append(roles, c.role)
				}
				pf("    table kind=%s rows=%d up=%v columns=%v", tb.Kind, len(tb.Rows), tb.Up, roles)
			}
			// the shape of a few wire lines, with relative positions (0 = left edge, 1 = right edge)
			shown := 0
			for _, l := range gl {
				if shown >= 4 {
					break
				}
				shown++
				parts := []string{}
				for _, w := range schematicWords(l) {
					parts = append(parts, fmt.Sprintf("%s@%.2f", shapeClass(w.Text), w.X/math.Max(p.Width, 1)))
				}
				pf("    wire line %d: %s", shown, strings.Join(parts, " "))
			}
			// if there are AWG fragments but no gauge lines, show how those fragments look
			if awgWords > 0 && len(gl) == 0 {
				n := 0
				for _, l := range pageLines(p) {
					if !strings.Contains(strings.ToUpper(l.Text), "AWG") || n >= 4 {
						continue
					}
					n++
					parts := []string{}
					for _, w := range l.Words {
						cls := []string{}
						for _, f := range strings.Fields(w.Text) {
							cls = append(cls, shapeClass(f))
						}
						parts = append(parts, "["+strings.Join(cls, " ")+"]")
					}
					pf("    line with AWG (fragments in brackets): %s", strings.Join(parts, " "))
				}
			}
		}
	}
	os.WriteFile(outPath, []byte(out.String()), 0o600)
}

// nameMatchStats tests, by counts only, whether a wire can be matched to its
// far end by a name printed on its line (signal name or wire number) that is
// printed again beside a pin elsewhere on the sheet or the drawing.
func nameMatchStats(pages []Page, pi int) string {
	p := pages[pi]
	edge := 0.03 * p.Width
	isName := func(w Word) bool {
		t := strings.TrimSpace(w.Text)
		u := strings.ToUpper(t)
		if len(t) < 2 || w.X < edge || w.X > p.Width-edge || strings.EqualFold(t, "AWG") || schematicColor(u) != "" || schematicGauge.MatchString(t) {
			return false
		}
		_, pin := pinAtEnd(t)
		return !pin
	}
	besidePin := func(pg Page, w Word) bool {
		for _, v := range pg.Words {
			if _, ok := pinAtEnd(strings.TrimSpace(v.Text)); ok && math.Abs(v.Y-w.Y) < 2 && math.Abs(v.X-w.X) < 6*math.Max(w.Size, 4) && v.X != w.X {
				return true
			}
		}
		return false
	}
	labels, withName, onceSheetPin, onceDrawingPin, many, none := 0, 0, 0, 0, 0, 0
	for _, l := range gaugeLines(p) {
		labels++
		var names []Word
		for _, w := range schematicWords(l) {
			if isName(w) {
				names = append(names, w)
			}
		}
		if len(names) == 0 {
			continue
		}
		withName++
		bestSheet, bestDrawing, anyMany := false, false, false
		for _, nm := range names {
			sheetHits, drawHits, sheetPin, drawPin := 0, 0, 0, 0
			for qi, pg := range pages {
				for _, w := range pg.Words {
					for _, f := range strings.Fields(w.Text) {
						if f != nm.Text || (qi == pi && math.Abs(w.Y-l.Y) < 2) {
							continue
						}
						ww := Word{Text: f, X: w.X, Y: w.Y, Size: w.Size}
						if qi == pi {
							sheetHits++
							if besidePin(pg, ww) {
								sheetPin++
							}
						}
						drawHits++
						if besidePin(pg, ww) {
							drawPin++
						}
					}
				}
			}
			if sheetHits == 1 && sheetPin == 1 {
				bestSheet = true
			}
			if drawHits == 1 && drawPin == 1 {
				bestDrawing = true
			}
			if drawHits > 1 {
				anyMany = true
			}
		}
		switch {
		case bestSheet:
			onceSheetPin++
		case bestDrawing:
			onceDrawingPin++
		case anyMany:
			many++
		default:
			none++
		}
	}
	return fmt.Sprintf("%d labels; %d with a name item on the line; name printed once more beside a pin on this sheet %d, on another sheet %d; printed several times %d; not printed elsewhere %d", labels, withName, onceSheetPin, onceDrawingPin, many, none)
}

// partMatchStats compares, by counts only, the part numbers printed on a sheet
// with the parts the drawing's connector headers name with a designator.
func partMatchStats(d PDFData, p Page) string {
	tags, _ := headerPartTags(d)
	known := map[string]bool{}
	for part := range tags {
		known[normPart(part)] = true
	}
	slashed, slashedKnown, heads, tails, joinedKnown, tailSuffixKnown := 0, 0, 0, 0, 0, 0
	for _, w := range p.Words {
		t := strings.ToUpper(strings.TrimSpace(w.Text))
		switch {
		case reSplitPNHead.MatchString(t):
			heads++
		case reSplitPNTail.MatchString(t):
			tails++
			for k := range known {
				if strings.HasSuffix(k, normPart(t)) {
					tailSuffixKnown++
					break
				}
			}
		case strings.Contains(t, "/") && strings.ContainsAny(t, "0123456789"):
			slashed++
			if known[normPart(t)] {
				slashedKnown++
			}
		}
	}
	for part := range headerPartNumbers(p) {
		_ = part
	}
	for _, v := range headerPartNumbers(p) {
		if known[normPart(v)] {
			joinedKnown++
		}
	}
	return fmt.Sprintf("parts with a designator on the drawing %d; on this sheet: one-piece part numbers %d (%d equal a known part), split heads %d, split tails %d (%d end a known part), joined head+tail under a designator equal to a known part %d", len(known), slashed, slashedKnown, heads, tails, tailSuffixKnown, joinedKnown)
}

// farEndItems describes, by shape and position only, what is printed around
// the far end of the first n traced wire lines: the 8 nearest items within
// 30 em, as shape@dx,dy in em (dx right of the end, dy above it).
func farEndItems(p Page, n int) []string {
	var out []string
	g := newSegGraph(p.Segs)
	var items []Word
	for _, l := range pageLines(p) {
		items = append(items, schematicWords(l)...)
	}
	for _, l := range pageLines(p) {
		if len(out) >= n {
			break
		}
		ws := schematicWords(l)
		for j, w := range ws {
			if !(j+1 < len(ws) && schematicGauge.MatchString(w.Text) && strings.EqualFold(ws[j+1].Text, "AWG")) || len(out) >= n {
				continue
			}
			em := lineEm(l)
			bi, bd := -1, math.Inf(1)
			for i, sg := range g.segs {
				x0, x1 := math.Min(sg[0], sg[2]), math.Max(sg[0], sg[2])
				dy := l.Y - (sg[1]+sg[3])/2
				if math.Abs(sg[1]-sg[3]) > 0.8 || w.X < x0-em || w.X > x1+em || dy < -0.6*em || dy > 1.6*em {
					continue
				}
				if d := math.Abs(dy - 0.3*em); d < bd {
					bi, bd = i, d
				}
			}
			if bi < 0 {
				continue
			}
			sg := g.segs[bi]
			ex, ey, _, kind := g.followInfo(bi, sg[2], sg[3])
			if sg[2] < sg[0] {
				ex, ey, _, kind = g.followInfo(bi, sg[0], sg[1])
			}
			type it struct {
				d float64
				s string
			}
			var near []it
			for _, v := range items {
				dx, dy := ((v.X+v.right())/2-ex)/em, (v.Y-ey)/em
				if d := math.Hypot(dx, dy); d <= 30 {
					rot := ""
					if v.Rotated {
						rot = "(rotated)"
					}
					sc := shapeClass(v.Text)
					if sc == "hyphenated code" {
						ps := []string{}
						for _, part := range strings.FieldsFunc(v.Text, func(r rune) bool { return r == '-' || r == '_' }) {
							ps = append(ps, shapeClass(part))
						}
						sc += "{" + strings.Join(ps, "+") + "}<" + abstractPattern(v.Text) + ">"
					}
					near = append(near, it{d, fmt.Sprintf("%s%s@%.0f,%.0f", sc, rot, dx, dy)})
				}
			}
			for _, v := range p.Words { // rotated text is not in the lines
				if !v.Rotated {
					continue
				}
				dx, dy := (v.X-ex)/em, (v.Y-ey)/em
				if d := math.Hypot(dx, dy); d <= 30 {
					near = append(near, it{d, fmt.Sprintf("%s(rotated)@%.0f,%.0f", shapeClass(v.Text), dx, dy)})
				}
			}
			sort.Slice(near, func(a, b int) bool { return near[a].d < near[b].d })
			parts := []string{}
			for k := 0; k < len(near) && k < 8; k++ {
				parts = append(parts, near[k].s)
			}
			out = append(out, fmt.Sprintf("end@%.2f,%.2f (%s): %s", ex/p.Width, ey/p.Height, kind, strings.Join(parts, " ")))
		}
	}
	return out
}

// abstractPattern reduces text to its form: each run of letters becomes A,
// each run of digits 9, other characters are kept. No letters or digits of
// the original survive.
func abstractPattern(t string) string {
	var b strings.Builder
	last := byte(0)
	for _, r := range strings.TrimSpace(t) {
		var c byte
		switch {
		case r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z':
			c = 'A'
		case r >= '0' && r <= '9':
			c = '9'
		case r == ' ':
			c = '_'
		case r < 128:
			c = byte(r)
		default:
			c = '?'
		}
		if (c == 'A' || c == '9') && c == last {
			continue
		}
		b.WriteByte(c)
		last = c
	}
	return b.String()
}

// endPatterns: for every traced wire line, the abstract pattern of what is
// printed at the wire's own height near each end (within 25 em outward and 3
// em inward), as pattern@dx (em, outward positive). No text survives.
func endPatterns(p Page) []string {
	var out []string
	g := newSegGraph(p.Segs)
	var items []Word
	for _, l := range pageLines(p) {
		items = append(items, schematicWords(l)...)
	}
	for _, l := range pageLines(p) {
		ws := schematicWords(l)
		for j, w := range ws {
			if !(j+1 < len(ws) && schematicGauge.MatchString(w.Text) && strings.EqualFold(ws[j+1].Text, "AWG")) {
				continue
			}
			em := lineEm(l)
			bi, bd := -1, math.Inf(1)
			for i, sg := range g.segs {
				x0, x1 := math.Min(sg[0], sg[2]), math.Max(sg[0], sg[2])
				dy := l.Y - (sg[1]+sg[3])/2
				if math.Abs(sg[1]-sg[3]) > 0.8 || w.X < x0-em || w.X > x1+em || dy < -0.6*em || dy > 1.6*em {
					continue
				}
				if d := math.Abs(dy - 0.3*em); d < bd {
					bi, bd = i, d
				}
			}
			if bi < 0 {
				out = append(out, "no line")
				continue
			}
			sg := g.segs[bi]
			ax, ay, _, _ := g.followInfo(bi, sg[0], sg[1])
			bx, by, _, _ := g.followInfo(bi, sg[2], sg[3])
			if ax > bx {
				ax, ay, bx, by = bx, by, ax, ay
			}
			side := func(x, y, dir float64) string {
				type it struct {
					d float64
					s string
				}
				var near []it
				for _, v := range items {
					if math.Abs(v.Y-y) > 1.0*em {
						continue
					}
					d := ((v.X+v.right())/2 - x) * dir / em
					if d < -3 || d > 25 {
						continue
					}
					near = append(near, it{d, fmt.Sprintf("%s@%.0f", abstractPattern(v.Text), d)})
				}
				sort.Slice(near, func(a, b int) bool { return near[a].d < near[b].d })
				ps := []string{}
				for k := 0; k < len(near) && k < 5; k++ {
					ps = append(ps, near[k].s)
				}
				return strings.Join(ps, " ")
			}
			out = append(out, fmt.Sprintf("left end@%.2f [%s] right end@%.2f [%s]", ax/p.Width, side(ax, ay, -1), bx/p.Width, side(bx, by, 1)))
		}
	}
	return out
}
