package main

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// General table reader (v2.7.2).
//
// Real drawings do not follow one template. This reader finds a HEADING BAND
// (one or two stacked lines of known column names, optionally with a group
// line such as FROM / TO above it), derives column extents from the heading
// words, then reads rows in whichever direction they run: below the heading
// (most tables) or ABOVE it (ASME Y14.34 parts lists built upward from the
// title block). The table kind is decided from the column roles present.

type tColumn struct {
	role   string
	label  string
	x0, x1 float64
}

type genericTable struct {
	Page    int
	Kind    string // bom | wire | conntable
	Cols    []tColumn
	Up      bool // rows run upward from the heading
	Title   string
	Rows    []map[string]string
	HeaderY float64
}

var headerVocab = map[string]string{
	// parts list / BOM
	"ITEM": "item", "ITEM NO": "item", "ITEM NUMBER": "item", "FIND": "item", "FIND NO": "item", "FIND NUMBER": "item", "F/N": "item", "FN": "item",
	"QTY": "qty", "QTY REQD": "qty", "QTY REQ": "qty", "QTY RQD": "qty", "QUANTITY": "qty", "QTY PER ASSY": "qty", "QTY/ASSY": "qty", "REQD": "qty", "QTY PER": "qty",
	"PART NUMBER": "part", "PART NO": "part", "PART #": "part", "P/N": "part", "PN": "part", "PART OR IDENTIFYING NO": "part", "PART OR IDENTIFYING NUMBER": "part",
	"IDENTIFYING NO": "part", "IDENTIFYING NUMBER": "part", "MFR PART NO": "part", "MFR PART NUMBER": "part", "MANUFACTURER PART NUMBER": "part", "MFG P/N": "part", "MFR P/N": "part", "PART/DWG NO": "part",
	"DESCRIPTION": "desc", "NOMENCLATURE": "desc", "NOMENCLATURE OR DESCRIPTION": "desc", "DESC": "desc", "NOUN": "desc",
	"CAGE": "cage", "CAGE CODE": "cage", "CODE IDENT": "cage", "CODE IDENT NO": "cage", "FSCM": "cage", "FSCM NO": "cage",
	"MFR": "vendor", "MANUFACTURER": "vendor", "VENDOR": "vendor", "MFG": "vendor",
	"REF DES": "ref", "REFDES": "ref", "REF": "ref", "REF DESIG": "ref", "REFERENCE DESIGNATOR": "ref", "REFERENCE DESIGNATION": "ref", "DESIGNATOR": "ref", "CONN": "ref", "CONNECTOR": "ref",
	"UM": "um", "U/M": "um", "UNIT": "um", "NOTES": "note", "NOTE": "note", "REMARKS": "note", "MATERIAL": "note", "SPECIFICATION": "note", "MATERIAL/SPEC": "note", "MATERIAL SPECIFICATION": "note",
	// wiring
	"PIN": "pin", "PIN NO": "pin", "PINS": "pin", "CONT": "pin", "CONT NO": "pin", "CONTACT": "pin", "CONTACT NO": "pin", "CAV": "pin", "CAV NO": "pin", "CAVITY": "pin", "TERM": "pin", "POS": "pin", "POSITION": "pin",
	"WIRE": "wire", "WIRE NO": "wire", "WIRE ID": "wire", "WIRE #": "wire", "CKT": "wire", "CKT NO": "wire", "CIRCUIT": "wire", "CONDUCTOR": "wire", "WIRE NUMBER": "wire",
	"SIGNAL": "signal", "SIGNAL NAME": "signal", "SIG": "signal", "SIG NAME": "signal", "FUNCTION": "signal", "NET": "signal", "NET NAME": "signal", "SIGNAL DESCRIPTION": "signal",
	"COLOR": "color", "CLR": "color", "COLOUR": "color", "WIRE COLOR": "color", "INSULATION COLOR": "color",
	"AWG": "awg", "GAGE": "awg", "GAUGE": "awg", "WIRE SIZE": "awg", "WIRE GAGE": "awg", "WIRE GAUGE": "awg", "GA": "awg", "SIZE": "awg",
	"FROM": "from", "SOURCE": "from", "FROM CONN": "from", "FROM REF DES": "from", "FROM REF": "from", "END A": "from",
	"TO": "to", "DEST": "to", "DESTINATION": "to", "TO CONN": "to", "TO REF DES": "to", "TO REF": "to", "TERMINATION": "to", "END B": "to",
	"FROM PIN": "frompin", "TO PIN": "topin", "WIRE TYPE": "other", "TYPE": "other", "LENGTH": "other", "SPEC": "other", "CABLE": "other", "SHIELD": "other", "TWIST": "other",
}

var groupWords = map[string]string{"FROM": "from", "TO": "to", "SOURCE": "from", "DESTINATION": "to", "END A": "from", "END B": "to", "ORIGIN": "from"}

func normHead(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.NewReplacer(".", "", ":", "", "_", " ", "(", "", ")", "").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

func headerRole(label string) string {
	n := normHead(label)
	// A heading the user taught on this PC wins over built-in vocabulary:
	// it reflects how their drawing standard uses the word.
	if r := learned.role(n); r != "" {
		return r
	}
	if r, ok := headerVocab[n]; ok {
		return r
	}
	return ""
}

// headerish reports whether a fragment is made of heading vocabulary words.
func headerish(text string) bool {
	n := normHead(text)
	if n == "" {
		return false
	}
	if knownHeading(n) {
		return true
	}
	parts := map[string]bool{"OR": true, "NO": true, "NAME": true, "DES": true, "REQD": true, "CODE": true, "IDENTIFYING": true, "NUMBER": true, "OR DESCRIPTION": true, "IDENTIFYING NO": true, "PART OR": true, "REF": true}
	if parts[n] {
		return true
	}
	known := 0
	toks := strings.Fields(n)
	for _, t := range toks {
		if _, ok := headerVocab[t]; ok || parts[t] {
			known++
		}
	}
	return len(toks) > 0 && known*2 >= len(toks)+1
}

type bandWord struct {
	w    Word
	line int
}

func lineEm(l evidenceLine) float64 {
	s, n := 0.0, 0
	for _, w := range l.Words {
		if w.Size > 0 {
			s += w.Size
			n++
		}
	}
	if n == 0 {
		return 7
	}
	return s / float64(n)
}

// buildHeader tries to form a heading band anchored on line i.
func buildHeader(lines []evidenceLine, i int) ([]tColumn, []int, bool) {
	base := lines[i]
	em := lineEm(base)
	hw := 0
	for _, w := range base.Words {
		if !w.Rotated && headerish(w.Text) {
			hw++
		}
	}
	if hw < 2 || hw*3 < len(base.Words)*2 {
		return nil, nil, false
	}
	band := []int{i}
	// Stacked heading lines above/below (a single-line heading can sit
	// vertically between the two halves of two-line headings).
	for _, j := range []int{i - 2, i - 1, i + 1, i + 2} {
		if j < 0 || j >= len(lines) || math.Abs(lines[j].Y-base.Y) > 2.4*em {
			continue
		}
		ok := len(lines[j].Words) > 0
		for _, w := range lines[j].Words {
			if !headerish(w.Text) {
				ok = false
			}
		}
		if ok {
			band = append(band, j)
		}
	}
	// Columns from the band line with the most fragments.
	baseIdx := i
	for _, j := range band {
		if len(lines[j].Words) > len(lines[baseIdx].Words) {
			baseIdx = j
		}
	}
	cols := []tColumn{}
	type acc struct {
		parts []Word
	}
	accs := []acc{}
	for _, w := range lines[baseIdx].Words {
		cols = append(cols, tColumn{x0: w.X, x1: w.right()})
		accs = append(accs, acc{parts: []Word{w}})
	}
	var groups []Word
	for _, j := range band {
		if j == baseIdx {
			continue
		}
		for _, w := range lines[j].Words {
			cx := (w.X + w.right()) / 2
			best, bd := -1, math.Inf(1)
			for k, c := range cols {
				var d float64
				if cx >= c.x0-em*0.6 && cx <= c.x1+em*0.6 {
					d = 0
				} else {
					d = math.Min(math.Abs(cx-c.x0), math.Abs(cx-c.x1))
				}
				// overlap test
				if w.X <= c.x1 && w.right() >= c.x0 {
					d = 0
				}
				if d < bd {
					best, bd = k, d
				}
			}
			if best >= 0 && bd <= em*1.2 {
				accs[best].parts = append(accs[best].parts, w)
				cols[best].x0 = math.Min(cols[best].x0, w.X)
				cols[best].x1 = math.Max(cols[best].x1, w.right())
			} else {
				// A heading on another band line that sits over no base column
				// is a column of its own (e.g. a one-line DESCRIPTION between
				// two-line headings).
				cols = append(cols, tColumn{x0: w.X, x1: w.right()})
				accs = append(accs, acc{parts: []Word{w}})
			}
		}
	}
	// A group line (FROM / TO) one line further up.
	top := band[0]
	for _, j := range band {
		if lines[j].Y > lines[top].Y {
			top = j
		}
	}
	for _, j := range []int{top - 1} {
		if j < 0 || lines[j].Y-lines[top].Y > 2.2*em {
			continue
		}
		all := true
		for _, w := range lines[j].Words {
			if _, g := groupWords[normHead(w.Text)]; !g {
				all = false
			}
		}
		if all {
			groups = append(groups, lines[j].Words...)
			band = append(band, j)
		}
	}
	idx := make([]int, len(cols))
	for k := range idx {
		idx[k] = k
	}
	sort.SliceStable(idx, func(a, b int) bool { return cols[idx[a]].x0 < cols[idx[b]].x0 })
	sc, sa := make([]tColumn, len(cols)), make([]acc, len(cols))
	for k, j := range idx {
		sc[k], sa[k] = cols[j], accs[j]
	}
	cols, accs = sc, sa
	roles := 0
	for k := range cols {
		ws := accs[k].parts
		sort.SliceStable(ws, func(a, b int) bool { return ws[a].Y > ws[b].Y })
		ss := []string{}
		for _, w := range ws {
			ss = append(ss, w.Text)
		}
		cols[k].label = strings.Join(ss, " ")
		cols[k].role = headerRole(cols[k].label)
		if cols[k].role == "" {
			// stacked labels sometimes read better per word ("FIND" + "NO.")
			for _, w := range ws {
				if r := headerRole(w.Text); r != "" && cols[k].role == "" {
					cols[k].role = r
				}
			}
		}
		if cols[k].role != "" {
			roles++
		}
	}
	if roles < 2 {
		return nil, nil, false
	}
	// Apply FROM/TO groups to designator/pin columns.
	if len(groups) > 0 {
		for k := range cols {
			if cols[k].role != "ref" && cols[k].role != "pin" {
				continue
			}
			cx := (cols[k].x0 + cols[k].x1) / 2
			best, bd := "", math.Inf(1)
			for _, g := range groups {
				d := math.Abs((g.X+g.right())/2 - cx)
				if d < bd {
					best, bd = groupWords[normHead(g.Text)], d
				}
			}
			switch {
			case best == "from" && cols[k].role == "ref":
				cols[k].role = "from"
			case best == "to" && cols[k].role == "ref":
				cols[k].role = "to"
			case best == "from" && cols[k].role == "pin":
				cols[k].role = "frompin"
			case best == "to" && cols[k].role == "pin":
				cols[k].role = "topin"
			}
		}
	}
	// In FROM/TO layouts, a pin column after the TO column is the far-end pin.
	sawTo := false
	for k := range cols {
		switch cols[k].role {
		case "to":
			sawTo = true
		case "pin":
			if sawTo {
				cols[k].role = "topin"
			}
		}
	}
	return cols, band, true
}

// splitColumns separates tables that share heading lines side by side: a wide
// horizontal gap where both sides are complete tables on their own.
func splitColumns(cols []tColumn, em float64) [][]tColumn {
	if len(cols) < 4 {
		return [][]tColumn{cols}
	}
	type gap struct {
		at int
		w  float64
	}
	gaps := []gap{}
	for k := 1; k < len(cols); k++ {
		if g := cols[k].x0 - cols[k-1].x1; g >= 8*em {
			gaps = append(gaps, gap{k, g})
		}
	}
	sort.Slice(gaps, func(a, b int) bool { return gaps[a].w > gaps[b].w })
	for _, g := range gaps {
		l, r := cols[:g.at], cols[g.at:]
		if tableKind(l) != "" && tableKind(r) != "" {
			return append(splitColumns(l, em), splitColumns(r, em)...)
		}
	}
	return [][]tColumn{cols}
}

func tableKind(cols []tColumn) string {
	has := map[string]bool{}
	for _, c := range cols {
		has[c.role] = true
	}
	switch {
	case has["part"] && (has["qty"] || has["cage"] || (has["item"] && has["desc"])):
		return "bom"
	case (has["pin"] || has["frompin"]) && (has["to"] || has["topin"] || has["wire"] || has["signal"] || has["from"]):
		return "wire"
	case has["from"] && has["to"] && (has["wire"] || has["signal"]):
		return "wire"
	case has["ref"] && (has["part"] || has["item"]) && !has["qty"]:
		return "conntable"
	}
	return ""
}

func assignCells(l evidenceLine, cols []tColumn, em float64) map[string]string {
	out := map[string]string{}
	left, right := cols[0].x0-3*em, cols[len(cols)-1].x1+12*em
	for _, w := range l.Words {
		if w.Rotated {
			continue
		}
		cx := (w.X + w.right()) / 2
		if cx < left || w.X > right {
			continue
		}
		best, bd := -1, math.Inf(1)
		for k, c := range cols {
			d := 0.0
			if cx < c.x0 {
				d = c.x0 - cx
			} else if cx > c.x1 {
				d = cx - c.x1
			}
			// prefer the column whose heading starts at/left of the value
			if d < bd-0.01 {
				best, bd = k, d
			}
		}
		if best < 0 || cols[best].role == "" {
			continue
		}
		r := cols[best].role
		if out[r] != "" {
			out[r] += " "
		}
		out[r] += strings.TrimSpace(w.Text)
	}
	return out
}

var (
	reQty      = regexp.MustCompile(`(?i)^(?:[0-9]{1,5}(?:\.[0-9]+)?|A/?R|AS REQ(?:UIRED|D)?)$`)
	reItemNo   = regexp.MustCompile(`^[0-9]{1,3}[A-Z]?$`)
	rePartLike = regexp.MustCompile(`(?i)^[A-Z0-9][A-Z0-9./_-]{2,40}$`)
)

func validRow(kind string, c map[string]string) bool {
	switch kind {
	case "bom":
		p := strings.ToUpper(strings.ReplaceAll(c["part"], " ", ""))
		if !rePartLike.MatchString(p) || !strings.ContainsAny(p, "0123456789") || headerRole(c["part"]) != "" {
			return false
		}
		return reItemNo.MatchString(strings.TrimSpace(c["item"])) || reQty.MatchString(strings.TrimSpace(c["qty"]))
	case "wire":
		pin := strings.TrimSpace(c["pin"] + c["frompin"])
		if pin == "" {
			// combined "P1-23" in the FROM cell, no separate pin column
			if _, p := parseDestination(c["from"]); p != "" {
				pin = p
			}
		}
		if pin == "" || !pinValue.MatchString(pin) {
			return false
		}
		return c["to"] != "" || c["topin"] != "" || c["wire"] != "" || c["signal"] != ""
	case "conntable":
		return reDestTag.MatchString(normalizedTag(c["ref"]))
	}
	return false
}

func readRows(lines []evidenceLine, from int, step int, band map[int]bool, kind string, cols []tColumn, em float64, startY float64) []map[string]string {
	rows := []map[string]string{}
	misses := 0
	lastY := startY
	pitch := 0.0
	for i := from; i >= 0 && i < len(lines); i += step {
		if band[i] {
			continue
		}
		l := lines[i]
		gap := math.Abs(lastY - l.Y)
		if (pitch > 0 && gap > math.Max(3.2*pitch, 3.5*em)) || (pitch == 0 && gap > 5*em) {
			break
		}
		c := assignCells(l, cols, em)
		if len(c) == 0 {
			misses++
			if misses >= 2 {
				break
			}
			continue
		}
		if !validRow(kind, c) {
			// a wrapped description continues the previous row (rows read
			// top-down have the wrap below, bottom-up above)
			if kind == "bom" && len(rows) > 0 && c["desc"] != "" && c["part"] == "" && c["item"] == "" && c["qty"] == "" {
				prev := rows[len(rows)-1]
				if step > 0 {
					prev["desc"] = strings.TrimSpace(prev["desc"] + " " + c["desc"])
				} else {
					prev["desc"] = strings.TrimSpace(c["desc"] + " " + prev["desc"])
				}
				lastY = l.Y
				continue
			}
			misses++
			if misses >= 2 {
				break
			}
			continue
		}
		misses = 0
		if pitch == 0 {
			pitch = gap
		} else {
			pitch = (2*pitch + gap) / 3
		}
		lastY = l.Y
		rows = append(rows, c)
	}
	return rows
}

func genericTables(p Page) []genericTable {
	if !p.PageAccurate {
		return nil
	}
	lines := pageLines(p)
	used := map[int]bool{}
	out := []genericTable{}
	for i := range lines {
		if used[i] {
			continue
		}
		cols, band, ok := buildHeader(lines, i)
		if !ok {
			continue
		}
		bandSet := map[int]bool{}
		minI, maxI := len(lines), -1
		hy := 0.0
		for _, j := range band {
			bandSet[j] = true
			used[j] = true
			if j < minI {
				minI = j
			}
			if j > maxI {
				maxI = j
				hy = lines[j].Y
			}
		}
		em := lineEm(lines[i])
		for _, seg := range splitColumns(cols, em) {
			kind := tableKind(seg)
			if kind == "" {
				continue
			}
			down := readRows(lines, maxI+1, 1, bandSet, kind, seg, em, lines[maxI].Y)
			up := readRows(lines, minI-1, -1, bandSet, kind, seg, em, lines[minI].Y)
			t := genericTable{Page: p.Number, Kind: kind, Cols: seg, HeaderY: hy}
			if len(up) > len(down) {
				t.Up, t.Rows = true, up
			} else {
				t.Rows = down
			}
			if len(t.Rows) == 0 {
				continue
			}
			if kind == "wire" {
				has := map[string]bool{}
				for _, c := range seg {
					has[c.role] = true
				}
				if !has["from"] {
					t.Title = tableTitleTag(lines, minI, seg[0].x0, seg[len(seg)-1].x1+160)
				}
			}
			out = append(out, t)
		}
	}
	return out
}

var reNotConnected = regexp.MustCompile(`(?i)^(?:-+|—|N/?C|NOT USED|UNUSED|SPARE|NO CONNECT(?:ION)?|OPEN)?$`)

// notConnectedRow is the single rule for "this pin-table row documents an
// unused cavity": no destination, no far-end pin and no wire, and either the
// signal says so (SPARE, N/C…) or the destination cell itself says so. A row
// that names a real signal but no destination stays a conductor candidate.
// wireRows and unusedRows both use it, so a cavity is never both.
func notConnectedRow(r map[string]string) bool {
	nc := func(k string) bool { return reNotConnected.MatchString(strings.TrimSpace(r[k])) }
	if !(nc("to") && nc("topin") && nc("wire")) {
		return false
	}
	return nc("signal") || strings.TrimSpace(r["to"]) != ""
}

var reConnectorNoun = regexp.MustCompile(`^\s*(?:CONNECTOR|RECEPTACLE)\b`)

func (t genericTable) bomRows() []BOMRow {
	out := []BOMRow{}
	order := "heading above rows"
	if t.Up {
		order = "heading BELOW rows (parts list built upward)"
	}
	for _, r := range t.Rows {
		vendor := r["vendor"]
		if vendor == "" && r["cage"] != "" {
			vendor = "CAGE " + r["cage"]
		}
		qty := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(r["qty"]), " ", ""))
		if qty == "AR" || strings.HasPrefix(qty, "ASREQ") {
			qty = "A/R"
		}
		if qty == "" {
			qty = "UNRESOLVED"
		}
		out = append(out, BOMRow{Item: strings.TrimSpace(r["item"]), Qty: qty, Part: strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(r["part"]), " ", "")), Vendor: vendor, Description: r["desc"], Reference: r["ref"], Page: t.Page, Status: "DRAFT — TABLE READER (" + order + "); CHECK SOURCE"})
	}
	return out
}

func (t genericTable) wireRows() []WireRow {
	out := []WireRow{}
	for i, r := range t.Rows {
		// Spare / not-connected cavities document an unused position, not a conductor.
		if notConnectedRow(r) {
			continue
		}
		pin := strings.TrimSpace(r["pin"] + r["frompin"])
		conn := normalizedTag(r["from"])
		if !reDestTag.MatchString(conn) {
			if tg, p := parseDestination(r["from"]); tg != "" && p != "" && pin == "" {
				conn, pin = tg, p
			} else {
				conn = t.Title
			}
		}
		if conn == "" {
			conn = "UNASSIGNED"
		}
		end, endPin := "", strings.TrimSpace(r["topin"])
		if tg := normalizedTag(r["to"]); reDestTag.MatchString(tg) {
			end = tg
		} else if r["to"] != "" {
			end, endPin = parseDestination(r["to"])
			if endPin == "" {
				endPin = strings.TrimSpace(r["topin"])
			}
		}
		endSignal := ""
		if end == "" {
			end, endSignal = "UNRESOLVED ENDPOINT", r["to"]
		}
		if !pinValue.MatchString(endPin) {
			endPin = ""
		}
		out = append(out, WireRow{Page: t.Page, Connector: conn, Cavity: pin, Signal: r["signal"], End: end, EndCavity: endPin, EndSignal: endSignal, Confidence: "DRAFT — TABLE READER (header-grounded)", Y: t.HeaderY - float64(i), Wire: r["wire"], Color: r["color"], AWG: r["awg"], Method: "table-reader"})
	}
	return out
}

// isAccessoryPart identifies parts that belong to a connector but are not a
// connector themselves (contacts, sealing plugs, caps, backshells, wire,
// sleeving). They must never become connector records.
func isAccessoryPart(pn, desc string) bool {
	p := strings.ToUpper(strings.ReplaceAll(pn, " ", ""))
	for _, pre := range []string{"MS27488", "M39029", "M85049", "AS85049", "SAE-AS85049", "M22759", "M23053", "M81044", "MS3367", "MS3420", "D38999/32", "D38999/33", "M83519", "MS25036", "M27500"} {
		if strings.HasPrefix(p, pre) {
			return true
		}
	}
	d := strings.ToUpper(desc)
	// "CONNECTOR, RECEPTACLE, ... ENVIRONMENT SEALING" describes a connector,
	// so the bare word SEALING only marks an accessory for other nouns.
	if !reConnectorNoun.MatchString(d) && strings.Contains(d, "SEALING") {
		return true
	}
	for _, k := range []string{"PLUG, SEAL", "SEAL PLUG", "CAP, PROTECT", "PROTECTIVE CAP", "DUST CAP", "BACKSHELL", "CONTACT, ", "CONTACT SOCKET", "CONTACT PIN", "SLEEV", "HEAT SHRINK", "WIRE,", "CABLE TIE", "STRAIN RELIEF", "FILLER"} {
		if strings.Contains(d, k) {
			return true
		}
	}
	return false
}

// connectorTableMap returns designator -> part number from connector tables,
// resolving FIND NO. references through the parts list.
func connectorTableMap(tables []genericTable, bom []BOMRow) map[string]string {
	byFind := map[string]string{}
	for _, b := range bom {
		if b.Item != "" {
			byFind[strings.TrimLeft(b.Item, "0")] = b.Part
		}
	}
	out := map[string]string{}
	for _, t := range tables {
		if t.Kind != "conntable" {
			continue
		}
		for _, r := range t.Rows {
			tag := normalizedTag(r["ref"])
			part := strings.ToUpper(strings.ReplaceAll(r["part"], " ", ""))
			if part == "" && r["item"] != "" {
				part = byFind[strings.TrimLeft(strings.TrimSpace(r["item"]), "0")]
			}
			if tag != "" && part != "" {
				out[tag] = part
			}
		}
	}
	return out
}

func tableSummary(t genericTable) string {
	roles := []string{}
	for _, c := range t.Cols {
		r := c.role
		if r == "" {
			r = "?"
		}
		roles = append(roles, r)
	}
	dir := "rows below heading"
	if t.Up {
		dir = "rows ABOVE heading"
	}
	return fmt.Sprintf("%s table, %d row(s), %s, columns [%s]", t.Kind, len(t.Rows), dir, strings.Join(roles, ", "))
}


// unusedRows returns rows of a pin/wire table that document a spare or
// not-connected cavity (they are skipped as conductors).
func (t genericTable) unusedRows() []UnusedCavity {
	out := []UnusedCavity{}
	if t.Kind != "wire" {
		return out
	}
	for _, r := range t.Rows {
		if !notConnectedRow(r) {
			continue
		}
		pin := strings.TrimSpace(r["pin"] + r["frompin"])
		conn := normalizedTag(r["from"])
		if !reDestTag.MatchString(conn) {
			if tg, p := parseDestination(r["from"]); tg != "" && p != "" && pin == "" {
				conn, pin = tg, p
			} else {
				conn = t.Title
			}
		}
		if conn == "" || pin == "" {
			continue
		}
		label := strings.TrimSpace(r["signal"])
		if label == "" {
			label = "no destination"
		}
		out = append(out, UnusedCavity{Connector: conn, Cavity: pin, Page: t.Page, Source: "pin table row marked " + label + " on sheet " + itoa(t.Page)})
	}
	return out
}
