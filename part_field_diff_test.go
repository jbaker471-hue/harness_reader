package main

import (
	"strings"
	"testing"
)

func TestPartFieldDiffNamesTheFieldsThatDiffer(t *testing.T) {
	for _, c := range []struct{ a, b, want string }{
		{"D38999/26FJ35SN", "D38999/26FJ35SA", "the keying letter"},
		{"D38999/26FJ35SN", "D38999/24FJ35SN", "the slash sheet (connector style)"},
		{"D38999/26FJ35SN", "D38999/24FJ35PN", "the slash sheet (connector style) and the contact type"},
		{"D38999/26FJ35SN", "D38999/26WJ35SN", "the finish class"},
		{"D38999/26FJ35SN", "D38999/26FJ35SN", ""},
		{"D38999/26FJ35SN", "EX-1000", ""},
	} {
		if got := partFieldDiff(c.a, c.b); got != c.want {
			t.Fatalf("%s vs %s: %q, want %q", c.a, c.b, got, c.want)
		}
	}
	conns := []Connector{{Tag: "TEST-J1", Part: "D38999/26FJ35SN"}}
	f, ok := headerPartFinding(conns, conns[0], "D38999/26FJ35SA", 4)
	if !ok || !strings.Contains(f.Message, "differ only in the keying letter") {
		t.Fatalf("the finding must name the differing field: %v %q", ok, f.Message)
	}
}
