package main

import (
	"os"
	"strings"
	"testing"
)

func TestSpecialtyMultiStepReferenceLoads(t *testing.T) {
	d, e := specialtyReference()
	if e != nil {
		t.Fatal(e)
	}
	contacts := d["contacts"].([]any)
	if len(contacts) < 160 {
		t.Fatalf("only %d specialty/manufacturer records", len(contacts))
	}
	found := false
	for _, v := range contacts {
		r := v.(map[string]any)
		if r["part"] == "M39029/113-625" {
			found = true
			s := r["steps"].([]any)
			if len(s) != 3 {
				t.Fatalf("twinax contact needs three operations, got %d", len(s))
			}
		}
	}
	if !found {
		t.Fatal("twinax contact absent")
	}
}
func TestDrawingProcessFlagsNotRawText(t *testing.T) {
	b, e := os.ReadFile(os.Getenv("TRIVEX_REGRESSION_PDF")) // authorized local copy only
	if e != nil {
		t.Skip("sample not mounted")
	}
	d, e := viewAnalysis(b)
	if e != nil {
		t.Fatal(e)
	}
	for _, k := range []string{"SOLDERING", "SHIELD_PREPARATION", "WIRE_STRIPPING"} {
		if !strings.Contains(strings.Join(d.AssemblyFeatures, ","), k) {
			t.Fatalf("missing %s", k)
		}
	}
	if strings.Contains(d.PdfText, "PROPRIETARY") {
		t.Fatal("raw text leaked into part identifiers")
	}
}
