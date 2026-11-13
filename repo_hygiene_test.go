package main

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// Regression for the 24 MB CURRENT_STATE.md: a text-encoding round trip was
// applied repeatedly, multiplying the same character about 2.2x each time.
// Tracked project text files must stay small and free of that corruption.
var mojibakeRun = regexp.MustCompile("[\\x{00c3}\\x{00c2}\\x{0192}\\x{00c6}\\x{00e2}\\x{20ac}\\x{2122}\\x{201a}\\x{2026}\\x{00a1}\\x{00a2}]{5,}")

func TestTrackedTextFilesAreSmallAndNotCorrupted(t *testing.T) {
	out, err := exec.Command("git", "ls-files").Output()
	if err != nil {
		t.Skip("not a git checkout")
	}
	for _, name := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if name == "" || strings.HasPrefix(name, "vendor/") {
			continue
		}
		low := strings.ToLower(name)
		if !(strings.HasSuffix(low, ".md") || strings.HasSuffix(low, ".txt") || strings.HasSuffix(low, ".go") || strings.HasSuffix(low, ".js") || strings.HasSuffix(low, ".html") || strings.HasSuffix(low, ".ps1") || strings.HasSuffix(low, ".yml")) {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			continue
		}
		if len(data) > 1_000_000 {
			t.Errorf("%s is %d bytes; text files over 1 MB are almost certainly corrupted or generated", name, len(data))
		}
		if mojibakeRun.Match(data) {
			t.Errorf("%s contains a run of repeated-encoding corruption", name)
		}
	}
}
