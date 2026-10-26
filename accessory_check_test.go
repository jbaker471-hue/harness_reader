package main

import (
	"strings"
	"testing"
)

func accFind(cs []AccCheck, check, conn string) AccCheck {
	for _, c := range cs {
		if c.Check == check && (conn == "" || c.Connector == conn) {
			return c
		}
	}
	return AccCheck{Status: "ABSENT"}
}

func TestSeriesIIIShell(t *testing.T) {
	for pn, want := range map[string]int{"D38999/26FJ35SN": 25, "D38999/26FA35PN": 9, "D38999/24WG35SA": 21, "D38999/20FB98PN": 11} {
		if s, _, ok := SeriesIIIShell(pn); !ok || s != want {
			t.Fatalf("%s shell %d ok=%v, want %d", pn, s, ok, want)
		}
	}
	if _, _, ok := SeriesIIIShell("MS27488-22-2"); ok {
		t.Fatal("accessory parsed as connector")
	}
}

func TestAccessoryCheckAgainstBOM(t *testing.T) {
	conns := []AccConnector{{Tag: "P1", Part: "D38999/26FJ35SN", Occupied: []string{"1", "2", "3", "4"}}}
	// Correct BOM
	ok, err := CheckAccessories(AccRequest{Connectors: conns, BOM: []AccBOM{
		{Item: "1", Part: "D38999/26FJ35SN", Qty: "1"}, {Item: "2", Part: "M39029/56-348", Qty: "4"},
		{Item: "3", Part: "MS27488-22-2", Qty: "124"}, {Item: "4", Part: "M85049/38S25W", Qty: "1"},
		{Item: "5", Part: "600-057", Qty: "1"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"Backshell", "Contacts", "Sealing plugs (unused cavities)"} {
		if s := accFind(ok, c, "").Status; s != "OK" {
			t.Fatalf("%s status %s: %+v", c, s, ok)
		}
	}
	if ct := accFind(ok, "Contacts", ""); len(ct.Tooling) == 0 || !strings.Contains(ct.Tooling[0], "67-022-01") {
		t.Fatalf("socket retention tip missing: %+v", ct)
	}
	band := accFind(ok, "Shield termination band", "")
	if band.Status != "INFO" || len(band.Tooling) == 0 || band.Tooling[0] != "Banding tool 600-061" {
		t.Fatalf("band tooling %+v", band)
	}
	// Wrong BOM: backshell size 21, pin contacts, too few plugs
	bad, _ := CheckAccessories(AccRequest{Connectors: conns, BOM: []AccBOM{
		{Item: "2", Part: "M39029/58-360", Qty: "4"}, {Item: "3", Part: "MS27488-22-2", Qty: "10"}, {Item: "4", Part: "M85049/38S21W", Qty: "1"},
	}})
	if accFind(bad, "Backshell size", "").Status != "MISMATCH" || accFind(bad, "Backshell", "P1").Status != "MISSING" {
		t.Fatalf("backshell size mismatch not caught: %+v", bad)
	}
	if accFind(bad, "Contacts", "").Status != "MISMATCH" {
		t.Fatalf("opposite-sex contacts not caught: %+v", bad)
	}
	if accFind(bad, "Sealing plugs (unused cavities)", "").Status != "MISMATCH" {
		t.Fatalf("plug shortfall not caught: %+v", bad)
	}
	// Unknown populated cavities -> VERIFY, never OK
	unk, _ := CheckAccessories(AccRequest{Connectors: []AccConnector{{Tag: "P1", Part: "D38999/26FJ35SN"}}, BOM: []AccBOM{{Item: "2", Part: "M39029/56-348", Qty: "128"}, {Item: "3", Part: "MS27488-22-2", Qty: "A/R"}}})
	if accFind(unk, "Contacts", "").Status != "VERIFY" || accFind(unk, "Sealing plugs (unused cavities)", "").Status != "VERIFY" {
		t.Fatalf("unknown cavity list must give VERIFY: %+v", unk)
	}
}
