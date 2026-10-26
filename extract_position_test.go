package main

import "testing"

func TestSequentialPDFTextPositions(t *testing.T) {
	b := `1 0 0 1 90 400 Tm /F1 11 Tf (P3) Tj 85 0 Td (14) Tj 0 -18 Td (15) Tj`
	words := parseBlock(b)
	if len(words) != 3 {
		t.Fatalf("expected 3 positioned words: %+v", words)
	}
	if words[0].Text != "P3" || words[0].X != 90 || words[0].Y != 400 {
		t.Fatalf("first %+v", words[0])
	}
	if words[1].Text != "14" || words[1].X < 170 {
		t.Fatalf("relative move ignored: %+v", words[1])
	}
	if words[2].Text != "15" || words[2].Y != 382 {
		t.Fatalf("subsequent relative move ignored: %+v", words[2])
	}
}
func TestPositionResetBySubsequentTM(t *testing.T) {
	w := parseBlock(`1 0 0 1 50 600 Tm (A) Tj 1 0 0 1 300 400 Tm (B) Tj`)
	if len(w) != 2 || w[0].X != 50 || w[1].X != 300 || w[1].Y != 400 {
		t.Fatalf("Tm updates failed %+v", w)
	}
}
