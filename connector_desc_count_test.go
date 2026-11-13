package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestPositionCountIsReadOnlyWhenTheDescriptionStatesOne(t *testing.T) {
	for _, c := range []struct {
		desc string
		want int
	}{
		{"CONNECTOR, PLUG, 37 POS, D-SUB", 37},
		{"RECEPTACLE 15 P, SOLDER CUP", 15},
		{"D38999 CONNECTOR 128P PLUG MALE, ALTERNATE KEYED", 128},
		{"HEADER 9 POSITIONS, 2.54MM", 9},
		{"SOCKET HOUSING, 12 WAY", 12},
		{"CONNECTOR, 9 POS, 22 AWG CONTACTS", 9},               // a gauge is not a count
		{"CONNECTOR, 9 POS, 9 PIN", 9},                        // the same count twice is one statement
		{"CONNECTOR, 9 POS OR 15 POS", 0},                     // two different counts: not a statement
		{"CABLE, 24 AWG, 2 COND, SHIELDED", 0},                // not a connector
		{"CABLE ASSEMBLY, 4 CONDUCTORS, PLUG ENDS", 0},        // conductors are not positions
		{"CONNECTOR, CRIMP, SEE DRAWING", 0},                  // nothing stated
		{"CONNECTOR, 1 POS", 0},                               // a single position is not a count to trust
		{"CONNECTOR BACKSHELL, SIZE 25, 90 DEGREE", 0},        // a size is not a count
		{"PLUG 600 POS", 0},                                   // beyond any real connector
		{"CONNECTOR 24C", 0},                                  // C is circuits on some parts and conductors on others: not read
	} {
		got, ok := positionsFromDescription(c.desc)
		if (c.want == 0) == ok || got != c.want {
			t.Fatalf("%q: got %d ok=%v, want %d", c.desc, got, ok, c.want)
		}
	}
}

func TestStatedPositionsReachTheConnectorAndNeverInventLetterLabels(t *testing.T) {
	// numbered pins with gaps and no NC list: the parts list settles the count
	a := Analysis{Connectors: []Connector{{Tag: "TEST-J2", Part: "EX-9000", CavityCount: 15, LayoutStatus: descCountLayout(15)}}}
	for _, p := range []string{"1", "2", "3", "7", "14"} {
		a.Wires = append(a.Wires, WireRow{Connector: "TEST-J2", Cavity: p, End: "OTHER-P1", EndCavity: p})
	}
	var m CavityModel
	for _, x := range buildCavityModels(&a, nil) {
		if x.Tag == "TEST-J2" {
			m = x
		}
	}
	if len(m.All) != 15 || m.AllVerified || !strings.Contains(m.AllSource, "ASSUMED") || m.AtLeast != 0 {
		t.Fatalf("a stated count of 15 gives 15 unverified positions: %+v", m)
	}
	// lettered pins: the count is kept on the connector, but no 1..N list is made up
	b := Analysis{Connectors: []Connector{{Tag: "TEST-J3", Part: "EX-9001", CavityCount: 55, LayoutStatus: descCountLayout(55)}}}
	for _, p := range []string{"A", "B", "C", "a"} {
		b.Wires = append(b.Wires, WireRow{Connector: "TEST-J3", Cavity: p, End: "OTHER-P1", EndCavity: "1"})
	}
	for _, x := range buildCavityModels(&b, nil) {
		if x.Tag == "TEST-J3" && (len(x.All) != 0 || !strings.Contains(x.AllSource, "lettered")) {
			t.Fatalf("lettered pins must not get numbered cavities: %+v", x)
		}
	}
}

// End to end through the PDF reader: a parts list that states "15 POS" gives the connector its count.
func TestPartsListPositionCountReachesTheConnector(t *testing.T) {
	text := func(x int, y float64, s string) string {
		return fmt.Sprintf("BT /F1 8 Tf 1 0 0 1 %d %.1f Tm (%s) Tj ET\n", x, y, s)
	}
	content := text(55, 770, "BENCH-J2") + text(790, 770, "TEST-P1")
	for i, pin := range []string{"1", "2", "3", "7"} {
		y := 720 - float64(i)*40
		content += text(55, y, pin) + text(72, y, "WHT") + text(420, y, "24") + text(440, y, "AWG") + text(765, y, "WHT") + text(790, y, pin) + text(820, y, "SIG")
	}
	content += text(70, 330, "ITEM") + text(150, 330, "PART NUMBER") + text(350, 330, "DESCRIPTION") + text(560, 330, "REF DES") + text(650, 330, "QTY")
	content += text(70, 310, "1") + text(150, 310, "EX-9000") + text(350, 310, "CONNECTOR, RECEPTACLE, 15 POS") + text(560, 310, "BENCH-J2") + text(650, 310, "1")
	content += text(70, 290, "2") + text(150, 290, "EX-9001") + text(350, 290, "CONNECTOR, PLUG, 9 POS") + text(560, 290, "TEST-P1") + text(650, 290, "1")
	v, err := viewAnalysis(onePagePDF(content, "[0 0 1000 800]"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, c := range v.Connectors {
		got[c.Name] = c.Cavities
	}
	if got["BENCH-J2"] != 15 || got["TEST-P1"] != 9 {
		t.Fatalf("stated counts did not reach the connectors: %+v", got)
	}
}
