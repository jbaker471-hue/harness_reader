package main

import (
	"strings"
	"testing"
)

// A connector with no catalog data: its positions come from the drawing when
// every pin-number check passes and wired + NC pins run 1..N with no gaps.
func TestPositionsFromWiredPlusNC(t *testing.T) {
	mk := func(wired []string, nc []string, status string) CavityModel {
		a := Analysis{Connectors: []Connector{{Tag: "SMALL-P5", Part: "XX9P"}}}
		for _, w := range wired {
			a.Wires = append(a.Wires, WireRow{Connector: "SMALL-P5", Cavity: w, End: "BIG-J1", EndCavity: "1" + w})
		}
		a.TableAudits = []TableAudit{{Page: 3, Title: "pin numbers SMALL-P5", Connectors: []string{"SMALL-P5"}, Status: status, Counted: true, NC: nc}}
		for _, m := range buildCavityModels(&a, nil) {
			if m.Tag == "SMALL-P5" {
				return m
			}
		}
		t.Fatal("no model")
		return CavityModel{}
	}
	m := mk([]string{"1", "2", "3", "4", "5", "7", "8"}, []string{"6", "9"}, "COMPLETE")
	if !m.AllVerified || strings.Join(m.All, ",") != "1,2,3,4,5,6,7,8,9" || strings.Join(m.Unused, ",") != "6,9" {
		t.Fatalf("want 9 positions with 6 and 9 unused: %+v", m)
	}
	// a gap (pin 6 neither wired nor NC): positions not proven
	m = mk([]string{"1", "2", "3", "4", "5", "7", "8"}, []string{"9"}, "COMPLETE")
	if m.AllVerified || len(m.All) != 0 || !strings.Contains(m.AllSource, "gap") {
		t.Fatalf("a gap must leave the positions unproven: %+v", m)
	}
	// a failing pin check: positions not taken from the drawing
	m = mk([]string{"1", "2"}, []string{"3", "4", "5", "6", "7", "8", "9"}, "INCOMPLETE")
	if m.AllVerified || len(m.All) != 0 {
		t.Fatalf("a failing pin check must not give positions: %+v", m)
	}
}
