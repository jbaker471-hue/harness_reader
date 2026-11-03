package main

import (
	"math"
	"regexp"
	"sort"
	"strings"
)

// Header-grounded pinout / wire-list tables, column based.
//
// Typical harness layouts handled:
//
//	CONNECTOR P2 PINOUT                 FROM  PIN  SIGNAL  TO   PIN
//	PIN  SIGNAL  WIRE  COLOR  TO        J12   1    VBUS    J21  3
//	A    +28V    W101  RED    TB1-4
//
// A row is only accepted under a heading that names a pin/cavity column and
// at least one wiring attribute. Column positions come from the heading, so
// the table can sit anywhere on any sheet. Every row is a DRAFT candidate.

type pinRole string

const (
	roleFrom   pinRole = "from"
	rolePin    pinRole = "pin"
	roleSignal pinRole = "signal"
	roleWire   pinRole = "wire"
	roleColor  pinRole = "color"
	roleAWG    pinRole = "awg"
	roleTo     pinRole = "to"
	roleToPin  pinRole = "topin"
	roleLength pinRole = "length"
)

var pinHeaderWords = []struct {
	words []string
	role  pinRole
}{
	{[]string{"FROM PIN", "SOURCE PIN"}, rolePin},
	{[]string{"TO PIN", "DEST PIN", "DESTINATION PIN", "END PIN"}, roleToPin},
	{[]string{"PIN NO", "PIN #", "PIN", "PINS", "CAV", "CAVITY", "CAV NO", "CONTACT", "CONTACT NO", "POS", "POSITION", "TERM", "TERMINAL"}, rolePin},
	{[]string{"SIGNAL NAME", "SIGNAL", "FUNCTION", "NET NAME", "NET", "CIRCUIT", "DESCRIPTION", "NAME"}, roleSignal},
	{[]string{"WIRE NO", "WIRE #", "WIRE ID", "WIRE", "CONDUCTOR", "CKT"}, roleWire},
	{[]string{"COLOR", "COLOUR", "CLR", "WIRE COLOR", "INSULATION COLOR"}, roleColor},
	{[]string{"AWG", "GAUGE", "WIRE SIZE", "WIRE GAUGE", "GA", "SIZE"}, roleAWG},
	{[]string{"LENGTH", "LEN", "WIRE LENGTH", "CUT LENGTH", "LENGTH IN", "LENGTH INCHES", "LENGTH FT", "LENGTH MM", "LENGTH CM"}, roleLength},
	{[]string{"TO", "DEST", "DESTINATION", "TERMINATION", "CONNECTS TO", "MATES TO", "TO CONN", "TO CONNECTOR"}, roleTo},
	{[]string{"FROM", "SOURCE", "FROM CONN", "FROM CONNECTOR", "CONN", "CONNECTOR", "REF DES", "REFDES"}, roleFrom},
}

var (
	pinValue     = regexp.MustCompile(`(?i)^(?:[A-Z]{1,2}[0-9]{0,3}[A-Z]?|[0-9]{1,4}[A-Z]?)$`)
	reDestPin    = regexp.MustCompile(`(?i)^((?:J|P|X|TB|CN|CON|PL|SK|S|SP|E|GND)[-_]?[0-9]{1,4}[A-Z]?)\s*(?:[-:/.]|\s+PIN\s+|\s+CAV\s+|\s+)\s*([A-Z]{0,2}[0-9]{1,4}[A-Z]?|[A-Z]{1,2})$`)
	reDestTag    = regexp.MustCompile(`^(?:[A-Z][A-Z0-9]*[-_])*(?:J|P|X|TB|CN|CON|PL|SK|S|SP|E)[-_]?[0-9]{1,4}[A-Z]?$`)
	reBOMHeading = regexp.MustCompile(`(?i)\b(?:QTY|QUANTITY|PART\s*(?:NUMBER|NO\.?|#)|P/N|ITEM|FIND\s+NO)\b`)
)

type headerCol struct {
	role pinRole
	x    float64
}

func normHeader(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.NewReplacer(".", " ", ":", " ", "_", " ").Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// headerColumns parses a line into ordered column roles, or nil.
func headerColumns(l evidenceLine) []headerCol {
	if reBOMHeading.MatchString(l.Text) {
		return nil
	}
	cols := []headerCol{}
	used := make([]bool, len(l.Words))
	for i := 0; i < len(l.Words); i++ {
		if used[i] {
			continue
		}
		matched := false
		// Try phrases of up to three adjacent fragments, longest first.
		for span := 3; span >= 1 && !matched; span-- {
			if i+span > len(l.Words) {
				continue
			}
			// Only fragments that touch each other form one heading cell;
			// separate adjacent headings ("WIRE" | "COLOR") must not merge.
			adjacent := true
			for k := i; k < i+span-1; k++ {
				a, b := l.Words[k], l.Words[k+1]
				end := a.X + float64(len([]rune(a.Text)))*0.62*math.Max(a.Size, 4)
				if b.X-end > 1.3*math.Max(a.Size, 4) {
					adjacent = false
				}
			}
			if !adjacent {
				continue
			}
			parts := []string{}
			for _, w := range l.Words[i : i+span] {
				parts = append(parts, w.Text)
			}
			phrase := normHeader(strings.Join(parts, " "))
			for _, h := range pinHeaderWords {
				for _, cand := range h.words {
					if phrase == cand {
						cols = append(cols, headerCol{h.role, l.Words[i].X})
						for k := i; k < i+span; k++ {
							used[k] = true
						}
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
		}
		if !matched {
			// An unrecognised word inside a heading row is allowed, but a
			// heading must be mostly recognised headings.
			cols = append(cols, headerCol{"", l.Words[i].X})
		}
	}
	known, pins, attrs := 0, 0, 0
	for _, c := range cols {
		switch c.role {
		case "":
		case rolePin, roleToPin:
			known++
			pins++
		default:
			known++
			attrs++
		}
	}
	if pins == 0 || attrs == 0 || known < 2 || known*3 < len(cols)*2 {
		return nil
	}
	// Disambiguate: "PIN" after FROM is the source pin, "PIN" after TO is the destination pin.
	sawTo := false
	pinSeen := 0
	for i := range cols {
		switch cols[i].role {
		case roleTo:
			sawTo = true
		case rolePin:
			pinSeen++
			if sawTo || pinSeen == 2 {
				cols[i].role = roleToPin
			}
		}
	}
	hasSource := false
	for _, c := range cols {
		if c.role == rolePin {
			hasSource = true
		}
	}
	if !hasSource {
		return nil
	}
	return cols
}

func cellsFor(l evidenceLine, cols []headerCol, right float64) map[pinRole]string {
	out := map[pinRole]string{}
	for _, w := range l.Words {
		if w.X < cols[0].x-14 || w.X > right {
			continue
		}
		idx := 0
		for i := range cols {
			// A cell belongs to the last column whose heading starts at or
			// before it (left-aligned tables), with a small tolerance for
			// centred values.
			if w.X >= cols[i].x-6 {
				idx = i
			}
		}
		r := cols[idx].role
		if r == "" {
			continue
		}
		if out[r] != "" {
			out[r] += " "
		}
		out[r] += strings.TrimSpace(w.Text)
	}
	return out
}

// tableTitleTag finds the connector named in a title just above a heading,
// e.g. "CONNECTOR P2 PINOUT" or "J1 (RECEPTACLE)".
func tableTitleTag(lines []evidenceLine, header int, x0, x1 float64) string {
	h := lines[header]
	for _, w := range h.Words {
		if t := reDestTag.FindString(normalizedTag(w.Text)); t != "" && !strings.HasPrefix(t, "S") {
			return t
		}
	}
	for i := header - 1; i >= 0 && i >= header-4; i-- {
		l := lines[i]
		if l.Y-h.Y > 60 {
			break
		}
		for _, w := range l.Words {
			if w.X < x0-60 || w.X > x1 {
				continue
			}
			for _, tok := range strings.Fields(strings.ToUpper(w.Text)) {
				tok = normalizedTag(tok)
				if reDestTag.MatchString(tok) && !strings.HasPrefix(tok, "S") && !strings.HasPrefix(tok, "E") {
					return tok
				}
			}
		}
	}
	return ""
}

func parseDestination(s string) (tag, pin string) {
	u := strings.Join(strings.Fields(s), " ")
	if m := reDestPin.FindStringSubmatch(u); len(m) == 3 {
		return normalizedTag(m[1]), m[2] // pin keeps its printed case (A and a are different cavities)
	}
	u = strings.ToUpper(u)
	if reDestTag.MatchString(u) {
		return normalizedTag(u), ""
	}
	return "", ""
}

func pinoutTableRows(p Page) []WireRow {
	if !p.PageAccurate {
		return nil
	}
	lines := pageLines(p)
	out := []WireRow{}
	for hi := 0; hi < len(lines); hi++ {
		cols := headerColumns(lines[hi])
		if cols == nil {
			continue
		}
		sort.Slice(cols, func(i, j int) bool { return cols[i].x < cols[j].x })
		right := cols[len(cols)-1].x + 160
		titleTag := ""
		hasFrom := false
		for _, c := range cols {
			if c.role == roleFrom {
				hasFrom = true
			}
		}
		if !hasFrom {
			titleTag = tableTitleTag(lines, hi, cols[0].x, right)
		}
		pitch := 0.0
		lastY := lines[hi].Y
		misses := 0
		for i := hi + 1; i < len(lines); i++ {
			l := lines[i]
			gap := lastY - l.Y
			if pitch > 0 && gap > 3.2*pitch+2 {
				break
			}
			if pitch == 0 && gap > 40 {
				break
			}
			if headerColumns(l) != nil || tableBOMContext.MatchString(l.Text) {
				break
			}
			c := cellsFor(l, cols, right)
			pin := strings.TrimSpace(c[rolePin])
			if pin == "" || !pinValue.MatchString(pin) {
				misses++
				if misses >= 2 {
					break
				}
				continue
			}
			attrs := 0
			for _, r := range []pinRole{roleSignal, roleWire, roleColor, roleAWG, roleTo, roleToPin} {
				if c[r] != "" {
					attrs++
				}
			}
			if attrs == 0 {
				continue
			}
			misses = 0
			if pitch == 0 {
				pitch = gap
			} else {
				pitch = (pitch*2 + gap) / 3
			}
			lastY = l.Y
			conn := titleTag
			if hasFrom {
				conn = ""
				if t := normalizedTag(c[roleFrom]); reDestTag.MatchString(t) {
					conn = t
				}
			}
			if conn == "" {
				conn = "UNASSIGNED"
			}
			end, endPin := "", ""
			if c[roleTo] != "" {
				end, endPin = parseDestination(c[roleTo])
			}
			if c[roleToPin] != "" && pinValue.MatchString(c[roleToPin]) {
				endPin = strings.TrimSpace(c[roleToPin])
			}
			endSignal := ""
			if end == "" {
				endSignal = c[roleTo]
				end = "UNRESOLVED ENDPOINT"
			}
			conf := "DRAFT — HEADER-GROUNDED PINOUT TABLE"
			if conn == "UNASSIGNED" {
				conf = "UNASSIGNED CONNECTOR — PINOUT TABLE WITHOUT TITLE; CHECK SOURCE"
			}
			out = append(out, WireRow{Page: p.Number, Connector: conn, Cavity: pin, Signal: strings.TrimSpace(c[roleSignal]), End: end, EndCavity: endPin, EndSignal: endSignal, Confidence: conf, Y: l.Y, Wire: c[roleWire], Color: c[roleColor], AWG: c[roleAWG], Length: strings.TrimSpace(c[roleLength]), Method: "pinout-table"})
		}
	}
	return out
}

// mergeWireRows removes exact repeats of the same assignment (for example a
// pinout table and a wire list that both describe P1:5 -> J2:3). Repeats that
// agree become corroboration; disagreeing repeats are kept so the duplicate
// check reports them.
func mergeWireRows(rows []WireRow) []WireRow {
	type key struct{ c, cav string }
	groups := map[key][]int{}
	order := []key{}
	for i, r := range rows {
		k := key{r.Connector, r.Cavity}
		if r.Connector == "UNASSIGNED" {
			k.cav = k.cav + "#" + itoa(i)
		}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], i)
	}
	out := []WireRow{}
	for _, k := range order {
		idx := groups[k]
		if len(idx) == 1 {
			out = append(out, rows[idx[0]])
			continue
		}
		agree := true
		best := rows[idx[0]]
		for _, i := range idx[1:] {
			r := rows[i]
			if r.End != "UNRESOLVED ENDPOINT" && best.End != "UNRESOLVED ENDPOINT" && (r.End != best.End || (r.EndCavity != "" && best.EndCavity != "" && r.EndCavity != best.EndCavity)) {
				agree = false
				break
			}
			if best.End == "UNRESOLVED ENDPOINT" && r.End != "UNRESOLVED ENDPOINT" {
				best.End, best.EndCavity = r.End, r.EndCavity
			}
			if best.EndCavity == "" {
				best.EndCavity = r.EndCavity
			}
			if best.Signal == "" {
				best.Signal = r.Signal
			}
			if best.Wire == "" {
				best.Wire = r.Wire
			}
			if best.Color == "" {
				best.Color = r.Color
			}
			if best.AWG == "" {
				best.AWG = r.AWG
			}
		}
		if agree {
			best.Corroboration = len(idx)
			best.Confidence = best.Confidence + " · listed consistently in " + itoa(len(idx)) + " places"
			out = append(out, best)
			continue
		}
		for _, i := range idx {
			out = append(out, rows[i])
		}
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}
