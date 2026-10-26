package main

import "testing"

func TestStructuredNetlistFallback(t *testing.T) {
	p := Page{Number: 1, Width: 850, Height: 610}
	texts := []string{"FROM PIN SIGNAL TO PIN", "J12 1 VBUS J21 3", "J12 2 RETURN J21 4", "J12 3 TXD_P J21 5", "J12 4 TXD_N J21 6"}
	for i, s := range texts {
		p.Words = append(p.Words, Word{Text: s, X: 75, Y: 550 - float64(i)*25, Size: 11})
	}
	a := findStructuredWireRows(p)
	if len(a) != 4 {
		t.Fatalf("structured table rows %d: %+v", len(a), a)
	}
	if a[0].Connector != "J12" || a[0].Cavity != "1" || a[0].End != "J21" || a[0].EndCavity != "3" {
		t.Fatalf("parsed %+v", a[0])
	}
}
func TestTextualBOMFallbackAndConnectors(t *testing.T) {
	p := Page{Number: 2, Width: 800, Height: 600}
	vals := []string{"Bill of Materials", "FIND QTY PART NUMBER DESCRIPTION", "1 2 M80-5100642 CONNECTOR HOUSING 6 WAY", "2 12 M80-0110005 CONTACT SOCKET", "3 A/R 123-123 SHRINK SLEEVE"}
	for i, s := range vals {
		p.Words = append(p.Words, Word{Text: s, X: 80, Y: 540 - float64(i)*30, Size: 10})
	}
	bom := findTextBOMRows(p)
	if len(bom) != 3 {
		t.Fatalf("BOM %d: %+v", len(bom), bom)
	}
	bom[0].Reference = "J10"
	c := connectorsFromDescribedParts(bom, nil)
	if len(c) != 1 || c[0].Part != "M80-5100642" || c[0].CavityCount != 0 {
		t.Fatalf("connector fallback %+v", c)
	}
}
func TestUnrelatedNotesNotMistakenForNetlist(t *testing.T) {
	p := Page{Number: 3, Width: 800, Height: 600, Words: []Word{{Text: "NOTES: FROM PIN TO PIN", X: 70, Y: 540}, {Text: "1 2 REFERENCE 3 4", X: 70, Y: 500}}}
	if got := findStructuredWireRows(p); len(got) != 0 {
		t.Fatalf("false positive %+v", got)
	}
	if got := findTextBOMRows(p); len(got) != 0 {
		t.Fatalf("false BOM %+v", got)
	}
}
