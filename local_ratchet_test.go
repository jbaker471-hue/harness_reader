package main

// LOCAL RATCHET — never part of an automated run. Skipped unless
// TRIVEX_SCORE_DIR (or TRIVEX_DRAWINGS_DIR) names a folder of drawings on this
// PC. Each PDF is read with the current source and compared with a baseline of
// plain numbers kept on this PC only. The run FAILS when a number that should
// only go up goes down (or one that should only go down goes up).
//
//   TRIVEX_RATCHET_SAVE=1    record new drawings and keep improvements
//   TRIVEX_RATCHET_ACCEPT=1  with SAVE, also accept a regression on purpose
//   TRIVEX_RATCHET_OUT=path  also write the report there
//
// The report holds drawing ordinals, metric names and numbers only.

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestLocalRatchet(t *testing.T) {
	dir := os.Getenv("TRIVEX_DRAWINGS_DIR")
	if dir == "" {
		dir = os.Getenv("TRIVEX_SCORE_DIR")
	}
	if dir == "" {
		t.Skip("local ratchet: set TRIVEX_DRAWINGS_DIR to a folder of drawings on this PC")
	}
	path := ratchetPath()
	base := loadRatchet(path)
	cur := map[string]map[string]int{}
	var unreadable int
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".pdf") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			unreadable++
			return nil
		}
		sum := sha256.Sum256(b)
		key := hex.EncodeToString(sum[:])[:16]
		x, err := ExtractDocument(b)
		if err != nil {
			unreadable++
			return nil
		}
		v, err := viewFromExtraction(x)
		if err != nil {
			unreadable++
			return nil
		}
		cur[key] = drawingMetrics(v)
		return nil
	})
	// One ordinal space over everything known, stable and name-free.
	union := ratchetFile{Drawings: map[string]map[string]int{}}
	for k, m := range base.Drawings {
		union.Drawings[k] = m
	}
	for k, m := range cur {
		if _, ok := union.Drawings[k]; !ok {
			union.Drawings[k] = m
		}
	}
	ord := ratchetOrder(union)
	keys := make([]string, 0, len(union.Drawings))
	for k := range union.Drawings {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var lines []string
	regress := 0
	for _, k := range keys {
		n := ord[k]
		c, found := cur[k]
		b, hadBase := base.Drawings[k]
		switch {
		case !found:
			lines = append(lines, "drawing "+itoa(n)+": in the baseline but not found in the folder")
		case !hadBase:
			lines = append(lines, "drawing "+itoa(n)+": new, not in the baseline yet")
		default:
			ch := compareRatchet(b, c)
			if len(ch) == 0 {
				lines = append(lines, "drawing "+itoa(n)+": same as the baseline")
			}
			for _, x := range ch {
				lines = append(lines, x.Line(n))
				if x.Regress {
					regress++
				}
			}
		}
	}
	if unreadable > 0 {
		lines = append(lines, itoa(unreadable)+" file(s) could not be read")
		regress++
	}
	if os.Getenv("TRIVEX_RATCHET_SAVE") == "1" {
		accept := os.Getenv("TRIVEX_RATCHET_ACCEPT") == "1"
		next := ratchetFile{Drawings: map[string]map[string]int{}}
		for k, m := range base.Drawings {
			next.Drawings[k] = m
		}
		for k, c := range cur {
			next.Drawings[k] = mergeRatchet(base.Drawings[k], c, accept)
		}
		if err := saveRatchet(path, next); err != nil {
			t.Fatalf("could not save the baseline: %v", err)
		}
		lines = append(lines, "baseline saved ("+itoa(len(next.Drawings))+" drawings)")
	}
	lines = append(lines, "ratchet: "+itoa(regress)+" regression(s)")
	out := strings.Join(lines, "\n") + "\n"
	if o := os.Getenv("TRIVEX_RATCHET_OUT"); o != "" {
		os.WriteFile(o, []byte(out), 0o600)
	}
	t.Log("\n" + out)
	if regress > 0 {
		t.Fail()
	}
}
