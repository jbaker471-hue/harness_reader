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

func TestConnectorLibraryRowsAreSourceLinkedAndNeverVerified(t *testing.T) {
	rows, err := connectorDocTable()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 50 {
		t.Fatalf("only %d rows", len(rows))
	}
	seen := map[string]bool{}
	for _, r := range rows {
		k := r.Kind + "|" + r.Code
		if seen[k] {
			t.Fatalf("duplicate row %s", k)
		}
		seen[k] = true
		if strings.Contains(strings.ToUpper(r.Status), "VERIFIED") && !strings.Contains(r.Status, "UNVERIFIED") {
			t.Fatalf("supplied row claims verification: %+v", r)
		}
		if len(r.Sources) == 0 && !strings.Contains(r.Status, "NO SOURCE URL") {
			t.Fatalf("row without a source or an explicit no-source status: %+v", r)
		}
		for _, s := range r.Sources {
			if !strings.HasPrefix(s, "https://") {
				t.Fatalf("source is not https: %q", s)
			}
		}
		for _, l := range r.Layout {
			if lookupConnectorDoc("layout-standard", l) == nil {
				t.Fatalf("%s names layout standard %q that is not in the library", r.Code, l)
			}
		}
	}
	if lookupConnectorDoc("layout-standard", "MIL-STD-1560") == nil || lookupConnectorDoc("tooling-spec", "SAE-AS81969") == nil {
		t.Fatal("priority 1 documents missing")
	}
}

func TestFamilyCarriesSpecAndLayoutStandard(t *testing.T) {
	d := DecodeConnectorFamily("D38999/26FD35PN")
	if len(d.Specs) != 1 || d.Specs[0].Code != "MIL-DTL-38999" || len(d.Layouts) != 1 || d.Layouts[0].Code != "MIL-STD-1560" || len(d.Layouts[0].Sources) == 0 {
		t.Fatalf("%+v", d)
	}
	if !strings.Contains(strings.Join(d.Notes, " "), "No reviewed insert layout") {
		t.Fatalf("missing-layout note absent: %v", d.Notes)
	}
	if d := DecodeConnectorFamily("M83723/71G1210N"); len(d.Layouts) != 2 {
		t.Fatalf("83723 maps to two layout standards: %+v", d.Layouts)
	}
}

func geoRow(arr, cav, x, y, view, src, page, status string) []string {
	return []string{arr, "MIL-STD-1560", "9", cav, x, y, "22D", view, "normal", src, page, "fig", status}
}

func TestInsertGeometryRowsAreChecked(t *testing.T) {
	good := [][]string{geoRow("9-35", "1", "0", "-3", "pin-front", "S", "5", "REVIEWED"), geoRow("9-35", "2", "3", "0", "pin-front", "S", "5", "REVIEWED")}
	g, p := buildInsertGeometry(good, "MIL-STD-1560", "9-35")
	if g == nil || len(p) != 0 || len(g.Positions) != 2 || g.View != "pin-front" || !strings.Contains(g.Basis, "page 5") {
		t.Fatalf("%+v %v", g, p)
	}
	if g, p := buildInsertGeometry(good, "MIL-STD-1560", "11-35"); g != nil || len(p) != 0 {
		t.Fatalf("other arrangement must be absent, not an error: %+v %v", g, p)
	}
	for name, rows := range map[string][][]string{
		"duplicate":  {geoRow("9-35", "1", "0", "0", "pin-front", "S", "5", "R"), geoRow("9-35", "1", "1", "1", "pin-front", "S", "5", "R")},
		"bad number": {geoRow("9-35", "1", "x", "0", "pin-front", "S", "5", "R")},
		"bad view":   {geoRow("9-35", "1", "0", "0", "top", "S", "5", "R")},
		"no source":  {geoRow("9-35", "1", "0", "0", "pin-front", "", "5", "R")},
		"mixed view": {geoRow("9-35", "1", "0", "0", "pin-front", "S", "5", "R"), geoRow("9-35", "2", "1", "0", "socket-front", "S", "5", "R")},
		"no status":  {geoRow("9-35", "1", "0", "0", "pin-front", "S", "5", "")},
	} {
		if g, p := buildInsertGeometry(rows, "MIL-STD-1560", "9-35"); g != nil || len(p) == 0 {
			t.Fatalf("%s: a bad row must reject the whole arrangement: %+v %v", name, g, p)
		}
	}
}

func TestShippedInsertGeometryLibraryIsValid(t *testing.T) {
	rows, err := readLibrary("CONNECTOR_INSERT_GEOMETRY.csv")
	if err != nil {
		t.Fatal(err)
	}
	arr := map[string]bool{}
	for _, r := range rows[1:] {
		arr[r[1]+"|"+r[0]] = true
	}
	for k := range arr {
		p := strings.SplitN(k, "|", 2)
		if g, probs := buildInsertGeometry(rows[1:], p[0], p[1]); g == nil || len(probs) != 0 {
			t.Fatalf("shipped arrangement %s is invalid: %v", k, probs)
		}
	}
}
