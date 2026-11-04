package main

import (
	"fmt"
	"strings"
	"testing"
)

// A synthetic P1 pinout table (PIN | WIRE | SIGNAL | TO). Each row is a set of
// cells; a cell may hold "\n" for a second printed line inside the same row.
// When ruled, every row is drawn as a grid band, like a real drawing table.
func buildAuditTablePDF(rows [][]string, ruled bool) []byte {
	return buildAuditTablePDFParts(rows, ruled, false)
}

// withParts adds a connector table naming 13-position D38999 parts, so the
// connectors have a catalog cavity inventory.
func buildAuditTablePDFParts(rows [][]string, ruled, withParts bool) []byte {
	var b strings.Builder
	txt := func(x, y float64, s string) {
		fmt.Fprintf(&b, "BT /F1 7 Tf 1 0 0 1 %.2f %.2f Tm (%s) Tj ET\n", x, y, pdfEsc(s))
	}
	rule := func(y float64) { fmt.Fprintf(&b, "0.5 w %.2f %.2f m %.2f %.2f l S\n", 90.0, y, 420.0, y) }
	xs := []float64{100, 160, 220, 320}
	txt(100, 730, "CONNECTOR P1 PINOUT")
	for i, h := range []string{"PIN", "WIRE", "SIGNAL", "TO"} {
		txt(xs[i], 700, h)
	}
	if ruled {
		rule(712)
		rule(696)
	}
	y := 686.0
	for _, r := range rows {
		lines := 1
		for i, c := range r {
			for k, part := range strings.Split(c, "\n") {
				if part != "" {
					txt(xs[i], y-float64(k)*9, part)
				}
				if k+1 > lines {
					lines = k + 1
				}
			}
		}
		bottom := y - 4 - float64(lines-1)*9
		if ruled {
			rule(bottom)
		}
		y = bottom - 10
	}
	if withParts {
		txt(100, 330, "CONNECTOR TABLE")
		txt(100, 312, "REF DES")
		txt(170, 312, "PART NUMBER")
		txt(100, 298, "P1")
		txt(170, 298, "D38999/26WB35PN")
		txt(100, 284, "J2")
		txt(170, 284, "D38999/26WB35SN")
	}
	txt(100, 120, "NOTES: SYNTHETIC NON-CUI TABLE AUDIT FIXTURE")
	w := &pdfWriter{}
	catalog := w.reserve()
	pages := w.reserve()
	helv := w.add("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	cs := w.stream("", []byte(b.String()))
	pg := w.add(fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>", pages, helv, cs))
	w.set(pages, fmt.Sprintf("<< /Type /Pages /Kids [%d 0 R] /Count 1 >>", pg))
	w.set(catalog, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pages))
	return w.bytes(catalog)
}

func auditsFor(t *testing.T, pdf []byte) ([]TableAudit, Analysis) {
	t.Helper()
	x, err := ExtractDocument(pdf)
	if err != nil {
		t.Fatal(err)
	}
	a, _, _, err := AnalyzeExtraction(x)
	if err != nil {
		t.Fatal(err)
	}
	return a.TableAudits, a
}

func goodRows(from, to int) [][]string {
	out := [][]string{}
	for i := from; i <= to; i++ {
		out = append(out, []string{itoa(i), "W" + itoa(i), "SIG_" + itoa(i), "J2-" + itoa(i)})
	}
	return out
}

func onlyAudit(t *testing.T, as []TableAudit) TableAudit {
	t.Helper()
	if len(as) != 1 {
		t.Fatalf("want exactly one pin-table audit, got %d: %+v", len(as), as)
	}
	return as[0]
}

func TestTableAuditCleanRuledTableConfirms(t *testing.T) {
	as, _ := auditsFor(t, buildAuditTablePDF(goodRows(1, 8), true))
	a := onlyAudit(t, as)
	if a.Status != "COMPLETE" || !a.Ruled || a.RuledRows != 8 || a.RowsRead != 8 || len(a.Missed) != 0 {
		t.Fatalf("clean ruled table: %+v", a)
	}
	if strings.Join(a.Connectors, ",") != "J2,P1" {
		t.Fatalf("connectors: %v", a.Connectors)
	}
}

func TestTableAuditWrappedSignalIsNotAMissedRow(t *testing.T) {
	rows := goodRows(1, 6)
	rows[2][2] = "SIG_3\nLONG NAME CONT"
	for _, ruled := range []bool{true, false} {
		as, _ := auditsFor(t, buildAuditTablePDF(rows, ruled))
		a := onlyAudit(t, as)
		if a.Status != "COMPLETE" || len(a.Missed) != 0 || a.RowsRead != 6 {
			t.Fatalf("ruled=%v: a wrapped signal must not be a missed row: %+v", ruled, a)
		}
	}
}

func TestTableAuditCatchesSkippedRow(t *testing.T) {
	rows := goodRows(1, 8)
	rows[3] = []string{"4, 5", "W4", "JUMPERED", "J2-4"} // two pins in one cell: the reader skips it
	for _, ruled := range []bool{true, false} {
		as, a := auditsFor(t, buildAuditTablePDF(rows, ruled))
		au := onlyAudit(t, as)
		if au.Status != "INCOMPLETE" || len(au.Missed) != 1 || !strings.Contains(au.Missed[0].Text, "W4") {
			t.Fatalf("ruled=%v: skipped row not caught: %+v", ruled, au)
		}
		found := false
		for _, f := range a.Findings {
			if f.Code == "TABLE_ROW_NOT_READ" && strings.Contains(f.Message, "P1") && f.Category == CatExtraction {
				found = true
			}
		}
		if !found {
			t.Fatalf("ruled=%v: no TABLE_ROW_NOT_READ finding naming P1", ruled)
		}
	}
}

func TestTableAuditCatchesRowsLostAfterReaderStops(t *testing.T) {
	rows := goodRows(1, 4)
	rows = append(rows, []string{"5, 6", "W5", "BAD", "J2-5"}, []string{"7, 8", "W7", "BAD", "J2-7"})
	rows = append(rows, goodRows(9, 12)...)
	for _, ruled := range []bool{true, false} {
		as, _ := auditsFor(t, buildAuditTablePDF(rows, ruled))
		a := onlyAudit(t, as)
		if a.Status != "INCOMPLETE" {
			t.Fatalf("ruled=%v: rows after the reader stopped were not caught: %+v", ruled, a)
		}
		joined := ""
		for _, m := range a.Missed {
			joined += m.Text + " | "
		}
		for _, want := range []string{"W5", "W7", "W9", "W12"} {
			if !strings.Contains(joined, want) {
				t.Fatalf("ruled=%v: %s not reported as missed (%d read): %s", ruled, want, a.RowsRead, joined)
			}
		}
	}
}

func TestTableAuditEmptyRuledRowsAreNotCounted(t *testing.T) {
	rows := append(goodRows(1, 5), []string{"", "", "", ""}, []string{"", "", "", ""})
	as, _ := auditsFor(t, buildAuditTablePDF(rows, true))
	a := onlyAudit(t, as)
	if a.Status != "COMPLETE" || a.RuledRows != 5 || len(a.Missed) != 0 {
		t.Fatalf("blank ruled rows must not count as rows: %+v", a)
	}
}
