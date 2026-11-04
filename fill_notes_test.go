package main

import (
	"strings"
	"testing"
)

func TestFillInstructionPhrasings(t *testing.T) {
	for _, tc := range []struct {
		text, policy, conns, finds, parts string
	}{
		{"INSTALL CONTACTS AND SEALING PLUGS IN ALL UNUSED CAVITIES.", "contact+plug", "", "", ""},
		{"UNUSED CAVITIES SHALL BE POPULATED WITH CONTACTS AND SEALING PLUGS PER FIND NO. 12.", "contact+plug", "", "12", ""},
		{"POPULATE ALL UNUSED CAVITIES WITH PINS AND MS27488-22-2 SEALING PLUGS.", "contact+plug", "", "", "MS27488-22-2"},
		{"UNUSED CONTACTS SHALL BE INSTALLED AND SEALED WITH SEALING PLUGS.", "contact+plug", "", "", ""},
		{"FILL UNUSED CAVITIES WITH M39029/58-360 CONTACTS AND PLUGS.", "contact+plug", "", "", "M39029/58-360"},
		{"SEAL UNUSED CAVITIES PER FIND NO. 3.", "plug-only", "", "3", ""},
		{"UNUSED CONTACT CAVITIES SHALL BE FILLED WITH SEALING PLUGS.", "plug-only", "", "", ""},
		{"SEALING PLUGS (ITEM 7) SHALL BE INSTALLED IN ALL UNUSED GROMMET HOLES.", "plug-only", "", "7", ""},
		{"INSTALL SEALING PLUGS IN UNUSED CAVITIES. DO NOT INSTALL CONTACTS IN UNUSED CAVITIES.", "plug-only", "", "", ""},
		{"CAVITIES NOT USED ON P1 AND J2 SHALL BE PLUGGED.", "plug-only", "J2,P1", "", ""},
		{"POPULATE UNUSED CAVITIES OF PLUG P3 WITH PINS.", "contact-only", "P3", "", ""},
		{"1. ALL CONTACTS PER M39029. SEAL UNUSED CAVITIES PER FIND NO. 3.", "plug-only", "", "3", ""},
	} {
		f, ok := parseFillInstruction(tc.text)
		if !ok || f.Policy != tc.policy || strings.Join(f.Connectors, ",") != tc.conns || strings.Join(f.FindNos, ",") != tc.finds || strings.Join(f.Parts, ",") != tc.parts {
			t.Errorf("%q: got ok=%v %+v; want %s conns=%q finds=%q parts=%q", tc.text, ok, f, tc.policy, tc.conns, tc.finds, tc.parts)
		}
	}
}

func TestFillInstructionIgnoresOtherNotes(t *testing.T) {
	for _, s := range []string{
		"ALL CONTACTS PER M39029.",
		"CRIMP CONTACTS PER MANUFACTURER INSTRUCTIONS.",
		"CONNECTOR PLUG P1 MATES WITH J1.",
		"UNUSED CAVITIES P1: 4, 7-9.", // which cavities, not what goes in them
		"INSTALL BACKSHELL PER FIND NO. 4.",
		"MARK HARNESS WITH PART NUMBER AND REVISION.",
	} {
		if f, ok := parseFillInstruction(s); ok {
			t.Errorf("%q is not a fill instruction, got %+v", s, f)
		}
	}
}

func TestFillPartsResolveThroughFindNumber(t *testing.T) {
	fs := []FillInstruction{{Policy: "plug-only", FindNos: []string{"3"}}, {Policy: "plug-only", FindNos: []string{"4"}}, {Policy: "contact+plug", Parts: []string{"MS27488-22-1"}}}
	bom := []BOMRow{{Item: "03", Part: "MS27488-22-2", Description: "PLUG, SEALING", Page: 3}, {Item: "4", Part: "M85049/38S13W", Description: "BACKSHELL", Page: 3}}
	got := resolveFillParts(fs, bom)
	if got[0].PlugPart != "MS27488-22-2" || !strings.Contains(got[0].PlugSource, "find no. 3") {
		t.Fatalf("find no. 3 should resolve to the sealing plug: %+v", got[0])
	}
	if got[1].PlugPart != "" {
		t.Fatalf("a find number that is not a plug must not become the plug: %+v", got[1])
	}
	if got[2].PlugPart != "MS27488-22-1" || got[2].PlugSource != "named in the note" {
		t.Fatalf("a plug named in the note must be used: %+v", got[2])
	}
}

func TestBuildPlanFollowsDrawingNotePolicy(t *testing.T) {
	// The request policy is contact+plug, but the drawing note for this connector says plugs only.
	c := planFixture()
	c.Policy, c.PolicySource = "plug-only", "drawing note 4 (sheet 1)"
	p, err := MakeBuildPlan(PlanRequest{BuildQty: 1, Policy: "contact+plug", Connectors: []PlanConnector{c}})
	if err != nil {
		t.Fatal(err)
	}
	cp := p.Connectors[0]
	if cp.Policy != "plug-only" || cp.PolicySource != "drawing note 4 (sheet 1)" {
		t.Fatalf("note policy not applied: %+v", cp)
	}
	for _, k := range cp.Kit {
		if strings.Contains(k.Purpose, "filler contacts") {
			t.Fatalf("plug-only note must not add filler contacts: %+v", cp.Kit)
		}
	}
	// contact-only: filler contacts, no plugs
	c.Policy = "contact-only"
	p, _ = MakeBuildPlan(PlanRequest{BuildQty: 1, Policy: "plug-only", Connectors: []PlanConnector{c}})
	filler, plugs := false, false
	for _, k := range p.Connectors[0].Kit {
		filler = filler || strings.Contains(k.Purpose, "filler contacts")
		plugs = plugs || strings.Contains(k.Purpose, "sealing plugs")
	}
	if !filler || plugs {
		t.Fatalf("contact-only note: want filler contacts and no plugs: %+v", p.Connectors[0].Kit)
	}
	// a plug named by the note is used, and a size that does not fit is flagged
	c.Policy, c.PlugPart = "plug-only", "MS27488-20-2"
	p, _ = MakeBuildPlan(PlanRequest{BuildQty: 1, Connectors: []PlanConnector{c}})
	cp = p.Connectors[0]
	named := false
	for _, k := range cp.Kit {
		named = named || k.Part == "MS27488-20-2"
	}
	if !named || !strings.Contains(strings.Join(cp.Issues, " "), "size 20") {
		t.Fatalf("named plug not used or size mismatch not flagged: %+v %v", cp.Kit, cp.Issues)
	}
}
