package main

import (
	"os"
	"strings"
	"testing"
)

func TestToUnicodeEmbeddedFonts(t *testing.T) {
	b, e := os.ReadFile("testdata/SYNTHETIC_TO_UNICODE.pdf")
	if e != nil {
		t.Fatal(e)
	}
	d, e := extractPDF(b)
	if e != nil {
		t.Fatal(e)
	}
	if len(d.Pages) != 1 {
		t.Fatalf("pages %d", len(d.Pages))
	}
	text := strings.Join(d.Pages[0].Lines, " ")
	for _, s := range []string{"BILL OF MATERIALS", "D38999/26FG35SA", "CONNECTOR J12", "VBUS"} {
		if !strings.Contains(text, s) {
			t.Errorf("not decoded %q: %s", s, text)
		}
	}
}
func TestUnicodeEncodedFullDrawing(t *testing.T) {
	b, e := os.ReadFile("testdata/SYNTHETIC_UNICODE_TABLE_NETLIST.pdf")
	if e != nil {
		t.Fatal(e)
	}
	a, e := viewAnalysis(b)
	if e != nil {
		t.Fatal(e)
	}
	if len(a.Wires) != 4 || len(a.Boms) != 3 {
		t.Errorf("glyph-font drawing failed: wires=%d bom=%d", len(a.Wires), len(a.Boms))
	}
	found := false
	for _, c := range a.Connectors {
		if c.Name == "J12" {
			found = true
		}
	}
	if !found {
		t.Errorf("glyph-font connector not identified: %+v", a.Connectors)
	}
}
func TestFormXObjectWithUnicodeFontsTogether(t *testing.T) {
	b, e := os.ReadFile("testdata/SYNTHETIC_FORM_UNICODE_TABLES.pdf")
	if e != nil {
		t.Fatal(e)
	}
	a, e := viewAnalysis(b)
	if e != nil {
		t.Fatal(e)
	}
	if len(a.Wires) != 4 || len(a.Boms) != 3 {
		t.Errorf("nested Form+glyph CMap failed: wires=%d BOM=%d", len(a.Wires), len(a.Boms))
	}
}
