package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// Pin/wire table completeness audit.
//
// Unused cavities are found by process of elimination, so a printed wire row
// the reader did not accept would turn a used cavity into an "unused" one.
// After a pin/wire table is read, this audit compares what was read with the
// drawing itself, two independent ways:
//
//  1. Leftover text: every printed line inside the table area (and the lines
//     that continue it below the last row read) that carries a pin or wire
//     value but was not read as a row is a missed row.
//  2. Ruled rows: when the table is drawn as a grid, every ruled row band that
//     holds text must contain a row that was read.
//
// A missed row holds process of elimination for every connector in the
// table. A ruled table whose every row was read confirms it.

type MissedRow struct {
	Text   string  `json:"text"`
	Reason string  `json:"reason"`
	Y      float64 `json:"y"`
}

type TableAudit struct {
	Page       int         `json:"page"`
	Title      string      `json:"title"`
	Connectors []string    `json:"connectors"`
	RowsRead   int         `json:"rowsRead"`
	Ruled      bool        `json:"ruled"`
	RuledRows  int         `json:"ruledRows"` // ruled bands holding text; 0 when not ruled
	Missed     []MissedRow `json:"missed"`
	Status     string      `json:"status"` // COMPLETE | INCOMPLETE
	Detail     string      `json:"detail"`
}

func auditWireTable(p Page, lines []evidenceLine, t genericTable, band map[int]bool, em float64, headerBottomY float64) TableAudit {
	a := TableAudit{Page: t.Page, Title: t.Title, Connectors: []string{}, Missed: []MissedRow{}, RowsRead: len(t.Rows)}
	seen := map[string]bool{}
	addConn := func(s string) {
		if s != "" && s != "UNASSIGNED" && s != "UNRESOLVED ENDPOINT" && !seen[s] {
			seen[s] = true
			a.Connectors = append(a.Connectors, s)
		}
	}
	addConn(t.Title)
	for _, r := range t.wireRows() {
		addConn(r.Connector)
		addConn(r.End)
	}
	for _, u := range t.unusedRows() {
		addConn(u.Connector)
	}
	sort.Strings(a.Connectors)

	colX0, colX1 := math.Inf(1), math.Inf(-1)
	for _, c := range t.Cols {
		colX0, colX1 = math.Min(colX0, c.x0), math.Max(colX1, c.x1)
	}
	tx0, tx1 := colX0-1.0*em, colX1+4*em
	inSpan := func(w Word) bool {
		cx := (w.X + w.right()) / 2
		return !w.Rotated && cx >= tx0 && cx <= tx1
	}
	// only the words of a line that fall inside this table
	clip := func(l evidenceLine) evidenceLine {
		o := evidenceLine{Y: l.Y}
		parts := []string{}
		for _, w := range l.Words {
			if inSpan(w) {
				o.Words = append(o.Words, w)
				parts = append(parts, strings.TrimSpace(w.Text))
			}
		}
		o.Text = strings.Join(parts, " ")
		return o
	}
	// A line carries a row if it has a pin/cavity value or a wire ID in this
	// table's columns. Other text (a wrapped signal name, a note) does not.
	keyLine := func(l evidenceLine) bool {
		c := assignCells(l, t.Cols, em)
		pin := strings.TrimSpace(c["pin"] + c["frompin"])
		if pin != "" && pinValue.MatchString(strings.Fields(pin)[0]) {
			return true
		}
		if _, pp := parseDestination(c["from"]); pp != "" {
			return true
		}
		return strings.TrimSpace(c["wire"]) != ""
	}
	rowAt := func(y, tol float64) bool {
		for _, ry := range t.RowY {
			if math.Abs(ry-y) <= tol {
				return true
			}
		}
		return false
	}
	pitch := 1.6 * em
	if len(t.RowY) >= 2 {
		gaps := []float64{}
		for i := 1; i < len(t.RowY); i++ {
			if g := math.Abs(t.RowY[i-1] - t.RowY[i]); g > 0.5 {
				gaps = append(gaps, g)
			}
		}
		if len(gaps) > 0 {
			sort.Float64s(gaps)
			pitch = gaps[len(gaps)/2]
		}
	}
	lastRowY := headerBottomY
	for _, y := range t.RowY {
		lastRowY = math.Min(lastRowY, y)
	}
	missedAt := map[int]bool{}
	addMissed := func(l evidenceLine, reason string) {
		k := int(math.Round(l.Y * 2))
		if missedAt[k] {
			return
		}
		missedAt[k] = true
		a.Missed = append(a.Missed, MissedRow{Text: l.Text, Reason: reason, Y: l.Y})
	}

	// 1a. Lines between the heading and the last row read.
	for i, l := range lines {
		if band[i] || l.Y >= headerBottomY-0.5 || l.Y < lastRowY-0.5 {
			continue
		}
		cl := clip(l)
		if len(cl.Words) == 0 || rowAt(l.Y, 1.0) || !keyLine(cl) {
			continue
		}
		addMissed(cl, "printed inside the table but not read as a row")
	}
	// 1b. Lines that continue the table below the last row read (the reader
	// stops after rows it cannot read, so the rest of the table can be lost).
	cursor := lastRowY
	for _, l := range lines {
		if l.Y >= lastRowY-0.5 {
			continue
		}
		if cursor-l.Y > math.Max(1.8*pitch, 1.5*em) {
			break
		}
		cl := clip(l)
		if len(cl.Words) == 0 {
			continue
		}
		if keyLine(cl) {
			addMissed(cl, "continues the table below the last row read")
			cursor = l.Y
		} else if len(assignCells(cl, t.Cols, em)) > 0 {
			cursor = l.Y // wrapped text inside the table; keep following it
		} else {
			break
		}
	}

	// 2. Ruled rows.
	if !p.RulesTruncated && len(t.RowY) > 0 {
		width := colX1 - colX0
		ys := []float64{}
		for _, s := range p.HRules {
			ov := math.Min(s[2], colX1) - math.Max(s[0], colX0)
			if ov >= 0.6*width {
				ys = append(ys, s[1])
			}
		}
		sort.Sort(sort.Reverse(sort.Float64Slice(ys)))
		merged := []float64{}
		for _, y := range ys {
			if len(merged) == 0 || merged[len(merged)-1]-y > 1.0 {
				merged = append(merged, y)
			}
		}
		firstRow := t.RowY[0]
		for _, y := range t.RowY {
			firstRow = math.Max(firstRow, y)
		}
		// The rule that closes the heading: the lowest rule above the first row
		// and not above the heading text.
		start := -1
		for k, y := range merged {
			if y > firstRow && y <= headerBottomY+0.5 {
				start = k
			}
		}
		if start >= 0 {
			bands := [][2]float64{}
			ref := 0.0
			for k := start; k+1 < len(merged); k++ {
				h := merged[k] - merged[k+1]
				if ref > 0 && h > 3*ref {
					break
				}
				if ref == 0 || h < ref {
					ref = h
				}
				bands = append(bands, [2]float64{merged[k], merged[k+1]})
			}
			if len(bands) >= 2 {
				a.Ruled = true
				for _, b := range bands {
					var inside []evidenceLine
					for i, l := range lines {
						if band[i] || l.Y >= b[0] || l.Y <= b[1] {
							continue
						}
						if cl := clip(l); len(cl.Words) > 0 {
							inside = append(inside, cl)
						}
					}
					if len(inside) == 0 {
						continue
					}
					a.RuledRows++
					read := false
					for _, ry := range t.RowY {
						if ry < b[0] && ry > b[1] {
							read = true
						}
					}
					if read {
						continue
					}
					for _, l := range inside {
						if keyLine(l) {
							addMissed(l, "a ruled row of the table that was not read")
						}
					}
				}
			}
		}
	}
	sort.SliceStable(a.Missed, func(i, j int) bool { return a.Missed[i].Y > a.Missed[j].Y })
	where := "pin/wire table on sheet " + itoa(a.Page)
	if a.Title != "" {
		where = a.Title + " " + where
	}
	switch {
	case len(a.Missed) > 0:
		a.Status = "INCOMPLETE"
		a.Detail = fmt.Sprintf("%s: %d printed row(s) with a pin or wire were not read.", where, len(a.Missed))
	case a.Ruled && a.RuledRows == a.RowsRead:
		a.Status = "COMPLETE"
		a.Detail = fmt.Sprintf("%s has %d ruled rows and every one was read.", where, a.RuledRows)
	case a.Ruled:
		a.Status = "COMPLETE"
		a.Detail = fmt.Sprintf("%s: every ruled row holding a pin or wire was read (%d rows read in %d ruled rows).", where, a.RowsRead, a.RuledRows)
	default:
		a.Status = "COMPLETE"
		a.Detail = fmt.Sprintf("%s: no unread pin or wire text in the table area (table is not ruled, so rows could not be counted).", where)
	}
	return a
}
