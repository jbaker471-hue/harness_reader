package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Learned heading layouts (v2.8).
//
// When a user assigns column roles to a table in Page review, the HEADING
// TEXT of each column and the role chosen are remembered on this PC. Every
// later drawing that uses the same heading words is then read automatically.
// Only heading text and roles are stored — never values, part numbers or
// any other drawing content. The file lives in the user's configuration
// folder (not the program folder, so the program folder stays deletable).

type LearnedHeading struct {
	Label   string    `json:"label"` // normalized heading text, e.g. "SYM DES"
	Role    string    `json:"role"`  // table-reader role, e.g. "ref", "pin", "qty"
	Kind    string    `json:"kind"`  // bom | wire | conntable (where it was taught)
	Taught  time.Time `json:"taught"`
	Uses    int       `json:"uses"`
}

type learnedStore struct {
	mu      sync.RWMutex
	path    string
	entries map[string]LearnedHeading
	loadErr string
}

var learned = &learnedStore{entries: map[string]LearnedHeading{}}

var validRoles = map[string]bool{"item": true, "qty": true, "part": true, "desc": true, "cage": true, "vendor": true, "ref": true, "um": true, "note": true,
	"pin": true, "wire": true, "signal": true, "color": true, "awg": true, "from": true, "to": true, "frompin": true, "topin": true, "other": true}

func learnedPath() string {
	if p := os.Getenv("TRIVEX_LAYOUT_FILE"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "Trivex Harness Auditor", "learned_heading_layouts.json")
}

func (l *learnedStore) load() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.path = learnedPath()
	l.entries = map[string]LearnedHeading{}
	l.loadErr = ""
	if l.path == "" {
		l.loadErr = "no user configuration folder"
		return
	}
	b, err := os.ReadFile(l.path)
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		l.loadErr = err.Error()
		return
	}
	var list []LearnedHeading
	if err := json.Unmarshal(b, &list); err != nil {
		l.loadErr = "layout file unreadable: " + err.Error()
		return
	}
	for _, e := range list {
		if e.Label != "" && validRoles[e.Role] {
			l.entries[e.Label] = e
		}
	}
}

func (l *learnedStore) saveLocked() error {
	if l.path == "" {
		return errors.New("no user configuration folder for learned layouts")
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(l.listLocked(), "", "  ")
	tmp := l.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, l.path)
}

func (l *learnedStore) listLocked() []LearnedHeading {
	out := []LearnedHeading{}
	for _, e := range l.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

func (l *learnedStore) list() []LearnedHeading {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.listLocked()
}

// role returns a learned role for a heading label ("" when none).
func (l *learnedStore) role(label string) string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if e, ok := l.entries[normHead(label)]; ok {
		return e.Role
	}
	return ""
}

func (l *learnedStore) has(label string) bool { return l.role(label) != "" }

// learn records heading -> role pairs. Labels that are already built-in
// vocabulary with the same role are not stored (nothing to learn).
func (l *learnedStore) learn(kind string, pairs []LearnedHeading) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, p := range pairs {
		label := normHead(p.Label)
		if label == "" || len(label) > 60 || !validRoles[p.Role] {
			continue
		}
		if r, ok := headerVocab[label]; ok && r == p.Role {
			continue
		}
		l.entries[label] = LearnedHeading{Label: label, Role: p.Role, Kind: kind, Taught: time.Now().UTC().Truncate(time.Second)}
		n++
	}
	if n == 0 {
		return 0, nil
	}
	return n, l.saveLocked()
}

func (l *learnedStore) forget(label string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, normHead(label))
	return l.saveLocked()
}

func (l *learnedStore) importList(list []LearnedHeading) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, e := range list {
		e.Label = normHead(e.Label)
		if e.Label == "" || len(e.Label) > 60 || !validRoles[e.Role] {
			continue
		}
		l.entries[e.Label] = e
		n++
	}
	return n, l.saveLocked()
}

// knownHeading reports whether a label is built-in or learned vocabulary.
func knownHeading(label string) bool {
	n := normHead(label)
	if _, ok := headerVocab[n]; ok {
		return true
	}
	return learned.has(n)
}

func init() { learned.load() }

// headingWordsFromGrid pairs the heading row of a Page-review grid with the
// roles the user chose there.
func headingWordsFromGrid(headers, roles []string) []LearnedHeading {
	out := []LearnedHeading{}
	for i, h := range headers {
		if i < len(roles) && strings.TrimSpace(h) != "" && roles[i] != "" {
			r := roles[i]
			if r == "description" {
				r = "desc"
			}
			// Which end a pin column belongs to is decided by its position
			// (after the TO column = far end), not by the heading word, which
			// is usually the same for both ("PIN", "CNTC").
			if r == "frompin" || r == "topin" {
				r = "pin"
			}
			out = append(out, LearnedHeading{Label: h, Role: r})
		}
	}
	return out
}
