package main

import (
	"fmt"
	"strings"
	"testing"
)

func synthHarnessLengthPDF(sheet1, sheet2 []string) []byte {
	var kids []string
	w := &pdfWriter{}
	cat, pages := w.reserve(), w.reserve()
	f := w.add("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	for _, lines := range [][]string{sheet1, sheet2} {
		var b strings.Builder
		for i, s := range lines {
			fmt.Fprintf(&b, "BT /F1 8 Tf 1 0 0 1 60 %.1f Tm (%s) Tj ET\n", 740-float64(i)*14, pdfEsc(s))
		}
		cs := w.stream("", []byte(b.String()))
		pg := w.add(fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>", pages, f, cs))
		kids = append(kids, fmt.Sprintf("%d 0 R", pg))
	}
	w.set(pages, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(kids)))
	w.set(cat, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pages))
	return w.bytes(cat)
}

func TestHarnessLengthFoundOnFirstSheets(t *testing.T) {
	pdf := synthHarnessLengthPDF(
		[]string{"NON-CUI SYNTHETIC", "HARNESS LENGTH: 48 IN +/- 1", "STRIP LENGTH .25", "SLEEVING LENGTH 6 IN", "LENGTH OF SLEEVE 3 IN", "CABLE LENGTH PER NOTE 3"},
		[]string{"OVERALL LENGTH 4 FT 6 IN", "ASSEMBLY LENGTH", "36.5", "WIRE LENGTH (REF) 120 MM"},
	)
	a, err := viewAnalysis(pdf)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, h := range a.HarnessLengths {
		got[fmt.Sprintf("%d|%s", h.Page, h.Label)] = h.Text
		if h.Page < 1 || h.Page > 3 {
			t.Fatalf("page out of range: %+v", h)
		}
	}
	want := map[string]string{
		"1|HARNESS LENGTH":  "48 IN",
		"2|OVERALL LENGTH":  "54 IN",
		"2|ASSEMBLY LENGTH": "36.5",
		"2|WIRE LENGTH":     "120 MM",
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s = %q, want %q (all %v)", k, got[k], v, got)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("strip, sleeve and note-reference lengths must not be reported: %v", got)
	}
	for _, h := range a.HarnessLengths {
		if h.Label == "HARNESS LENGTH" && h.Tolerance != "±1" {
			t.Fatalf("tolerance %q", h.Tolerance)
		}
	}
}

func TestHarnessLengthAbsentIsEmptyNotInvented(t *testing.T) {
	a, err := viewAnalysis(synthHarnessLengthPDF([]string{"NON-CUI SYNTHETIC", "NOTES: 1. SEE SHEET 2"}, []string{"NOTHING HERE"}))
	if err != nil {
		t.Fatal(err)
	}
	if a.HarnessLengths == nil || len(a.HarnessLengths) != 0 {
		t.Fatalf("expected an empty, non-nil list: %#v", a.HarnessLengths)
	}
}

func TestHarnessLengthOnlyReadsFirstThreeSheets(t *testing.T) {
	pages := []Page{{Number: 4, Words: []Word{{Text: "HARNESS", X: 10, Y: 100}, {Text: "LENGTH", X: 60, Y: 100}, {Text: "10", X: 110, Y: 100}}}}
	if got := harnessLengths(pages); len(got) != 0 {
		t.Fatalf("sheet 4 must be ignored: %v", got)
	}
}
