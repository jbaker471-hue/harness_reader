package main

import (
	"fmt"
	"os"
	"testing"
)

// Records produced by the UNMODIFIED v2.6 build on its own fixtures. New
// readers must not add, drop or reassign anything on these drawings.
func TestV26FixtureParity(t *testing.T) {
	want := map[string]string{
		"SYNTHETIC_ALTERNATE_LAYOUT.pdf":      "1/4/3 J7=D38999/26FG35SA ",
		"SYNTHETIC_FORM_UNICODE_TABLES.pdf":   "1/4/3 J12=D38999/26FG35SA ",
		"SYNTHETIC_FORM_XOBJECT_TABLES.pdf":   "1/4/3 J12=D38999/26FG35SA ",
		"SYNTHETIC_GRAPHIC_CONNECTOR.pdf":     "0/0/0 ",
		"SYNTHETIC_RECOGNITION_TEST.pdf":      "1/4/2 P1=D38999/26FJ35SA ",
		"SYNTHETIC_TO_UNICODE.pdf":            "1/0/1 UNASSIGNED=D38999/26FG35SA ",
		"SYNTHETIC_UNICODE_TABLE_NETLIST.pdf": "1/4/3 J12=D38999/26FG35SA ",
		"SYNTHETIC_V24_TABLE_NETLIST.pdf":     "1/4/3 J12=D38999/26FG35SA ",
	}
	for f, exp := range want {
		b, err := os.ReadFile("testdata/" + f)
		if err != nil {
			t.Fatal(err)
		}
		a, err := AnalyzeDocument(b)
		if err != nil {
			t.Fatal(err)
		}
		got := fmt.Sprintf("%d/%d/%d ", len(a.Connectors), len(a.Wires), len(a.BOM))
		for _, c := range a.Connectors {
			got += c.Tag + "=" + c.Part + " "
		}
		if got != exp {
			t.Fatalf("%s: got %q, v2.6 produced %q", f, got, exp)
		}
	}
}
