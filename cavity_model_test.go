package main

import (
	"strings"
	"testing"
)

func modelFor(t *testing.T, pdf []byte, tag string) (CavityModel, ViewAnalysis) {
	v, err := viewAnalysis(pdf)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range v.CavityModels {
		if m.Tag == tag {
			return m, v
		}
	}
	t.Fatalf("no cavity model for %s: %+v", tag, v.CavityModels)
	return CavityModel{}, v
}

func TestCavitiesUsedAndUnusedFromTheDrawing(t *testing.T) {
	m, v := modelFor(t, BuildSyntheticFaceViews(true), "P3")
	j := func(s []string) string { return strings.Join(s, ",") }
	if j(m.All) != "1,2,3,4,5,6,7,8" {
		t.Fatalf("cavity set %v (%s)", m.All, m.AllSource)
	}
	if !m.AllVerified || !strings.Contains(m.AllSource, "face view drawn on sheet 1") {
		t.Fatalf("cavity set should come from the drawn face view: %s", m.AllSource)
	}
	if j(m.Wired) != "1,2,3,5,6" || j(m.Unused) != "4,7,8" {
		t.Fatalf("wired %v unused %v", m.Wired, m.Unused)
	}
	sortCavities(m.SolidMarked)
	sortCavities(m.OpenMarked)
	if j(m.SolidMarked) != "1,2,3,5,6" || j(m.OpenMarked) != "4,7,8" {
		t.Fatalf("marks solid %v open %v", m.SolidMarked, m.OpenMarked)
	}
	ex := map[string]int{}
	for _, u := range m.ExplicitUnused {
		ex[u.Cavity]++
	}
	if ex["4"] != 2 || ex["7"] != 2 || ex["8"] != 1 { // pin-table row + note, once each
		t.Fatalf("explicit unused (spare rows + note) %+v", m.ExplicitUnused)
	}
	if len(m.Views) != 2 || !strings.HasPrefix(m.MirrorCheck, "OK") {
		t.Fatalf("views %d mirror %q", len(m.Views), m.MirrorCheck)
	}
	if len(m.Conflicts) != 0 {
		t.Fatalf("unexpected conflicts %v", m.Conflicts)
	}
	for _, w := range v.Wires {
		if w.Connector == "P3" && (w.Cavity == "4" || w.Cavity == "7") {
			t.Fatalf("spare/N-C row treated as a conductor: %+v", w)
		}
	}
	// Far-end connector J7 also gets its wired cavities.
	j7, _ := modelFor(t, BuildSyntheticFaceViews(true), "J7")
	if j(j7.Wired) != "1,2,3,5,6" {
		t.Fatalf("J7 wired %v", j7.Wired)
	}
}

func TestRearViewNotMirroredIsAConflict(t *testing.T) {
	m, _ := modelFor(t, BuildSyntheticFaceViews(false), "P3")
	if !strings.HasPrefix(m.MirrorCheck, "CONFLICT") || len(m.Conflicts) == 0 {
		t.Fatalf("identical front/rear views not flagged: %q %v", m.MirrorCheck, m.Conflicts)
	}
}

func TestParseCavityList(t *testing.T) {
	if got := strings.Join(parseCavityList("4, 7-9 & 12 THRU 14; A"), ","); got != "4,7,8,9,12,13,14,A" {
		t.Fatalf("got %s", got)
	}
}

func TestMissingRecognitionDoesNotDeclareUnused(t *testing.T) {
 cases := []struct {
  name string
  signal, wire, color, awg, end, pin string
  unused bool
 }{
  {"missing fields", "", "", "", "", "", "", false},
  {"signal without wire ID", "CONTROL", "", "", "", "", "", false},
  {"wired spare", "SPARE", "WIRE-EXAMPLE", "", "", "", "", false},
  {"colored spare", "SPARE", "", "BLU", "", "", "", false},
  {"gauged spare", "SPARE", "", "", "24", "", "", false},
  {"spare with destination", "SPARE", "", "", "", "J9", "2", false},
  {"explicit spare", "SPARE", "-", "", "", "-", "", true},
  {"explicit unused destination", "", "", "", "", "N/C", "", true},
  {"unknown dash destination", "", "", "", "", "-", "", false},
  {"signal contradicts destination", "CONTROL", "", "", "", "N/C", "", false},
 }
 for _, tc := range cases {t.Run(tc.name, func(t *testing.T) {
  row := map[string]string{"signal":tc.signal,"wire":tc.wire,"color":tc.color,"awg":tc.awg,"to":tc.end,"topin":tc.pin}
  if got:=notConnectedRow(row); got!=tc.unused {t.Fatalf("table unused=%v want %v",got,tc.unused)}
  end, endSignal := "UNRESOLVED ENDPOINT",tc.end
  if tc.end=="J9" {end,endSignal=tc.end,""}
  rows, unused:=splitUnusedRows([]WireRow{{Connector:"P8",Cavity:"a",Signal:tc.signal,Wire:tc.wire,Color:tc.color,AWG:tc.awg,End:end,EndSignal:endSignal,EndCavity:tc.pin}})
  if (len(unused)==1)!=tc.unused {t.Fatalf("fallback unused=%+v want %v",unused,tc.unused)}
  if len(rows)+len(unused)!=1 {t.Fatal("row lost or duplicated")}
 })}
}
