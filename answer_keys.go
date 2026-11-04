package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Answer keys: the right answers for a drawing, entered once by the user and
// kept ONLY on this PC (in the user configuration folder, never in the source
// tree). A key is matched to a drawing by the drawing's checksum. The scorer
// compares what the app reads with the key and reports numbers only: no file
// names, designators, part numbers, drawing text or checksums.

type AnswerConnector struct {
	Tag       string `json:"tag"`
	Positions int    `json:"positions"`
	Wired     int    `json:"wired"`
	Unused    int    `json:"unused"`
}

type AnswerKey struct {
	SHA256     string            `json:"sha256"`
	Saved      string            `json:"saved"`
	Conductors int               `json:"conductors"`
	Connectors []AnswerConnector `json:"connectors"`
}

var reHex64 = regexp.MustCompile(`^[a-f0-9]{64}$`)

func answerDir() string {
	if p := os.Getenv("TRIVEX_ANSWER_DIR"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "Trivex Harness Auditor", "answer_keys")
}

func answerFile(sha string) string {
	return filepath.Join(answerDir(), "key-"+sha[:16]+".json")
}

func validAnswerKey(k *AnswerKey) error {
	k.SHA256 = strings.ToLower(strings.TrimSpace(k.SHA256))
	if !reHex64.MatchString(k.SHA256) {
		return errors.New("no drawing is open")
	}
	if k.Conductors < 0 || k.Conductors > 100000 {
		return errors.New("conductors must be between 0 and 100000")
	}
	if len(k.Connectors) == 0 || len(k.Connectors) > 500 {
		return errors.New("enter at least one connector")
	}
	seen := map[string]bool{}
	for i := range k.Connectors {
		c := &k.Connectors[i]
		c.Tag = strings.ToUpper(strings.TrimSpace(c.Tag))
		if c.Tag == "" || len(c.Tag) > 40 || seen[c.Tag] {
			return fmt.Errorf("connector %d needs a unique designator", i+1)
		}
		seen[c.Tag] = true
		if c.Positions < 0 || c.Wired < 0 || c.Unused < 0 || c.Positions > 1000 || (c.Positions > 0 && c.Wired+c.Unused != c.Positions) {
			return fmt.Errorf("%s: wired + not used must equal the positions", c.Tag)
		}
	}
	return nil
}

func saveAnswerKey(k AnswerKey) error {
	if err := validAnswerKey(&k); err != nil {
		return err
	}
	dir := answerDir()
	if dir == "" {
		return errors.New("no user configuration folder")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	k.Saved = time.Now().UTC().Format(time.RFC3339)
	b, _ := json.MarshalIndent(k, "", "  ")
	tmp := answerFile(k.SHA256) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, answerFile(k.SHA256))
}

func loadAnswerKey(sha string) (AnswerKey, bool) {
	sha = strings.ToLower(sha)
	if !reHex64.MatchString(sha) || answerDir() == "" {
		return AnswerKey{}, false
	}
	b, err := os.ReadFile(answerFile(sha))
	if err != nil {
		return AnswerKey{}, false
	}
	var k AnswerKey
	if json.Unmarshal(b, &k) != nil || k.SHA256 != sha {
		return AnswerKey{}, false
	}
	return k, true
}

func listAnswerKeys() []AnswerKey {
	out := []AnswerKey{}
	files, _ := filepath.Glob(filepath.Join(answerDir(), "key-*.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var k AnswerKey
		if json.Unmarshal(b, &k) == nil && validAnswerKey(&k) == nil {
			out = append(out, k)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Saved < out[j].Saved })
	return out
}

// DrawingScore compares what was read with the answer key. Counts only.
type DrawingScore struct {
	Connectors      int  `json:"connectors"`      // in the key
	ConnectorsFound int  `json:"connectorsFound"` // read with the same designator
	PositionsRight  int  `json:"positionsRight"`  // connectors whose position count matches
	WiredRight      int  `json:"wiredRight"`      // connectors whose wired count matches
	UnusedRight     int  `json:"unusedRight"`     // connectors whose not-used count matches
	WiredKey        int  `json:"wiredKey"`        // wired cavities in the key, all connectors
	WiredRead       int  `json:"wiredRead"`       // wired cavities read, on the key's connectors
	CondKey         int  `json:"conductorsKey"`
	CondRead        int  `json:"conductorsRead"`
	Pass            bool `json:"pass"`
}

// readAnswers is what the app read, in answer-key form.
func readAnswers(v ViewAnalysis) (map[string]AnswerConnector, int) {
	got := map[string]AnswerConnector{}
	for _, c := range v.Connectors {
		t := strings.ToUpper(strings.TrimSpace(c.Name))
		if t != "" && t != "UNASSIGNED" {
			got[t] = AnswerConnector{Tag: t, Positions: c.Cavities}
		}
	}
	for _, m := range v.CavityModels {
		t := strings.ToUpper(strings.TrimSpace(m.Tag))
		a := got[t]
		a.Tag = t
		if m.AllVerified && len(m.All) > 0 {
			a.Positions = len(m.All)
		}
		a.Wired = len(m.Wired)
		if a.Positions > 0 {
			a.Unused = a.Positions - a.Wired
		}
		got[t] = a
	}
	// conductors: one per distinct pair of ends
	pairs := map[string]bool{}
	for _, w := range v.Wires {
		a, b := strings.ToUpper(w.Connector)+":"+w.Cavity, strings.ToUpper(w.End)+":"+w.EndCavity
		if a > b {
			a, b = b, a
		}
		pairs[a+"|"+b] = true
	}
	return got, len(pairs)
}

func scoreDrawing(v ViewAnalysis, k AnswerKey) DrawingScore {
	got, cuts := readAnswers(v)
	s := DrawingScore{Connectors: len(k.Connectors), CondKey: k.Conductors, CondRead: cuts}
	for _, want := range k.Connectors {
		s.WiredKey += want.Wired
		g, ok := got[want.Tag]
		if !ok {
			continue
		}
		s.ConnectorsFound++
		s.WiredRead += g.Wired
		if g.Positions == want.Positions {
			s.PositionsRight++
		}
		if g.Wired == want.Wired {
			s.WiredRight++
		}
		if g.Positions > 0 && g.Unused == want.Unused {
			s.UnusedRight++
		}
	}
	n := s.Connectors
	s.Pass = s.ConnectorsFound == n && s.PositionsRight == n && s.WiredRight == n && s.UnusedRight == n && (k.Conductors == 0 || cuts == k.Conductors)
	return s
}

func (s DrawingScore) Line(i int) string {
	res := "FAIL"
	if s.Pass {
		res = "PASS"
	}
	cuts := fmt.Sprintf("conductors %d/%d", s.CondRead, s.CondKey)
	if s.CondKey == 0 {
		cuts = "conductors not in key"
	}
	return fmt.Sprintf("drawing %d: %s · connectors found %d/%d · positions right %d/%d · wired right %d/%d (%d of %d cavities) · not used right %d/%d · %s",
		i, res, s.ConnectorsFound, s.Connectors, s.PositionsRight, s.Connectors, s.WiredRight, s.Connectors, s.WiredRead, s.WiredKey, s.UnusedRight, s.Connectors, cuts)
}
