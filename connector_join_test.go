package main

import "testing"

func identityWires(tag string, pins ...string) []WireRow {
	out := []WireRow{}
	for _, p := range pins {
		out = append(out, WireRow{Connector: tag, Cavity: p})
	}
	return out
}

func TestSplitConnectorRecordsAreJoined(t *testing.T) {
	a := Analysis{
		Connectors: []Connector{
			{Tag: "W1-Q5", Pages: []int{2}},
			{Tag: "Q5", Part: "NINE-POS-1", CavityCount: 9, Family: "FAM", Pages: []int{1}},
			{Tag: "W1-Q6", Part: "OWN-1", CavityCount: 4, Pages: []int{2}},
		},
		Wires: append(identityWires("W1-Q5", "1", "2", "3"), identityWires("W1-Q6", "1")...),
	}
	got := joinSplitConnectors(&a)
	if len(got) != 1 || got[0] != [2]string{"Q5", "W1-Q5"} {
		t.Fatalf("joined pairs: %v", got)
	}
	if len(a.Connectors) != 2 {
		t.Fatalf("the short record must be gone: %+v", a.Connectors)
	}
	m := a.Connectors[0]
	if m.Tag != "W1-Q5" || m.Part != "NINE-POS-1" || m.CavityCount != 9 || m.Family != "FAM" || len(m.Pages) != 2 {
		t.Fatalf("the wired record must take the part, positions and pages: %+v", m)
	}
	// a second run changes nothing
	if again := joinSplitConnectors(&a); len(again) != 0 || len(a.Connectors) != 2 {
		t.Fatalf("joining twice must be a no-op: %v %+v", again, a.Connectors)
	}
}

func TestSplitConnectorJoinNeedsACertainMatch(t *testing.T) {
	short := Connector{Tag: "Q5", Part: "NINE-POS-1", CavityCount: 9}
	cases := map[string]Analysis{
		"two wired records end in the short name": {
			Connectors: []Connector{{Tag: "W1-Q5"}, {Tag: "W2-Q5"}, short},
			Wires:      append(identityWires("W1-Q5", "1"), identityWires("W2-Q5", "1")...),
		},
		"the wired record has its own part": {
			Connectors: []Connector{{Tag: "W1-Q5", Part: "OWN-1"}, short},
			Wires:      identityWires("W1-Q5", "1"),
		},
		"the wired record has its own positions": {
			Connectors: []Connector{{Tag: "W1-Q5", CavityCount: 4}, short},
			Wires:      identityWires("W1-Q5", "1"),
		},
		"the short record has wires of its own": {
			Connectors: []Connector{{Tag: "W1-Q5"}, short},
			Wires:      append(identityWires("W1-Q5", "1"), identityWires("Q5", "2")...),
		},
		"more wired cavities than the short record has positions": {
			Connectors: []Connector{{Tag: "W1-Q5"}, {Tag: "Q5", Part: "TWO-POS-1", CavityCount: 2}},
			Wires:      identityWires("W1-Q5", "1", "2", "3"),
		},
		"the longer record has no wires": {
			Connectors: []Connector{{Tag: "W1-Q5"}, short},
		},
		"the short record has no resolved part": {
			Connectors: []Connector{{Tag: "W1-Q5"}, {Tag: "Q5", Part: "NOT RESOLVED", CavityCount: 9}},
			Wires:      identityWires("W1-Q5", "1"),
		},
		"the short record has no position count": {
			Connectors: []Connector{{Tag: "W1-Q5"}, {Tag: "Q5", Part: "NINE-POS-1"}},
			Wires:      identityWires("W1-Q5", "1"),
		},
		"the name only contains the short name, it does not end in it": {
			Connectors: []Connector{{Tag: "Q5-W1"}, {Tag: "XQ5"}, short},
			Wires:      append(identityWires("Q5-W1", "1"), identityWires("XQ5", "1")...),
		},
		"no connectors at all": {},
	}
	for name, a := range cases {
		n := len(a.Connectors)
		if got := joinSplitConnectors(&a); len(got) != 0 || len(a.Connectors) != n {
			t.Errorf("%s: nothing may be joined, got %v", name, got)
		}
	}
}

func TestSplitConnectorJoinCountsFarEndsAndRepeatedPinsOnce(t *testing.T) {
	// the wired record is named only as the far end of its wires, and one pin carries two wires
	a := Analysis{
		Connectors: []Connector{{Tag: "w1-q5"}, {Tag: "Q5", Part: "TWO-POS-1", CavityCount: 2}},
		Wires: []WireRow{
			{Connector: "SRC", Cavity: "1", End: "W1-Q5", EndCavity: "1"},
			{Connector: "SRC", Cavity: "2", End: "W1-Q5", EndCavity: "1"},
			{Connector: "SRC", Cavity: "3", End: "W1-Q5", EndCavity: "2"},
		},
	}
	if got := joinSplitConnectors(&a); len(got) != 1 || a.Connectors[0].CavityCount != 2 {
		t.Fatalf("three wires on two pins fit a two-position connector: %v %+v", got, a.Connectors)
	}
}
