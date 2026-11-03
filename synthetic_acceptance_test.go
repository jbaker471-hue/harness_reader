package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyntheticAcceptancePack(t *testing.T) {
	cases := []struct {
		file       string
		wires      int
		bomMinimum int
		text       []string
		features   []string
		noBOM      bool
	}{
		{"01_clean_two_connector.pdf", 4, 4, []string{"ASSEMBLE PER IPC/WHMA-A-620", "VERIFY CONTINUITY"}, nil, false},
		{"02_banded_backshell_shield_bond.pdf", 2, 6, []string{"TWISTED SHIELDED PAIR", "SHIELD BAND", "BACKSHELL"}, []string{"SHIELD_PREPARATION"}, false},
		{"03_float_and_solder_sleeve.pdf", 2, 5, []string{"FLOAT THE C2 SHIELD", "SOLDER SLEEVE", "CONTACT 9"}, []string{"SHIELD_PREPARATION", "SOLDERING"}, false},
		{"04_clamp_and_crimp_ring.pdf", 4, 6, []string{"CLAMP BACKSHELL", "CRIMP RING TOOL", "DOCUMENTED AS EQUIVALENT"}, []string{"TERMINAL_CRIMPING"}, false},
		{"05_cots_branch_endpoint.pdf", 1, 5, []string{"PURCHASED COMMERCIAL CABLE ASSEMBLY", "CB-P3", "NOT A CONNECTOR"}, nil, false},
		{"06_conflict_and_missing_data.pdf", 2, 0, []string{"FLOAT THE C3 SHIELD", "TERMINATE THE C3 SHIELD TO THE BACKSHELL", "LENGTH IS NOT STATED"}, []string{"SHIELD_PREPARATION"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join("output", "pdf", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			v, err := viewAnalysis(b)
			if err != nil {
				t.Fatal(err)
			}
			codes := make([]string, 0, len(v.Issues))
			for _, issue := range v.Issues {
				codes = append(codes, issue.Code)
			}
			t.Logf("pages=%d words=%d connectors=%d wires=%d bom=%d notes=%d features=%v issues=%v interpretation=%s", v.Pages, v.Words, len(v.Connectors), len(v.Wires), len(v.Boms), len(v.DrawingNotes), v.AssemblyFeatures, codes, v.Interpretation)
			if v.Pages != 2 {
				t.Errorf("pages=%d, want 2", v.Pages)
			}
			if len(v.Connectors) != 2 {
				t.Errorf("connectors=%d, want exactly 2: %#v", len(v.Connectors), v.Connectors)
			}
			for _, c := range v.Connectors {
				if strings.EqualFold(c.Name, "CB-P3") {
					t.Errorf("branch label CB-P3 was incorrectly promoted to a connector")
				}
			}
			if len(v.Wires) != tc.wires {
				t.Errorf("wires=%d, want %d: %#v", len(v.Wires), tc.wires, v.Wires)
			}
			if len(v.Boms) < tc.bomMinimum {
				t.Errorf("bom=%d, want at least %d: %#v", len(v.Boms), tc.bomMinimum, v.Boms)
			}
			if tc.noBOM && len(v.Boms) != 0 {
				t.Errorf("intentionally absent BOM produced %d rows", len(v.Boms))
			}
			featureText := strings.Join(v.AssemblyFeatures, ",")
			if strings.Contains(featureText, "LABELING") {
				t.Errorf("SCHEMATIC LABELS heading must not create a physical labeling operation")
			}
			for _, want := range tc.features {
				if !strings.Contains(featureText, want) {
					t.Errorf("assembly features missing %q: %v", want, v.AssemblyFeatures)
				}
			}
			noteText := make([]string, 0, len(v.DrawingNotes))
			for _, note := range v.DrawingNotes {
				noteText = append(noteText, note.Text)
			}
			upper := strings.ToUpper(strings.Join(noteText, " "))
			for _, want := range tc.text {
				if !strings.Contains(upper, want) {
					t.Errorf("drawing note register missing %q; notes=%#v", want, v.DrawingNotes)
				}
			}
		})
	}
}
