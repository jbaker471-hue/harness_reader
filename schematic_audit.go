package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Completeness audit for wiring diagrams read by pairedSchematicRows.
//
// On a wiring diagram each wire is one printed line: pin + colour beside the
// left connector block, colour + pin beside the right one. After the rows are
// read, every other line in the same column area that still carries a
// pin + colour label at either connector is a wire that was not read (an
// ambiguous line, a one-sided line, or a label the reader could not pair).
// It is reported exactly like a missed table row.
func auditSchematic(p Page, rows []WireRow) (TableAudit, bool) {
	if len(rows) == 0 {
		return TableAudit{}, false
	}
	a := TableAudit{Page: p.Number, Title: "wiring diagram", Connectors: []string{}, Missed: []MissedRow{}, RowsRead: len(rows)}
	seen := map[string]bool{}
	for _, r := range rows {
		for _, t := range []string{r.Connector, r.End} {
			if t != "" && !seen[t] {
				seen[t] = true
				a.Connectors = append(a.Connectors, t)
			}
		}
	}
	sort.Strings(a.Connectors)
	readY := map[int]bool{}
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, r := range rows {
		readY[int(math.Round(r.Y))] = true
		minY, maxY = math.Min(minY, r.Y), math.Max(maxY, r.Y)
	}
	isRead := func(y float64) bool {
		k := int(math.Round(y))
		return readY[k] || readY[k-1] || readY[k+1]
	}
	type ep struct{ x, em float64 }
	endpoints := func(l evidenceLine) (left, right []ep) {
		ws := schematicWords(l)
		em := lineEm(l)
		for i := 0; i+1 < len(ws); i++ {
			x, y := ws[i], ws[i+1]
			if y.X-x.right() > 4*em || y.X <= x.X {
				continue
			}
			if schematicPin.MatchString(x.Text) && schematicColor(y.Text) != "" {
				left = append(left, ep{x.X, em})
			}
			if schematicColor(x.Text) != "" && schematicPin.MatchString(y.Text) {
				right = append(right, ep{y.X, em})
			}
		}
		return
	}
	lines := pageLines(p)
	// Column positions from the rows that were read.
	var lx, rx []float64
	pitch := 0.0
	var ys []float64
	for _, l := range lines {
		if !isRead(l.Y) {
			continue
		}
		ys = append(ys, l.Y)
		L, R := endpoints(l)
		if len(L) == 1 {
			lx = append(lx, L[0].x)
		}
		if len(R) == 1 {
			rx = append(rx, R[0].x)
		}
	}
	median := func(v []float64) float64 {
		if len(v) == 0 {
			return math.NaN()
		}
		s := append([]float64(nil), v...)
		sort.Float64s(s)
		return s[len(s)/2]
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(ys)))
	gaps := []float64{}
	for i := 1; i < len(ys); i++ {
		gaps = append(gaps, ys[i-1]-ys[i])
	}
	pitch = median(gaps)
	if math.IsNaN(pitch) || pitch <= 0 {
		pitch = 20
	}
	mlx, mrx := median(lx), median(rx)
	near := func(x, m, em float64) bool { return !math.IsNaN(m) && math.Abs(x-m) <= 3*em }
	for _, l := range lines {
		if isRead(l.Y) || l.Y > maxY+3*pitch || l.Y < minY-3*pitch {
			continue
		}
		L, R := endpoints(l)
		hit := false
		for _, e := range L {
			hit = hit || near(e.x, mlx, e.em)
		}
		for _, e := range R {
			hit = hit || near(e.x, mrx, e.em)
		}
		if hit {
			a.Missed = append(a.Missed, MissedRow{Text: strings.TrimSpace(l.Text), Reason: "a pin and colour beside the connector that was not read as a wire", Y: l.Y})
		}
	}
	where := "wiring diagram on sheet " + itoa(p.Number)
	if len(a.Missed) > 0 {
		a.Status = "INCOMPLETE"
		a.Detail = fmt.Sprintf("%s: %d printed wire line(s) with a pin and colour were not read.", where, len(a.Missed))
	} else {
		a.Status = "COMPLETE"
		a.Detail = fmt.Sprintf("%s: every pin-and-colour label beside %s was read (%d wires).", where, strings.Join(a.Connectors, " and "), len(rows))
	}
	return a, true
}
