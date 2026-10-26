package main

import (
	"os"
	"strings"
	"testing"
)

func TestPDFWithTextualBOMAndStructuredNetlist(t *testing.T) {
	b, e := os.ReadFile("testdata/SYNTHETIC_V24_TABLE_NETLIST.pdf")
	if e != nil {
		t.Fatal(e)
	}
	a, e := viewAnalysis(b)
	if e != nil {
		t.Fatal(e)
	}
	if a.Pages != 5 {
		t.Fatalf("page count %d", a.Pages)
	}
	if len(a.Wires) != 4 {
		t.Fatalf("conductor rows %d %+v", len(a.Wires), a.Wires)
	}
	if len(a.Boms) != 3 {
		t.Fatalf("BOM rows %d %+v", len(a.Boms), a.Boms)
	}
	found := false
	for _, c := range a.Connectors {
		if c.Name == "J12" && strings.Contains(c.Part, "D38999/26FG35SA") {
			found = true
		}
	}
	if !found {
		t.Fatalf("drawing connector not found %+v", a.Connectors)
	}
}
