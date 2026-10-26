package main

import (
	"os"
	"strings"
	"testing"
)

func TestCADFormXObjectTextExtraction(t *testing.T) {
	b, e := os.ReadFile("testdata/SYNTHETIC_FORM_XOBJECT_TABLES.pdf")
	if e != nil {
		t.Fatal(e)
	}
	d, e := extractPDF(b)
	if e != nil {
		t.Fatal(e)
	}
	if len(d.Pages) != 5 {
		t.Fatalf("pages=%d", len(d.Pages))
	}
	if len(d.Pages[0].Words) == 0 {
		t.Fatal("no text decoded inside Form XObject")
	}
	a, e := viewAnalysis(b)
	if e != nil {
		t.Fatal(e)
	}
	if len(a.Wires) != 4 || len(a.Boms) != 3 {
		t.Fatalf("Form XObject regression wires=%d bom=%d", len(a.Wires), len(a.Boms))
	}
	matched := false
	for _, c := range a.Connectors {
		if c.Name == "J12" && strings.Contains(c.Part, "D38999/26FG35SA") {
			matched = true
		}
	}
	if !matched {
		t.Fatalf("Form XObject connector missing %+v", a.Connectors)
	}
}
