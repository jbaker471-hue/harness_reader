package main

import (
	"fmt"
	"strings"
	"testing"
)

// A NON-CUI wiring-sheet layout: one connector on the left fans out to three
// connector blocks stacked down the right side. The right-side pin numbers are
// right-aligned (different widths end at the same edge) and no colour words are
// printed. Every wire must be read, each pin under its own block's header.
func fanoutPage(hideMiddleHeader bool) Page {
	p := Page{Number: 7, Width: 1200, Height: 1100, PageAccurate: true}
	add := func(t string, x, y float64) {
		p.Words = append(p.Words, Word{Text: t, X: x, Y: y, Size: 6, X1: x + 3.6*float64(len(t))})
	}
	right := func(t string, edge, y float64) { // right-aligned at edge
		p.Words = append(p.Words, Word{Text: t, X: edge - 3.6*float64(len(t)), Y: y, Size: 6, X1: edge})
	}
	add("SRC-P1", 120, 1010)
	type blk struct {
		tag  string
		head float64
		pins []string
		ys   []float64
	}
	blocks := []blk{
		{"DST-P1", 1010, []string{"1", "12", "112"}, []float64{980, 965, 950}},
		{"DST-P2", 925, []string{"7", "105"}, []float64{905, 890}},
		{"DST-P3", 865, []string{"3", "48", "9", "120"}, []float64{845, 830, 815, 800}},
	}
	left := 1
	for _, b := range blocks {
		if !(hideMiddleHeader && b.tag == "DST-P2") {
			add(b.tag, 830, b.head)
		}
		for i, pin := range b.pins {
			y := b.ys[i]
			add(fmt.Sprint(left), 150, y)
			add("24", 500, y)
			add("AWG", 512, y)
			right(pin, 840, y)
			left++
		}
	}
	return p
}

func TestFanOutWiringSheetIsReadByBlock(t *testing.T) {
	rows, why := positionalSchematicRows(fanoutPage(false), nil, nil)
	if why != "" || len(rows) != 9 {
		t.Fatalf("want 9 wires and no reason, got %d (%q)", len(rows), why)
	}
	got := []string{}
	for _, r := range rows {
		got = append(got, r.Connector+":"+r.Cavity+">"+r.End+":"+r.EndCavity)
	}
	want := "SRC-P1:1>DST-P1:1 SRC-P1:2>DST-P1:12 SRC-P1:3>DST-P1:112 SRC-P1:4>DST-P2:7 SRC-P1:5>DST-P2:105 SRC-P1:6>DST-P3:3 SRC-P1:7>DST-P3:48 SRC-P1:8>DST-P3:9 SRC-P1:9>DST-P3:120"
	if strings.Join(got, " ") != want {
		t.Fatalf("got  %s\nwant %s", strings.Join(got, " "), want)
	}
}

func TestFanOutBlockWithoutHeaderIsNotGivenTheBlockAbove(t *testing.T) {
	rows, why := positionalSchematicRows(fanoutPage(true), nil, nil)
	for _, r := range rows {
		if r.EndCavity == "7" || r.EndCavity == "105" {
			t.Fatalf("a block with no header must not borrow the header above it: %+v", r)
		}
	}
	// The middle block's own header is missing and nothing else explains the
	// gap, so the lines below it are held (not guessed) and explained.
	if len(rows) != 3 || !strings.Contains(why, "6 label(s) have a pin whose connector block header was not matched") {
		t.Fatalf("want the 3 wires above the gap and the other 6 labels explained, got %d (%q)", len(rows), why)
	}
}

// A main connector wired to a smaller connector, which is wired on to a third:
// each printed line carries two gauge labels, one per wire, and the middle
// connector has a pin column on each side of its block.
func TestChainedConnectorsAreReadPerGaugeLabel(t *testing.T) {
	p := Page{Number: 8, Width: 1400, Height: 1100, PageAccurate: true}
	add := func(t string, x, y float64) {
		p.Words = append(p.Words, Word{Text: t, X: x, Y: y, Size: 6, X1: x + 3.6*float64(len(t))})
	}
	add("MAIN-P1", 100, 1010)
	add("MID-J2", 600, 1010)
	add("END-P3", 1150, 1010)
	for i, y := range []float64{980, 965, 950, 935} {
		add(fmt.Sprint(10+i), 150, y) // main connector pin
		add("22", 350, y)
		add("AWG", 362, y)
		add(fmt.Sprint(i+1), 590, y) // middle connector, wires coming in
		add(fmt.Sprint(i+5), 660, y) // middle connector, wires going on
		add("24", 900, y)
		add("AWG", 912, y)
		add(fmt.Sprint(20+i), 1160, y) // end connector
	}
	rows, why := positionalSchematicRows(p, nil, nil)
	got := []string{}
	for _, r := range rows {
		got = append(got, r.Connector+":"+r.Cavity+">"+r.End+":"+r.EndCavity+"/"+r.AWG)
	}
	want := "MAIN-P1:10>MID-J2:1/22 MID-J2:5>END-P3:20/24 MAIN-P1:11>MID-J2:2/22 MID-J2:6>END-P3:21/24 MAIN-P1:12>MID-J2:3/22 MID-J2:7>END-P3:22/24 MAIN-P1:13>MID-J2:4/22 MID-J2:8>END-P3:23/24"
	if why != "" || strings.Join(got, " ") != want {
		t.Fatalf("got  %s (%q)\nwant %s", strings.Join(got, " "), why, want)
	}
}
