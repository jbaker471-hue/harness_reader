package main

import (
	"strings"
	"testing"
)

func decodeOrFail(t *testing.T, pn string) WireDecode {
	t.Helper()
	d, err := DecodeWirePart(pn)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func fieldByLabel(d WireDecode, label string) *WireField {
	for i := range d.Fields {
		if d.Fields[i].Label == label {
			return &d.Fields[i]
		}
	}
	return nil
}

func TestM27500DecodesSuppliedExamples(t *testing.T) {
	cases := []struct {
		pn, gauge, comp, spec, shield, jacket string
		conductors                            int
	}{
		{"M27500-22RC2S06", "22", "Silver-coated copper conductor with PTFE insulation", "AS22759/11", "Single silver-coated copper braid", "White PTFE", 2},
		{"M27500-24RC2S06", "24", "Silver-coated copper conductor with PTFE insulation", "AS22759/11", "Single silver-coated copper braid", "White PTFE", 2},
		{"M27500-20RC2S06", "20", "Silver-coated copper conductor with PTFE insulation", "AS22759/11", "Single silver-coated copper braid", "White PTFE", 2},
		{"M27500-22SB2T23", "22", "Crosslinked ETFE insulation (conductor plating not supplied)", "AS22759/32", "Single tin-coated copper braid", "White crosslinked ETFE", 2},
		{"m27500-22te2t14", "22", "Tin-coated copper conductor with ETFE insulation", "AS22759/16", "Single tin-coated copper braid", "White ETFE", 2},
	}
	for _, c := range cases {
		d := decodeOrFail(t, c.pn)
		if !d.Recognized || !d.Decoded || d.Gauge != c.gauge || d.Conductors != c.conductors {
			t.Fatalf("%s: %+v", c.pn, d)
		}
		for label, want := range map[string]string{"Component wire": c.comp, "Shield": c.shield, "Outer jacket": c.jacket} {
			if f := fieldByLabel(d, label); f == nil || f.Value != want {
				t.Fatalf("%s %s = %+v, want %q", c.pn, label, f, want)
			}
		}
		if f := fieldByLabel(d, "Component wire"); f.Spec != c.spec || len(f.Sources) == 0 || !strings.Contains(f.Status, "UNVERIFIED") {
			t.Fatalf("%s component not source-linked/unverified: %+v", c.pn, f)
		}
		if len(d.Unresolved) != 0 {
			t.Fatalf("%s unexpected unresolved %v", c.pn, d.Unresolved)
		}
	}
}

func TestM27500SuppliedSPExampleLeavesConstructionUnstated(t *testing.T) {
	d := decodeOrFail(t, "M27500-22SP2S23")
	f := fieldByLabel(d, "Component wire")
	if f == nil || f.Spec != "AS22759/43" || !strings.Contains(f.Value, "construction not supplied") {
		t.Fatalf("SP must cite its slash sheet without inventing a construction: %+v", f)
	}
}

func TestUnknownCodesAreUnresolvedNotGuessed(t *testing.T) {
	d := decodeOrFail(t, "M27500-22ZZ2Q99")
	if !d.Decoded || d.Conductors != 2 || d.Gauge != "22" {
		t.Fatalf("structure should still decode: %+v", d)
	}
	if len(d.Unresolved) != 3 {
		t.Fatalf("want component, shield and jacket unresolved, got %v", d.Unresolved)
	}
	for _, l := range []string{"Component wire", "Shield", "Outer jacket"} {
		if f := fieldByLabel(d, l); f == nil || f.Value != "UNRESOLVED" || len(f.Sources) != 0 {
			t.Fatalf("%s must be UNRESOLVED with no source: %+v", l, f)
		}
	}
}

func TestNoTwistOrShieldScopeAssumptions(t *testing.T) {
	d := decodeOrFail(t, "M27500-22RC2S06")
	notes := strings.Join(d.Notes, " ")
	for _, want := range []string{"Do not assume the conductors are a twisted pair", "individually shielded"} {
		if !strings.Contains(notes, want) {
			t.Fatalf("missing caution %q in %q", want, notes)
		}
	}
}

func TestOtherFamiliesAndSlashSheets(t *testing.T) {
	for pn, family := range map[string]string{
		"M22759/16-22-9":        "SAE AS22759",
		"AS22759/11-22-9":       "SAE AS22759",
		"M16878/4-22-9":         "MIL-DTL-16878",
		"M25038/3-22":           "MIL-DTL-25038",
		"MIL-DTL-24643/64":      "MIL-DTL-24643",
		"MIL-DTL-24640/15":      "MIL-DTL-24640",
		"RG-58C/U":              "MIL-DTL-17",
		"M81044/9-22-9":         "SAE AS81044",
		"AS85485-22":            "SAE AS85485",
		"NEMA WC27500-22RC2S06": "M27500 / NEMA WC27500",
	} {
		d := decodeOrFail(t, pn)
		if !d.Recognized || d.Family != family || len(d.FamilySources) == 0 {
			t.Fatalf("%s -> %+v, want family %s", pn, d, family)
		}
	}
	d := decodeOrFail(t, "MIL-DTL-24643/64")
	if f := fieldByLabel(d, "Slash sheet"); f == nil || !strings.Contains(f.Value, "LSTPSJ") {
		t.Fatalf("slash sheet not reported: %+v", d.Fields)
	}
	if got := decodeOrFail(t, "D38999/26FJ35SN"); got.Recognized {
		t.Fatalf("connector must not read as wire: %+v", got)
	}
	if got := decodeOrFail(t, ""); got.Recognized || len(got.Notes) == 0 {
		t.Fatalf("empty part: %+v", got)
	}
}

func TestAS22759DirectDecodeAndMissingSlash(t *testing.T) {
	d := decodeOrFail(t, "M22759/16-22-9")
	if f := fieldByLabel(d, "Slash sheet"); f == nil || f.Spec != "AS22759/16" || d.Gauge != "22" {
		t.Fatalf("%+v", d)
	}
	if len(d.Unresolved) == 0 || !strings.Contains(d.Unresolved[0], "Color code 9") {
		t.Fatalf("color code must stay undecoded: %v", d.Unresolved)
	}
	d = decodeOrFail(t, "M22759/99-22-9")
	if f := fieldByLabel(d, "Slash sheet"); f == nil || f.Value != "UNRESOLVED" {
		t.Fatalf("slash sheet absent from the library must be unresolved: %+v", d.Fields)
	}
}

func TestWireSpecLibraryRowsAreSourceLinkedAndNeverVerified(t *testing.T) {
	rows, err := wireSpecTable()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 40 {
		t.Fatalf("only %d rows", len(rows))
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if len(r.Sources) == 0 {
			t.Fatalf("row without a source: %+v", r)
		}
		for _, s := range r.Sources {
			if !strings.HasPrefix(s, "https://") {
				t.Fatalf("source is not an https URL: %q", s)
			}
		}
		if !strings.Contains(r.Status, "UNVERIFIED") {
			t.Fatalf("supplied row must not claim verification: %+v", r)
		}
		k := r.Kind + "|" + r.Code
		if seen[k] {
			t.Fatalf("duplicate row %s", k)
		}
		seen[k] = true
	}
	if a := lookupWireCode(rows, "family", "85485"); a == nil || !strings.Contains(a.Note, "Not a general arc-resistant") {
		t.Fatalf("AS85485 must not be described as an arc-resistant wire standard: %+v", a)
	}
}

func constructionOf(t *testing.T, pn, text string) CableConstruction {
	t.Helper()
	d, err := DecodeWireLine(pn, text)
	if err != nil || d.Construction == nil {
		t.Fatalf("%s: %v %+v", pn, err, d)
	}
	return *d.Construction
}

func TestTwistedShieldedPairConstruction(t *testing.T) {
	c := constructionOf(t, "M27500-22RC2S06", "CABLE, TWISTED SHIELDED PAIR, 22 AWG")
	if c.SignalConductors != 2 || c.Groups.Value != "1 pair" || c.Twist.Value != "twisted" || c.ShieldPresent != "yes" {
		t.Fatalf("%+v", c)
	}
	if !strings.Contains(c.OverallShield.Value, "silver-coated copper braid") || !strings.Contains(c.OverallShield.Status, "UNVERIFIED") {
		t.Fatalf("shield must come from the part number and stay unverified: %+v", c.OverallShield)
	}
	if c.IndividualShields.Status != attrFromText || !strings.Contains(c.IndividualShields.Value, "single group") {
		t.Fatalf("%+v", c.IndividualShields)
	}
	if c.Drain.Status != attrNotStated || c.Drain.Value != "" {
		t.Fatalf("drain must not be assumed: %+v", c.Drain)
	}
	if !strings.Contains(strings.Join(c.Notes, " "), "does not use a connector cavity") {
		t.Fatalf("shield must be called out as not a signal conductor: %v", c.Notes)
	}
}

func TestPartNumberAloneStatesNeitherTwistNorPairing(t *testing.T) {
	c := constructionOf(t, "M27500-22RC2S06", "CABLE")
	if c.Twist.Status != attrNotStated || c.Groups.Status != attrNotStated || c.Twist.Value != "" {
		t.Fatalf("twist and pairing must be NOT STATED: %+v", c)
	}
	if !strings.Contains(strings.Join(c.Notes, " "), "Do not assume the conductors are twisted pairs") {
		t.Fatalf("%v", c.Notes)
	}
}

func TestMultiPairOverallShieldDoesNotImplyIndividualShields(t *testing.T) {
	c := constructionOf(t, "M27500-22RC6S06", "CABLE, 3 TWISTED PAIRS, OVERALL SHIELD")
	if c.Groups.Value != "3 pairs" || c.SignalConductors != 6 {
		t.Fatalf("%+v", c)
	}
	if c.IndividualShields.Status != attrNotStated || !strings.Contains(strings.Join(c.Notes, " "), "Do not assume an overall shield means individual shields") {
		t.Fatalf("individual shielding must stay unstated: %+v %v", c.IndividualShields, c.Notes)
	}
	c = constructionOf(t, "M27500-22RC6S06", "CABLE, 3 PAIRS, INDIVIDUALLY SHIELDED, OVERALL SHIELD, DRAIN WIRE")
	if c.IndividualShields.Status != attrFromText || c.Drain.Value != "drain wire" || c.SignalConductors != 6 {
		t.Fatalf("drain is not a signal conductor and individual shields are stated: %+v", c)
	}
}

func TestConstructionConflictsAreFlagged(t *testing.T) {
	c := constructionOf(t, "M27500-22RC4S06", "CABLE, 3 TWISTED PAIRS")
	if !strings.Contains(strings.Join(c.Notes, " "), "CONFLICT") {
		t.Fatalf("pair count versus conductor count conflict not flagged: %v", c.Notes)
	}
	c = constructionOf(t, "M27500-22RC2S06", "CABLE, UNSHIELDED PAIR")
	if c.ShieldPresent != "yes" || !strings.Contains(strings.Join(c.Notes, " "), "CONFLICT") {
		t.Fatalf("unshielded text versus shield code: %+v", c)
	}
}

func TestUnshieldedCodeAndUnknownShield(t *testing.T) {
	if c := constructionOf(t, "M27500-22RC2U06", "CABLE"); c.ShieldPresent != "no" || c.OverallShield.Value != "none" {
		t.Fatalf("%+v", c)
	}
	if c := constructionOf(t, "M27500-22RC2Q06", "CABLE"); c.ShieldPresent != "unknown" {
		t.Fatalf("unknown shield code must stay unknown: %+v", c)
	}
	if d, _ := DecodeWireLine("D38999/26FJ35SN", "CONNECTOR"); d.Construction != nil {
		t.Fatal("connectors have no cable construction")
	}
}
