package main

import (
	"strings"
	"testing"
)

func TestHeaderPartDisagreementIsGraded(t *testing.T) {
	conns := []Connector{
		{Tag: "RIG-P5", Part: "D38999/26WJ35PN", CavityCount: 128},
		{Tag: "BOX-P5", Part: "D38999/26WJ35SN", CavityCount: 128},
	}
	for _, tc := range []struct{ header, code string }{
		{"D38999/26WJ35PN", ""},                        // same
		{"D38999 / 26WJ35PN", ""},                      // formatting
		{"M38999/26WJ35PN", ""},                        // leading M
		{"D38999/26WJ35PN-LC", ""},                     // suffix added on one side
		{"D38999/26WJ35SN", "CONNECTOR_PART_MISMATCH"}, // the other connector's part
		{"D38999/26FJ35PA", "CONNECTOR_PART_MISMATCH"}, // same insert J35P, finish/key differ
		{"D38999/26WJ35SA", "CONNECTOR_PART_MISMATCH"}, // pins vs sockets
		{"D38999/26WG35PN", "CONNECTOR_PART_MISMATCH"}, // different insert
	} {
		f, ok := headerPartFinding(conns, conns[0], tc.header, 4)
		if tc.code == "" {
			if ok {
				t.Errorf("%s: want no finding, got %s", tc.header, f.Code)
			}
			continue
		}
		if !ok || f.Code != tc.code || f.Severity != "ERROR" || f.Connector != "RIG-P5" || f.Observed == "" || f.Expected == "" {
			t.Errorf("%s: want %s with connector and both parts, got ok=%v %+v", tc.header, tc.code, ok, f)
		}
	}
	f, _ := headerPartFinding(conns, conns[0], "D38999/20WJ35PN", 4)
	if !strings.Contains(f.Message, "insert J35P") || !strings.Contains(f.Message, "not") {
		t.Errorf("same insert, different slash sheet: message should say the layout is the same but the connector is not: %s", f.Message)
	}
	f, _ = headerPartFinding(conns, conns[0], "D38999/26WJ35SN", 4)
	if !strings.Contains(f.Message, "BOX-P5") {
		t.Errorf("header part of another connector should be named: %s", f.Message)
	}
	// mating halves, the other one listed without a designator
	conns2 := []Connector{{Tag: "RIG-J5", Part: "D38999/20WJ35SN"}, {Tag: "UNASSIGNED", Part: "D38999/26WJ35PN"}}
	f, ok := headerPartFinding(conns2, conns2[0], "D38999/26WJ35PN", 4)
	if !ok || f.Severity != "ERROR" || !strings.Contains(f.Message, "mating halves") || strings.Contains(f.Message, "of UNASSIGNED") || !strings.Contains(f.Message, "designator was not read") {
		t.Errorf("mating-half message wrong: %+v", f)
	}
}
