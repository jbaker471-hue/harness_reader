package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestShieldFloatAndSensorLeadNotesAreReportedAsCategoriesOnly(t *testing.T) {
	text := func(x int, y float64, s string) string {
		return fmt.Sprintf("BT /F1 8 Tf 1 0 0 1 %d %.1f Tm (%s) Tj ET\n", x, y, s)
	}
	has := func(content, want string) bool {
		v, err := viewAnalysis(onePagePDF(content, "[0 0 1000 800]"))
		if err != nil {
			t.Fatal(err)
		}
		return strings.Contains(","+strings.Join(v.AssemblyFeatures, ",")+",", ","+want+",")
	}
	notes := text(60, 700, "NOTES:") + text(60, 680, "1. FLOAT ALL SHIELDS AT THE SENSOR END.") + text(60, 660, "2. SOLDER RTD LEADS TO THE HARNESS WIRES.")
	if !has(notes, "SHIELD_FLOAT") || !has(notes, "SENSOR_LEADS") || !has(notes, "SOLDERING") {
		t.Fatal("float, sensor lead and solder notes must each give their category")
	}
	plain := text(60, 700, "NOTES:") + text(60, 680, "1. MARK PER EXAMPLE-STD-1.") + text(60, 660, "2. PART NUMBER EX-SHIRTDX-1.")
	if has(plain, "SHIELD_FLOAT") || has(plain, "SENSOR_LEADS") {
		t.Fatal("no float or sensor note: no category (RTD inside another word does not count)")
	}
}
