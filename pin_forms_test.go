package main

import (
	"fmt"
	"strings"
	"testing"
)

// A NON-CUI two-connector sheet whose far-end pins are printed in a given form.
func pinFormPage(far func(i int) string) Page {
	p := Page{Number: 9, Width: 1200, Height: 1100, PageAccurate: true}
	add := func(t string, x, y float64) {
		p.Words = append(p.Words, Word{Text: t, X: x, Y: y, Size: 6, X1: x + 3.6*float64(len(t))})
	}
	add("NEAR-P1", 120, 1010)
	add("FAR-P2", 830, 1010)
	for i := 0; i < 6; i++ {
		y := 980 - 15*float64(i)
		add(fmt.Sprint(i+1), 150, y)
		add("24", 500, y)
		add("AWG", 512, y)
		add(far(i), 840, y)
	}
	return p
}

// Contact forms other than plain numbers are NOT read (they matched ordinary
// text on a field drawing); the reason names their shape so the format can be
// added deliberately, with a test, once it is known.
func TestFarEndPinFormsAreDescribedNotRead(t *testing.T) {
	letters := []string{"A", "B", "C", "D", "E", "F"}
	for _, tc := range []struct {
		name  string
		far   func(int) string
		shape string
	}{
		{"letters", func(i int) string { return letters[i] }, "6 single letter"},
		{"doubled letters", func(i int) string { return letters[i] + letters[i] }, "6 doubled letter"},
		{"leading zeros", func(i int) string { return fmt.Sprintf("%02d", i+1) }, "6 2-digit number with leading zero"},
		{"glued colour after", func(i int) string { return fmt.Sprintf("%dWHT", i+1) }, "6 number glued to a colour word"},
		{"glued colour before", func(i int) string { return fmt.Sprintf("BLU%d", i+1) }, "6 colour word glued to a number"},
	} {
		rows, why := positionalSchematicRows(pinFormPage(tc.far), nil, nil)
		if len(rows) != 0 || !strings.Contains(why, "that side holds "+tc.shape) {
			t.Errorf("%s: want nothing read and the shape %q named, got %d rows (%q)", tc.name, tc.shape, len(rows), why)
		}
	}
	rows, why := positionalSchematicRows(pinFormPage(func(i int) string { return fmt.Sprint(i + 1) }), nil, nil)
	if len(rows) != 6 || why != "" {
		t.Fatalf("plain numbers must still be read: %d (%q)", len(rows), why)
	}
}

func TestUnknownPinFormIsDescribedByShapeOnly(t *testing.T) {
	rows, why := positionalSchematicRows(pinFormPage(func(i int) string { return fmt.Sprintf("FAR-%d", i+1) }), nil, nil)
	if len(rows) != 0 {
		t.Fatalf("an unknown pin form must not be read: %+v", rows)
	}
	if !strings.Contains(why, "6 label(s) have no pin on one side (6 on the right); that side holds 6 hyphenated code") {
		t.Fatalf("reason should describe the far side by shape: %q", why)
	}
	if strings.Contains(why, "FAR") {
		t.Fatalf("reason must not quote drawing text: %q", why)
	}
}

func TestPinTokenRejectsWordsAndColours(t *testing.T) {
	for _, s := range []string{"AWG", "WHT", "BK", "TO", "I", "O", "1000", "0", "SHLD", "A", "D", "AA", "01", "12WHT"} {
		if p, ok := pinToken(s); ok {
			t.Errorf("%q is not a contact label, read as %q", s, p)
		}
	}
}
