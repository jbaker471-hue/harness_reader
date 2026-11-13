package main

import (
	"fmt"
	"strings"
	"testing"
)

// Four drawn wires from a named connector's pins to a block of pin numbers that carries no name anywhere.
func unnamedFarBlockPDF(namedLeft bool) []byte {
	text := func(x int, y float64, s string) string {
		return fmt.Sprintf("BT /F1 8 Tf 1 0 0 1 %d %.1f Tm (%s) Tj ET\n", x, y, s)
	}
	content := ""
	if namedLeft {
		content += text(140, 720, "TEST-J1")
	}
	for i := 0; i < 4; i++ {
		y := 650 - float64(i)*40
		content += text(150, y, fmt.Sprint(i+1))
		content += fmt.Sprintf("0.5 w 175 %.1f m 640 %.1f l S\n", y+3, y+3)
		content += text(380, y+6, "24 AWG")
		content += text(655, y, fmt.Sprint(i+1))
	}
	return onePagePDF(content, "[0 0 800 800]")
}

func TestWireToAnUnnamedBlockStillGivesTheNamedConnectorsPin(t *testing.T) {
	v, err := viewAnalysis(unnamedFarBlockPDF(true))
	if err != nil {
		t.Fatal(err)
	}
	pins := map[string]bool{}
	for _, w := range v.Wires {
		if w.Connector == "TEST-J1" {
			pins[w.Cavity] = true
			if w.End != "" || !strings.Contains(strings.ToLower(w.EndSignal), "no printed name") || w.Method != "traced-wire-unnamed-block" {
				t.Fatalf("a wire to an unnamed block is a one-ended row that says so: %+v", w)
			}
		}
	}
	if len(pins) != 4 || !pins["1"] || !pins["4"] {
		t.Fatalf("every pin of the named connector has a wire: %+v", v.Wires)
	}
}

func TestNoWireWhenNeitherEndHasANamedConnector(t *testing.T) {
	v, err := viewAnalysis(unnamedFarBlockPDF(false))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range v.Wires {
		if w.Method == "traced-wire-unnamed-block" {
			t.Fatalf("with no named connector at either end nothing attaches: %+v", w)
		}
	}
}

// A jumper: a drawn wire between two pins of one connector, both ends in the same column of the same block.
func jumperPDF() []byte {
	text := func(x int, y float64, s string) string {
		return fmt.Sprintf("BT /F1 8 Tf 1 0 0 1 %d %.1f Tm (%s) Tj ET\n", x, y, s)
	}
	content := text(140, 720, "TEST-J1")
	for i := 0; i < 4; i++ { // four ordinary wires to a named far connector so the sheet reads as a wiring sheet
		y := 650 - float64(i)*40
		content += text(150, y, fmt.Sprint(i+1))
		content += fmt.Sprintf("0.5 w 175 %.1f m 640 %.1f l S\n", y+3, y+3)
		content += text(380, y+6, "24 AWG")
		content += text(655, y, fmt.Sprint(i+1))
	}
	content += text(700, 720, "BENCH-P2")
	// the jumper: pins 7 and 8 of the first connector, joined by a wire that leaves the block and returns to it
	yA, yB := 480.0, 455.0
	content += text(150, yA, "7") + text(150, yB, "8")
	xr := 520.0
	content += fmt.Sprintf("0.5 w 175 %.1f m %.1f %.1f l %.1f %.1f l 175 %.1f l S\n", yA+3, xr, yA+3, xr, yB+3, yB+3)
	content += text(300, yA+6, "24 AWG")
	return onePagePDF(content, "[0 0 800 800]")
}

func TestJumperBetweenTwoPinsOfOneConnectorIsRead(t *testing.T) {
	v, err := viewAnalysis(jumperPDF())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, w := range v.Wires {
		if w.Method == "traced-wire-jumper" {
			found = true
			if w.Connector != "TEST-J1" || w.End != "TEST-J1" || w.Cavity == w.EndCavity || !strings.Contains(w.EndSignal, "jumper") {
				t.Fatalf("a jumper joins two different pins of one connector: %+v", w)
			}
		}
	}
	if !found {
		t.Logf("wires read: %d", len(v.Wires))
		for _, w := range v.Wires {
			t.Logf("  %s:%s > %s:%s (%s)", w.Connector, w.Cavity, w.End, w.EndCavity, w.Method)
		}
		t.Fatal("the jumper between pins 7 and 8 was not read")
	}
}

// Pin letters printed about two text heights above their line ends, in a column other wires already use.
func TestOffsetPinLabelInAPinColumnIsThatEndsPin(t *testing.T) {
	p := Page{Number: 3, Width: 1200, Height: 1000, PageAccurate: true}
	add := func(s string, x, y float64) {
		p.Words = append(p.Words, Word{Text: s, X: x, Y: y, Size: 6, X1: x + 3.6*float64(len(s))})
	}
	add("NEAR-P1", 120, 960)
	add("FAR-J2", 890, 960)
	for i := 0; i < 6; i++ {
		y := 920 - 30*float64(i)
		add(fmt.Sprint(i+1), 150, y)
		add("20", 400, y)
		add("AWG", 412, y)
		p.Segs = append(p.Segs, Segment{160, y - 2, 900, y - 2})
		off := 0.0
		if i >= 3 {
			off = 11 // printed about 1.8 em above the line: beyond the first look
		}
		add(string(rune('A'+i)), 906, y+off)
	}
	rows, _ := tracedWireRows(p, nil, nil)
	got := []string{}
	for _, r := range rows {
		got = append(got, r.Connector+":"+r.Cavity+">"+r.End+":"+r.EndCavity)
	}
	want := "NEAR-P1:1>FAR-J2:A NEAR-P1:2>FAR-J2:B NEAR-P1:3>FAR-J2:C NEAR-P1:4>FAR-J2:D NEAR-P1:5>FAR-J2:E NEAR-P1:6>FAR-J2:F"
	if strings.Join(got, " ") != want {
		t.Fatalf("got  %v\nwant %s", got, want)
	}
	// a label that two bare ends could both claim is given to neither
	q := Page{Number: 3, Width: 1200, Height: 1000, PageAccurate: true}
	addq := func(s string, x, y float64) {
		q.Words = append(q.Words, Word{Text: s, X: x, Y: y, Size: 6, X1: x + 3.6*float64(len(s))})
	}
	addq("NEAR-P1", 120, 960)
	addq("FAR-J2", 890, 960)
	for i := 0; i < 5; i++ {
		y := 920 - 30*float64(i)
		if i >= 3 {
			y = 920 - 30*3 - 10*float64(i-3) // two lines 10 apart sharing one label between them
		}
		addq(fmt.Sprint(i+1), 150, y)
		addq("20", 400, y)
		addq("AWG", 412, y)
		q.Segs = append(q.Segs, Segment{160, y - 2, 900, y - 2})
		if i < 3 {
			addq(string(rune('A'+i)), 906, y)
		}
	}
	addq("D", 906, 920-30*3+9)
	rows, _ = tracedWireRows(q, nil, nil)
	for _, r := range rows {
		if r.EndCavity == "D" && r.Cavity == "5" {
			t.Fatalf("a label two ends could claim must not be assigned by guess: %+v", r)
		}
	}
}

// A wire whose line is broken by a symbol part-way and carries on at the same height to its pin.
func TestLineBrokenBySymbolStillReachesItsPin(t *testing.T) {
	build := func(gapFrom, gapTo float64, labelInGap bool) []WireRow {
		p := Page{Number: 3, Width: 1400, Height: 1000, PageAccurate: true}
		add := func(s string, x, y float64) {
			p.Words = append(p.Words, Word{Text: s, X: x, Y: y, Size: 6, X1: x + 3.6*float64(len(s))})
		}
		add("NEAR-P1", 120, 960)
		add("FAR-J2", 1090, 960)
		for i := 0; i < 5; i++ {
			y := 920 - 30*float64(i)
			add(fmt.Sprint(i+1), 150, y)
			add("20", 400, y)
			add("AWG", 412, y)
			add(string(rune('A'+i)), 1106, y)
			if i < 3 {
				p.Segs = append(p.Segs, Segment{160, y - 2, 1100, y - 2})
				continue
			}
			// broken line: drawn up to the gap, then on to the pin; two slanted strokes mark the break
			p.Segs = append(p.Segs, Segment{160, y - 2, gapFrom, y - 2}, Segment{gapTo, y - 2, 1100, y - 2},
				Segment{gapFrom, y - 2, gapFrom + 4, y + 3}, Segment{gapFrom, y - 2, gapFrom + 4, y - 7})
			if labelInGap {
				add("22", gapFrom+40, y)
				add("AWG", gapFrom+52, y)
			}
		}
		rows, _ := tracedWireRows(p, nil, nil)
		return rows
	}
	ends := func(rows []WireRow) string {
		got := []string{}
		for _, r := range rows {
			got = append(got, r.Cavity+">"+r.EndCavity)
		}
		return strings.Join(got, " ")
	}
	if got := ends(build(800, 830, false)); got != "1>A 2>B 3>C 4>D 5>E" {
		t.Fatalf("a short break must not stop the wire: %s", got)
	}
	// most of the way is not drawn: not the same wire
	if got := ends(build(500, 1000, false)); strings.Contains(got, "4>D") || strings.Contains(got, "5>E") {
		t.Fatalf("a long undrawn stretch must not be bridged: %s", got)
	}
}
