package main

import (
	"os"
	"strings"
	"testing"
)

func TestConnectorToContactManufacturerCrossReference(t *testing.T) {
	cases := []struct {
		connector, part, sex string
		count                int
	}{
		{"D38999/26FG35SA", "M39029/56-348", "SOCKET", 79},
		{"D38999/26FJ35PA", "M39029/58-360", "PIN", 128},
		{"D38999/24FH35SN", "M39029/56-348", "SOCKET", 100},
		{"D38999/26FG35HN", "M39029/107-620", "PIN", 79},
		{"D38999/26FG35JN", "M39029/106-614", "SOCKET", 79},
	}
	for _, tt := range cases {
		t.Run(tt.connector, func(t *testing.T) {
			v := ResolveConnectorContacts(tt.connector)
			if v.Status != "manufacturer-catalog-candidate" || v.ContactSex != tt.sex || v.PositionCount != tt.count || len(v.Contacts) != 1 || v.Contacts[0].Part != tt.part || v.Contacts[0].Qty != tt.count {
				t.Fatalf("invalid resolution: %+v", v)
			}
		})
	}
}
func TestNonStandardOrUnlistedConnectorCannotInferTools(t *testing.T) {
	for _, p := range []string{"D38999/26FG35AN", "D38999/26FG35BN", "D38999/26FG41SA", "D38999/26FG75SA", "D38999/26FG35XA", "D38999/26FG35", "M85049/38-21N", "D38999/26FJ35SNJUNK"} {
		v := ResolveConnectorContacts(p)
		if len(v.Contacts) != 0 {
			t.Fatalf("unsafe guess for %s: %+v", p, v)
		}
		if v.Status == "manufacturer-catalog-candidate" {
			t.Fatalf("unsafe review status for %s", p)
		}
	}
}
func TestRecommendedContactsHaveMatchingDMCRecords(t *testing.T) {
	rows, e := crimpReference()
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range []string{"D38999/26FG35SA", "D38999/26FJ35PA", "D38999/26FG35HN", "D38999/26FG35JN"} {
		r := ResolveConnectorContacts(p)
		for _, contact := range r.Contacts {
			valid := false
			for _, anyrow := range rows {
				x := anyrow.(map[string]any)
				if x["part"] == contact.Part && x["sex"] == contact.Sex && x["contactSize"] == contact.ContactSize {
					if len(x["routes"].([][]string)) == 0 {
						t.Fatalf("no DMC route for %s", contact.Part)
					}
					valid = true
				}
			}
			if !valid {
				t.Errorf("%s has no matching DMC contact/tooling mapping", p)
			}
		}
	}
}
func TestContactMappingsIncludedInDrawingAnalysis(t *testing.T) {
	// Private customer drawings aren't distributed with the application.
	// This test runs only where the previously approved sample is mounted.
	// Local regression only: set TRIVEX_REGRESSION_PDF to the authorized copy.
	path := os.Getenv("TRIVEX_REGRESSION_PDF")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Skip("sample not mounted")
	}
	v, err := viewAnalysis(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Boms) != 8 || len(v.Wires) != 64 {
		t.Fatalf("drawing parser regression: %d BOM, %d nets", len(v.Boms), len(v.Wires))
	}
	found := false
	for _, c := range v.Connectors {
		if strings.EqualFold(c.Part, "D38999/26FG35SA") {
			found = true
			if len(c.ContactCandidates) != 1 || c.ContactCandidates[0].Part != "M39029/56-348" {
				t.Fatalf("scan did not carry contact mapping: %+v", c)
			}
		}
	}
	if !found {
		t.Fatal("P1 source connector missing")
	}
	for _, f := range v.Issues {
		if strings.Contains(f.Observed, "112") && strings.Contains(f.Observed, "113") {
			return
		}
	}
	t.Fatal("must preserve cavity 112/113 drawing hold")
}
