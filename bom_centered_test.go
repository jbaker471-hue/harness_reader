package main

import (
	"fmt"
	"strings"
	"testing"
)

// A bottom-up parts list (header row at the bottom) whose cells are vertically centred on the item line, so a
// description or a hyphen-wrapped reference designator has one baseline above and one below it.
func centeredBottomUpBOMPage() []byte {
	text := func(x int, y float64, s string) string {
		return fmt.Sprintf("BT /F1 7 Tf 1 0 0 1 %d %.1f Tm (%s) Tj ET\n", x, y, s)
	}
	content := ""
	for _, h := range []struct {
		x int
		s string
	}{{620, "ITEM NO"}, {660, "QTY"}, {690, "UNIT"}, {800, "DESCRIPTION"}, {1005, "REF DES"}, {1070, "SPECIFICATION/MANUFACTURE"}, {1190, "MANUFACTURE P/N"}, {1295, "SECOND P/N"}, {1395, "NOTES OR REMARKS"}} {
		content += text(h.x, 100, h.s)
	}
	type row struct {
		item, qty, unit string
		desc, ref       []string
		mfr, part       string
	}
	rows := []row{
		{"1", "1", "EA", []string{"EXAMPLE CONNECTOR PLUG MALE,", "ALTERNATE KEYED"}, []string{"TEST-", "P1"}, "EXAMPLE MAKER", "EX-1000/26AB11PA"},
		{"2", "1", "EA", []string{"EXAMPLE CONNECTOR PLUG FEMALE,", "ALTERNATE KEYED"}, []string{"BENCH-", "P2"}, "EXAMPLE MAKER", "EX-1000/26AB11SA"},
		{"3", "2", "EA", []string{"EXAMPLE BACKSHELL"}, nil, "OTHER MAKER", "EX-2000/20-25N"},
		{"4", "64", "EA", []string{"22AWG SEALING PLUG"}, nil, "EXAMPLE MAKER", "EX-3000-22-2"},
	}
	for i, r := range rows {
		y := 140 + float64(i)*40
		for _, c := range []struct {
			x int
			s string
		}{{620, r.item}, {660, r.qty}, {690, r.unit}, {1070, r.mfr}, {1190, r.part}, {1295, r.part}} {
			content += text(c.x, y, c.s)
		}
		// the first line of a wrapped cell sits above the item line, the second below it
		for k, s := range r.desc {
			content += text(800, y+4.5-float64(k)*9*float64(min(1, len(r.desc)-1)), s)
		}
		for k, s := range r.ref {
			content += text(1005, y+4.5-float64(k)*9, s)
		}
	}
	return onePagePDF(content, "[0 0 1500 700]")
}

func TestCenteredBottomUpBOMKeepsWrappedCellsWithTheirRow(t *testing.T) {
	a, err := AnalyzeDocument(centeredBottomUpBOMPage())
	if err != nil {
		t.Fatal(err)
	}
	if len(a.BOM) != 4 {
		t.Fatalf("expected 4 BOM rows, got %d: %+v", len(a.BOM), a.BOM)
	}
	byItem := map[string]BOMRow{}
	for _, r := range a.BOM {
		byItem[r.Item] = r
	}
	if got := byItem["1"].Reference; got != "TEST-P1" {
		t.Fatalf("item 1 reference %q, want TEST-P1 (hyphen-wrapped cell must join without a space)", got)
	}
	if got := byItem["2"].Reference; got != "BENCH-P2" {
		t.Fatalf("item 2 reference %q, want BENCH-P2", got)
	}
	if got := byItem["1"].Description; !strings.Contains(got, "MALE") || !strings.Contains(got, "ALTERNATE KEYED") {
		t.Fatalf("item 1 description lost a wrapped line: %q", got)
	}
	if got := byItem["2"].Description; !strings.Contains(got, "FEMALE") || !strings.Contains(got, "ALTERNATE KEYED") {
		t.Fatalf("item 2 description lost a wrapped line: %q", got)
	}
	if byItem["3"].Reference != "" || byItem["4"].Reference != "" {
		t.Fatalf("rows without a reference designator picked one up: %+v %+v", byItem["3"], byItem["4"])
	}
	if byItem["3"].Description != "EXAMPLE BACKSHELL" || byItem["4"].Description != "22AWG SEALING PLUG" {
		t.Fatalf("single-line descriptions changed: %q / %q", byItem["3"].Description, byItem["4"].Description)
	}
}

// The hyphen-wrapped designator is one connector tag, not a second connector named after its last half.
func TestHyphenWrappedDesignatorMakesOneConnectorPerTag(t *testing.T) {
	text := func(x int, y float64, s string) string {
		return fmt.Sprintf("BT /F1 7 Tf 1 0 0 1 %d %.1f Tm (%s) Tj ET\n", x, y, s)
	}
	content := ""
	for _, h := range []struct {
		x int
		s string
	}{{620, "ITEM NO"}, {660, "QTY"}, {690, "UNIT"}, {800, "DESCRIPTION"}, {1005, "REF DES"}, {1190, "MANUFACTURE P/N"}} {
		content += text(h.x, 100, h.s)
	}
	content += text(620, 140, "1") + text(660, 140, "1") + text(690, 140, "EA") + text(1190, 140, "D38999/26FJ35SA")
	content += text(800, 144.5, "D38999 CONNECTOR 128P PLUG FEMALE,") + text(800, 135.5, "ALTERNATE KEYED")
	content += text(1005, 144.5, "BENCH-") + text(1005, 135.5, "P2")
	content += text(620, 180, "2") + text(660, 180, "1") + text(690, 180, "EA") + text(1190, 180, "D38999/26FJ35PA")
	content += text(800, 184.5, "D38999 CONNECTOR 128P PLUG MALE,") + text(800, 175.5, "ALTERNATE KEYED")
	content += text(1005, 184.5, "TEST-") + text(1005, 175.5, "P1")
	a, err := AnalyzeDocument(onePagePDF(content, "[0 0 1500 700]"))
	if err != nil {
		t.Fatal(err)
	}
	tags := map[string]bool{}
	for _, c := range a.Connectors {
		tags[c.Tag] = true
	}
	if !tags["BENCH-P2"] || !tags["TEST-P1"] || tags["P2"] || tags["P1"] || len(a.Connectors) != 2 {
		t.Fatalf("connectors %+v", a.Connectors)
	}
}
