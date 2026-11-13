package main

import (
	"strings"
	"testing"
)

func TestAnswerKeySaveLoadAndScore(t *testing.T) {
	t.Setenv("TRIVEX_ANSWER_DIR", t.TempDir())
	sha := strings.Repeat("ab", 32)
	k := AnswerKey{SHA256: sha, Conductors: 3, Connectors: []AnswerConnector{{Tag: "src-p1", Positions: 4, Wired: 3, Unused: 1}, {Tag: "DST-P2", Positions: 4, Wired: 3, Unused: 1}}}
	if err := saveAnswerKey(k); err != nil {
		t.Fatal(err)
	}
	got, ok := loadAnswerKey(sha)
	if !ok || got.Connectors[0].Tag != "SRC-P1" || len(listAnswerKeys()) != 1 {
		t.Fatalf("key not saved/loaded: %+v", got)
	}
	bad := k
	bad.Connectors = []AnswerConnector{{Tag: "X", Positions: 4, Wired: 3, Unused: 2}}
	if saveAnswerKey(bad) == nil {
		t.Fatal("wired + not used must equal positions")
	}
	v := ViewAnalysis{
		Connectors:   []ViewConnector{{Name: "SRC-P1", Cavities: 4}, {Name: "DST-P2", Cavities: 4}},
		CavityModels: []CavityModel{{Tag: "SRC-P1", All: []string{"1", "2", "3", "4"}, AllVerified: true, Wired: []string{"1", "2", "3"}}, {Tag: "DST-P2", All: []string{"1", "2", "3", "4"}, AllVerified: true, Wired: []string{"1", "2"}}},
		Wires:        []WireRow{{Connector: "SRC-P1", Cavity: "1", End: "DST-P2", EndCavity: "1"}, {Connector: "DST-P2", Cavity: "2", End: "SRC-P1", EndCavity: "2"}, {Connector: "SRC-P1", Cavity: "3", End: "DST-P2", EndCavity: "3"}},
	}
	s := scoreDrawing(v, got)
	line := s.Line(1)
	if s.Pass || s.WiredRight != 1 || s.UnusedRight != 1 || s.PositionsRight != 2 || s.CondRead != 3 {
		t.Fatalf("score wrong: %+v", s)
	}
	for _, leak := range []string{"SRC", "DST", sha[:8]} {
		if strings.Contains(line, leak) {
			t.Fatalf("score line must carry numbers only, found %q in %q", leak, line)
		}
	}
	v.CavityModels[1].Wired = []string{"1", "2", "3"}
	if s = scoreDrawing(v, got); !s.Pass {
		t.Fatalf("matching reading must pass: %+v", s)
	}
}
