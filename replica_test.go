package main

import (
	"sort"
	"strings"
	"testing"
)

// End-to-end regression on a NON-CUI replica of a real drawing's layout:
// D38999 128-position plug to plug, wiring diagrams on three sheets, an
// upward parts list, flag notes. Known answer: 86 wired, 42 unused per end.
func TestReplicaDrawingKnownAnswer(t *testing.T) {
	d := buildReplicaDrawing()
	x, err := ExtractDocument(d.pdf)
	if err != nil {
		t.Fatal(err)
	}
	v, err := viewFromExtraction(x)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Wires) != 86 {
		t.Fatalf("wires read = %d, want 86", len(v.Wires))
	}
	want := map[string]bool{}
	for _, u := range d.used {
		want[itoa(u)] = true
	}
	got := map[string]map[string]bool{"RIG-P5": {}, "BOX-P5": {}}
	for _, w := range v.Wires {
		if w.Connector != "RIG-P5" || w.End != "BOX-P5" || w.Cavity != w.EndCavity {
			t.Fatalf("wire %+v: want RIG-P5:n to BOX-P5:n", w)
		}
		got["RIG-P5"][w.Cavity], got["BOX-P5"][w.EndCavity] = true, true
	}
	for k, s := range got {
		if len(s) != 86 {
			t.Fatalf("%s wired cavities = %d", k, len(s))
		}
		for c := range want {
			if !s[c] {
				t.Fatalf("%s: pin %s not read", k, c)
			}
		}
	}
	wantUnused := []string{}
	for _, u := range d.unused {
		wantUnused = append(wantUnused, itoa(u))
	}
	for _, m := range v.CavityModels {
		u := append([]string(nil), m.Unused...)
		sortCavities(u)
		if strings.Join(u, ",") != strings.Join(wantUnused, ",") {
			t.Fatalf("%s unused = %v, want %v", m.Tag, u, wantUnused)
		}
	}
	if len(v.CavityModels) != 2 {
		t.Fatalf("cavity models: %d", len(v.CavityModels))
	}
	if len(v.TableAudits) != 3 {
		t.Fatalf("want one completeness audit per wiring sheet, got %+v", v.TableAudits)
	}
	for _, a := range v.TableAudits {
		if a.Status != "COMPLETE" || a.Title != "wiring diagram" {
			t.Fatalf("audit %+v", a)
		}
	}
	if len(v.FillInstructions) != 1 || v.FillInstructions[0].Policy != "either" || v.FillInstructions[0].Note != "10" {
		t.Fatalf("note 10 must be read as either: %+v", v.FillInstructions)
	}
	statuses := map[string]string{}
	for _, c := range v.CaptureChecks {
		statuses[c.ID+" "+c.Connector] = c.Status
	}
	for _, k := range []string{"CONDUCTOR_COUNT ", "UNUSED_CAVITIES RIG-P5", "UNUSED_CAVITIES BOX-P5"} {
		if statuses[k] != CheckAgree {
			t.Fatalf("%s = %q, want AGREE (%v)", k, statuses[k], statuses)
		}
	}
}

func TestReplicaDrawingCatchesUnreadWireLines(t *testing.T) {
	for _, mode := range []string{"ambiguous", "onesided"} {
		d := buildReplicaVariant(mode)
		x, err := ExtractDocument(d.pdf)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := viewFromExtraction(x)
		incomplete := []string{}
		for _, a := range v.TableAudits {
			if a.Status == "INCOMPLETE" && len(a.Missed) == 1 {
				incomplete = append(incomplete, itoa(a.Page))
			}
		}
		sort.Strings(incomplete)
		wantSheet := map[string]string{"ambiguous": "3", "onesided": "4"}[mode]
		if strings.Join(incomplete, ",") != wantSheet {
			t.Fatalf("%s: want exactly one missed line on sheet %s, audits %+v", mode, wantSheet, v.TableAudits)
		}
		disagree := 0
		for _, c := range v.CaptureChecks {
			if (c.ID == "CONDUCTOR_COUNT" || c.ID == "UNUSED_CAVITIES") && c.Status == CheckDisagree {
				disagree++
			}
		}
		if disagree != 3 {
			t.Fatalf("%s: conductor and both empty-cavity counts must disagree, got %d", mode, disagree)
		}
	}
}
