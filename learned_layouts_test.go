package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withTempLayouts(t *testing.T) string {
	p := filepath.Join(t.TempDir(), "layouts.json")
	learned.mu.Lock()
	oldPath, oldEntries, oldErr := learned.path, learned.entries, learned.loadErr
	learned.path, learned.entries, learned.loadErr = p, map[string]LearnedHeading{}, ""
	learned.mu.Unlock()
	t.Cleanup(func() {
		learned.mu.Lock()
		learned.path, learned.entries, learned.loadErr = oldPath, oldEntries, oldErr
		learned.mu.Unlock()
	})
	return p
}

// A house-standard format is unreadable until taught once; afterwards it is
// read automatically, and only heading text + roles are stored.
func TestTeachUnknownHeadingFormatOnce(t *testing.T) {
	path := withTempLayouts(t)
	pdf := BuildSyntheticUnknownHeadings()
	before, _ := AnalyzeDocument(pdf)
	if len(before.BOM) != 0 {
		t.Fatalf("expected unknown BOM format to be unread before teaching: %d rows", len(before.BOM))
	}
	for _, w := range before.Wires {
		// Positional guesses may appear, but never as an assigned mapping.
		if w.Connector != "UNASSIGNED" && w.End != "UNRESOLVED ENDPOINT" {
			t.Fatalf("confident mapping from an unknown format before teaching: %+v", w)
		}
	}
	x, _ := ExtractDocument(pdf)
	// User selects each table in Page review and assigns roles; the UI then
	// sends the heading row and roles to /api/layouts/learn.
	r1, _ := analyzeRegion(x, RegionRequest{Page: 1, Box: Box{50, 85, 420, 140}, Kind: "bom"})
	r2, _ := analyzeRegion(x, RegionRequest{Page: 1, Box: Box{50, 185, 420, 240}, Kind: "pinout"})
	if len(r1.Grid) == 0 || len(r2.Grid) == 0 {
		t.Fatalf("regions not read: %v / %v", r1.Lines, r2.Lines)
	}
	if _, err := learned.learn("bom", headingWordsFromGrid(r1.Grid[0], []string{"item", "qty", "part", "description"})); err != nil {
		t.Fatal(err)
	}
	if _, err := learned.learn("wire", headingWordsFromGrid(r2.Grid[0], []string{"wire", "from", "pin", "to", "topin", "signal"})); err != nil {
		t.Fatal(err)
	}
	after, _ := AnalyzeDocument(pdf)
	if len(after.BOM) != 3 || after.BOM[2].Part != "M85049/38S15W" {
		t.Fatalf("taught BOM not read: %+v", after.BOM)
	}
	want := map[string]string{"P4:1": "J9:7", "P4:2": "J9:8", "P4:3": "J9:9"}
	if len(after.Wires) != 3 {
		t.Fatalf("taught wire table not read: %+v", after.Wires)
	}
	for _, w := range after.Wires {
		if want[w.Connector+":"+w.Cavity] != w.End+":"+w.EndCavity {
			t.Fatalf("wrong mapping %s:%s -> %s:%s", w.Connector, w.Cavity, w.End, w.EndCavity)
		}
	}
	// The stored file contains headings and roles only.
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"D38999", "M39029", "DATA_P", "P4", "J9", "C1"} {
		if strings.Contains(string(b), leak) {
			t.Fatalf("learned layout file contains drawing content %q:\n%s", leak, b)
		}
	}
	// Persistence: reload from disk and read again.
	learned.load()
	learned.mu.Lock()
	learned.path = path
	learned.mu.Unlock()
	learned.mu.Lock()
	learned.entries = map[string]LearnedHeading{}
	learned.mu.Unlock()
	bb, _ := os.ReadFile(path)
	os.Setenv("TRIVEX_LAYOUT_FILE", path)
	defer os.Unsetenv("TRIVEX_LAYOUT_FILE")
	learned.load()
	_ = bb
	again, _ := AnalyzeDocument(pdf)
	if len(again.BOM) != 3 || len(again.Wires) != 3 {
		t.Fatalf("learned layouts not persisted: %d BOM %d wires", len(again.BOM), len(again.Wires))
	}
	// Forgetting a heading takes effect.
	if err := learned.forget("IDENT"); err != nil {
		t.Fatal(err)
	}
	gone, _ := AnalyzeDocument(pdf)
	if len(gone.BOM) != 0 {
		t.Fatalf("forgotten heading still used: %d BOM rows", len(gone.BOM))
	}
}
