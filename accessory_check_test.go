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
	for _, c := range []string{"Backshell"} {
		if s := accFind(ok, c, "").Status; s != "OK" {
			t.Fatalf("%s status %s: %+v", c, s, ok)
		}
	}
	band := accFind(ok, "Shield termination band", "")
	if band.Status != "INFO" || len(band.Tooling) != 2 || !strings.Contains(band.Tooling[0], "600-061") || !strings.Contains(band.Tooling[1], "DBS-2200") || !strings.Contains(band.Detail, "M81306/1B") {
		t.Fatalf("band tooling %+v", band)
	}
	// Wrong BOM: backshell size 21, pin contacts, too few plugs
	bad, _ := CheckAccessories(AccRequest{Connectors: conns, BOM: []AccBOM{
		{Item: "2", Part: "M39029/58-360", Qty: "4"}, {Item: "3", Part: "MS27488-22-2", Qty: "10"}, {Item: "4", Part: "M85049/38S21W", Qty: "1"},
	}})
	if accFind(bad, "Backshell size", "").Status != "MISMATCH" || accFind(bad, "Backshell", "P1").Status != "MISSING" {
		t.Fatalf("backshell size mismatch not caught: %+v", bad)
	}
	// Contact/plug quantities belong exclusively to the policy-aware build plan.
	for _, row := range append(ok, bad...) {
		if row.Check == "Contacts" || row.Check == "Sealing plugs (unused cavities)" {
			t.Fatalf("duplicate contact/plug check: %+v", row)
		}
	}
}

func TestBandedBackshellRequiresExactBandAndSelectsExactTool(t *testing.T) {
	base := AccRequest{BOM: []AccBOM{{Item: "3", Part: "M85049/82-25N", Qty: "2", Description: "BANDING BACKSHELL"}}}
	checks, err := CheckAccessories(base)
	if err != nil {
		t.Fatal(err)
	}
	unresolved := accFind(checks, "Shield termination band/tool", "")
	if unresolved.Status != "UNRESOLVED" || !strings.Contains(strings.Join(unresolved.Tooling, " "), "exact band P/N") {
		t.Fatalf("missing-band safeguard: %+v", checks)
	}
	base.BOM = append(base.BOM, AccBOM{Item: "10", Part: "600-052", Qty: "2", Description: "BAND, 0.240 IN"})
	checks, err = CheckAccessories(base)
	if err != nil {
		t.Fatal(err)
	}
	band := accFind(checks, "Shield termination band", "")
	if band.Status != "INFO" || len(band.Tooling) != 2 || !strings.Contains(band.Tooling[0], "600-058") || !strings.Contains(band.Tooling[1], "DBS-2100") || !strings.Contains(band.Detail, "M81306/1A") {
		t.Fatalf("exact standard band tooling: %+v", checks)
	}
	if accFind(checks, "Shield termination band/tool", "").Status != "ABSENT" {
		t.Fatalf("exact band should clear unresolved band-tool check: %+v", checks)
	}
}

func TestBackshellTerminationStylesRequireCorrectDistinctTooling(t *testing.T) {
	checks, err := CheckAccessories(AccRequest{BOM: []AccBOM{
		{Item: "1", Part: "M85049/20-25N", Description: "BANDING BACKSHELL"},
		{Item: "2", Part: "M85049/38S25W", Description: "STRAIN RELIEF CLAMP"},
		{Item: "3", Part: "TXR54AB00-2216AI", Description: "TINEL-LOCK BACKSHELL"},
		{Item: "4", Part: "203M0XX", Description: "FACTORY-ATTACHED BRAID WITH MAGNEFORM CRIMP RING"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	crimp := accFind(checks, "Crimp-ring termination tooling", "")
	if crimp.Status != "UNRESOLVED" || !strings.Contains(strings.Join(crimp.Tooling, " "), "special crimp-ring") {
		t.Fatalf("crimp-ring tooling not held: %+v", checks)
	}
	if accFind(checks, "Shield termination band/tool", "").Status != "ABSENT" {
		t.Fatalf("M85049/20 misleading description must not turn it into a banding backshell: %+v", checks)
	}
	clamp := accFind(checks, "Backshell clamp driver", "")
	if clamp.Status != "VERIFY" || !strings.Contains(strings.Join(clamp.Tooling, " "), "flat-blade") {
		t.Fatalf("clamp driver not identified: %+v", checks)
	}
	if tinel := accFind(checks, "Tinel-Lock ring installation", ""); tinel.Status != "INFO" || !strings.Contains(strings.Join(tinel.Tooling, " "), "AD-5000") {
		t.Fatalf("Tinel tool not identified: %+v", checks)
	}
	if factory := accFind(checks, "Factory crimp-ring termination", ""); factory.Status != "INFO" || len(factory.Tooling) != 0 {
		t.Fatalf("factory-preterminated braid must not demand field crimp tooling: %+v", checks)
	}
}
