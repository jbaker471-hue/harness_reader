package main

import "testing"

// A wiring sheet that prints only the tail of each connector's part number
// (no designator, no "D38999/") names the block from the one part on the
// drawing that ends with that tail; a tail shared by two parts holds.
func TestColumnNamedFromPartTailOnly(t *testing.T) {
	p := Page{Number: 3, Width: 1200, Height: 1000, PageAccurate: true}
	p.Words = []Word{{Text: "26WJ35PN", X: 120, Y: 950, Size: 6, X1: 150}}
	pins := map[int]Word{0: {Text: "1", X: 150, Y: 920, Size: 6}}
	tags := map[string]string{"D38999/26WJ35PN": "SRC-P1", "D38999/26WJ35SN": "DST-P2"}
	if tag, via := columnConnector(p, 150, pins, tags, 1e9); tag != "SRC-P1" || via != "part" {
		t.Fatalf("tail must name the one part ending with it: %q %q", tag, via)
	}
	tags["M83723/26WJ35PN"] = "OTHER-J3" // a different part with the same tail: ambiguous
	if tag, _ := columnConnector(p, 150, pins, tags, 1e9); tag != "" {
		t.Fatalf("a tail ending two parts must not name a connector: %q", tag)
	}
}
