package main

import (
	"strings"
	"testing"
)

func TestConnectorFamilyFromPartNumber(t *testing.T) {
	cases := map[string]string{
		"D38999/26FD35PN": "circular", "MS3106A18-1S": "circular", "M83723/71G1210N": "circular",
		"M83513/01-A01N": "micro-d", "DT06-2S": "rectangular", "M24308/2-1": "d-sub", "DB25P": "d-sub", "DE9S": "d-sub",
		"ZZZ-123": "unknown", "": "unknown",
	}
	for pn, shape := range cases {
		if got := DecodeConnectorFamily(pn); got.Shape != shape {
			t.Fatalf("%q shape = %q, want %q (%+v)", pn, got.Shape, shape, got)
		}
	}
	if got := DecodeConnectorFamily("ZZZ-123"); got.Recognized || got.Geometry != nil || len(got.Notes) == 0 {
		t.Fatalf("unknown family must not be guessed: %+v", got)
	}
}

func TestSeriesIIIReadsShellSizeSexCountButNoPositions(t *testing.T) {
	d := DecodeConnectorFamily("D38999/26FD35PN")
	if d.ShellSize != "15" || d.Positions != 37 || d.Sex != "PIN" || d.Geometry != nil {
		t.Fatalf("%+v", d)
	}
	if !strings.Contains(d.Status, "UNVERIFIED") {
		t.Fatalf("family must not read as verified: %q", d.Status)
	}
}

func TestDSubStandardLayouts(t *testing.T) {
	for _, c := range []struct {
		pn     string
		n, top int
	}{{"DE9P", 9, 5}, {"DA15S", 15, 8}, {"DB25P", 25, 13}, {"DC37P", 37, 19}} {
		d := DecodeConnectorFamily(c.pn)
		if d.Geometry == nil || len(d.Geometry.Positions) != c.n || d.Positions != c.n {
			t.Fatalf("%s: %+v", c.pn, d)
		}
		seen := map[string]bool{}
		topRow := 0
		for _, p := range d.Geometry.Positions {
			if seen[p.Label] {
				t.Fatalf("%s duplicate label %s", c.pn, p.Label)
			}
			seen[p.Label] = true
			if p.Y < 0 {
				topRow++
			}
		}
		if topRow != c.top {
			t.Fatalf("%s top row has %d, want %d", c.pn, topRow, c.top)
		}
		if !strings.Contains(d.Geometry.Status, "UNVERIFIED") {
			t.Fatalf("computed layout must stay unverified")
		}
	}
}

func TestDSubWithoutSexOrWithWrongShellDrawsNoPositions(t *testing.T) {
	if d := DecodeConnectorFamily("DB25"); d.Geometry != nil {
		t.Fatalf("no sex stated, orientation unknown: %+v", d)
	}
	d := DecodeConnectorFamily("DE25P")
	if d.Geometry != nil || !strings.Contains(strings.Join(d.Notes, " "), "CONFLICT") {
		t.Fatalf("shell letter conflict not flagged: %+v", d)
	}
	if d := DecodeConnectorFamily("DD50P"); d.Geometry != nil {
		t.Fatalf("three-row shell layout is not modelled: %+v", d)
	}
}
