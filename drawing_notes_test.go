package main

import (
	"strconv"
	"strings"
	"testing"
)

// A NOTES block shaped like a typical D-size drawing: numbers at the margin, wrapped text indented under the
// words, a lettered sub-item, a flagged number with no dot, a DELETED note, and a parts table that shares the
// baselines further to the right.
func inventedNotesPage() Page {
	p := Page{Number: 1, Width: 1530, Height: 985, PageAccurate: true}
	add := func(x float64, y float64, s string) {
		p.Words = append(p.Words, Word{Text: s, X: x, Y: y, Size: 7, X1: x + 0.55*7*float64(len(s))})
	}
	add(83, 930, "NOTE:")
	add(113, 930, "UNLESS")
	add(143, 930, "OTHERWISE")
	add(185, 930, "SPECIFIED")
	y := 908.0
	note := func(num string, lines ...string) {
		add(85, y, num)
		for i, l := range lines {
			if i > 0 {
				y -= 10
			}
			x := 107.0
			for _, w := range strings.Fields(l) {
				add(x, y, w)
				x += 0.55*7*float64(len(w)) + 3
			}
		}
		y -= 22
	}
	note("1.", "INTERPRET DRAWING IAW EXAMPLE-STD-1.")
	note("2.", "DELETED")
	note("3.", "MARK IAW EXAMPLE-STD-2, CHARACTERS SHALL BE BLACK. MULTIPLE LINES OF CHARACTERS", "ARE ALLOWED.")
	note("5.", "ELECTRICAL REQUIREMENTS", "A. CONTINUITY SHALL BE CONTINUOUS AND NOT EXCEED 2.0 OHMS.")
	note("10", "ALL UNUSED CONTACT OPENINGS SHALL BE POPULATED CONTACTS OR SEALING PLUGS.") // a flagged number: no dot
	// the parts table on the same baselines, to the right of the notes
	for i, yy := range []float64{880, 858, 836, 814} {
		add(625, yy, string(rune('1'+i)))
		add(660, yy, "EA")
		add(760, yy, "EXAMPLE CONNECTOR")
		add(1190, yy, "EX-1000")
	}
	return p
}

func TestNumberedNotesAreReadWithTheirWrappedLinesAndNotTheTable(t *testing.T) {
	got := drawingNotes([]Page{inventedNotesPage()})
	want := map[string]string{
		"1":  "INTERPRET DRAWING IAW EXAMPLE-STD-1.",
		"2":  "DELETED",
		"3":  "MARK IAW EXAMPLE-STD-2, CHARACTERS SHALL BE BLACK. MULTIPLE LINES OF CHARACTERS ARE ALLOWED.",
		"5":  "ELECTRICAL REQUIREMENTS A. CONTINUITY SHALL BE CONTINUOUS AND NOT EXCEED 2.0 OHMS.",
		"10": "ALL UNUSED CONTACT OPENINGS SHALL BE POPULATED CONTACTS OR SEALING PLUGS.",
	}
	if len(got) != len(want) {
		t.Fatalf("notes: %+v", got)
	}
	for _, n := range got {
		if want[n.ID] != n.Text {
			t.Fatalf("note %s = %q, want %q", n.ID, n.Text, want[n.ID])
		}
		if strings.Contains(n.Text, "EXAMPLE CONNECTOR") || strings.Contains(n.Text, "EX-1000") {
			t.Fatalf("the parts table leaked into note %s: %q", n.ID, n.Text)
		}
	}
	if got[0].ID != "1" || got[len(got)-1].ID != "10" {
		t.Fatalf("notes are in numeric order: %+v", got)
	}
}

func TestNoNotesWithoutANotesHeading(t *testing.T) {
	p := inventedNotesPage()
	p.Words = p.Words[4:] // drop the heading
	if got := drawingNotes([]Page{p}); len(got) != 0 {
		t.Fatalf("numbers with no NOTES heading were taken as notes: %+v", got)
	}
	// a page that is only a parts list
	q := Page{Number: 1, Width: 1530, Height: 985, PageAccurate: true}
	for i := 0; i < 6; i++ {
		y := 800 - float64(i)*20
		q.Words = append(q.Words, Word{Text: string(rune('1' + i)), X: 90, Y: y, Size: 7}, Word{Text: "EA", X: 130, Y: y, Size: 7})
	}
	if got := drawingNotes([]Page{q}); len(got) != 0 {
		t.Fatalf("a parts list is not a notes block: %+v", got)
	}
}

// End to end through the PDF reader, not only hand-built words.
func TestNotesAreReadFromARealPDFPage(t *testing.T) {
	p := inventedNotesPage()
	content := ""
	for _, w := range p.Words {
		content += fmtText(w.X, w.Y, w.Text)
	}
	v, err := viewAnalysis(onePagePDF(content, "[0 0 1530 985]"))
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{}
	for _, n := range v.DrawingNotes {
		ids = append(ids, n.ID)
		if strings.Contains(n.Text, "EXAMPLE CONNECTOR") {
			t.Fatalf("the parts table leaked into note %s: %q", n.ID, n.Text)
		}
	}
	if strings.Join(ids, ",") != "1,2,3,5,10" {
		t.Fatalf("notes read from the PDF: %v", v.DrawingNotes)
	}
}

func fmtText(x, y float64, s string) string {
	return "BT /F1 7 Tf 1 0 0 1 " + strings.TrimRight(strings.TrimRight(strconvF(x), "0"), ".") + " " + strings.TrimRight(strings.TrimRight(strconvF(y), "0"), ".") + " Tm (" + s + ") Tj ET\n"
}

func strconvF(f float64) string { return strconv.FormatFloat(f, 'f', 2, 64) }
