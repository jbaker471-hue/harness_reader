package main

import (
	"math"
	"strings"
	"testing"
)

func TestJobActualSaveLoadListAndDelete(t *testing.T) {
	t.Setenv("TRIVEX_ACTUALS_DIR", t.TempDir())
	sha := strings.Repeat("ab", 32)
	other := strings.Repeat("cd", 32)
	a := JobActual{SHA256: strings.ToUpper(sha), ActualHours: 9.5, EstimateHours: 8.75, Lines: []ActualLine{{Key: "strip", Count: 40, Minutes: 6.7}, {Key: "insert", Count: 20, Minutes: 50.7}}}
	if err := saveJobActual(a); err != nil {
		t.Fatal(err)
	}
	got, ok := loadJobActual(sha)
	if !ok || got.ActualHours != 9.5 || len(got.Lines) != 2 || got.Saved == "" {
		t.Fatalf("saved job not read back: %+v %v", got, ok)
	}
	// saving again for the same drawing replaces the record; another drawing adds one
	a.ActualHours = 10
	if err := saveJobActual(a); err != nil {
		t.Fatal(err)
	}
	b := a
	b.SHA256 = other
	if err := saveJobActual(b); err != nil {
		t.Fatal(err)
	}
	all := listJobActuals()
	if len(all) != 2 {
		t.Fatalf("want 2 jobs, got %d", len(all))
	}
	for _, j := range all {
		if j.SHA256 != "" {
			t.Fatal("the list across jobs must not carry a drawing checksum")
		}
	}
	if got, _ := loadJobActual(sha); got.ActualHours != 10 {
		t.Fatalf("a second save must replace the first: %+v", got)
	}
	if _, ok := loadJobActual(strings.Repeat("ef", 32)); ok {
		t.Fatal("a drawing with no saved job must not load one")
	}
	if err := deleteJobActual(sha); err != nil {
		t.Fatal(err)
	}
	if err := deleteJobActual(sha); err != nil {
		t.Fatal("deleting twice must not fail: ", err)
	}
	if _, ok := loadJobActual(sha); ok || len(listJobActuals()) != 1 {
		t.Fatal("a deleted job must be gone")
	}
}

func TestJobActualRejectsUnusableRecords(t *testing.T) {
	t.Setenv("TRIVEX_ACTUALS_DIR", t.TempDir())
	sha := strings.Repeat("ab", 32)
	line := []ActualLine{{Key: "strip", Count: 1, Minutes: 1}}
	bad := []JobActual{
		{SHA256: "", ActualHours: 5, Lines: line},
		{SHA256: "../../x", ActualHours: 5, Lines: line},
		{SHA256: sha, ActualHours: 0, Lines: line},
		{SHA256: sha, ActualHours: -2, Lines: line},
		{SHA256: sha, ActualHours: math.NaN(), Lines: line},
		{SHA256: sha, ActualHours: math.Inf(1), Lines: line},
		{SHA256: sha, ActualHours: 5, EstimateHours: -1, Lines: line},
		{SHA256: sha, ActualHours: 5},
		{SHA256: sha, ActualHours: 5, Lines: []ActualLine{{Key: "bad key!", Count: 1, Minutes: 1}}},
		{SHA256: sha, ActualHours: 5, Lines: []ActualLine{{Key: "strip", Count: -1, Minutes: 1}}},
		{SHA256: sha, ActualHours: 5, Lines: []ActualLine{{Key: "strip", Count: 1, Minutes: math.NaN()}}},
		{SHA256: sha, ActualHours: 5, Lines: make([]ActualLine, 201)},
	}
	for i, a := range bad {
		if saveJobActual(a) == nil {
			t.Fatalf("record %d must be rejected", i)
		}
	}
	if len(listJobActuals()) != 0 {
		t.Fatal("a rejected record must not be written")
	}
	if deleteJobActual("not-a-checksum") == nil {
		t.Fatal("delete needs a drawing checksum")
	}
}
