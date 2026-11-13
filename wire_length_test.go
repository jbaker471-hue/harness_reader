package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestLengthHeaderRoleAndWireRows(t *testing.T) {
	for _, h := range []string{"LENGTH", "Length (IN)", "Cut Length", "WIRE LENGTH"} {
		if got := headerRole(h); got != "length" {
			t.Fatalf("header %q mapped to %q, want length", h, got)
		}
	}
	tbl := genericTable{Page: 1, Kind: "wire", Title: "P1", Rows: []map[string]string{
		{"pin": "1", "wire": "W1", "to": "J2-4", "awg": "22", "color": "RED", "length": " 12.5 "},
		{"pin": "2", "wire": "W2", "to": "J2-5", "awg": "22", "color": "BLK"},
	}}
	rows := tbl.wireRows()
	if len(rows) != 2 {
		t.Fatalf("got %d rows", len(rows))
	}
	if rows[0].Length != "12.5" {
		t.Fatalf("length not carried and trimmed: %q", rows[0].Length)
	}
	if rows[1].Length != "" {
		t.Fatalf("missing length must stay empty, got %q", rows[1].Length)
	}
}

func TestPinoutHeaderRecognizesLength(t *testing.T) {
	found := false
	for _, h := range pinHeaderWords {
		if h.role == roleLength {
			found = true
		}
	}
	if !found {
		t.Fatal("pinout header vocabulary has no length role")
	}
	if !validRoles["length"] {
		t.Fatal("learned layouts must accept the length role")
	}
}

// synthWireListWithLength is a one-sheet NON-CUI wire list with a LENGTH
// column. W3 deliberately has no length: it must stay empty, not be invented.
func synthWireListWithLength() []byte {
	var b strings.Builder
	cell := func(x, y float64, s string) {
		fmt.Fprintf(&b, "BT /F1 7 Tf 1 0 0 1 %.1f %.1f Tm (%s) Tj ET\n", x, y, pdfEsc(s))
	}
	cell(40, 750, "NON-CUI SYNTHETIC WIRE LIST WITH LENGTH")
	cell(80, 690, "WIRE LIST")
	cell(170, 676, "FROM")
	cell(290, 676, "TO")
	for i, h := range []string{"WIRE NO.", "REF DES", "PIN", "REF DES", "PIN", "SIGNAL NAME", "GAGE", "COLOR", "LENGTH (IN)"} {
		cell([]float64{80, 150, 200, 270, 320, 370, 470, 510, 580}[i], 664, h)
	}
	rows := [][]string{
		{"W1", "P1", "1", "J2", "A", "PWR_28V", "22", "WHT", "12.5"},
		{"W2", "P1", "2", "J2", "B", "PWR_RTN", "22", "BLK", "3 FT"},
		{"W3", "P1", "3", "J2", "C", "CAN_H", "24", "WHT/GRN", ""},
		{"W4", "P1", "4", "J2", "D", "CAN_L", "24", "WHT/BLU", "18"},
	}
	for r, row := range rows {
		for i, v := range row {
			if v != "" {
				cell([]float64{80, 155, 202, 275, 322, 370, 472, 510, 590}[i], 650-float64(r)*12, v)
			}
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

func TestWireLengthSurvivesPDFExtraction(t *testing.T) {
	a, err := viewAnalysis(synthWireListWithLength())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, w := range a.Wires {
		got[w.Wire] = w.Length
	}
	want := map[string]string{"W1": "12.5", "W2": "3 FT", "W3": "", "W4": "18"}
	for k, v := range want {
		l, ok := got[k]
		if !ok {
			t.Fatalf("wire %s not extracted; got %+v", k, a.Wires)
		}
		if l != v {
			t.Fatalf("wire %s length %q, want %q (all: %+v)", k, l, v, got)
		}
	}
}
