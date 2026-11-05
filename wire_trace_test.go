package main

import (
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
	want := "NEAR-P1:5/traced-wire-to-ferrule/crimp ferrule: (DEV-27) NEAR-P1:6/traced-wire-to-ferrule/crimp ferrule: (DEV-6-DEV-10) NEAR-P1:7/traced-wire-to-ferrule/crimp ferrule: FAN"
	if strings.Join(got, " ") != want || !strings.Contains(why, "1 label(s): no pin label at one end") {
		t.Fatalf("got %v (%q)", got, why)
	}
}
