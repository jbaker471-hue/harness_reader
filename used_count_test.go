package main

import (
	"fmt"
	"strings"
	"testing"
)

func usedPage(n int, lines ...string) Page {
	p := Page{Number: n}
	for i, l := range lines {
		for j, w := range strings.Fields(l) {
			p.Words = append(p.Words, Word{Text: w, X: float64(40 + j*50), Y: float64(100 + i*20)})
		}
	}
	return p
}

func TestStatedUsedCountsAreReadPerConnector(t *testing.T) {
	d := PDFData{Pages: []Page{usedPage(1,
		"P1: 24 CONTACTS USED",
		"J2 CONTACTS USED: 12",
		"NOTE 5 QTY CONTACTS USED 8 FOR X3",
		"P4 1 P5 2 24 CONTACTS USED", // two designators: the count does not belong to either
		"CONTACTS USED 9",            // no designator
		"J6 USED CONTACTS (7)",
		"TB7 NUMBER OF CONTACTS USED = 40000", // not plausible
	)}}
	got := map[string]int{}
	for _, s := range statedUsedCounts(d) {
		got[s.Connector] = s.Count
		if s.Page != 1 || s.Source == "" || s.Conflict {
			t.Fatalf("%+v", s)
		}
	}
	want := map[string]int{"P1": 24, "J2": 12, "X3": 8, "J6": 7}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s: got %d want %d (%v)", k, got[k], v, got)
		}
	}
}

func TestConflictingStatedCountsAreFlagged(t *testing.T) {
	d := PDFData{Pages: []Page{usedPage(1, "P1: 24 CONTACTS USED"), usedPage(2, "P1: 20 CONTACTS USED")}}
	s := statedUsedCounts(d)
	if len(s) != 1 || !s[0].Conflict {
		t.Fatalf("%+v", s)
	}
}

func usedModel(t *testing.T, stated, wired int) CavityModel {
	t.Helper()
	ws := []WireRow{}
	for i := 1; i <= wired; i++ {
		ws = append(ws, WireRow{Connector: "P7", Cavity: itoa(i), End: "J8", EndCavity: itoa(i)})
	}
	a := Analysis{Connectors: []Connector{{Tag: "P7", Part: "D38999/26WB35PN", CavityCount: 13}}, Wires: ws}
	if stated > 0 {
		a.UsedCounts = []StatedUsed{{Connector: "P7", Count: stated, Page: 1, Source: "statement on sheet 1"}}
	}
	for _, m := range buildCavityModels(&a, nil) {
		if m.Tag == "P7" {
			return m
		}
	}
	t.Fatal("no model")
	return CavityModel{}
}

func TestStatedUsedCountCorroboratesOrContradictsTheWiring(t *testing.T) {
	m := usedModel(t, 5, 5) // 13-cavity inventory, 5 stated, 5 wired: the other 8 are not used
	if !strings.HasPrefix(m.UnusedStatus, "CORROBORATED") || len(m.Unused) != 8 || m.UsedStated != 5 || len(m.Conflicts) != 0 {
		t.Fatalf("%+v", m)
	}
	m = usedModel(t, 8, 5) // 8 stated but only 5 wired: three used cavities are not found, so unused cannot be decided
	if !strings.HasPrefix(m.UnusedStatus, "HELD") || len(m.Unused) != 0 || !strings.Contains(m.UnusedStatus, "8 contacts used but only 5") {
		t.Fatalf("%+v", m)
	}
	m = usedModel(t, 3, 5) // fewer stated than wired: the sources contradict each other
	if len(m.Conflicts) == 0 || !strings.HasPrefix(m.UnusedStatus, "HELD") || len(m.Unused) != 0 {
		t.Fatalf("%+v", m)
	}
	m = usedModel(t, 20, 5) // more used than the inventory has cavities
	if len(m.Conflicts) == 0 {
		t.Fatalf("%+v", m)
	}
	m = usedModel(t, 0, 5) // nothing stated: behaviour is unchanged
	if !strings.HasPrefix(m.UnusedStatus, "CANDIDATE") || len(m.Unused) != 8 || m.UsedStated != 0 {
		t.Fatalf("%+v", m)
	}
}

// synthWireListWithUsedNote is the synthetic wire list from the length test plus one note stating how
// many contacts are used at P1. Four of P1's cavities carry wires.
func synthWireListWithUsedNote(note string) []byte {
	var b strings.Builder
	cell := func(x, y float64, s string) {
		fmt.Fprintf(&b, "BT /F1 7 Tf 1 0 0 1 %.1f %.1f Tm (%s) Tj ET\n", x, y, pdfEsc(s))
	}
	cell(40, 750, "NON-CUI SYNTHETIC WIRE LIST WITH A USED CONTACT COUNT")
	cell(80, 720, note)
	cell(80, 690, "WIRE LIST")
	cell(170, 676, "FROM")
	cell(290, 676, "TO")
	for i, h := range []string{"WIRE NO.", "REF DES", "PIN", "REF DES", "PIN", "SIGNAL NAME", "GAGE", "COLOR"} {
		cell([]float64{80, 150, 200, 270, 320, 370, 470, 510}[i], 664, h)
	}
	rows := [][]string{
		{"W1", "P1", "1", "J2", "A", "PWR_28V", "22", "WHT"},
		{"W2", "P1", "2", "J2", "B", "PWR_RTN", "22", "BLK"},
		{"W3", "P1", "3", "J2", "C", "CAN_H", "24", "WHT/GRN"},
		{"W4", "P1", "4", "J2", "D", "CAN_L", "24", "WHT/BLU"},
	}
	for r, row := range rows {
		for i, v := range row {
			cell([]float64{80, 155, 202, 275, 322, 370, 472, 510}[i], 650-float64(r)*12, v)
		}
	}
	w := &pdfWriter{}
	cat, pages := w.reserve(), w.reserve()
	f := w.add("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	cs := w.stream("", []byte(b.String()))
	pg := w.add(fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 842 792] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>", pages, f, cs))
	w.set(pages, fmt.Sprintf("<< /Type /Pages /Kids [%d 0 R] /Count 1 >>", pg))
	w.set(cat, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pages))
	return w.bytes(cat)
}

func TestStatedUsedCountSurvivesPDFExtraction(t *testing.T) {
	model := func(note string) CavityModel {
		a, err := viewAnalysis(synthWireListWithUsedNote(note))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range a.CavityModels {
			if m.Tag == "P1" {
				return m
			}
		}
		t.Fatalf("no model for P1: %+v", a.CavityModels)
		return CavityModel{}
	}
	if m := model("P1: 4 CONTACTS USED"); m.UsedStated != 4 || len(m.Wired) != 4 || strings.HasPrefix(m.UnusedStatus, "HELD") {
		t.Fatalf("matching count: %+v", m)
	}
	if m := model("P1: 6 CONTACTS USED"); m.UsedStated != 6 || !strings.HasPrefix(m.UnusedStatus, "HELD") || len(m.Unused) != 0 {
		t.Fatalf("a count above the wired cavities must hold: %+v", m)
	}
	if m := model("P1: 2 CONTACTS USED"); len(m.Conflicts) == 0 || !strings.HasPrefix(m.UnusedStatus, "HELD") {
		t.Fatalf("a count below the wired cavities must conflict: %+v", m)
	}
	if m := model("NOTE: SEE SHEET 2"); m.UsedStated != 0 {
		t.Fatalf("no statement must give no count: %+v", m)
	}
}
