package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCompareRatchetDirections(t *testing.T) {
	base := map[string]int{"wires": 10, "issues_error": 2, "connectors": 4, "bom_tables": 1}
	// better, worse and neutral moves
	cur := map[string]int{"wires": 12, "issues_error": 3, "connectors": 5, "bom_tables": 1}
	ch := compareRatchet(base, cur)
	got := map[string]ratchetChange{}
	for _, c := range ch {
		got[c.Metric] = c
	}
	if got["wires"].Regress {
		t.Error("more wires must not be a regression")
	}
	if !got["issues_error"].Regress {
		t.Error("more error issues must be a regression")
	}
	if got["connectors"].Regress {
		t.Error("a watched metric never fails")
	}
	if _, ok := got["bom_tables"]; ok {
		t.Error("an unchanged metric is not reported")
	}
	if c := compareRatchet(base, map[string]int{"wires": 9, "issues_error": 2, "connectors": 4, "bom_tables": 1}); len(c) != 1 || !c[0].Regress {
		t.Errorf("fewer wires must be one regression, got %+v", c)
	}
	// a metric missing from the baseline (older baseline) is not a regression
	if c := compareRatchet(map[string]int{}, cur); len(c) != 0 {
		t.Errorf("metrics absent from the baseline must be ignored, got %+v", c)
	}
}

func TestMergeRatchetNeverLowersTheBarSilently(t *testing.T) {
	base := map[string]int{"wires": 10, "issues_error": 2, "connectors": 4}
	cur := map[string]int{"wires": 8, "issues_error": 1, "connectors": 6, "bom_tables": 2}
	m := mergeRatchet(base, cur, false)
	if m["wires"] != 10 {
		t.Errorf("a drop in wires must not lower the baseline without accept, got %d", m["wires"])
	}
	if m["issues_error"] != 1 {
		t.Errorf("fewer error issues is an improvement and is kept, got %d", m["issues_error"])
	}
	if m["connectors"] != 6 {
		t.Errorf("a watched metric follows the reading, got %d", m["connectors"])
	}
	if m["bom_tables"] != 2 {
		t.Errorf("a new metric is recorded, got %d", m["bom_tables"])
	}
	if a := mergeRatchet(base, cur, true); a["wires"] != 8 {
		t.Errorf("accept takes the regression on purpose, got %d", a["wires"])
	}
}

func TestRatchetFileRoundTripAndOrdinals(t *testing.T) {
	p := filepath.Join(t.TempDir(), "r", "baseline.json")
	f := ratchetFile{Drawings: map[string]map[string]int{"bbbb": {"wires": 2}, "aaaa": {"wires": 1}}}
	if err := saveRatchet(p, f); err != nil {
		t.Fatal(err)
	}
	g := loadRatchet(p)
	if g.Drawings["aaaa"]["wires"] != 1 || g.Drawings["bbbb"]["wires"] != 2 {
		t.Fatalf("round trip lost data: %+v", g)
	}
	o := ratchetOrder(g)
	if o["aaaa"] != 1 || o["bbbb"] != 2 {
		t.Fatalf("ordinals must follow sorted checksums: %+v", o)
	}
	if loadRatchet(filepath.Join(t.TempDir(), "missing.json")).Drawings == nil {
		t.Error("a missing baseline must load as empty, not nil")
	}
}

// Metrics from the synthetic fixtures are deterministic, non-trivial, and the
// report line never carries anything but a drawing number, a metric name and numbers.
func TestDrawingMetricsDeterministicOnSyntheticFixtures(t *testing.T) {
	ents, _ := os.ReadDir("testdata")
	checked := 0
	line := regexp.MustCompile(`^drawing \d+: [a-z_]+ -?\d+ -> -?\d+ \((better|WORSE|changed)\)$`)
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".pdf") {
			continue
		}
		b, err := os.ReadFile(filepath.Join("testdata", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		read := func() map[string]int {
			x, err := ExtractDocument(b)
			if err != nil {
				t.Skip("fixture not readable here: " + e.Name())
			}
			v, err := viewFromExtraction(x)
			if err != nil {
				t.Skip("fixture not analysable here: " + e.Name())
			}
			return drawingMetrics(v)
		}
		a, c := read(), read()
		for _, rm := range ratchetMetrics {
			if a[rm.Name] != c[rm.Name] {
				t.Errorf("%s: metric %s is not deterministic (%d vs %d)", e.Name(), rm.Name, a[rm.Name], c[rm.Name])
			}
		}
		if a["pages"] < 1 {
			t.Errorf("%s: no pages counted", e.Name())
		}
		for _, ch := range compareRatchet(map[string]int{"wires": a["wires"] + 1, "issues_error": a["issues_error"] - 1}, a) {
			if !line.MatchString(ch.Line(1)) {
				t.Errorf("report line has unexpected content: %q", ch.Line(1))
			}
		}
		checked++
	}
	if checked == 0 {
		t.Skip("no fixtures")
	}
}
