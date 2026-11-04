package main

// LOCAL SCORER — never part of an automated run. It is skipped unless
// TRIVEX_SCORE_DIR names a folder of drawings on this PC. Each PDF in that
// folder whose checksum matches a saved answer key is read with the current
// source and compared with the key. The output is numbers only (drawing 1,
// drawing 2, ...): no file names, designators, part numbers, drawing text or
// checksums, so it can be reported without any drawing content leaving the PC.
// TRIVEX_SCORE_OUT, when set, receives the same lines.

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalAnswerKeyScore(t *testing.T) {
	dir := os.Getenv("TRIVEX_SCORE_DIR")
	if dir == "" {
		t.Skip("local scorer: set TRIVEX_SCORE_DIR to a folder of drawings on this PC")
	}
	keys := listAnswerKeys()
	byHash := map[string]string{}
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".pdf") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		h := sha256.Sum256(b)
		byHash[hex.EncodeToString(h[:])] = p
		return nil
	})
	lines := []string{}
	pass := 0
	for i, k := range keys {
		p, ok := byHash[k.SHA256]
		if !ok {
			lines = append(lines, "drawing "+itoa(i+1)+": not found in the folder")
			continue
		}
		b, _ := os.ReadFile(p)
		x, err := ExtractDocument(b)
		if err != nil {
			lines = append(lines, "drawing "+itoa(i+1)+": could not be read")
			continue
		}
		v, err := viewFromExtraction(x)
		if err != nil {
			lines = append(lines, "drawing "+itoa(i+1)+": could not be analysed")
			continue
		}
		s := scoreDrawing(v, k)
		if s.Pass {
			pass++
		}
		lines = append(lines, s.Line(i+1))
	}
	lines = append(lines, "total: "+itoa(pass)+" of "+itoa(len(keys))+" drawings pass")
	out := strings.Join(lines, "\n") + "\n"
	if o := os.Getenv("TRIVEX_SCORE_OUT"); o != "" {
		os.WriteFile(o, []byte(out), 0o600)
	}
	t.Log("\n" + out)
}
