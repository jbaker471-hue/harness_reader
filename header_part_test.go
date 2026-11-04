package main

import "testing"

func TestHeaderPartDisagreementIsGraded(t *testing.T) {
	conns := []Connector{
		{Tag: "RIG-P5", Part: "D38999/26WJ35PN", CavityCount: 128},
		{Tag: "BOX-P5", Part: "D38999/26WJ35SN", CavityCount: 128},
	}
	for _, tc := range []struct{ header, code string }{
		{"D38999/26WJ35PN", ""},                              // same
		{"D38999 / 26WJ35PN", ""},                            // formatting
		{"M38999/26WJ35PN", ""},                              // leading M
		{"D38999/26WJ35PN-LC", ""},                           // suffix added on one side
		{"D38999/26WJ35SN", "CONNECTOR_PART_PAIRING_REVIEW"}, // the other connector's part
		{"D38999/26FJ35PA", "CONNECTOR_PART_SUFFIX_DIFFERS"}, // same insert J35P, finish/key differ
		{"D38999/26WJ35SA", "CONNECTOR_PART_MISMATCH"},       // pins vs sockets
		{"D38999/26WG35PN", "CONNECTOR_PART_MISMATCH"},       // different insert
	} {
		f, ok := headerPartFinding(conns, conns[0], tc.header, 4)
		if tc.code == "" {
			if ok {
				t.Errorf("%s: want no finding, got %s", tc.header, f.Code)
			}
			continue
		}
		if !ok || f.Code != tc.code || f.Connector != "RIG-P5" || f.Observed == "" || f.Expected == "" {
			t.Errorf("%s: want %s with connector and both parts, got ok=%v %+v", tc.header, tc.code, ok, f)
		}
	}
}
