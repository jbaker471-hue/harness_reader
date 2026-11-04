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
	if len(v.TableAudits) != 6 {
		t.Fatalf("want a line audit and a gauge-label count per wiring sheet, got %+v", v.TableAudits)
	}
	for _, a := range v.TableAudits {
		if a.Status != "COMPLETE" || (a.Title != "wiring diagram" && a.Title != "wire gauge labels") {
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

// A conductor listed for a pin counts whatever its colour word: four-colour
// cables, no colour at all, unfamiliar colour words, a stray extra pin label.
func TestReplicaDrawingReadsWiresWithoutRelyingOnColour(t *testing.T) {
	for _, mode := range []string{"quad", "nocolour", "oddcolour", "ambiguous", "quadnobom", "nosheet3", "nosheet3nobom"} {
		d := buildReplicaVariant(mode)
		x, err := ExtractDocument(d.pdf)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := viewFromExtraction(x)
		if len(v.Wires) != len(d.wires) {
			t.Fatalf("%s: read %d of %d wires", mode, len(v.Wires), len(d.wires))
		}
		for _, w := range v.Wires {
			if w.Cavity != w.EndCavity {
				t.Fatalf("%s: wrong pairing %+v", mode, w)
			}
		}
		for _, m := range v.CavityModels {
			if len(m.Unused) != len(d.unused) {
				t.Fatalf("%s: %s unused %d, want %d", mode, m.Tag, len(m.Unused), len(d.unused))
			}
		}
		for _, a := range v.TableAudits {
			if a.Status != "COMPLETE" {
				t.Fatalf("%s: %+v", mode, a)
			}
		}
	}
}

// Wires that cannot be read are never silently turned into unused cavities.
func TestReplicaDrawingCatchesUnreadWireLines(t *testing.T) {
	for _, tc := range []struct{ mode, want string }{
		{"onesided", "wire gauge labels 4,wiring diagram 4"},
		{"noheader3", "wire gauge labels 3"},
		{"noheader3nobom", "wire gauge labels 3"},
	} {
		d := buildReplicaVariant(tc.mode)
		x, err := ExtractDocument(d.pdf)
		if err != nil {
			t.Fatal(err)
		}
		v, _ := viewFromExtraction(x)
		bad := []string{}
		for _, a := range v.TableAudits {
			if a.Status == "INCOMPLETE" {
				if len(a.Missed) == 0 || len(a.Connectors) != 2 {
					t.Fatalf("%s: incomplete audit must list the missed lines and both connectors: %+v", tc.mode, a)
				}
				bad = append(bad, a.Title+" "+itoa(a.Page))
			}
		}
		sort.Strings(bad)
		if strings.Join(bad, ",") != tc.want {
			t.Fatalf("%s: incomplete audits %v, want %s", tc.mode, bad, tc.want)
		}
		disagree := 0
		for _, c := range v.CaptureChecks {
			if (c.ID == "CONDUCTOR_COUNT" || c.ID == "UNUSED_CAVITIES") && c.Status == CheckDisagree {
				disagree++
			}
		}
		if strings.Contains(tc.mode, "nobom") {
			continue // no parts list: the gauge-label count alone must hold it
		}
		if disagree != 3 {
			t.Fatalf("%s: conductor and both empty-cavity counts must disagree, got %d", tc.mode, disagree)
		}
	}
}

// A sheet whose designators cannot be read is matched by the part number in
// its header, and said so; the same part labelled two ways holds both.
func TestReplicaDrawingConnectorHeaders(t *testing.T) {
	d := buildReplicaVariant("nosheet3")
	x, _ := ExtractDocument(d.pdf)
	v, _ := viewFromExtraction(x)
	review := false
	for _, i := range v.Issues {
		review = review || (i.Code == "CONNECTOR_MATCHED_BY_PART" && len(i.Pages) == 1 && i.Pages[0] == 3)
	}
	if len(v.Wires) != len(d.wires) || !review {
		t.Fatalf("nosheet3: want every wire read and a review note for sheet 3: wires %d, issues %+v", len(v.Wires), v.Issues)
	}
	d = buildReplicaVariant("typo3")
	x, _ = ExtractDocument(d.pdf)
	v, _ = viewFromExtraction(x)
	held := false
	for _, a := range v.TableAudits {
		if a.Title == "connector header" && a.Status == "INCOMPLETE" && strings.Join(a.Connectors, ",") == "R1G-P5,RIG-P5" {
			held = true
		}
	}
	if !held {
		t.Fatalf("typo3: a part labelled with two designators must hold both: %+v", v.TableAudits)
	}
}
