package main

import (
	"os"
	"slices"
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

func TestAccessoryBandToolReferenceLoads(t *testing.T) {
	rows, err := accessoryReference()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"600-052":      "600-058",
		"M85049/128-7": "600-061",
		"M85049/20":    "crimp-ring",
		"M85049/38":    "flat-blade",
		"TXR":          "AD-5000",
	}
	for part, tool := range want {
		found := false
		for _, value := range rows {
			row := value.(map[string]any)
			for _, match := range row["matches"].([]string) {
				if match == part && strings.Contains(row["requirement"].(string), tool) {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("no exact %s -> %s band-tool reference", part, tool)
		}
	}
	for _, value := range rows {
		row := value.(map[string]any)
		matches := row["matches"].([]string)
		if slices.Contains(matches, "600-052") {
			if row["specification"] != "M81306/1A" || !slices.Contains(row["alternates"].([]string), "DMC DBS-2100") || !strings.Contains(row["alternateStatus"].(string), "qualified-equivalent") {
				t.Fatalf("standard band alternate basis incomplete: %+v", row)
			}
		}
		if slices.Contains(matches, "600-057") {
			if row["specification"] != "M81306/1B" || !slices.Contains(row["alternates"].([]string), "DMC DBS-2200") {
				t.Fatalf("micro band alternate basis incomplete: %+v", row)
			}
		}
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

func TestWireCableAndToolEquivalenceLibrariesLoad(t *testing.T) {
	wires, err := wireCableReference()
	if err != nil {
		t.Fatal(err)
	}
	if len(wires) < 12 {
		t.Fatalf("only %d wire/cable family records", len(wires))
	}
	foundWire := false
	for _, value := range wires {
		row := value.(map[string]any)
		if strings.Contains(row["family"].(string), "AS22759 medium-weight") {
			foundWire = strings.Contains(row["preparation"].(string), "strip") && strings.Contains(row["status"].(string), "EXACT SLASH SHEET")
		}
	}
	if !foundWire {
		t.Fatal("AS22759 construction/preparation safeguard missing")
	}
	equivs, err := toolEquivalenceReference()
	if err != nil {
		t.Fatal(err)
	}
	foundAF8 := false
	for _, value := range equivs {
		row := value.(map[string]any)
		if row["specification"] == "AS22520/1" {
			foundAF8 = strings.Contains(row["primary"].(string), "AF8") && strings.Contains(row["conditions"].(string), "exact turret/positioner")
		}
	}
	if !foundAF8 {
		t.Fatal("AS22520/1 manual/powered route conditions missing")
	}
}
