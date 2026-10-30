package main

import (
	"fmt"
	"testing"
)

func unitPage(n int, lines ...string) Page {
	p := Page{Number: n, Width: 1000, Height: 800, PageAccurate: true}
	for i, l := range lines {
		p.Words = append(p.Words, Word{Text: l, X: 100, Y: 700 - float64(i)*20, Size: 8})
	}
	return p
}

func TestDrawingUnitIsReadFromTheDrawingsOwnWords(t *testing.T) {
	for _, c := range []struct {
		lines []string
		unit  string
	}{
		{[]string{"UNLESS OTHERWISE SPECIFIED", "DIM ARE IN INCHES", "TOL ON ANGLE ±1°"}, "IN"},
		{[]string{"UNLESS OTHERWISE SPECIFIED DIMENSIONS ARE IN MILLIMETERS"}, "MM"},
		{[]string{"ALL DIMENSIONS IN INCHES"}, "IN"},
		{[]string{"NOTE: DIMENSIONS ARE IN FEET"}, "FT"},
		{[]string{"UNITS: MM"}, "MM"},
		{[]string{"DIMENSIONS ARE IN CM"}, "CM"},
		{[]string{"NOTE 1", "INTERPRET DRAWING IAW MIL-DTL-31000"}, ""},
		{[]string{"CUT 5 FT OF CABLE"}, ""},
	} {
		got := drawingUnitStatement([]Page{unitPage(1, c.lines...)})
		switch {
		case c.unit == "" && got != nil:
			t.Fatalf("%v: invented a unit %+v", c.lines, got)
		case c.unit != "" && (got == nil || got.Unit != c.unit):
			t.Fatalf("%v: got %+v, want %s", c.lines, got, c.unit)
		}
	}
	// two different statements are not a unit
	if got := drawingUnitStatement([]Page{unitPage(1, "DIMENSIONS ARE IN INCHES"), unitPage(2, "DIMENSIONS ARE IN MM")}); got != nil {
		t.Fatalf("conflicting statements must give no unit: %+v", got)
	}
	// the statement and its page are kept so the note can be pointed back to
	if got := drawingUnitStatement([]Page{unitPage(3, "DIM ARE IN INCHES")}); got.Page != 3 || got.Text == "" {
		t.Fatalf("page and text: %+v", got)
	}
}

func TestUnitFromTheDrawingIsAppliedToUnitlessDimensions(t *testing.T) {
	du := &DrawingUnit{Unit: "MM", Text: "DIMENSIONS ARE IN MM", Page: 1}
	got := applyDrawingUnit([]HarnessLength{{Label: "DIMENSION", Text: "5486", Tolerance: "-0.0/+25"}, {Label: "CUT", Text: "5 FT"}, {Label: "DIMENSION", Text: "48 IN"}}, du)
	if got[0].Text != "5486 MM" || got[0].UnitNote == "" || got[1].Text != "5 FT" || got[2].Text != "48 IN" {
		t.Fatalf("units applied wrongly: %+v", got)
	}
	if u := withUpperLengths(got)[0].Upper; u != "5511 MM" {
		t.Fatalf("upper in the drawing's unit: %q", u)
	}
	if n := applyDrawingUnit([]HarnessLength{{Label: "DIMENSION", Text: "216"}}, nil); n[0].Text != "216" {
		t.Fatal("without a stated unit nothing is added")
	}
	// millimetres convert for the cable-count check: 5486 mm is 216 in
	if c := lengthCandidatesInches([]HarnessLength{{Text: "5486.4 MM"}}); len(c) != 1 || fmt.Sprintf("%.1f", c[0]) != "216.0" {
		t.Fatalf("mm to inches: %v", c)
	}
}

func TestUnitStatementEndToEndAndTheMissingUnitAlert(t *testing.T) {
	text := func(x int, y float64, s string) string {
		return fmt.Sprintf("BT /F1 8 Tf 1 0 0 1 %d %.1f Tm (%s) Tj ET\n", x, y, s)
	}
	base := text(300, 380, "216") + text(330, 380, "-0.0/+1.0")
	with, err := viewAnalysis(onePagePDF(base+text(100, 100, "UNLESS OTHERWISE SPECIFIED")+text(100, 90, "DIM ARE IN INCHES"), "[0 0 1000 800]"))
	if err != nil {
		t.Fatal(err)
	}
	if with.DrawingUnit == nil || with.DrawingUnit.Unit != "IN" || with.DrawingUnit.Page != 1 {
		t.Fatalf("unit statement: %+v", with.DrawingUnit)
	}
	if len(with.HarnessLengths) != 1 || with.HarnessLengths[0].Text != "216 IN" || with.HarnessLengths[0].Upper != "217 IN" {
		t.Fatalf("lengths: %+v", with.HarnessLengths)
	}
	for _, i := range with.Issues {
		if i.Code == "LENGTH_UNIT_NOT_STATED" {
			t.Fatal("a stated unit must not raise the missing-unit alert")
		}
	}
	without, err := viewAnalysis(onePagePDF(base, "[0 0 1000 800]"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, i := range without.Issues {
		if i.Code == "LENGTH_UNIT_NOT_STATED" {
			found = true
		}
	}
	if without.DrawingUnit != nil || !found {
		t.Fatalf("no statement: unit %+v alert %v", without.DrawingUnit, found)
	}
}
