package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func onePagePDF(content string, media string) []byte {
	w := &pdfWriter{}
	cat, pages := w.reserve(), w.reserve()
	f := w.add("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	cs := w.stream("", []byte(content))
	pg := w.add(fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox %s /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>", pages, media, f, cs))
	w.set(pages, fmt.Sprintf("<< /Type /Pages /Kids [%d 0 R] /Count 1 >>", pg))
	w.set(cat, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pages))
	return w.bytes(cat)
}

// Separate text-show strings one space apart stay separate fragments (the
// v2.6 recognizers depend on that), while one string keeps its spaces.
func TestFragmentsFollowTextShowBoundaries(t *testing.T) {
	c := "BT /F1 10 Tf 1 0 0 1 100 500 Tm (USB_D+) Tj ET\n" +
		"BT /F1 10 Tf 1 0 0 1 142.2 500 Tm (54) Tj ET\n" + // "USB_D+" is 39.2 pt wide in Helvetica 10: 0.3 em gap
		"BT /F1 10 Tf 1 0 0 1 100 480 Tm (BILL OF MATERIALS) Tj ET\n"
	x, err := ExtractDocument(onePagePDF(c, "[0 0 612 792]"))
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, w := range x.Layouts[0].Words {
		got = append(got, w.Text)
	}
	if strings.Join(got, "|") != "USB_D+|54|BILL OF MATERIALS" {
		t.Fatalf("fragments %q", got)
	}
}

func TestDenseSheetPerformance(t *testing.T) {
	var b strings.Builder
	b.WriteString("0.3 w\n")
	for i := 0; i < 30000; i++ { // ~60k vector segments
		x, y := float64(i%300)*8+20, float64(i/300)*15+20
		fmt.Fprintf(&b, "%.1f %.1f m %.1f %.1f l %.1f %.1f l S\n", x, y, x+6, y, x+6, y+5)
	}
	for r := 0; r < 120; r++ {
		for c := 0; c < 20; c++ {
			fmt.Fprintf(&b, "BT /F1 6 Tf 1 0 0 1 %d %d Tm (SIG_%03d_%02d) Tj ET\n", 40+c*120, 60+r*13, r, c)
		}
	}
	pdf := onePagePDF(b.String(), "[0 0 2448 1584]") // D-size sheet
	t0 := time.Now()
	x, err := ExtractDocument(pdf)
	if err != nil {
		t.Fatal(err)
	}
	el := time.Since(t0)
	pl := x.Layouts[0]
	t.Logf("D-size sheet: %d chars, %d fragments, %d path segments in %v", pl.Text.Chars, len(pl.Words), pl.PathSegments, el)
	if len(pl.Words) != 2400 {
		t.Fatalf("fragments %d", len(pl.Words))
	}
	if el > 60*time.Second {
		t.Fatalf("too slow: %v", el)
	}
}
