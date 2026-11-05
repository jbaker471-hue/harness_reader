package main

import (
	"sort"
	"math"
	"fmt"
	"strings"
	"testing"
)

// NON-CUI layouts copied from the structure report of real drawings:
//   - the far-end pin is NOT on the same text line as the near end: the wire
//     line jogs down before reaching the far block;
//   - the far block uses contact letters (A, B, ... AA);
//   - border zone letters sit on the same text lines at the page edges;
//   - the far block is named in a non-standard style (ACU1), the near block
//     with a standard designator.
func tracedPage(nameFar string) Page {
	p := Page{Number: 3, Width: 1200, Height: 1000, PageAccurate: true}
	add := func(t string, x, y float64) {
		p.Words = append(p.Words, Word{Text: t, X: x, Y: y, Size: 6, X1: x + 3.6*float64(len(t))})
	}
	seg := func(x0, y0, x1, y1 float64) { p.Segs = append(p.Segs, Segment{x0, y0, x1, y1}) }
	add("NEAR-P1", 120, 960)
	add("D38999/", 120, 950)
	add("26WJ35PN", 120, 942)
	if nameFar != "" {
		add(nameFar, 860, 960)
	}
	// block outlines
	seg(160, 930, 160, 700)
	seg(860, 930, 860, 600)
	far := []string{"A", "B", "C", "D", "E", "F", "AA", "BB"}
	for i, pin := range far {
		y := 920 - 20*float64(i)
		fy := y - 9 - 4*float64(i) // the far end lands lower: the line jogs
		add(fmt.Sprint(i+1), 150, y)
		add("WHT", 165, y)
		add("24", 400, y)
		add("AWG", 412, y)
		add("Z", 5, y) // border zone letter on the same text line
		add("Y", 1190, y)
		seg(160, y-2, 600, y-2)   // wire leaves the near block under its label
		seg(600, y-2, 600, fy-2)  // jog
		seg(600, fy-2, 860, fy-2) // and reaches the far block
		add("BLK", 830, fy)
		add(pin, 866, fy) // far pin beside the far block, on another baseline
	}
	return p
}

func TestTracedWiresFollowTheDrawnLine(t *testing.T) {
	rows, why := tracedWireRows(tracedPage("ACU1"), nil, nil)
	got := []string{}
	for _, r := range rows {
		got = append(got, r.Connector+":"+r.Cavity+">"+r.End+":"+r.EndCavity)
	}
	want := "NEAR-P1:1>ACU1:A NEAR-P1:2>ACU1:B NEAR-P1:3>ACU1:C NEAR-P1:4>ACU1:D NEAR-P1:5>ACU1:E NEAR-P1:6>ACU1:F NEAR-P1:7>ACU1:AA NEAR-P1:8>ACU1:BB"
	if why != "" || strings.Join(got, " ") != want {
		t.Fatalf("got  %s (%q)\nwant %s", strings.Join(got, " "), why, want)
	}
	for _, r := range rows {
		if r.Cavity == "Z" || r.EndCavity == "Y" || r.AWG != "24" {
			t.Fatalf("zone letters must never be pins, gauge must be kept: %+v", r)
		}
	}
}

func TestTracedWiresHoldUnnamedBlock(t *testing.T) {
	rows, why := tracedWireRows(tracedPage(""), nil, nil)
	if len(rows) != 0 || !strings.Contains(why, "8 label(s): an end whose connector block could not be named") {
		t.Fatalf("an unnamed far block must hold its wires and say why: %d %q", len(rows), why)
	}
}

func TestTracedWiresSkipWhatWasRead(t *testing.T) {
	p := tracedPage("ACU1")
	read := []WireRow{{Connector: "NEAR-P1", Cavity: "1", End: "ACU1", EndCavity: "A"}}
	rows, _ := tracedWireRows(p, read, nil)
	if len(rows) != 7 {
		t.Fatalf("a wire already read must not be added twice: %d", len(rows))
	}
}

// A wire line broken for text (a short gap) is followed across the gap.
func TestTracedWiresJumpShortGaps(t *testing.T) {
	p := tracedPage("ACU1")
	var segs []Segment
	for _, s := range p.Segs {
		if s[0] == 600 && s[2] == 860 { // break the last run of each wire
			segs = append(segs, Segment{600, s[1], 700, s[3]}, Segment{712, s[1], 860, s[3]})
			continue
		}
		segs = append(segs, s)
	}
	p.Segs = segs
	rows, why := tracedWireRows(p, nil, nil)
	if len(rows) != 8 || why != "" {
		t.Fatalf("broken lines must be followed across short gaps: %d %q", len(rows), why)
	}
}

// Wires drawn as stacked zig-zags touch their neighbours at every bend; each
// wire must still be followed along its own line to its own far pin.
func TestTracedZigzagWiresTouchingNeighbours(t *testing.T) {
	p := Page{Number: 4, Width: 1200, Height: 1000, PageAccurate: true}
	add := func(t string, x, y float64) {
		p.Words = append(p.Words, Word{Text: t, X: x, Y: y, Size: 6, X1: x + 3.6*float64(len(t))})
	}
	seg := func(x0, y0, x1, y1 float64) { p.Segs = append(p.Segs, Segment{x0, y0, x1, y1}) }
	add("NEAR-P1", 120, 960)
	add("FAR-P2", 860, 960)
	const pitch = 10.0 // zig-zag amplitude = half the pitch, so neighbours touch
	far := []string{"7", "3", "12", "5", "9", "1"}
	for i := 0; i < 6; i++ {
		y := 920 - pitch*float64(i)
		add(fmt.Sprint(i+1), 150, y+2)
		add("24", 400, y+2)
		add("AWG", 412, y+2)
		seg(160, y, 600, y)
		// zig-zag from x=600 to 720; even wires bend up, odd wires bend down, so
		// each even wire's bends land exactly on the bends of the wire above it
		dir := 1.0
		if i%2 == 1 {
			dir = -1
		}
		xs := []float64{600, 620, 640, 660, 680, 700, 720}
		for k := 0; k+1 < len(xs); k++ {
			ya, yb := y, y+dir*pitch/2
			if k%2 == 1 {
				ya, yb = yb, ya
			}
			seg(xs[k], ya, xs[k+1], yb)
		}
		seg(720, y, 860, y)
		add(far[i], 866, y+2)
	}
	// CAD exports draw lines twice: duplicate every segment
	p.Segs = append(p.Segs, p.Segs...)
	// prove the neighbours really touch
	g := newSegGraph(p.Segs)
	if len(g.at[snapKey(620, 905)]) < 4 {
		t.Fatalf("fixture: wires 2 and 3 must share a bend point, got %d segments there", len(g.at[snapKey(620, 905)]))
	}
	rows, why := tracedWireRows(p, nil, nil)
	got := []string{}
	for _, r := range rows {
		got = append(got, r.Cavity+">"+r.EndCavity)
	}
	if strings.Join(got, " ") != "1>7 2>3 3>12 4>5 5>9 6>1" {
		t.Fatalf("zig-zag wires: got %v (%q)", got, why)
	}
}

// Where the far block's edge is drawn as pieces split at every wire, the wire
// must stop at its pin, not turn and run along the block edge.
func TestTracedWiresStopAtThePinNotAlongTheBlockEdge(t *testing.T) {
	p := tracedPage("ACU1")
	var segs []Segment
	for _, s := range p.Segs {
		if s[0] == 860 && s[2] == 860 { // replace the far block edge by pieces between wire ends
			continue
		}
		segs = append(segs, s)
	}
	ys := []float64{930}
	for i := 0; i < 8; i++ {
		y := 920 - 20*float64(i)
		ys = append(ys, y-9-4*float64(i)-2)
	}
	ys = append(ys, 600)
	for k := 0; k+1 < len(ys); k++ {
		segs = append(segs, Segment{860, ys[k], 860, ys[k+1]})
	}
	p.Segs = segs
	rows, why := tracedWireRows(p, nil, nil)
	got := []string{}
	for _, r := range rows {
		got = append(got, r.Cavity+">"+r.EndCavity)
	}
	if strings.Join(got, " ") != "1>A 2>B 3>C 4>D 5>E 6>F 7>AA 8>BB" {
		t.Fatalf("got %v (%q)", got, why)
	}
}

// A wire that ends at a printed connector-pin reference: the reference is the
// far end. Something that is not designator-pin (a wire spec) is not.
func TestTracedWireEndsAtPrintedReference(t *testing.T) {
	p := Page{Number: 4, Width: 1200, Height: 1000, PageAccurate: true}
	add := func(t string, x, y float64) {
		p.Words = append(p.Words, Word{Text: t, X: x, Y: y, Size: 6, X1: x + 3.6*float64(len(t))})
	}
	add("NEAR-P1", 120, 960)
	refs := []string{"DST-P9-12", "DST-P9-4", "AUX-J3-A", "M22759-16"}
	for i, ref := range refs {
		y := 920 - 20*float64(i)
		add(fmt.Sprint(i+1), 150, y)
		add("24", 400, y)
		add("AWG", 412, y)
		p.Segs = append(p.Segs, Segment{160, y - 2, 900, y - 2})
		add(ref, 940, y) // printed about 7 em past the end of the line
		add("10-LETTER", 900, y+30)
	}
	rows, why := tracedWireRows(p, nil, nil)
	got := []string{}
	for _, r := range rows {
		got = append(got, r.Connector+":"+r.Cavity+">"+r.End+":"+r.EndCavity)
	}
	if strings.Join(got, " ") != "NEAR-P1:1>DST-P9:12 NEAR-P1:2>DST-P9:4 NEAR-P1:3>AUX-J3:A" || !strings.Contains(why, "1 label(s): no pin label at one end") {
		t.Fatalf("got %v (%q)", got, why)
	}
}

// NON-CUI copy of a ferrule layout: wires leave a connector and end at a crimp ferrule
// (drawn as a small square) with a label beside it, with or without a device
// reference in parentheses. Each wires its own connector pin; the far end is
// a lead, not a connector. A label with no box is not a lead.
func TestTracedWireEndingAtLabelledLeadWiresItsOwnPin(t *testing.T) {
	p := Page{Number: 4, Width: 1200, Height: 1000, PageAccurate: true}
	add := func(t string, x, y float64) {
		p.Words = append(p.Words, Word{Text: t, X: x, Y: y, Size: 6, X1: x + 3.6*float64(len(t))})
	}
	add("NEAR-P1", 120, 960)
	labels := []string{"(DEV-27)", "(DEV-6-DEV-10)", "FAN", "NOBOX"}
	for i, lbl := range labels {
		y := 920 - 20*float64(i)
		add(fmt.Sprint(i+5), 150, y)
		add("24", 400, y)
		add("AWG", 412, y)
		p.Segs = append(p.Segs, Segment{160, y - 2, 900, y - 2})
		if lbl != "NOBOX" { // the terminal box: a 9 pt square at the end of the line
			p.Segs = append(p.Segs, Segment{900, y - 6.5, 909, y - 6.5}, Segment{900, y + 2.5, 909, y + 2.5}, Segment{900, y - 6.5, 900, y + 2.5}, Segment{909, y - 6.5, 909, y + 2.5})
		}
		add(lbl, 930, y)
		add("SIGNAL_NAME", 990, y)
	}
	rows, why := tracedWireRows(p, nil, nil)
	got := []string{}
	for _, r := range rows {
		got = append(got, r.Connector+":"+r.Cavity+"/"+r.Method+"/"+r.EndSignal)
	}
	// under the shop's rule a line numbered at one end only wires that pin;
	// its other end (a label with no ferrule box) is not a connector pin
	want := "NEAR-P1:5/traced-wire-to-ferrule/crimp ferrule: (DEV-27) NEAR-P1:6/traced-wire-to-ferrule/crimp ferrule: (DEV-6-DEV-10) NEAR-P1:7/traced-wire-to-ferrule/crimp ferrule: FAN NEAR-P1:8/traced-wire-one-end/no pin number at this end (not a connector pin)"
	if strings.Join(got, " ") != want || why != "" {
		t.Fatalf("got %v (%q)", got, why)
	}
}

// A line already read by another reader (both ends) is not read again as a
// one-ended wire on its numbered pin.
func TestOneEndedWireNeverDuplicatesARead(t *testing.T) {
	p := tracedPage("") // no far connector header: the far ends are not on a connector
	// make every far end unnumbered: the near pins would be read as one-ended
	var words []Word
	for _, w := range p.Words {
		if w.X == 866 {
			continue
		}
		words = append(words, w)
	}
	p.Words = words
	read := []WireRow{{Connector: "NEAR-P1", Cavity: "1", End: "OTHER-P2", EndCavity: "9"}}
	rows, _ := tracedWireRows(p, read, nil)
	for _, r := range rows {
		if r.Connector == "NEAR-P1" && r.Cavity == "1" {
			t.Fatalf("pin already carrying a wire must not get a second, one-ended wire: %+v", r)
		}
	}
	if len(rows) != 7 {
		t.Fatalf("want the other 7 wires as one-ended, got %d", len(rows))
	}
}

// An unnumbered end that lands under a connector's header (here a connector
// with a single wire) is a pin whose number was not read: the line is held,
// not read as a one-ended wire.
func TestBareEndUnderAConnectorHeaderIsHeld(t *testing.T) {
	p := Page{Number: 3, Width: 1200, Height: 1000, PageAccurate: true}
	add := func(t string, x, y float64) {
		p.Words = append(p.Words, Word{Text: t, X: x, Y: y, Size: 6, X1: x + 3.6*float64(len(t))})
	}
	add("NEAR-P1", 120, 960)
	add("SMALL-J7", 880, 960) // the far connector's header, above where the line ends
	for i := 0; i < 3; i++ {
		y := 920 - 20*float64(i)
		add(fmt.Sprint(i+1), 150, y)
		add("24", 400, y)
		add("AWG", 412, y)
		p.Segs = append(p.Segs, Segment{160, y - 2, 900, y - 2})
	}
	rows, _ := tracedWireRows(p, nil, nil)
	if len(rows) != 0 {
		t.Fatalf("ends under a connector header with no number must be held, got %+v", rows)
	}
}

// NON-CUI copy of the field layout: pins in a column beside a connector block,
// two cable shields terminated to pins ("SHIELD 9"), and a stub line labelled
// with a pin list and "NC" across the block for the no-connect pins.
func TestNoConnectListAndShieldPins(t *testing.T) {
	p := Page{Number: 3, Width: 1200, Height: 1000, PageAccurate: true}
	add := func(t string, x, y float64) {
		p.Words = append(p.Words, Word{Text: t, X: x, Y: y, Size: 6, X1: x + 3.6*float64(len(t))})
	}
	add("NEAR-P1", 120, 960)
	add("FAR-J2", 830, 960)
	p.Segs = append(p.Segs, Segment{860, 950, 860, 600}, Segment{890, 950, 890, 600}) // the block
	pins := []string{"8", "7", "3", "4"}
	for i, pin := range pins {
		y := 920 - 20*float64(i)
		add(fmt.Sprint(i+1), 150, y)
		add("24", 400, y)
		add("AWG", 412, y)
		p.Segs = append(p.Segs, Segment{160, y - 2, 860, y - 2})
		add("WHT", 830, y)
		add(pin, 850, y)
	}
	add("SHIELD", 822, 840) // shield terminated to pin 9
	add("9", 850, 840)
	add("SHIELD", 822, 780) // shield terminated to pin 15
	add("15", 846, 780)
	add("5,6,16-19", 820, 820) // the NC stub and its list
	add("NC", 900, 818)
	rows, _, blocks := traceSheet(p, nil, nil)
	rows = append(rows, shieldPinRows(p, blocks, rows)...)
	used := map[string]bool{}
	for _, r := range rows {
		if r.Connector == "FAR-J2" {
			used[r.Cavity] = true
		}
		if r.End == "FAR-J2" {
			used[r.EndCavity] = true
		}
	}
	for _, pin := range []string{"8", "7", "3", "4", "9", "15"} {
		if !used[pin] {
			t.Fatalf("pin %s must be used (shield pins included): %v", pin, used)
		}
	}
	audits := pinNumberAudits(p, blocks, rows)
	ok := false
	for _, a := range audits {
		if a.Title == "pin numbers FAR-J2" {
			ok = a.Status == "COMPLETE" && strings.Contains(a.Detail, "6 pin(s) are stated NC")
			if !ok {
				t.Fatalf("FAR-J2 check: %+v", a)
			}
		}
	}
	if !ok {
		t.Fatalf("no pin check for FAR-J2: %+v", audits)
	}
	// a wire to a pin the NC list names is a conflict
	rows = append(rows, WireRow{Connector: "FAR-J2", Cavity: "17", End: "X", EndCavity: "1"})
	for _, a := range pinNumberAudits(p, blocks, rows) {
		if a.Title == "pin numbers FAR-J2" && a.Status != "INCOMPLETE" {
			t.Fatalf("a wire on an NC pin must be flagged: %+v", a)
		}
	}
	// a shield pin above another connector's header is not that connector's
	r := p
	r.Words = append(append([]Word{}, p.Words...), Word{Text: "BELOW-P5", X: 830, Y: 700, Size: 6, X1: 860}, Word{Text: "WHT", X: 830, Y: 680, Size: 6, X1: 841}, Word{Text: "1", X: 850, Y: 680, Size: 6, X1: 854})
	r.Words = append(r.Words, Word{Text: "1", X: 150, Y: 680, Size: 6, X1: 154}, Word{Text: "24", X: 400, Y: 680, Size: 6, X1: 407}, Word{Text: "AWG", X: 412, Y: 680, Size: 6, X1: 423})
	r.Segs = append(append([]Segment{}, p.Segs...), Segment{160, 678, 860, 678})
	rows3, _, blocks3 := traceSheet(r, nil, nil)
	for _, row := range shieldPinRows(r, blocks3, rows3) {
		if row.Connector == "BELOW-P5" {
			t.Fatalf("a shield pin above another connector's header must not be given to it: %+v", row)
		}
	}
	// a list below another connector's header belongs to that connector
	q := p
	q.Words = append(append([]Word{}, p.Words...), Word{Text: "LOWER-P8", X: 830, Y: 700, Size: 6, X1: 860}, Word{Text: "3-9", X: 840, Y: 660, Size: 6, X1: 851}, Word{Text: "NC", X: 900, Y: 659, Size: 6, X1: 907})
	rows2, _, blocks2 := traceSheet(q, nil, nil)
	rows2 = append(rows2, shieldPinRows(q, blocks2, rows2)...)
	for _, a := range pinNumberAudits(q, blocks2, rows2) {
		if a.Title == "pin numbers FAR-J2" && a.Status != "COMPLETE" {
			t.Fatalf("an NC list under another connector's header must not apply here: %+v", a)
		}
	}
	if got := strings.Join(expandPinList("5,6,16-19"), " "); got != "5 6 16 17 18 19" {
		t.Fatalf("expandPinList: %s", got)
	}
}

// The gauge number of "24 AWG" is never read as a pin, even where a line
// ends right at its label.
func TestGaugeNumberIsNeverAPin(t *testing.T) {
	p := Page{Number: 4, Width: 1200, Height: 1000, PageAccurate: true}
	add := func(t string, x, y float64) {
		p.Words = append(p.Words, Word{Text: t, X: x, Y: y, Size: 6, X1: x + 3.6*float64(len(t))})
	}
	add("NEAR-P1", 120, 960)
	add("MID-J2", 395, 960)
	for i := 0; i < 4; i++ {
		y := 920 - 20*float64(i)
		add(fmt.Sprint(i+1), 150, y)
		add("24", 400, y)
		add("AWG", 412, y)
		p.Segs = append(p.Segs, Segment{160, y - 2, 402, y - 2}) // the line stops under its gauge label
	}
	rows, _ := tracedWireRows(p, nil, nil)
	for _, r := range rows {
		if r.EndCavity == "24" || r.Cavity == "24" {
			t.Fatalf("a gauge number was read as a pin: %+v", r)
		}
	}
}

// ringChords draws a circle of radius r centred at (cx, cy) the way curves
// reach the reader: as short straight chords.
func ringChords(cx, cy, r float64) []Segment {
	var out []Segment
	n := 12
	for i := 0; i < n; i++ {
		a0, a1 := 2*math.Pi*float64(i)/float64(n), 2*math.Pi*float64(i+1)/float64(n)
		out = append(out, Segment{cx + r*math.Cos(a0), cy + r*math.Sin(a0), cx + r*math.Cos(a1), cy + r*math.Sin(a1)})
	}
	return out
}

// NON-CUI: wires from a named connector end on a block with no name whose
// ends carry letters. An end drawn as a small circle is a ring terminal, one
// drawn as a small square is a crimp ferrule; each wires the named
// connector's pin. An end with neither stays held.
func TestUnnamedEndsDrawnAsTerminationsWireTheNamedPin(t *testing.T) {
	p := Page{Number: 5, Width: 1200, Height: 1000, PageAccurate: true}
	add := func(t string, x, y float64) {
		p.Words = append(p.Words, Word{Text: t, X: x, Y: y, Size: 6, X1: x + 3.6*float64(len(t))})
	}
	add("NEAR-P1", 120, 960)
	for i, kind := range []string{"ring", "box", "none"} {
		y := 920 - 20*float64(i)
		add(fmt.Sprint(i+1), 150, y)
		add("20", 400, y)
		add("AWG", 412, y)
		p.Segs = append(p.Segs, Segment{160, y - 2, 900, y - 2})
		switch kind {
		case "ring":
			p.Segs = append(p.Segs, ringChords(904.5, y-2, 4.5)...)
		case "box":
			p.Segs = append(p.Segs, Segment{900, y - 6.5, 909, y - 6.5}, Segment{900, y + 2.5, 909, y + 2.5}, Segment{900, y - 6.5, 900, y + 2.5}, Segment{909, y - 6.5, 909, y + 2.5})
		}
		add(string(rune('A'+i)), 912, y)
	}
	rows, _ := tracedWireRows(p, nil, nil)
	got := []string{}
	for _, r := range rows {
		got = append(got, r.Connector+":"+r.Cavity+"/"+r.Method+"/"+r.EndSignal)
	}
	want := "NEAR-P1:1/traced-wire-to-ring/ring terminal: A NEAR-P1:2/traced-wire-to-ferrule/crimp ferrule: B"
	if strings.Join(got, " ") != want {
		t.Fatalf("got  %v\nwant %s", got, want)
	}
}

// NON-CUI: a wire ending at a small circle with a label and no pin number is
// a ring terminal on its numbered pin.
func TestLabelledRingEndIsARingTerminal(t *testing.T) {
	p := Page{Number: 6, Width: 1200, Height: 1000, PageAccurate: true}
	add := func(t string, x, y float64) {
		p.Words = append(p.Words, Word{Text: t, X: x, Y: y, Size: 6, X1: x + 3.6*float64(len(t))})
	}
	add("NEAR-P1", 120, 960)
	for i, lbl := range []string{"CHASSIS_GND", "GND_2", "GND_3"} {
		y := 920 - 20*float64(i)
		add(fmt.Sprint(i+4), 150, y)
		add("20", 400, y)
		add("AWG", 412, y)
		p.Segs = append(p.Segs, Segment{160, y - 2, 900, y - 2})
		p.Segs = append(p.Segs, ringChords(904.5, y-2, 4.5)...)
		add(lbl, 930, y)
	}
	rows, why := tracedWireRows(p, nil, nil)
	if len(rows) != 3 || rows[0].Method != "traced-wire-to-ring" || rows[0].EndSignal != "ring terminal: CHASSIS_GND" || rows[0].Cavity != "4" {
		t.Fatalf("got %+v (%q)", rows, why)
	}
}

// NON-CUI: a stack of ring terminals drawn a few em past the line ends, with
// no labels, beside a numbered connector end in the same column. Each ring
// end is a termination on its numbered pin, not an unread pin number.
func TestRingTerminalsInAColumnAreKept(t *testing.T) {
	p := Page{Number: 7, Width: 1200, Height: 1000, PageAccurate: true}
	add := func(t string, x, y float64) {
		p.Words = append(p.Words, Word{Text: t, X: x, Y: y, Size: 6, X1: x + 3.6*float64(len(t))})
	}
	add("NEAR-P1", 120, 960)
	add("FAR-J2", 905, 960)
	for i := 0; i < 4; i++ {
		y := 920 - 24*float64(i)
		add(fmt.Sprint(i+1), 150, y)
		add("20", 400, y)
		add("AWG", 412, y)
		p.Segs = append(p.Segs, Segment{160, y - 2, 900, y - 2})
		if i == 3 {
			add("7", 905, y) // a real pin on the far connector
			continue
		}
		p.Segs = append(p.Segs, ringChords(930, y-2, 6)...)
	}
	rows, why := tracedWireRows(p, nil, nil)
	got := []string{}
	for _, r := range rows {
		got = append(got, r.Connector+":"+r.Cavity+"/"+r.Method+"/"+r.End+":"+r.EndCavity)
	}
	want := "NEAR-P1:1/traced-wire-to-ring/: NEAR-P1:2/traced-wire-to-ring/: NEAR-P1:3/traced-wire-to-ring/: NEAR-P1:4/traced-wire/FAR-J2:7"
	sort.Strings(got)
	if strings.Join(got, " ") != want {
		t.Fatalf("got  %v (%q)\nwant %s", got, why, want)
	}
}

// NON-CUI: lines numbered at one end only, whose other ends sit in the same
// column as numbered ends of other wires but with nothing printed at them,
// are one-ended wires: there is no unread number to wait for.
func TestUnprintedEndsInAPinColumnAreOneEnded(t *testing.T) {
	p := Page{Number: 8, Width: 1200, Height: 1000, PageAccurate: true}
	add := func(t string, x, y float64) {
		p.Words = append(p.Words, Word{Text: t, X: x, Y: y, Size: 6, X1: x + 3.6*float64(len(t))})
	}
	add("NEAR-P1", 120, 960)
	add("FAR-J2", 905, 960)
	for i := 0; i < 4; i++ {
		y := 920 - 24*float64(i)
		add(fmt.Sprint(i+1), 150, y)
		add("20", 400, y)
		add("AWG", 412, y)
		p.Segs = append(p.Segs, Segment{160, y - 2, 900, y - 2})
		if i == 3 {
			add("7", 905, y)
		}
	}
	rows, _ := tracedWireRows(p, nil, nil)
	got := []string{}
	for _, r := range rows {
		got = append(got, r.Cavity+"/"+r.Method)
	}
	sort.Strings(got)
	// the three unprinted ends lie under FAR-J2's header, so they are held as
	// its pins with unread numbers; only the numbered wire is read
	if strings.Join(got, " ") != "4/traced-wire" {
		t.Fatalf("got %v", got)
	}
}
