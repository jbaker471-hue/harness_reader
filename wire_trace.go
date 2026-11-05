package main

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// Wire tracing: a wiring diagram is read along the LINES drawn for the wires,
// not by which text happens to share a baseline. Every conductor carries a
// gauge label ("24 AWG") printed on or just above its line. From the label the
// drawn line is followed through its bends to both ends. The pin at each end
// is the pin label printed beside that end (a number, or a contact letter such
// as A or AA); border zone letters at the page edge are never pins. The
// connector at each end is the block the end lands on, named once from the
// header above the block.

var (
	// a drawing note reference printed where a wire ends, e.g. "(SH-3)"
	// contact letters: one letter (not I or O) or a doubled letter (AA, BB)
	rePinLetterSlot = regexp.MustCompile(`^(?:[A-HJ-NP-Za-hj-np-z]|([A-HJ-NP-Z])[A-HJ-NP-Z])$`)
	// block names in other styles than reRefDesignator (e.g. ACU1, W2P3, A1J4)
	reBlockName = regexp.MustCompile(`^(?:[A-Z][A-Z0-9]*-)?[A-Z]{1,4}[0-9]{1,4}[A-Z]?(?:[A-Z]{1,3}[0-9]{1,4}[A-Z]?)?$`)
)

func pinAtEnd(t string) (string, bool) {
	t = strings.TrimSpace(t)
	if reDigitPin.MatchString(t) {
		return t, true
	}
	if m := rePinLetterSlot.FindStringSubmatch(t); m != nil {
		if len(t) == 2 && t[0] != t[1] {
			return "", false
		}
		return t, true
	}
	return "", false
}

type traceEnd struct {
	note  bool   // the wire ends at a terminal box with a label (a lead), not a connector
	lead  string // that label
	tag   string // set when the end is a printed connector-pin reference
	x, y  float64
	pin   Word
	pinID string
	color string
	label int // index of the gauge label this end belongs to
}

// segGraph joins segments that share an end point.
type segGraph struct {
	segs []Segment
	at   map[[2]int][]int
}

func snapKey(x, y float64) [2]int {
	return [2]int{int(math.Round(x / 0.75)), int(math.Round(y / 0.75))}
}

func newSegGraph(segs []Segment) *segGraph {
	// CAD exports often draw the same line more than once (stroke over fill,
	// repeated paths); a duplicate must not look like a branch.
	seen := map[[4]int]bool{}
	var uniq []Segment
	for _, s := range segs {
		a, b := snapKey(s[0], s[1]), snapKey(s[2], s[3])
		if a == b {
			continue
		}
		if b[0] < a[0] || (b[0] == a[0] && b[1] < a[1]) {
			a, b = b, a
		}
		k := [4]int{a[0], a[1], b[0], b[1]}
		if !seen[k] {
			seen[k] = true
			uniq = append(uniq, s)
		}
	}
	segs = uniq
	g := &segGraph{segs: segs, at: map[[2]int][]int{}}
	for i, s := range segs {
		g.at[snapKey(s[0], s[1])] = append(g.at[snapKey(s[0], s[1])], i)
		g.at[snapKey(s[2], s[3])] = append(g.at[snapKey(s[2], s[3])], i)
	}
	return g
}

// follow walks from segment i out through its end (x,y) while the line
// continues with exactly one other segment, and returns the far point.
func (g *segGraph) follow(i int, x, y float64) (float64, float64) {
	x, y, _, _ = g.followInfo(i, x, y)
	return x, y
}

// followGaps follows the line, and where it ends free, jumps a short gap (a
// break left for text, up to maxGap) to a segment that carries on in the same
// direction at the same height, then keeps following. Junctions stop it.
func (g *segGraph) followGaps(i int, x, y, maxGap float64, stop func(x, y float64) bool) (float64, float64, string) {
	cur := i
	for hop := 0; hop < 6; hop++ {
		nx, ny, _, kind := g.followStop(cur, x, y, stop)
		if kind != "free" {
			return nx, ny, kind
		}
		// direction of travel from the last segment reaching (nx,ny)
		dir := 0.0
		for _, j := range g.at[snapKey(nx, ny)] {
			s := g.segs[j]
			if math.Abs(s[1]-s[3]) <= 0.8 {
				if snapKey(s[2], s[3]) == snapKey(nx, ny) {
					dir = math.Copysign(1, s[2]-s[0])
				} else {
					dir = math.Copysign(1, s[0]-s[2])
				}
			}
		}
		if dir == 0 {
			return nx, ny, kind
		}
		best, bd, bx := -1, math.Inf(1), 0.0
		for j, s := range g.segs {
			if math.Abs(s[1]-s[3]) > 0.8 || math.Abs((s[1]+s[3])/2-ny) > 0.8 {
				continue
			}
			for _, ex := range []float64{s[0], s[2]} {
				d := (ex - nx) * dir
				if d > 0.5 && d <= maxGap && d < bd {
					best, bd, bx = j, d, ex
				}
			}
		}
		if best < 0 {
			return nx, ny, kind
		}
		s := g.segs[best]
		cur, x, y = best, s[0], s[1]
		if bx == s[0] {
			x, y = s[2], s[3]
		}
		if (x-nx)*dir <= 0 { // walked the wrong way along it
			return nx, ny, kind
		}
	}
	return x, y, "limit"
}

// followInfo also says how many segments were walked and how the line ended
// ("free", "junction" or "limit"), for structure-only diagnostics.
func (g *segGraph) followInfo(i int, x, y float64) (float64, float64, int, string) {
	return g.followStop(i, x, y, nil)
}

// followStop follows like followInfo and also stops ("pin") at the first
// point for which stop returns true: a wire ends where its pin label is.
func (g *segGraph) followStop(i int, x, y float64, stop func(x, y float64) bool) (float64, float64, int, string) {
	seen := map[int]bool{i: true}
	// direction of arrival at (x,y) along segment i
	s0 := g.segs[i]
	px, py := s0[0], s0[1]
	if snapKey(s0[0], s0[1]) == snapKey(x, y) {
		px, py = s0[2], s0[3]
	}
	dx, dy := x-px, y-py
	for step := 0; step < 400; step++ {
		if stop != nil && stop(x, y) {
			return x, y, step, "pin"
		}
		var next []int
		for _, j := range g.at[snapKey(x, y)] {
			if !seen[j] {
				next = append(next, j)
			}
		}
		if len(next) == 0 {
			return x, y, step, "free"
		}
		j := -1
		if len(next) == 1 {
			j = next[0]
		} else {
			j = g.zigzagChoice(next, x, y, dx, dy)
			if j < 0 {
				return x, y, step, "junction" // a real branch or splice: do not guess
			}
		}
		seen[j] = true
		s := g.segs[j]
		nx, ny := s[0], s[1]
		if snapKey(s[0], s[1]) == snapKey(x, y) {
			nx, ny = s[2], s[3]
		}
		dx, dy, x, y = nx-x, ny-y, nx, ny
	}
	return x, y, 400, "limit"
}

// zigzagChoice picks how a wire line continues where other lines touch it.
// Wires drawn as stacked zig-zags touch their neighbours at every bend; the
// wire's own line keeps heading the same way across the sheet and, on a
// diagonal, turns back the other way (up then down). A horizontal line goes
// on horizontally. Anything else, or two equally good choices, is a real
// junction and returns -1.
func (g *segGraph) zigzagChoice(cands []int, x, y, dx, dy float64) int {
	diag := math.Abs(dy) > 0.2*math.Abs(dx) && math.Abs(dx) > 0.2
	horiz := math.Abs(dy) <= 0.2*math.Abs(dx)
	best, bestScore, tie := -1, 0, false
	for _, j := range cands {
		s := g.segs[j]
		ex, ey := s[2], s[3]
		if snapKey(s[2], s[3]) == snapKey(x, y) {
			ex, ey = s[0], s[1]
		}
		cx, cy := ex-x, ey-y
		if cx*dx <= 0 { // must keep crossing the sheet the same way
			continue
		}
		score := 0
		switch {
		case diag && math.Abs(cy) > 0.2*math.Abs(cx) && cy*dy < 0 && math.Abs(math.Abs(cy/cx)-math.Abs(dy/dx)) < 0.35*math.Abs(dy/dx):
			score = 3 // the zig-zag turning back, same steepness
		case horiz && math.Abs(cy) <= 0.2*math.Abs(cx):
			score = 3 // straight on
		case diag && math.Abs(cy) <= 0.2*math.Abs(cx):
			score = 2 // zig-zag ends, horizontal again
		case horiz && math.Abs(cy) > 0.2*math.Abs(cx):
			score = 1 // a zig-zag starts
		}
		switch {
		case score > bestScore:
			best, bestScore, tie = j, score, false
		case score == bestScore && score > 0:
			tie = true
		}
	}
	if best < 0 || tie {
		return -1
	}
	return best
}

// traceDebug describes, by structure only, how tracing went for the first n
// gauge labels of a page: segments walked, where each end stopped (0-1 page
// position), how it stopped, and how far (in em) the nearest pin-shaped and
// nearest any printed item are from each end.
func traceDebug(p Page, n int) []string {
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
				if math.Abs(sg[1]-sg[3]) > 0.8 {
					continue
				}
				x0, x1 := math.Min(sg[0], sg[2]), math.Max(sg[0], sg[2])
				dy := l.Y - (sg[1]+sg[3])/2
				if w.X < x0-em || w.X > x1+em || dy < -0.6*em || dy > 1.6*em {
					continue
				}
				if d := math.Abs(dy - 0.3*em); d < bd {
					bi, bd = i, d
				}
			}
			if bi < 0 {
				out = append(out, "label: no line")
				continue
			}
			sg := g.segs[bi]
			desc := fmt.Sprintf("label@%.2f,%.2f line %.2f-%.2f (%.1f em below)", w.X/p.Width, l.Y/p.Height, math.Min(sg[0], sg[2])/p.Width, math.Max(sg[0], sg[2])/p.Width, (l.Y-(sg[1]+sg[3])/2)/em)
			for _, e := range [][2]float64{{sg[0], sg[1]}, {sg[2], sg[3]}} {
				ex, ey, steps, kind := g.followInfo(bi, e[0], e[1])
				np, na := math.Inf(1), math.Inf(1)
				npShape := ""
				for _, it := range items {
					d := math.Hypot((it.X+it.right())/2-ex, it.Y-ey) / em
					if d < na {
						na = d
					}
					if _, ok := pinAtEnd(it.Text); ok && d < np {
						np, npShape = d, shapeOfPinItem(it.Text)
						_ = npShape
					}
				}
				// how many segments touch the end point anywhere (not only at their ends)
				touch := 0
				for _, t := range p.Segs {
					if distToSeg(ex, ey, t) < 0.8 {
						touch++
					}
				}
				desc += fmt.Sprintf(" | end@%.2f,%.2f after %d segs, %s, touching %d, nearest pin-shaped %.1f em (%s), nearest item %.1f em", ex/p.Width, ey/p.Height, steps, kind, touch, np, npShape, na)
				if kind == "junction" {
					// the lines meeting there: direction (degrees, 0 = right, 90 = up) and length in em
					cands := []string{}
					for _, j := range g.at[snapKey(ex, ey)] {
						t := g.segs[j]
						ox, oy := t[2], t[3]
						if snapKey(t[2], t[3]) == snapKey(ex, ey) {
							ox, oy = t[0], t[1]
						}
						cands = append(cands, fmt.Sprintf("%.0f°/%.1fem", math.Atan2(oy-ey, ox-ex)*180/math.Pi, math.Hypot(ox-ex, oy-ey)/em))
					}
					desc += " [lines here: " + strings.Join(cands, " ") + "]"
				}
			}
			out = append(out, desc)
		}
	}
	return out
}

func shapeOfPinItem(t string) string {
	if reDigitPin.MatchString(t) {
		return "number"
	}
	if len(t) == 2 {
		return "doubled letter"
	}
	return "letter"
}

func distToSeg(x, y float64, s Segment) float64 {
	dx, dy := s[2]-s[0], s[3]-s[1]
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Hypot(x-s[0], y-s[1])
	}
	t := math.Max(0, math.Min(1, ((x-s[0])*dx+(y-s[1])*dy)/l2))
	return math.Hypot(x-(s[0]+t*dx), y-(s[1]+t*dy))
}

// tracedWireRows reads the wires of a wiring sheet by following their lines.
// Wires already read (rows) are kept and not repeated. why explains, by
// structure only, why some gauge labels gave no wire.
func tracedWireRows(p Page, rows []WireRow, partTags map[string]string) (out []WireRow, why string) {
	out, why, _ = traceSheet(p, rows, partTags)
	return out, why
}

// tracedBlock is one named block of wire ends: its connector and the pin
// labels the drawn lines landed on.
type tracedBlock struct {
	tag     string
	pins    []Word
	ceiling float64 // the lowest pin of the block above in the same column
}

func traceSheet(p Page, rows []WireRow, partTags map[string]string) (out []WireRow, why string, blocks []tracedBlock) {
	if !p.PageAccurate || p.Width <= 0 || len(p.Segs) == 0 {
		return nil, "", nil
	}
	type label struct {
		x, y, em float64
		gauge    string
	}
	var labels []label
	for _, l := range pageLines(p) {
		ws := schematicWords(l)
		for j, w := range ws {
			if j+1 < len(ws) && schematicGauge.MatchString(w.Text) && strings.EqualFold(ws[j+1].Text, "AWG") {
				labels = append(labels, label{w.X, l.Y, lineEm(l), w.Text})
			} else if m := reGaugeLabel.FindStringSubmatch(w.Text); m != nil {
				labels = append(labels, label{w.X, l.Y, lineEm(l), m[1]})
			}
		}
	}
	if len(labels) < 3 {
		return nil, "", nil
	}
	g := newSegGraph(p.Segs)
	// words, split into single items, for pin and colour lookup
	var items []Word
	for _, l := range pageLines(p) {
		items = append(items, schematicWords(l)...)
	}
	edge := 0.03 * p.Width
	findPin := func(x, y, em float64) (Word, string, bool) {
		best, bestD, id := Word{}, math.Inf(1), ""
		for _, w := range items {
			if w.X < edge || w.X > p.Width-edge {
				continue // border zone letters and numbers
			}
			pid, ok := pinAtEnd(w.Text)
			if !ok {
				continue
			}
			cx := (w.X + w.right()) / 2
			dx, dy := math.Abs(cx-x), math.Abs(w.Y-y)
			if dx > 5*em || dy > 1.4*em {
				continue
			}
			if d := dx + 2*dy; d < bestD {
				best, bestD, id = w, d, pid
			}
		}
		return best, id, id != ""
	}
	// The pin printed at the wire's height but further out than findPin looks
	// (beyond the colour word or inside a wide block): the nearest pin-shaped
	// item at the same height, outward from the wire, within 25 em, with no
	// gauge label in between.
	findPinOutward := func(x, y, dir, em float64) (Word, string, bool) {
		best, bestD, id := Word{}, math.Inf(1), ""
		for _, w := range items {
			if w.X < edge || w.X > p.Width-edge || math.Abs(w.Y-y) > 1.0*em {
				continue
			}
			pid, ok := pinAtEnd(w.Text)
			if !ok {
				continue
			}
			d := ((w.X+w.right())/2 - x) * dir
			if d < 0 || d > 25*em || d >= bestD {
				continue
			}
			best, bestD, id = w, d, pid
		}
		if id == "" {
			return Word{}, "", false
		}
		for _, w := range items { // a gauge number in between means another wire
			if math.Abs(w.Y-y) <= 1.0*em && schematicGauge.MatchString(w.Text) {
				if d := ((w.X+w.right())/2 - x) * dir; d > 0 && d < bestD {
					return Word{}, "", false
				}
			}
		}
		return best, id, true
	}
	// A wire that ends at a printed connector-pin reference ("P10-12",
	// "GSE-P10-12") at its own height, outward within 12 em: the reference is
	// the far end. The front must be a designator and the back a pin.
	findRef := func(x, y, dir, em float64) (string, string, bool) {
		best, bestD, pin := "", math.Inf(1), ""
		for _, w := range items {
			if math.Abs(w.Y-y) > 1.0*em {
				continue
			}
			d := (w.X - x) * dir
			if dir < 0 {
				d = (x - w.right())
			}
			if d < -0.5*em || d > 12*em || d >= bestD {
				continue
			}
			t := strings.ToUpper(strings.TrimSpace(w.Text))
			k := strings.LastIndexAny(t, "-_")
			if k <= 0 || k == len(t)-1 {
				continue
			}
			tag, pn := t[:k], t[k+1:]
			if reRefDesignator.FindString(tag) != tag {
				continue
			}
			if id, ok := pinAtEnd(pn); ok {
				best, bestD, pin = tag, d, id
			}
		}
		return best, pin, best != ""
	}
	// A wire that ends at a crimp ferrule (drawn as a small square at the end
	// of the line) with a label printed beside it at the same height is
	// terminated in a ferrule, not in a connector pin. Both the box and the label must be
	// there. Returns the label.
	findLead := func(x, y, dir, em float64) (string, bool) {
		h, v := 0, 0
		for _, s := range g.segs {
			x0, x1 := math.Min(s[0], s[2]), math.Max(s[0], s[2])
			y0, y1 := math.Min(s[1], s[3]), math.Max(s[1], s[3])
			// inside a box region just past the end of the line
			lo, hi := x-0.5*em, x+4*em
			if dir < 0 {
				lo, hi = x-4*em, x+0.5*em
			}
			if x0 < lo || x1 > hi || y0 < y-2*em || y1 > y+2*em {
				continue
			}
			lw, lh := x1-x0, y1-y0
			switch {
			case lh <= 0.3 && lw >= 0.6*em && lw <= 3.5*em:
				h++
			case lw <= 0.3 && lh >= 0.6*em && lh <= 3.5*em:
				v++
			}
		}
		if h < 2 || v < 2 {
			return "", false
		}
		best, bestD := "", math.Inf(1)
		for _, w := range items {
			if math.Abs(w.Y-y) > 1.0*em {
				continue
			}
			d := (w.X - x) * dir
			if dir < 0 {
				d = x - w.right()
			}
			if d >= 1*em && d <= 14*em && d < bestD {
				best, bestD = strings.TrimSpace(w.Text), d
			}
		}
		return best, best != ""
	}
	findColour := func(x, y, em float64) string {
		best, bestD := "", math.Inf(1)
		for _, w := range items {
			c := schematicColor(strings.ToUpper(w.Text))
			if c == "" {
				continue
			}
			dx, dy := math.Abs((w.X+w.right())/2-x), math.Abs(w.Y-y)
			if dx > 8*em || dy > 1.4*em {
				continue
			}
			if d := dx + 2*dy; d < bestD {
				best, bestD = c, d
			}
		}
		return best
	}
	noLine, noPin, noConn, same := 0, 0, 0, 0
	var bareOf []int // per pair: which end has no number (0 none)
	var ends []traceEnd
	pairs := [][2]int{}
	for li, lb := range labels {
		// the wire line runs under (or through) its label
		bi, bd := -1, math.Inf(1)
		for i, s := range g.segs {
			if math.Abs(s[1]-s[3]) > 0.8 {
				continue
			}
			x0, x1 := math.Min(s[0], s[2]), math.Max(s[0], s[2])
			if lb.x < x0-lb.em || lb.x > x1+lb.em {
				continue
			}
			dy := lb.y - (s[1]+s[3])/2 // positive: line below the label baseline
			if dy < -0.6*lb.em || dy > 1.6*lb.em {
				continue
			}
			if d := math.Abs(dy - 0.3*lb.em); d < bd {
				bi, bd = i, d
			}
		}
		if bi < 0 {
			noLine++
			continue
		}
		s := g.segs[bi]
		// a wire ends at the first point along it with a pin label right beside it
		atPin := func(x, y float64) bool {
			for _, w := range items {
				if w.X < edge || w.X > p.Width-edge {
					continue
				}
				if _, ok := pinAtEnd(w.Text); ok && math.Abs((w.X+w.right())/2-x) <= 2*lb.em && math.Abs(w.Y-y) <= 1.0*lb.em {
					return true
				}
			}
			return false
		}
		ax, ay, _ := g.followGaps(bi, s[0], s[1], 4*lb.em, atPin)
		bx, by, _ := g.followGaps(bi, s[2], s[3], 4*lb.em, atPin)
		if ax > bx {
			ax, ay, bx, by = bx, by, ax, ay
		}
		pa, ida, okA := findPin(ax, ay, lb.em)
		pb, idb, okB := findPin(bx, by, lb.em)
		refA, refB := "", ""
		if !okA {
			if t, pin, ok := findRef(ax, ay, -1, lb.em); ok {
				refA, ida, okA, pa = t, pin, true, Word{X: ax, Y: ay}
			} else {
				pa, ida, okA = findPinOutward(ax, ay, -1, lb.em)
			}
		}
		if !okB {
			if t, pin, ok := findRef(bx, by, 1, lb.em); ok {
				refB, idb, okB, pb = t, pin, true, Word{X: bx, Y: by}
			} else {
				pb, idb, okB = findPinOutward(bx, by, 1, lb.em)
			}
		}
		noteA, noteB := false, false
		leadA, leadB := "", ""
		if !okA && okB {
			if lbl, ok := findLead(ax, ay, -1, lb.em); ok {
				noteA, okA, pa, leadA = true, true, Word{X: ax, Y: ay}, lbl
			}
		}
		if !okB && okA {
			if lbl, ok := findLead(bx, by, 1, lb.em); ok {
				noteB, okB, pb, leadB = true, true, Word{X: bx, Y: by}, lbl
			}
		}
		// A line with a number at one end only: that pin is used on its
		// connector; the other end is not a connector pin.
		bare := 0 // which end has no number: 1 = a, 2 = b
		if okA && !okB {
			noteB, okB, pb, bare = true, true, Word{X: bx, Y: by}, 2
		} else if okB && !okA {
			noteA, okA, pa, bare = true, true, Word{X: ax, Y: ay}, 1
		}
		if !okA || !okB || (pa.X == pb.X && pa.Y == pb.Y) {
			noPin++
			continue
		}
		ends = append(ends, traceEnd{noteA, leadA, refA, ax, ay, pa, ida, findColour(ax, ay, lb.em), li}, traceEnd{noteB, leadB, refB, bx, by, pb, idb, findColour(bx, by, lb.em), li})
		pairs = append(pairs, [2]int{len(ends) - 2, len(ends) - 1})
		bareOf = append(bareOf, bare)
	}
	// An end with no number that lands where other wires end WITH numbers (the
	// same block edge, within 40 em up or down) is in a connector's pin column:
	// its number was not read. That wire is held, never counted as one-ended.
	{
		keepEnds, keepPairs, dropped := []traceEnd{}, [][2]int{}, 0
		for k, pr := range pairs {
			bad := false
			if bareOf[k] != 0 {
				e := ends[pr[0]]
				if bareOf[k] == 2 {
					e = ends[pr[1]]
				}
				for _, o := range ends {
					if o.pinID != "" && math.Abs(o.x-e.x) <= 3 && math.Abs(o.y-e.y) <= 40*labels[e.label].em && !(o.x == e.x && o.y == e.y) {
						bad = true
						break
					}
				}
				// an unnumbered end that lands under a connector's header is that
				// connector's pin whose number was not read, even when no other
				// numbered wire ends there (a connector with one or two wires)
				if !bad {
					at := map[int]Word{0: {X: e.x, Y: e.y, Size: labels[e.label].em}}
					if t, _ := columnConnector(p, e.x, at, partTags, math.Inf(1)); t != "" {
						bad = true
					} else if t, _ := blockNameAbove(p, e.x, at, math.Inf(1)); t != "" {
						bad = true
					}
				}
			}
			if bad {
				dropped++
				continue
			}
			keepEnds = append(keepEnds, ends[pr[0]], ends[pr[1]])
			keepPairs = append(keepPairs, [2]int{len(keepEnds) - 2, len(keepEnds) - 1})
		}
		ends, pairs = keepEnds, keepPairs
		noPin += dropped
	}
	// Connector of each end: ends landing on the same block edge share an x;
	// a column of ends is split into blocks at headers printed between them.
	tagOf := make([]string, len(ends))
	viaOf := make([]string, len(ends))
	order := make([]int, len(ends))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return ends[order[a]].x < ends[order[b]].x })
	var cols [][]int
	for _, i := range order {
		if ends[i].tag != "" {
			tagOf[i], viaOf[i] = ends[i].tag, "designator"
			continue
		}
		if ends[i].note {
			tagOf[i], viaOf[i] = "", "note"
			continue
		}
		if n := len(cols); n > 0 && math.Abs(ends[i].x-ends[cols[n-1][0]].x) <= 3 {
			cols[n-1] = append(cols[n-1], i)
			continue
		}
		cols = append(cols, []int{i})
	}
	isHeader := func(w Word) bool {
		t := strings.ToUpper(strings.TrimSpace(w.Text))
		return !w.Rotated && t != "" && (reRefDesignator.FindString(t) == t || reSplitPNHead.MatchString(t))
	}
	for _, col := range cols {
		sort.Slice(col, func(a, b int) bool { return ends[col[a]].y > ends[col[b]].y })
		x := ends[col[0]].x
		start, ceiling := 0, math.Inf(1)
		flush := func(end int) {
			part := col[start:end]
			pins := map[int]Word{}
			for k, i := range part {
				pins[k] = ends[i].pin
			}
			tag, via := columnConnector(p, x, pins, partTags, ceiling)
			if tag == "" {
				tag, via = blockNameAbove(p, x, pins, ceiling)
			}
			for _, i := range part {
				tagOf[i], viaOf[i] = tag, via
			}
			if tag != "" {
				b := tracedBlock{tag: tag, ceiling: ceiling}
				for _, i := range part {
					b.pins = append(b.pins, ends[i].pin)
				}
				blocks = append(blocks, b)
			}
			ceiling = ends[part[len(part)-1]].y
			start = end
		}
		for k := 1; k < len(col); k++ {
			hi, lo := ends[col[k-1]].y, ends[col[k]].y
			between := false
			for _, w := range p.Words {
				if w.Y < hi && w.Y > lo && math.Abs(w.X-x) <= p.Width*.12 && isHeader(w) {
					between = true
					break
				}
			}
			if between {
				flush(k)
			}
		}
		flush(len(col))
	}
	pairRead := map[string]bool{}
	occupied := map[string]bool{} // tag:pin already carrying a wire read by another reader
	for _, r := range rows {
		occupied[strings.ToUpper(r.Connector)+":"+r.Cavity] = true
		occupied[strings.ToUpper(r.End)+":"+r.EndCavity] = true
		pairRead[r.Connector+":"+r.Cavity+">"+r.End+":"+r.EndCavity] = true
		pairRead[r.End+":"+r.EndCavity+">"+r.Connector+":"+r.Cavity] = true
	}
	for _, pr := range pairs {
		a, b := ends[pr[0]], ends[pr[1]]
		ta, tb := tagOf[pr[0]], tagOf[pr[1]]
		if a.note || b.note {
			// one end on a connector pin, the other at a drawing note (for
			// example a shield-float callout): the pin is wired; the far end
			// is recorded as not on a connector
			c, e := a, b
			tc := ta
			if a.note {
				c, e, tc = b, a, tb
			}
			if tc == "" {
				noConn++
				continue
			}
			// a line already read by another reader is not read again as one-ended
			if occupied[strings.ToUpper(tc)+":"+c.pinID] {
				continue
			}
			occupied[strings.ToUpper(tc)+":"+c.pinID] = true
			if e.lead != "" {
				out = append(out, WireRow{Page: p.Number, Connector: tc, Cavity: c.pinID, EndSignal: "crimp ferrule: " + e.lead, Color: c.color, AWG: labels[c.label].gauge, Y: labels[c.label].y, Method: "traced-wire-to-ferrule",
					Confidence: "DRAFT — TRACED ALONG THE DRAWN WIRE TO A CRIMP FERRULE (DRAWN AS A SMALL BOX WITH A LABEL), NOT A CONNECTOR PIN; CHECK SOURCE"})
			} else {
				out = append(out, WireRow{Page: p.Number, Connector: tc, Cavity: c.pinID, EndSignal: "no pin number at this end (not a connector pin)", Color: c.color, AWG: labels[c.label].gauge, Y: labels[c.label].y, Method: "traced-wire-one-end",
					Confidence: "DRAFT — TRACED ALONG THE DRAWN WIRE; ONLY ONE END HAS A PIN NUMBER; CHECK SOURCE"})
			}
			continue
		}
		switch {
		case ta == "" || tb == "":
			noConn++
			continue
		case ta == tb:
			same++
			continue
		}
		if pairRead[ta+":"+a.pinID+">"+tb+":"+b.pinID] {
			continue
		}
		conf := "DRAFT — TRACED ALONG THE DRAWN WIRE; CHECK SOURCE"
		switch {
		case viaOf[pr[0]] == "part" || viaOf[pr[1]] == "part":
			conf = "DRAFT — TRACED ALONG THE DRAWN WIRE; CONNECTOR MATCHED BY THE HEADER PART NUMBER (DESIGNATOR NOT READ); CHECK SOURCE"
		case viaOf[pr[0]] != "designator" || viaOf[pr[1]] != "designator":
			conf = "DRAFT — TRACED ALONG THE DRAWN WIRE; CONNECTOR NAMED FROM A NON-STANDARD BLOCK NAME; CHECK SOURCE"
		}
		out = append(out, WireRow{Page: p.Number, Connector: ta, Cavity: a.pinID, End: tb, EndCavity: b.pinID, Color: a.color, AWG: labels[a.label].gauge, Y: labels[a.label].y, Method: "traced-wire", Confidence: conf})
	}
	if missed := len(labels) - len(out) - len(rows); missed > 0 {
		parts := []string{}
		for _, c := range []struct {
			n int
			s string
		}{{noLine, "no drawn line found under the label"}, {noPin, "no pin label at one end of the drawn line"}, {noConn, "an end whose connector block could not be named"}, {same, "both ends on blocks named the same connector"}} {
			if c.n > 0 {
				parts = append(parts, fmt.Sprintf("%d label(s): %s", c.n, c.s))
			}
		}
		why = fmt.Sprintf("wire tracing: %d gauge labels, %d wires traced", len(labels), len(out))
		if len(parts) > 0 {
			why += "; " + strings.Join(parts, "; ")
		}
	}
	return out, why, blocks
}

// blockNameAbove names a block from the text printed above it when no
// standard designator or header part is found: the nearest short name such
// as ACU1 or A1J4 directly above the block, never text above the block over it.
func blockNameAbove(p Page, x float64, pins map[int]Word, ceiling float64) (string, string) {
	if t, v := blockNameWithin(p, x, pins, ceiling, 12); t != "" {
		return t, v
	}
	return blockNameWithin(p, x, pins, ceiling, 20)
}

func blockNameWithin(p Page, x float64, pins map[int]Word, ceiling, limit float64) (string, string) {
	top, size := math.Inf(-1), 6.0
	for _, w := range pins {
		if w.Y > top {
			top, size = w.Y, math.Max(w.Size, 4)
		}
	}
	best, bestS, n := "", math.Inf(1), 0
	for _, w := range p.Words {
		t := strings.ToUpper(strings.TrimSpace(w.Text))
		if w.Rotated || w.Y >= ceiling || !reBlockName.MatchString(t) || schematicColor(t) != "" || reSplitPNTail.MatchString(t) || strings.HasSuffix(t, "AWG") {
			continue
		}
		dy, dx := w.Y-top, math.Abs(w.X-x)
		if dy <= 0.3*size || dy > limit*size || dx > p.Width*.12 {
			continue
		}
		if s := dy + 0.3*dx; s < bestS-size {
			best, bestS, n = t, s, 1
		} else if math.Abs(s-bestS) <= size && t != best {
			n++
		}
	}
	if n == 1 {
		return best, "name"
	}
	return "", ""
}

// nameDebug describes, by shape and distance only, the text above each column
// of traced wire ends: for the top pin of the column, the nearest designator-
// or name-shaped items within 40 em above, as shape@dx,dy (em; dx right of
// the column, dy above the top pin).
func nameDebug(p Page) []string {
	rows, _ := tracedWireRows(p, nil, nil)
	_ = rows
	g := newSegGraph(p.Segs)
	_ = g
	var out []string
	// use the pin columns of the column reader: numbers/letters beside gauge lines
	type col struct{ x, top, em float64 }
	var cols []col
	for _, l := range gaugeLines(p) {
		for _, w := range schematicWords(l) {
			if _, ok := pinAtEnd(w.Text); !ok || w.X < 0.03*p.Width || w.X > 0.97*p.Width {
				continue
			}
			found := false
			for i := range cols {
				if math.Abs(cols[i].x-w.X) < 6 {
					found = true
					if w.Y > cols[i].top {
						cols[i].top = w.Y
					}
				}
			}
			if !found {
				cols = append(cols, col{w.X, w.Y, math.Max(w.Size, 4)})
			}
		}
	}
	for _, c := range cols {
		type cand struct {
			d     float64
			shape string
		}
		var cs []cand
		for _, w := range p.Words {
			t := strings.ToUpper(strings.TrimSpace(w.Text))
			kind := ""
			switch {
			case t != "" && reRefDesignator.FindString(t) == t:
				kind = "DESIGNATOR"
			case reSplitPNHead.MatchString(t):
				kind = "PARTHEAD"
			case reSplitPNTail.MatchString(t):
				kind = "PARTTAIL"
			case reBlockName.MatchString(t) && schematicColor(t) == "":
				kind = "NAME"
			case strings.Contains(t, "/") && strings.ContainsAny(t, "0123456789"):
				kind = "SLASHED"
			default:
				continue
			}
			dx, dy := (w.X-c.x)/c.em, (w.Y-c.top)/c.em
			if dy <= 0 || dy > 40 || math.Abs(dx) > 40 {
				continue
			}
			rot := ""
			if w.Rotated {
				rot = "(rotated)"
			}
			cs = append(cs, cand{math.Hypot(dx, dy), fmt.Sprintf("%s%s@%.0f,%.0f", kind, rot, dx, dy)})
		}
		sort.Slice(cs, func(a, b int) bool { return cs[a].d < cs[b].d })
		parts := []string{}
		for i := 0; i < len(cs) && i < 6; i++ {
			parts = append(parts, cs[i].shape)
		}
		out = append(out, fmt.Sprintf("pin column at %.2f: above it %s", c.x/p.Width, strings.Join(parts, " ")))
	}
	return out
}

// pinNumberAudits checks each traced connector of a wiring sheet two ways: the
// pin numbers printed beside it, and the wires read on the sheet that land on
// it. Every printed number must have exactly one wire. A number with no wire
// (a missed wire) or with two wires (a conflict) holds the connector; when they
// agree exactly the audit confirms its used pins on that sheet.
func pinNumberAudits(p Page, blocks []tracedBlock, pageRows []WireRow) []TableAudit {
	var out []TableAudit
	byTag := map[string][]tracedBlock{}
	order := []string{}
	for _, b := range blocks {
		if _, ok := byTag[b.tag]; !ok {
			order = append(order, b.tag)
		}
		byTag[b.tag] = append(byTag[b.tag], b)
	}
	var items []Word
	for _, l := range pageLines(p) {
		items = append(items, schematicWords(l)...)
	}
	edge := 0.03 * p.Width
	isNC := func(t string) bool {
		u := strings.ToUpper(strings.TrimSpace(t))
		return u == "NC" || u == "N/C" || u == "N.C." || u == "(NC)"
	}
	var ncWords []Word
	for _, w := range items {
		if isNC(w.Text) {
			ncWords = append(ncWords, w)
		}
	}
	for _, tag := range order {
		printed := map[string]bool{}
		printedAt := map[string]Word{}
		pitchOf := map[string]float64{}
		for _, b := range byTag[tag] {
			if len(b.pins) == 0 {
				continue
			}
			cx, top, bot, em := 0.0, math.Inf(-1), math.Inf(1), 6.0
			real := 0
			for _, w := range b.pins {
				if w.Text == "" { // an end with no pin label (a ferrule or one-numbered-end line)
					continue
				}
				real++
				cx += (w.X + w.right()) / 2
				top, bot = math.Max(top, w.Y), math.Min(bot, w.Y)
				em = math.Max(w.Size, 4)
			}
			if real == 0 {
				continue
			}
			cx /= float64(real)
			pitch := 1.6 * em
			if real > 1 {
				pitch = math.Max((top-bot)/float64(real-1), em)
			}
			for _, w := range items {
				if w.X < edge || w.X > p.Width-edge || math.Abs((w.X+w.right())/2-cx) > 1.2*em {
					continue
				}
				if w.Y > top+0.5*em || w.Y < bot-2.5*pitch || w.Y >= b.ceiling {
					continue
				}
				if id, ok := pinAtEnd(w.Text); ok {
					printed[id] = true
					printedAt[id], pitchOf[id] = w, pitch
				}
			}
		}
		if len(printed) == 0 {
			continue
		}
		landed := map[string]int{}
		for _, r := range pageRows {
			if strings.EqualFold(r.Connector, tag) && r.Cavity != "" {
				landed[r.Cavity]++
			}
			if strings.EqualFold(r.End, tag) && r.EndCavity != "" {
				landed[r.EndCavity]++
			}
		}
		// A printed pin marked NC (no connect) is stated unused by the drawing:
		// NC printed beside the number, or just below a group of numbers.
		nc := map[string]bool{}
		for _, n := range ncWords {
			ncx, ncem := (n.X+n.right())/2, math.Max(n.Size, 4)
			// NC printed right beside a number
			for id, w := range printedAt {
				if math.Abs((w.X+w.right())/2-ncx) > 10*ncem {
					continue
				}
				dy := w.Y - n.Y
				if math.Abs(dy) <= 1.0*ncem && math.Abs((w.X+w.right())/2-ncx) > 0.5*ncem {
					nc[id] = true
				}
			}
		}
		// A stub line into the block labelled with a pin list ("5,6,16-19")
		// and NC across the block on the same line: every listed pin is a
		// stated no-connect of this connector.
		ncListed := map[string]bool{}
		for _, b := range byTag[tag] {
			cx, top, bot := blockSpan(b)
			if math.IsNaN(cx) {
				continue
			}
			for _, w := range items {
				ids := expandPinList(w.Text)
				if ids == nil || w.Y > top+12*math.Max(w.Size, 4) || w.Y < bot-20*math.Max(w.Size, 4) || w.Y >= b.ceiling || math.Abs(w.right()-cx) > 8*math.Max(w.Size, 4) {
					continue
				}
				// each NC list belongs to one connector only: the block it sits in,
				// or the one nearest to it
				if owner := nearestBlock(blocks, w); owner != b.tag {
					continue
				}
				// never across another connector's header (the list then belongs
				// to that connector, even if its block was not named)
				if headerBetweenY(p, cx, w.Y, top, bot) {
					continue
				}
				for _, n := range ncWords {
					if math.Abs(n.Y-w.Y) <= 1.5*math.Max(w.Size, 4) && n.X > w.right() && n.X-w.right() <= 20*math.Max(w.Size, 4) {
						for _, id := range ids {
							ncListed[id] = true
						}
						break
					}
				}
			}
		}
		for id := range ncListed {
			nc[id] = true
			if landed[id] > 0 {
				printed[id] = true
			}
		}
		missed := []MissedRow{}
		for id := range printed {
			switch {
			case nc[id] && landed[id] > 0:
				missed = append(missed, MissedRow{Text: tag + ":" + id, Reason: "pin marked NC (no connect) but a wire was read to it"})
			case !nc[id] && landed[id] == 0:
				missed = append(missed, MissedRow{Text: tag + ":" + id, Reason: "pin number printed beside the connector with no wire read to it"})
			}
		}
		for id, n := range landed {
			if n > 1 {
				missed = append(missed, MissedRow{Text: tag + ":" + id, Reason: fmt.Sprintf("%d wires read to the same pin", n)})
			}
		}
		sort.Slice(missed, func(i, j int) bool { return missed[i].Text < missed[j].Text })
		a := TableAudit{Page: p.Number, Title: "pin numbers " + tag, Connectors: []string{tag}, RowsRead: len(printed) - countMissing(printed, landed) + countNCUnwired(nc, landed), Counted: true, RuledRows: len(printed), Missed: missed}
		if len(missed) > 0 {
			a.Status = "INCOMPLETE"
			a.Detail = fmt.Sprintf("%s on sheet %d: %d pin number(s) printed beside it, %d with a wire; %d problem(s)", tag, p.Number, len(printed), a.RowsRead, len(missed))
		} else {
			a.Status = "COMPLETE"
			a.Detail = fmt.Sprintf("%s on sheet %d: every one of the %d pin numbers printed beside it has exactly one wire", tag, p.Number, len(printed)-countIn(nc, printed))
			if len(nc) > 0 {
				a.Detail = fmt.Sprintf("%s on sheet %d: every pin number printed beside it has exactly one wire (%d); %d pin(s) are stated NC (no connect)", tag, p.Number, len(printed)-countIn(nc, printed), len(nc))
			}
		}
		out = append(out, a)
	}
	return out
}

func countMissing(printed map[string]bool, landed map[string]int) int {
	n := 0
	for id := range printed {
		if landed[id] == 0 {
			n++
		}
	}
	return n
}

func countNCUnwired(nc map[string]bool, landed map[string]int) int {
	n := 0
	for id := range nc {
		if landed[id] == 0 {
			n++
		}
	}
	return n
}

func countIn(set map[string]bool, in map[string]bool) int {
	n := 0
	for id := range set {
		if in[id] {
			n++
		}
	}
	return n
}

var rePinList = regexp.MustCompile(`^[0-9]{1,3}(?:-[0-9]{1,3})?(?:,[0-9]{1,3}(?:-[0-9]{1,3})?)*$`)

// expandPinList reads "5,6,16-19" as 5 6 16 17 18 19; nil when the text is
// not a pin list with at least a comma or a range.
func expandPinList(t string) []string {
	t = strings.ReplaceAll(strings.TrimSpace(t), " ", "")
	if !rePinList.MatchString(t) || !strings.ContainsAny(t, ",-") {
		return nil
	}
	var out []string
	for _, part := range strings.Split(t, ",") {
		if k := strings.Index(part, "-"); k > 0 {
			a, b := atoiSafe(part[:k]), atoiSafe(part[k+1:])
			if a < 1 || b < a || b-a > 200 {
				return nil
			}
			for n := a; n <= b; n++ {
				out = append(out, itoa(n))
			}
			continue
		}
		out = append(out, strings.TrimLeft(part, "0"))
	}
	return out
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return -1
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// blockSpan is the centre x and the top and bottom of a block's numbered pins.
func blockSpan(b tracedBlock) (cx, top, bot float64) {
	top, bot = math.Inf(-1), math.Inf(1)
	n := 0
	for _, w := range b.pins {
		if w.Text == "" {
			continue
		}
		n++
		cx += (w.X + w.right()) / 2
		top, bot = math.Max(top, w.Y), math.Min(bot, w.Y)
	}
	if n == 0 {
		return math.NaN(), 0, 0
	}
	return cx / float64(n), top, bot
}

// shieldPinRows reads cable shields terminated to a connector pin: "SHIELD"
// printed right before a pin number in a traced block's pin column. That pin
// is used (by the shield), though the shield carries no gauge label. A
// "SHIELD" with no number uses no pin.
func shieldPinRows(p Page, blocks []tracedBlock, pageRows []WireRow) []WireRow {
	var items []Word
	for _, l := range pageLines(p) {
		items = append(items, schematicWords(l)...)
	}
	used := map[string]bool{}
	for _, r := range pageRows {
		used[strings.ToUpper(r.Connector)+":"+r.Cavity] = true
		used[strings.ToUpper(r.End)+":"+r.EndCavity] = true
	}
	var out []WireRow
	for _, b := range blocks {
		cx, top, bot := blockSpan(b)
		if math.IsNaN(cx) {
			continue
		}
		for i, w := range items {
			em := math.Max(w.Size, 4)
			id, ok := pinAtEnd(w.Text)
			if !ok || math.Abs((w.X+w.right())/2-cx) > 1.2*em || w.Y > top+12*em || w.Y < bot-20*em || w.Y >= b.ceiling {
				continue
			}
			if i == 0 || !strings.EqualFold(strings.TrimSpace(items[i-1].Text), "SHIELD") || math.Abs(items[i-1].Y-w.Y) > 1.0*em || w.X-items[i-1].right() > 2.5*em {
				continue
			}
			// a shield pin belongs to one connector: the nearest block, never
			// across another connector's header
			if nearestBlock(blocks, w) != b.tag || headerBetweenY(p, cx, w.Y, top, bot) {
				continue
			}
			k := strings.ToUpper(b.tag) + ":" + id
			if used[k] {
				continue
			}
			used[k] = true
			out = append(out, WireRow{Page: p.Number, Connector: b.tag, Cavity: id, EndSignal: "cable shield terminated to this pin", Y: w.Y, Method: "shield-to-pin", Confidence: "DRAFT — SHIELD TERMINATED TO A PIN (SHIELD + PIN NUMBER AT THE CONNECTOR); CHECK SOURCE"})
		}
	}
	return out
}

// nearestBlock is the tag of the traced block a printed item belongs to: the
// block whose pin column it is beside and whose pins span its height, or the
// one closest to it vertically.
func nearestBlock(blocks []tracedBlock, w Word) string {
	best, bestD := "", math.Inf(1)
	em := math.Max(w.Size, 4)
	for _, b := range blocks {
		cx, top, bot := blockSpan(b)
		if math.IsNaN(cx) || math.Abs(w.right()-cx) > 8*em {
			continue
		}
		d := 0.0
		switch {
		case w.Y > top:
			d = w.Y - top
		case w.Y < bot:
			d = bot - w.Y
		}
		if d < bestD {
			best, bestD = b.tag, d
		}
	}
	return best
}

// headerBetweenY reports whether a connector header (a designator or a split
// part-number head) is printed, in the column at x, between height y and the
// block spanning top..bot.
func headerBetweenY(p Page, x, y, top, bot float64) bool {
	lo, hi := y, bot
	if y > top {
		lo, hi = top, y
	}
	if y >= bot && y <= top {
		return false
	}
	for _, w := range p.Words {
		t := strings.ToUpper(strings.TrimSpace(w.Text))
		if w.Rotated || w.Y <= lo || w.Y >= hi || math.Abs(w.X-x) > p.Width*.12 {
			continue
		}
		if (t != "" && reRefDesignator.FindString(t) == t) || reSplitPNHead.MatchString(t) {
			return true
		}
	}
	return false
}
