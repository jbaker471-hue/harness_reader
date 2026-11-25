package main

// Regression ratchet. A drawing's reading is summarised as a few plain numbers
// (wires read, connectors with positions, audits complete, ...). A baseline of
// those numbers is kept ONLY on the owner's PC (user configuration folder,
// never the repository), keyed by a short checksum prefix. A later run fails
// when a number that should only go up goes down, or a number that should only
// go down goes up. Output is numbers and metric names only: no file names,
// designators, part numbers, drawing text or checksums.
//
// The pure functions here are covered by synthetic tests in CI. The local
// runner (local_ratchet_test.go) is skipped unless a drawings folder is named.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Direction says how a metric may move without being a regression.
const (
	ratchetUp    = "up"    // must not decrease
	ratchetDown  = "down"  // must not increase
	ratchetWatch = "watch" // reported when it changes, never fails
)

type ratchetMetric struct {
	Name string
	Dir  string
}

// ratchetMetrics is the fixed, ordered list of measured numbers.
var ratchetMetrics = []ratchetMetric{
	{"wires", ratchetUp},
	{"wires_with_connector_and_pin", ratchetUp},
	{"connectors_with_positions", ratchetUp},
	{"connectors", ratchetWatch},
	{"table_audits_complete", ratchetUp},
	{"table_audits_incomplete", ratchetDown},
	{"missed_rows", ratchetDown},
	{"issues_error", ratchetDown},
	{"issues_total", ratchetWatch},
	{"unused_cavities", ratchetWatch},
	{"bom_tables", ratchetUp},
	{"harness_lengths", ratchetUp},
	{"capture_checks", ratchetWatch},
	{"pages", ratchetWatch},
}

// drawingMetrics turns one reading into the ratchet numbers.
func drawingMetrics(v ViewAnalysis) map[string]int {
	m := map[string]int{}
	m["wires"] = len(v.Wires)
	for _, w := range v.Wires {
		if w.Connector != "" && w.Connector != "UNASSIGNED" && w.Cavity != "" {
			m["wires_with_connector_and_pin"]++
		}
	}
	m["connectors"] = len(v.Connectors)
	for _, c := range v.Connectors {
		if c.Cavities > 0 {
			m["connectors_with_positions"]++
		}
	}
	for _, a := range v.TableAudits {
		if a.Status == "COMPLETE" {
			m["table_audits_complete"]++
		} else {
			m["table_audits_incomplete"]++
		}
		m["missed_rows"] += len(a.Missed)
	}
	for _, i := range v.Issues {
		m["issues_total"]++
		if strings.EqualFold(i.Severity, "ERROR") {
			m["issues_error"]++
		}
	}
	m["unused_cavities"] = len(v.Unused)
	m["bom_tables"] = len(v.Boms)
	m["harness_lengths"] = len(v.HarnessLengths)
	m["capture_checks"] = len(v.CaptureChecks)
	m["pages"] = v.Pages
	return m
}

// ratchetChange is one metric that moved.
type ratchetChange struct {
	Metric   string
	Was, Now int
	Dir      string
	Regress  bool
}

// compareRatchet lists every metric that differs from the baseline.
func compareRatchet(base, cur map[string]int) []ratchetChange {
	var out []ratchetChange
	for _, rm := range ratchetMetrics {
		was, hadBase := base[rm.Name]
		now := cur[rm.Name]
		if !hadBase || was == now {
			continue
		}
		c := ratchetChange{Metric: rm.Name, Was: was, Now: now, Dir: rm.Dir}
		switch rm.Dir {
		case ratchetUp:
			c.Regress = now < was
		case ratchetDown:
			c.Regress = now > was
		}
		out = append(out, c)
	}
	return out
}

// ratchetLine formats one change for the report: names and numbers only.
func (c ratchetChange) Line(drawing int) string {
	tag := "better"
	switch {
	case c.Regress:
		tag = "WORSE"
	case c.Dir == ratchetWatch:
		tag = "changed"
	}
	return fmt.Sprintf("drawing %d: %s %d -> %d (%s)", drawing, c.Metric, c.Was, c.Now, tag)
}

// mergeRatchet returns the new baseline. Improvements and new metrics are
// kept; a regression is only taken when accept is true, so a bad run can never
// quietly lower the bar.
func mergeRatchet(base, cur map[string]int, accept bool) map[string]int {
	out := map[string]int{}
	for k, v := range base {
		out[k] = v
	}
	for _, rm := range ratchetMetrics {
		now := cur[rm.Name]
		was, had := base[rm.Name]
		switch {
		case !had:
			out[rm.Name] = now
		case was == now:
		case rm.Dir == ratchetWatch:
			out[rm.Name] = now
		case rm.Dir == ratchetUp && now > was, rm.Dir == ratchetDown && now < was:
			out[rm.Name] = now
		case accept:
			out[rm.Name] = now
		}
	}
	return out
}

// ratchetFile is the on-PC baseline: short checksum prefix -> metrics.
type ratchetFile struct {
	Drawings map[string]map[string]int `json:"drawings"`
}

func ratchetPath() string {
	if p := os.Getenv("TRIVEX_RATCHET_DIR"); p != "" {
		return filepath.Join(p, "baseline.json")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "Trivex Harness Auditor", "ratchet", "baseline.json")
}

func loadRatchet(path string) ratchetFile {
	f := ratchetFile{Drawings: map[string]map[string]int{}}
	b, err := os.ReadFile(path)
	if err != nil {
		return f
	}
	json.Unmarshal(b, &f)
	if f.Drawings == nil {
		f.Drawings = map[string]map[string]int{}
	}
	return f
}

func saveRatchet(path string, f ratchetFile) error {
	if path == "" {
		return fmt.Errorf("no configuration folder")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(f, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// ratchetOrder gives each baseline checksum a stable ordinal (1, 2, ...) that
// reveals nothing about the drawing.
func ratchetOrder(f ratchetFile) map[string]int {
	keys := make([]string, 0, len(f.Drawings))
	for k := range f.Drawings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	o := map[string]int{}
	for i, k := range keys {
		o[k] = i + 1
	}
	return o
}
