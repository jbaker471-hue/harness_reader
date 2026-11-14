package main

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Job actuals: the hours a finished harness really took, entered by the user and
// kept ONLY on this PC (in the user configuration folder, never in the source
// tree). Each record is matched to a drawing by its checksum and stores numbers
// only: the actual hours and the estimate's operation counts at the time. The
// app compares them across jobs to show which operation times run long or short.

type ActualLine struct {
	Key     string  `json:"key"`
	Count   float64 `json:"count"`
	Minutes float64 `json:"minutes"`
}

type JobActual struct {
	SHA256        string       `json:"sha256,omitempty"`
	Saved         string       `json:"saved"`
	ActualHours   float64      `json:"actualHours"`
	EstimateHours float64      `json:"estimateHours"`
	Lines         []ActualLine `json:"lines"`
}

var reActualKey = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,39}$`)

func actualsDir() string {
	if p := os.Getenv("TRIVEX_ACTUALS_DIR"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "Trivex Harness Auditor", "job_actuals")
}

func actualFile(sha string) string {
	return filepath.Join(actualsDir(), "actual-"+sha[:16]+".json")
}

func finiteIn(v, lo, hi float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= lo && v <= hi
}

func validJobActual(a *JobActual) error {
	a.SHA256 = strings.ToLower(strings.TrimSpace(a.SHA256))
	if !reHex64.MatchString(a.SHA256) {
		return errors.New("no drawing is open")
	}
	if !finiteIn(a.ActualHours, 0.01, 10000) {
		return errors.New("actual hours must be more than 0 and no more than 10000")
	}
	if !finiteIn(a.EstimateHours, 0, 10000) {
		return errors.New("the estimate is not a usable number")
	}
	if len(a.Lines) == 0 || len(a.Lines) > 200 {
		return errors.New("the estimate has no operations to compare against")
	}
	for _, l := range a.Lines {
		if !reActualKey.MatchString(l.Key) || !finiteIn(l.Count, 0, 1e6) || !finiteIn(l.Minutes, 0, 1e7) {
			return errors.New("the estimate has an operation that cannot be saved")
		}
	}
	return nil
}

func saveJobActual(a JobActual) error {
	if err := validJobActual(&a); err != nil {
		return err
	}
	dir := actualsDir()
	if dir == "" {
		return errors.New("no user configuration folder")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	a.Saved = time.Now().UTC().Format(time.RFC3339)
	b, _ := json.MarshalIndent(a, "", "  ")
	tmp := actualFile(a.SHA256) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, actualFile(a.SHA256))
}

func deleteJobActual(sha string) error {
	sha = strings.ToLower(sha)
	if !reHex64.MatchString(sha) || actualsDir() == "" {
		return errors.New("no drawing is open")
	}
	if err := os.Remove(actualFile(sha)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func loadJobActual(sha string) (JobActual, bool) {
	sha = strings.ToLower(sha)
	if !reHex64.MatchString(sha) || actualsDir() == "" {
		return JobActual{}, false
	}
	b, err := os.ReadFile(actualFile(sha))
	if err != nil {
		return JobActual{}, false
	}
	var a JobActual
	if json.Unmarshal(b, &a) != nil || a.SHA256 != sha || validJobActual(&a) != nil {
		return JobActual{}, false
	}
	return a, true
}

// listJobActuals returns every saved job, oldest first, with the checksum removed:
// the comparison across jobs needs only the numbers.
func listJobActuals() []JobActual {
	dir := actualsDir()
	out := []JobActual{}
	if dir == "" {
		return out
	}
	names, _ := filepath.Glob(filepath.Join(dir, "actual-*.json"))
	sort.Strings(names)
	for _, n := range names {
		b, err := os.ReadFile(n)
		if err != nil {
			continue
		}
		var a JobActual
		if json.Unmarshal(b, &a) != nil || validJobActual(&a) != nil {
			continue
		}
		a.SHA256 = ""
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Saved < out[j].Saved })
	return out
}
