package main

// Remembered facts: things the owner has CONFIRMED once, kept ONLY on this PC
// (user configuration folder, never the repository, never sent anywhere), so the
// app does not ask again and does not guess where it was already told.
//
// First fact kind: "positions" - the number of positions of a connector part
// number. Part numbers are stored here because they are the key; the file is
// local data like an answer key. Nothing in this file is written to the source
// tree, to logs, or to a diagnostic report.
//
// The store is NOT loaded at package start. main() loads it, so automated tests
// (which never call load) can never read a developer's real facts.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	factPositions = "positions"
	factMinValue  = 2
	factMaxValue  = 500
)

type KnownFact struct {
	Kind      string `json:"kind"`
	Key       string `json:"key"`   // normalised part number
	Part      string `json:"part"`  // as confirmed, for display
	Value     int    `json:"value"` // positions
	Confirmed int    `json:"confirmed"`
	Updated   string `json:"updated"`
}

type factStore struct {
	mu      sync.RWMutex
	path    string
	entries map[string]KnownFact
	loadErr string
}

var facts = &factStore{entries: map[string]KnownFact{}}

// factPartKey keeps the full identifier: upper case, whitespace removed, nothing else.
func factPartKey(part string) string {
	return strings.ToUpper(strings.Join(strings.Fields(part), ""))
}

func factsPath() string {
	if p := os.Getenv("TRIVEX_FACTS_DIR"); p != "" {
		return filepath.Join(p, "facts.json")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "Trivex Harness Auditor", "facts", "facts.json")
}

func entryKey(kind, part string) string { return kind + "|" + factPartKey(part) }

// load reads the file at path (or the default). A missing file is an empty store.
func (s *factStore) load(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.path = path
	s.entries = map[string]KnownFact{}
	s.loadErr = ""
	if path == "" {
		s.loadErr = "no user configuration folder"
		return
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		s.loadErr = err.Error()
		return
	}
	var list []KnownFact
	if err := json.Unmarshal(b, &list); err != nil {
		s.loadErr = "facts file unreadable: " + err.Error()
		return
	}
	for _, f := range list {
		if f.Kind != factPositions || f.Key == "" || f.Value < factMinValue || f.Value > factMaxValue {
			continue // anything not understood is ignored, never guessed at
		}
		s.entries[f.Kind+"|"+f.Key] = f
	}
}

// positions returns the count the owner confirmed for a part, if any. With no
// store loaded (tests, or no config folder) it always answers no.
func (s *factStore) positions(part string) (int, bool) {
	if s == nil || strings.TrimSpace(part) == "" {
		return 0, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	f, ok := s.entries[entryKey(factPositions, part)]
	return f.Value, ok
}

func (s *factStore) saveLocked() error {
	if s.path == "" {
		return errors.New("no user configuration folder to save to")
	}
	list := make([]KnownFact, 0, len(s.entries))
	for _, f := range s.entries {
		list = append(list, f)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Key < list[j].Key })
	b, err := json.MarshalIndent(list, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// setPositions remembers (or corrects) a part's position count.
func (s *factStore) setPositions(part string, n int) error {
	part = strings.TrimSpace(part)
	if part == "" || len(part) > 80 {
		return errors.New("enter the full part number")
	}
	if n < factMinValue || n > factMaxValue {
		return fmt.Errorf("positions must be between %d and %d", factMinValue, factMaxValue)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := entryKey(factPositions, part)
	f := s.entries[k]
	prev := s.entries[k]
	if f.Value == n {
		f.Confirmed++
	} else {
		f.Confirmed = 1
	}
	f.Kind, f.Key, f.Part, f.Value = factPositions, factPartKey(part), part, n
	f.Updated = time.Now().UTC().Format("2006-01-02")
	s.entries[k] = f
	if err := s.saveLocked(); err != nil {
		if prev.Key == "" {
			delete(s.entries, k)
		} else {
			s.entries[k] = prev
		}
		return err
	}
	return nil
}

func (s *factStore) forgetPositions(part string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := entryKey(factPositions, part)
	prev, ok := s.entries[k]
	if !ok {
		return nil
	}
	delete(s.entries, k)
	if err := s.saveLocked(); err != nil {
		s.entries[k] = prev
		return err
	}
	return nil
}

func (s *factStore) list() []KnownFact {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]KnownFact, 0, len(s.entries))
	for _, f := range s.entries {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

const confirmedCountStatus = "COUNT CONFIRMED BY THE OWNER (REMEMBERED ON THIS PC); NOT FROM THE DRAWING, LAYOUT UNKNOWN"

func confirmedCountLayout(n int) string {
	return confirmedCountStatus + " (" + strconv.Itoa(n) + " positions)"
}

// isStatedCount is true for a position count that was stated or confirmed but whose layout is unknown.
func isStatedCount(status string) bool {
	return strings.HasPrefix(status, descCountStatus) || strings.HasPrefix(status, confirmedCountStatus)
}
