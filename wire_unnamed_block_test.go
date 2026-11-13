package main

import (
	"fmt"
	"strings"
	"testing"
)

// Four drawn wires from a named connector's pins to a block of pin numbers that carries no name anywhere.
func unnamedFarBlockPDF(namedLeft bool) []byte {
	text := func(x int, y float64, s string) string {
		return fmt.Sprintf("BT /F1 8 Tf 1 0 0 1 %d %.1f Tm (%s) Tj ET\n", x, y, s)
	}
	content := ""
	if namedLeft {
		content += text(140, 720, "TEST-J1")
	}
	for i := 0; i < 4; i++ {
		y := 650 - float64(i)*40
		content += text(150, y, fmt.Sprint(i+1))
		content += fmt.Sprintf("0.5 w 175 %.1f m 640 %.1f l S\n", y+3, y+3)
		content += text(380, y+6, "24 AWG")
		content += text(655, y, fmt.Sprint(i+1))
	}
	return onePagePDF(content, "[0 0 800 800]")
}

func TestWireToAnUnnamedBlockStillGivesTheNamedConnectorsPin(t *testing.T) {
	v, err := viewAnalysis(unnamedFarBlockPDF(true))
	if err != nil {
		t.Fatal(err)
	}
	pins := map[string]bool{}
	for _, w := range v.Wires {
		if w.Connector == "TEST-J1" {
			pins[w.Cavity] = true
			if w.End != "" || !strings.Contains(strings.ToLower(w.EndSignal), "no printed name") || w.Method != "traced-wire-unnamed-block" {
				t.Fatalf("a wire to an unnamed block is a one-ended row that says so: %+v", w)
			}
		}
	}
	if len(pins) != 4 || !pins["1"] || !pins["4"] {
		t.Fatalf("every pin of the named connector has a wire: %+v", v.Wires)
	}
}

func TestNoWireWhenNeitherEndHasANamedConnector(t *testing.T) {
	v, err := viewAnalysis(unnamedFarBlockPDF(false))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range v.Wires {
		if w.Method == "traced-wire-unnamed-block" {
			t.Fatalf("with no named connector at either end nothing attaches: %+v", w)
		}
	}
}
