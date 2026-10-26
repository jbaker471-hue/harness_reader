package main

import (
	"fmt"
	"strings"
	"testing"
)

// Defense-drawing layout: parts list built upward with the heading at the
// bottom, two-line headings, FIND NO. at the right, grouped FROM/TO wire list,
// connector table resolving REF DES via FIND NO., stacked pin-table headings.
func TestASMEStyleDrawing(t *testing.T) {
	a, err := AnalyzeDocument(BuildSyntheticASME())
	if err != nil {
		t.Fatal(err)
	}
	conns := map[string]string{}
	for _, c := range a.Connectors {
		conns[c.Tag] = c.Part
	}
	if len(a.Connectors) != 2 || conns["P1"] != "D38999/26FJ35SN" || conns["J2"] != "D38999/26FA35PN" {
		t.Fatalf("connectors %+v", a.Connectors)
	}
	for _, c := range a.Connectors {
		if strings.HasPrefix(c.Part, "MS27488") || strings.HasPrefix(c.Part, "D38999/32") || strings.HasPrefix(c.Part, "M39029") {
			t.Fatalf("accessory treated as connector: %+v", c)
		}
	}
	if len(a.BOM) != 6 {
		t.Fatalf("BOM rows %d %+v", len(a.BOM), a.BOM)
	}
	byItem := map[string]BOMRow{}
	for _, b := range a.BOM {
		byItem[b.Item] = b
	}
	if byItem["2"].Part != "M39029/56-348" || byItem["2"].Qty != "128" || byItem["3"].Qty != "A/R" || byItem["6"].Qty != "A/R" || !strings.Contains(byItem["5"].Description, "RECEPTACLE") {
		t.Fatalf("BOM content %+v", a.BOM)
	}
	want := map[string]string{"P1:1": "J2:A", "P1:2": "J2:B", "P1:3": "J2:C", "P1:4": "J2:D"}
	if len(a.Wires) != 4 {
		t.Fatalf("conductors %d %+v", len(a.Wires), a.Wires)
	}
	for _, w := range a.Wires {
		k := w.Connector + ":" + w.Cavity
		if want[k] != w.End+":"+w.EndCavity {
			t.Fatalf("%s maps to %s:%s", k, w.End, w.EndCavity)
		}
		if k != "P1:4" && w.Corroboration != 2 {
			t.Fatalf("%s should be corroborated by pin table and wire list: %+v", k, w)
		}
	}
	for _, f := range a.Findings {
		if f.Category == CatConflict || f.Code == "NO_CAVITY_ROWS" || f.Code == "BOM_NOT_FOUND" || f.Code == "NO_CONNECTORS" {
			t.Fatalf("unexpected finding %+v", f)
		}
	}
}

// Column readers must never run on non-page-accurate (legacy) coordinates:
// there, mirrored frames swap columns and produce wrong pin mappings.
func TestTableReadersRequirePageAccurateGeometry(t *testing.T) {
	b, _ := BuildSyntheticVariant("rotate180")
	d, _ := extractPDFEnhanced26(b)
	for _, p := range d.Pages {
		if p.PageAccurate || len(genericTables(p)) != 0 || len(pinoutTableRows(p)) != 0 {
			t.Fatal("table reader ran on legacy coordinates")
		}
	}
}

func TestAccessoryParts(t *testing.T) {
	for _, c := range []struct {
		pn, desc string
		acc      bool
	}{
		{"MS27488-22-2", "", true}, {"D38999/32F25R", "", true}, {"M85049/38-25N", "", true}, {"M39029/56-348", "", true},
		{"ABC-123", "PLUG, SEALING", true}, {"D38999/26FJ35SN", "CONNECTOR, PLUG", false}, {"D38999/24WA35PN", "", false},
	} {
		if isAccessoryPart(c.pn, c.desc) != c.acc {
			t.Fatalf("%s/%s accessory=%v", c.pn, c.desc, !c.acc)
		}
	}
}

func TestCombinedEndpointsAndCaseSensitivePins(t *testing.T) {
	row := func(y int, cells ...string) string {
		xs := []int{60, 130, 230, 330, 380}
		s := ""
		for i, c := range cells {
			s += fmt.Sprintf("BT /F1 7 Tf 1 0 0 1 %d %d Tm (%s) Tj ET\n", xs[i], y, c)
		}
		return s
	}
	c := "BT /F1 9 Tf 1 0 0 1 60 560 Tm (WIRE RUN LIST) Tj ET\n" +
		row(540, "WIRE ID", "FROM", "TO", "AWG", "COLOR") +
		row(526, "W10", "P1-A", "J5-1", "22", "WHT") +
		row(514, "W11", "P1-a", "J5-2", "22", "BLU") +
		row(502, "W12", "P1-B", "J5-3", "24", "ORN")
	a, err := AnalyzeDocument(onePagePDF(c, "[0 0 612 792]"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, w := range a.Wires {
		got[w.Connector+":"+w.Cavity] = w.End + ":" + w.EndCavity + "/" + w.Wire
	}
	want := map[string]string{"P1:A": "J5:1/W10", "P1:a": "J5:2/W11", "P1:B": "J5:3/W12"}
	if len(a.Wires) != 3 {
		t.Fatalf("wires %+v", a.Wires)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s -> %q, want %q (all %v)", k, got[k], v, got)
		}
	}
	for _, f := range a.Findings {
		if f.Code == "DUPLICATE_CAVITY" {
			t.Fatalf("A and a reported as duplicate: %+v", f)
		}
	}
}

func TestContactSexFromKeyedPartNumber(t *testing.T) {
	v, err := viewAnalysis(BuildSyntheticASME())
	if err != nil {
		t.Fatal(err)
	}
	sex := map[string]string{}
	for _, c := range v.Connectors {
		sex[c.Name] = c.Side
	}
	if sex["P1"] != "SOCKET" || sex["J2"] != "PIN" {
		t.Fatalf("contact sex %v (D38999/26FJ35SN is socket, /26FA35PN is pin)", sex)
	}
}
