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

// With no NC list at all, wired pins that run 1..N with every pin check passing are the whole connector.
func TestPositionsFromWiredPinsWithNoNCList(t *testing.T) {
	mk := func(wired []string, status string) CavityModel {
		a := Analysis{Connectors: []Connector{{Tag: "SMALL-P5", Part: "XX9P"}}}
		for _, w := range wired {
			a.Wires = append(a.Wires, WireRow{Connector: "SMALL-P5", Cavity: w, End: "BIG-J1", EndCavity: "1" + w})
		}
		a.TableAudits = []TableAudit{{Page: 3, Title: "pin numbers SMALL-P5", Connectors: []string{"SMALL-P5"}, Status: status, Counted: true}}
		for _, m := range buildCavityModels(&a, nil) {
			if m.Tag == "SMALL-P5" {
				return m
			}
		}
		t.Fatal("no model")
		return CavityModel{}
	}
	m := mk([]string{"1", "2", "3", "4"}, "COMPLETE")
	if !m.AllVerified || strings.Join(m.All, ",") != "1,2,3,4" || len(m.Unused) != 0 || !strings.Contains(m.AllSource, "none unused") {
		t.Fatalf("pins 1-4 all wired, no NC: want 4 positions and none unused: %+v", m)
	}
	// a gap: not proven, but the lowest possible count is kept and said
	m = mk([]string{"1", "2", "3", "5"}, "COMPLETE")
	if m.AllVerified || len(m.All) != 0 || m.AtLeast != 5 || !strings.Contains(m.AllSource, "at least 5") {
		t.Fatalf("a gap leaves the positions unproven with a lower bound of 5: %+v", m)
	}
	// a failing pin check: nothing is taken from the drawing
	m = mk([]string{"1", "2", "3", "4"}, "INCOMPLETE")
	if m.AllVerified || len(m.All) != 0 || m.AtLeast != 0 {
		t.Fatalf("a failing pin check gives nothing: %+v", m)
	}
	// upper-case letters A, B, C count the same way
	m = mk([]string{"A", "B", "C"}, "COMPLETE")
	if !m.AllVerified || strings.Join(m.All, ",") != "A,B,C" {
		t.Fatalf("pins A-C all wired: want 3 positions: %+v", m)
	}
	m = mk([]string{"A", "B", "D"}, "COMPLETE")
	if m.AllVerified || len(m.All) != 0 || m.AtLeast != 4 {
		t.Fatalf("a gap in the letters is a lower bound of 4: %+v", m)
	}
	// numbers mixed with letters, lower case, or doubled letters are not read as a sequence
	for _, wired := range [][]string{{"1", "A"}, {"a", "b"}, {"A", "BB"}} {
		if m = mk(wired, "COMPLETE"); m.AllVerified || len(m.All) != 0 || m.AtLeast != 0 {
			t.Fatalf("%v must give no count: %+v", wired, m)
		}
	}
}
