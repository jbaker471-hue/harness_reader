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

func TestFarEndPinForms(t *testing.T) {
	letters := []string{"A", "B", "C", "D", "E", "F"}
	for _, tc := range []struct {
		name string
		far  func(int) string
		want []string
	}{
		{"letters", func(i int) string { return letters[i] }, letters},
		{"doubled letters", func(i int) string { return letters[i] + letters[i] }, []string{"AA", "BB", "CC", "DD", "EE", "FF"}},
		{"lower-case letters", func(i int) string { return strings.ToLower(letters[i]) }, []string{"a", "b", "c", "d", "e", "f"}},
		{"leading zeros", func(i int) string { return fmt.Sprintf("%02d", i+1) }, []string{"1", "2", "3", "4", "5", "6"}},
		{"glued colour after", func(i int) string { return fmt.Sprintf("%dWHT", i+1) }, []string{"1", "2", "3", "4", "5", "6"}},
		{"glued colour before", func(i int) string { return fmt.Sprintf("BLU%d", i+1) }, []string{"1", "2", "3", "4", "5", "6"}},
	} {
		rows, why := positionalSchematicRows(pinFormPage(tc.far), nil, nil)
		got := []string{}
		for _, r := range rows {
			if r.Connector != "NEAR-P1" || r.End != "FAR-P2" {
				t.Fatalf("%s: wrong connectors %+v", tc.name, r)
			}
			got = append(got, r.EndCavity)
		}
		if why != "" || strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("%s: got %v (%q), want %v", tc.name, got, why, tc.want)
		}
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
	for _, s := range []string{"AWG", "WHT", "BK", "TO", "I", "O", "1000", "0", "SHLD"} {
		if p, ok := pinToken(s); ok {
			t.Errorf("%q is not a contact label, read as %q", s, p)
		}
	}
}
