package main

import (
	"strings"
	"testing"
)

func TestGuideInventoryExactPartMatch(t *testing.T) {
	for _, part := range []string{"D38999/26WB35PN", "D38999/20FB35SA", "D38999/24WB35JA"} {
		g, ok := guideInventory(part)
		if !ok || len(g.IDs) != 13 || g.IDs[0] != "1" || g.IDs[12] != "13" || !strings.Contains(g.Source, "Glenair") {
			t.Fatalf("%s: %+v %v", part, g, ok)
		}
	}
	for _, part := range []string{"D38999/26WB35PN EXTRA", "D38999/26WB35PNX", "D38999/26WB99PN", "D38999/21WB35PN", "D38999/26WK35PN"} {
		if _, ok := guideInventory(part); ok {
			t.Fatal("unsupported part accepted: " + part)
		}
	}
}
func TestGuideInventorySubtractsUniqueBothEnds(t *testing.T) {
	a := Analysis{Connectors: []Connector{{Tag: "P7", Part: "D38999/26WB35PN", CavityCount: 13}}, Wires: []WireRow{{Connector: "P7", Cavity: "1", End: "J8", EndCavity: "4"}, {Connector: "J9", Cavity: "3", End: "P7", EndCavity: "2"}, {Connector: "P7", Cavity: "5", End: "J8", EndCavity: "6"}, {Connector: "P7", Cavity: "1", End: "J8", EndCavity: "4"}}}
	ms := buildCavityModels(&a, nil)
	var m CavityModel
	for _, v := range ms {
		if v.Tag == "P7" {
			m = v
		}
	}
	if !m.AllVerified || len(m.All) != 13 || strings.Join(m.Wired, ",") != "1,2,5" || strings.Join(m.Unused, ",") != "3,4,6,7,8,9,10,11,12,13" {
		t.Fatalf("%+v", m)
	}
	a.Wires = append(a.Wires, WireRow{Connector: "P7", Cavity: "14"})
	ms = buildCavityModels(&a, nil)
	for _, v := range ms {
		if v.Tag == "P7" && len(v.Conflicts) == 0 {
			t.Fatal("out-of-inventory wire not flagged")
		}
	}
}
func TestAllGuideRowsAndToolLibraries(t *testing.T) {
	for _, shell := range "ABCDEFGHJ" {
		p := "D38999/26W" + string(shell) + "35PN"
		g, ok := guideInventory(p)
		n, _ := getLayout(p)
		if !ok || len(g.IDs) != n || n < 1 {
			t.Fatal(p)
		}
	}
	if err := validateReferenceLibraries(); err != nil {
		t.Fatal(err)
	}
	dmc, e := crimpReference()
	if e != nil || len(dmc) < 294 {
		t.Fatal("DMC source incomplete")
	}
	other, e := specialtyReference()
	if e != nil || len(other["contacts"].([]any)) < 162 {
		t.Fatal("manufacturer/specialty sources incomplete")
	}
}
