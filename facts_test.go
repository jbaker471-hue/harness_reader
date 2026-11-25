package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestFacts(t *testing.T) (*factStore, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "facts", "facts.json")
	s := &factStore{entries: map[string]KnownFact{}}
	s.load(p)
	return s, p
}

func TestFactsRoundTripAndKeyKeepsEveryCharacter(t *testing.T) {
	s, p := newTestFacts(t)
	if _, ok := s.positions("ABC-12"); ok {
		t.Fatal("an empty store must know nothing")
	}
	if err := s.setPositions("  abc-12 x ", 61); err != nil {
		t.Fatal(err)
	}
	for _, same := range []string{"ABC-12X", "abc-12 x", " ABC-12   X"} {
		if n, ok := s.positions(same); !ok || n != 61 {
			t.Errorf("%q should match the confirmed part, got %d %v", same, n, ok)
		}
	}
	// A dash, slash or letter is part of the identifier: no fuzzy matching.
	for _, other := range []string{"ABC12X", "ABC-12", "ABC-12Y", "D38999/ABC-12X"} {
		if _, ok := s.positions(other); ok {
			t.Errorf("%q must not match", other)
		}
	}
	s2 := &factStore{entries: map[string]KnownFact{}}
	s2.load(p)
	if n, ok := s2.positions("ABC-12X"); !ok || n != 61 {
		t.Errorf("fact must survive a reload, got %d %v", n, ok)
	}
	if err := s.setPositions("ABC-12X", 61); err != nil {
		t.Fatal(err)
	}
	if got := s.list(); len(got) != 1 || got[0].Confirmed != 2 {
		t.Errorf("confirming the same count twice is counted, got %+v", got)
	}
	if err := s.setPositions("ABC-12X", 128); err != nil {
		t.Fatal(err)
	}
	if got := s.list(); got[0].Value != 128 || got[0].Confirmed != 1 {
		t.Errorf("a corrected count replaces the old one, got %+v", got)
	}
	if err := s.forgetPositions("abc-12x"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.positions("ABC-12X"); ok {
		t.Error("forget must remove the fact")
	}
}

func TestFactsRejectBadInputAndBadFiles(t *testing.T) {
	s, p := newTestFacts(t)
	for _, c := range []struct {
		part string
		n    int
	}{{"", 10}, {"   ", 10}, {"X", 1}, {"X", 0}, {"X", -4}, {"X", 501}, {strings.Repeat("A", 81), 10}} {
		if err := s.setPositions(c.part, c.n); err == nil {
			t.Errorf("setPositions(%q,%d) must be refused", c.part, c.n)
		}
	}
	if len(s.list()) != 0 {
		t.Error("refused input must store nothing")
	}
	// A hand-edited or damaged file: bad entries are ignored, never guessed at.
	os.MkdirAll(filepath.Dir(p), 0o700)
	os.WriteFile(p, []byte(`[{"kind":"positions","key":"GOOD1","part":"GOOD1","value":37},{"kind":"positions","key":"BAD1","value":9999},{"kind":"other","key":"X","value":5},{"kind":"positions","key":"","value":5}]`), 0o600)
	s.load(p)
	if n, ok := s.positions("GOOD1"); !ok || n != 37 {
		t.Errorf("the valid entry must load, got %d %v", n, ok)
	}
	if len(s.list()) != 1 {
		t.Errorf("invalid entries must be dropped, got %+v", s.list())
	}
	os.WriteFile(p, []byte(`not json`), 0o600)
	s.load(p)
	if len(s.list()) != 0 || s.loadErr == "" {
		t.Errorf("an unreadable file loads as empty with the error kept, got %d / %q", len(s.list()), s.loadErr)
	}
	none := &factStore{entries: map[string]KnownFact{}}
	none.load("")
	if err := none.setPositions("X", 10); err == nil {
		t.Error("with no configuration folder nothing can be saved")
	}
	if _, ok := none.positions("X"); ok {
		t.Error("an unsaved fact must not be remembered")
	}
}

func TestFactsNotLoadedByTests(t *testing.T) {
	// The package-level store is never loaded at init, so a developer's real facts cannot reach any test.
	if len(facts.list()) != 0 || facts.path != "" {
		t.Fatalf("the global store must start empty and unloaded, got path %q with %d entries", facts.path, len(facts.list()))
	}
}

func withFacts(t *testing.T, s *factStore) {
	t.Helper()
	old := facts
	facts = s
	t.Cleanup(func() { facts = old })
}

func TestGetLayoutPriority(t *testing.T) {
	const unknownPart = "ACME-9000-37"
	s, _ := newTestFacts(t)
	withFacts(t, s)
	if n, st := getLayout(unknownPart); n != 0 || st != "UNKNOWN" {
		t.Fatalf("an unknown part has no count until the owner confirms one: %d %q", n, st)
	}
	if err := s.setPositions(unknownPart, 37); err != nil {
		t.Fatal(err)
	}
	n, st := getLayout(unknownPart)
	if n != 37 || !strings.HasPrefix(st, confirmedCountStatus) || !isStatedCount(st) {
		t.Errorf("the owner's confirmed count must be used and flagged as stated: %d %q", n, st)
	}
	if n2, st2 := getLayout("ACME-9000-38"); n2 != 0 || st2 != "UNKNOWN" {
		t.Errorf("a different part is unaffected, got %d %q", n2, st2)
	}
	if !isStatedCount(descCountLayout(9)) || isStatedCount("CATALOG COUNT; CAVITY MAP ASSUMED NUMERIC") || isStatedCount("UNKNOWN") || isStatedCount("") {
		t.Error("isStatedCount must be true only for stated or confirmed counts")
	}
	// A reviewed guide inventory is verified data and is never overridden by a remembered count.
	const guided = "D38999/26WG35PN"
	s.setPositions(guided, 12)
	if n, st := getLayout(guided); n != 79 || !strings.HasPrefix(st, "GUIDE INVENTORY") {
		t.Errorf("a reviewed guide inventory must win over a remembered count: %d %q", n, st)
	}
}

// End to end: a remembered count changes the reading of a real fixture, and forgetting restores it.
func TestFactsChangeTheReadingAndForgetRestoresIt(t *testing.T) {
	var data []byte
	var err error
	for _, name := range []string{"testdata/SYNTHETIC_V24_TABLE_NETLIST.pdf", "testdata/SYNTHETIC_RECOGNITION_TEST.pdf", "testdata/SYNTHETIC_ALTERNATE_LAYOUT.pdf"} {
		if data, err = os.ReadFile(name); err != nil {
			continue
		}
		x, e := ExtractDocument(data)
		if e != nil {
			continue
		}
		v, e := viewFromExtraction(x)
		if e != nil {
			continue
		}
		for _, c := range v.Connectors {
			if c.Part == "" {
				continue
			}
			if _, guide := guideInventory(c.Part); guide {
				continue
			}
			s, _ := newTestFacts(t)
			withFacts(t, s)
			want := c.Cavities + 7
			if want > factMaxValue || want < 2 {
				want = 37
			}
			s.setPositions(c.Part, want)
			x2, _ := ExtractDocument(data)
			v2, e := viewFromExtraction(x2)
			if e != nil {
				t.Fatal(e)
			}
			found := false
			for _, c2 := range v2.Connectors {
				if c2.Part == c.Part && c2.Name == c.Name {
					found = true
					if c2.Cavities != want {
						t.Errorf("the remembered count must be used: got %d want %d", c2.Cavities, want)
					}
				}
			}
			if !found {
				t.Fatal("connector disappeared after remembering a count")
			}
			s.forgetPositions(c.Part)
			x3, _ := ExtractDocument(data)
			v3, _ := viewFromExtraction(x3)
			for _, c3 := range v3.Connectors {
				if c3.Part == c.Part && c3.Name == c.Name && c3.Cavities != c.Cavities {
					t.Errorf("forgetting must restore the reading: got %d want %d", c3.Cavities, c.Cavities)
				}
			}
			return
		}
	}
	t.Skip("no synthetic fixture has a connector with a decoded count outside the guide inventory")
}
