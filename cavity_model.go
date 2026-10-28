package main

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Per-connector cavity model (v2.8): which cavities exist, which carry a
// wire, and which the drawing itself declares unused — with the source of
// every statement. Unused = (all cavities) − (wired cavities).

type UnusedCavity struct {
	Connector string `json:"connector"`
	Cavity    string `json:"cavity"`
	Page      int    `json:"page"`
	Source    string `json:"source"`
}

type FaceView struct {
	Page      int         `json:"page"`
	Side      string      `json:"side"`
	Box       Box         `json:"box"`
	R         float64     `json:"r"`
	Positions []CavityPos `json:"positions"`
	Labeled   int         `json:"labeled"`
}

type CavityModel struct {
	UnusedStatus   string         `json:"unusedStatus"`
	Tag            string         `json:"tag"`
	Part           string         `json:"part"`
	All            []string       `json:"all"`
	AllSource      string         `json:"allSource"`
	AllVerified    bool           `json:"allVerified"` // drawing or reviewed exact-part guide inventory; wiring coverage is separate
	Wired          []string       `json:"wired"`
	ExplicitUnused []UnusedCavity `json:"explicitUnused"`
	Unused         []string       `json:"unused"`
	SolidMarked    []string       `json:"solidMarked"`
	OpenMarked     []string       `json:"openMarked"`
	Views          []FaceView     `json:"views"`
	MirrorCheck    string         `json:"mirrorCheck"`
	Conflicts      []string       `json:"conflicts"`
}

var (
	reUnusedNote  = regexp.MustCompile(`(?i)\b(?:UNUSED|SPARE|NOT\s+USED|UNPOPULATED|EMPTY)\s+(?:CAVITIES|CAVITY|CONTACTS|CONTACT|POSITIONS|PINS|SOCKETS)\b(?:\s+(?:OF|ON|IN|FOR))?\s*((?:J|P|X|TB|CN|CON|PL|SK)[-_]?[0-9]{1,4}[A-Z]?)?\s*[:=-]?\s*([A-Za-z0-9 ,;&/-]+?)(?:\.|$)`)
	reTagFirst    = regexp.MustCompile(`(?i)\b((?:J|P|X|TB|CN|CON|PL|SK)[-_]?[0-9]{1,4}[A-Z]?)\s*[:,-]?\s+(?:UNUSED|SPARE|NOT\s+USED|UNPOPULATED)\b`)
	reRange       = regexp.MustCompile(`^([0-9]{1,4})\s*(?:-|THRU|THROUGH|TO)\s*([0-9]{1,4})$`)
	reSpareSignal = regexp.MustCompile(`(?i)^(?:SPARE|N/?C|NOT USED|UNUSED|NO CONNECT(?:ION)?)$`)
)

// parseCavityList turns "4, 7-9 & 12" into [4 7 8 9 12]; letters kept as-is.
// Reading stops at the first word that is not part of the list, so trailing
// prose ("... TO BE FILLED WITH MS27488-22") never adds cavities.
func parseCavityList(s string) []string {
	s = strings.NewReplacer(",", " , ", ";", " , ", "&", " , ").Replace(s)
	toks := strings.Fields(s)
	out := []string{}
	stop := map[string]bool{"TO": true, "BE": true, "OF": true, "IN": true, "ON": true, "AT": true, "IS": true, "BY": true, "OR": true, "AS": true, "NO": true}
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t == "," || strings.EqualFold(t, "AND") {
			continue
		}
		// "7 - 9", "7 THRU 9", "7 TO 9": a range written with spaces.
		if i+2 < len(toks) && (toks[i+1] == "-" || strings.EqualFold(toks[i+1], "THRU") || strings.EqualFold(toks[i+1], "THROUGH") || strings.EqualFold(toks[i+1], "TO")) {
			if m := reRange.FindStringSubmatch(t + "-" + toks[i+2]); m != nil {
				out = append(out, expandRange(m[1], m[2])...)
				i += 2
				continue
			}
		}
		if m := reRange.FindStringSubmatch(t); m != nil {
			out = append(out, expandRange(m[1], m[2])...)
			continue
		}
		if stop[strings.ToUpper(t)] || !pinValue.MatchString(t) {
			break
		}
		out = append(out, t)
	}
	return out
}

func expandRange(from, to string) []string {
	a, _ := strconv.Atoi(from)
	b, _ := strconv.Atoi(to)
	out := []string{}
	if b >= a && b-a <= 200 {
		for i := a; i <= b; i++ {
			out = append(out, strconv.Itoa(i))
		}
	}
	return out
}

// explicitUnusedFromNotes reads sentences such as "UNUSED CAVITIES P1: 4, 7-9".
func explicitUnusedFromNotes(d PDFData) []UnusedCavity {
	out := []UnusedCavity{}
	for _, p := range d.Pages {
		for _, l := range pageLines(p) {
			text := l.Text
			for _, m := range reUnusedNote.FindAllStringSubmatch(text, -1) {
				tag := normalizedTag(m[1])
				if tag == "" {
					if t := reTagFirst.FindStringSubmatch(text); t != nil {
						tag = normalizedTag(t[1])
					}
				}
				tag = strings.ReplaceAll(strings.ReplaceAll(tag, "-", ""), "_", "")
				for _, c := range parseCavityList(m[2]) {
					out = append(out, UnusedCavity{Connector: tag, Cavity: c, Page: p.Number, Source: "note on sheet " + strconv.Itoa(p.Number)})
				}
			}
		}
	}
	return out
}

// splitUnusedRows moves rows that document a not-connected cavity (SPARE,
// N/C, NOT USED) out of the conductor list only without physical-wire evidence.
func splitUnusedRows(rows []WireRow) ([]WireRow, []UnusedCavity) {
	keep := []WireRow{}
	unused := []UnusedCavity{}
	nc := func(s string) bool { return reNotConnected.MatchString(strings.TrimSpace(s)) }
	for _, r := range rows {
		noEnd := r.End == "UNRESOLVED ENDPOINT" && nc(r.EndSignal) && r.EndCavity == ""
		marked := explicitNotConnected(r.Signal) || (nc(r.Signal) && explicitNotConnected(r.EndSignal))
		noWire := nc(r.Wire) && nc(r.Color) && nc(r.AWG)
		if noEnd && marked && noWire && r.Connector != "" && r.Connector != "UNASSIGNED" {
			unused = append(unused, UnusedCavity{Connector: r.Connector, Cavity: r.Cavity, Page: r.Page, Source: "row marked " + strings.TrimSpace(firstNonEmpty(r.Signal, "not connected")) + " on sheet " + strconv.Itoa(r.Page)})
			continue
		}
		keep = append(keep, r)
	}
	return keep, unused
}

func firstNonEmpty(a ...string) string {
	for _, s := range a {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

func sortCavities(c []string) {
	sort.SliceStable(c, func(i, j int) bool {
		a, ea := strconv.Atoi(c[i])
		b, eb := strconv.Atoi(c[j])
		switch {
		case ea == nil && eb == nil:
			return a < b
		case ea == nil:
			return true
		case eb == nil:
			return false
		}
		return c[i] < c[j]
	})
}

// mirrorCheck compares a front and a rear view of the same connector: the
// rear (wire side) must be the front mirrored left-right.
func mirrorCheck(front, rear FaceView) string {
	pos := func(v FaceView) map[string][2]float64 {
		m := map[string][2]float64{}
		for _, p := range v.Positions {
			if p.Label != "" && v.R > 0 {
				m[p.Label] = [2]float64{p.X / v.R, p.Y / v.R}
			}
		}
		return m
	}
	f, r := pos(front), pos(rear)
	common, mirrored, same := 0, 0, 0
	for k, a := range f {
		b, ok := r[k]
		if !ok {
			continue
		}
		common++
		if math.Abs(a[0]+b[0]) < 0.08 && math.Abs(a[1]-b[1]) < 0.08 {
			mirrored++
		}
		if math.Abs(a[0]-b[0]) < 0.08 && math.Abs(a[1]-b[1]) < 0.08 {
			same++
		}
	}
	switch {
	case common < 3:
		return "Front and rear views share too few labelled cavities to compare."
	case mirrored == common:
		return fmt.Sprintf("OK — rear view is the front view mirrored (%d cavities compared).", common)
	case same == common:
		return fmt.Sprintf("CONFLICT — front and rear views are drawn identically (%d cavities); one caption or view is wrong.", common)
	}
	return fmt.Sprintf("CONFLICT — %d of %d labelled cavities are not mirrored between the front and rear views.", common-mirrored, common)
}

func buildCavityModels(a *Analysis, diagrams []InsertDiagram) []CavityModel {
	out := []CavityModel{}
	tags := []string{}
	parts := map[string]string{}
	counts := map[string]int{}
	for _, c := range a.Connectors {
		if c.Tag == "" || c.Tag == "UNASSIGNED" {
			continue
		}
		if _, ok := parts[c.Tag]; !ok {
			tags = append(tags, c.Tag)
		}
		parts[c.Tag], counts[c.Tag] = c.Part, c.CavityCount
	}
	for _, w := range a.Wires {
		for _, t := range []string{w.Connector, w.End} {
			if t == "" || t == "UNASSIGNED" || t == "UNRESOLVED ENDPOINT" || !reDestTag.MatchString(t) {
				continue
			}
			if _, ok := parts[t]; !ok {
				tags = append(tags, t)
				parts[t] = "NOT RESOLVED"
			}
		}
	}
	sort.Strings(tags)
	for _, tag := range tags {
		m := CavityModel{Tag: tag, Part: parts[tag], All: []string{}, Wired: []string{}, ExplicitUnused: []UnusedCavity{}, Unused: []string{}, SolidMarked: []string{}, OpenMarked: []string{}, Views: []FaceView{}, Conflicts: []string{}}
		for _, d := range diagrams {
			if d.NearTag != tag {
				continue
			}
			m.Views = append(m.Views, FaceView{Page: d.Page, Side: d.ViewSide, Box: d.Box, R: d.Outline.R, Positions: d.Positions, Labeled: d.Labeled})
		}
		// Cavity set: labelled source view or exact-part reviewed guide inventory.
		guide, hasGuide := guideInventory(parts[tag])
		best := -1
		for i, v := range m.Views {
			if len(v.Positions) > 0 && float64(v.Labeled) >= 0.8*float64(len(v.Positions)) && (best < 0 || v.Labeled > m.Views[best].Labeled) {
				best = i
			}
		}
		if best >= 0 {
			seen := map[string]bool{}
			for _, p := range m.Views[best].Positions {
				if p.Label != "" && !seen[p.Label] {
					seen[p.Label] = true
					m.All = append(m.All, p.Label)
				}
				switch {
				case p.Label == "":
				case p.Mark == "solid":
					m.SolidMarked = append(m.SolidMarked, p.Label)
				default:
					m.OpenMarked = append(m.OpenMarked, p.Label)
				}
			}
			m.AllSource = fmt.Sprintf("face view drawn on sheet %d (%s): %d cavities, %d labelled", m.Views[best].Page, m.Views[best].Side, len(m.Views[best].Positions), m.Views[best].Labeled)
			m.AllVerified = m.Views[best].Labeled == len(m.Views[best].Positions)
			if counts[tag] > 0 && counts[tag] != len(m.All) {
				m.Conflicts = append(m.Conflicts, fmt.Sprintf("The face view shows %d cavities but the catalog lists %d positions for %s.", len(m.All), counts[tag], parts[tag]))
			}
		} else if hasGuide {
			m.All = append([]string{}, guide.IDs...)
			m.AllSource = guide.Source
			m.AllVerified = true
		} else if counts[tag] > 0 {
			for i := 1; i <= counts[tag]; i++ {
				m.All = append(m.All, strconv.Itoa(i))
			}
			m.AllSource = fmt.Sprintf("catalog position count %d for %s; cavity numbering 1…%d ASSUMED — confirm against the insert arrangement", counts[tag], parts[tag], counts[tag])
		} else {
			m.AllSource = "unknown — no face view on the drawing and no catalog count for this part"
		}
		if hasGuide && best >= 0 {
			expected := map[string]bool{}
			for _, id := range guide.IDs {
				expected[id] = true
			}
			for _, id := range m.All {
				if !expected[id] {
					m.Conflicts = append(m.Conflicts, "Drawn cavity "+id+" is absent from the exact-part guide inventory ("+guide.Source+").")
				}
			}
			if !m.AllVerified {
				m.All = append([]string{}, guide.IDs...)
				m.AllSource = guide.Source
				m.AllVerified = true
			}
		}
		// Wired cavities (either end of a conductor).
		seen := map[string]bool{}
		for _, w := range a.Wires {
			if w.Connector == tag && w.Cavity != "" && !seen[w.Cavity] {
				seen[w.Cavity] = true
				m.Wired = append(m.Wired, w.Cavity)
			}
			if w.End == tag && w.EndCavity != "" && !seen[w.EndCavity] {
				seen[w.EndCavity] = true
				m.Wired = append(m.Wired, w.EndCavity)
			}
		}
		for _, u := range a.Unused {
			if u.Connector == tag {
				m.ExplicitUnused = append(m.ExplicitUnused, u)
				if seen[u.Cavity] {
					m.Conflicts = append(m.Conflicts, fmt.Sprintf("Cavity %s is wired but also declared unused (%s).", u.Cavity, u.Source))
				}
			}
		}
		inAll := map[string]bool{}
		for _, c := range m.All {
			inAll[c] = true
		}
		if m.AllVerified {
			for _, c := range m.Wired {
				if !inAll[c] {
					m.Conflicts = append(m.Conflicts, fmt.Sprintf("Cavity %s is wired but does not exist in the sourced cavity inventory.", c))
				}
			}
		}
		for _, c := range m.All {
			if !seen[c] {
				m.Unused = append(m.Unused, c)
			}
		}
		if len(m.All) == 0 {
			// No cavity inventory: the drawing's own unused statements are
			// still facts, but the list may be incomplete (AllSource says so).
			done := map[string]bool{}
			for _, u := range m.ExplicitUnused {
				if !seen[u.Cavity] && !done[u.Cavity] {
					done[u.Cavity] = true
					m.Unused = append(m.Unused, u.Cavity)
				}
			}
		} else {
			for _, u := range m.ExplicitUnused {
				if !inAll[u.Cavity] {
					m.Conflicts = append(m.Conflicts, fmt.Sprintf("Cavity %s is declared unused (%s) but is not in the cavity set (%s).", u.Cavity, u.Source, m.AllSource))
				}
			}
		}
		sortCavities(m.All)
		sortCavities(m.Wired)
		sortCavities(m.Unused)
		sortCavities(m.SolidMarked)
		sortCavities(m.OpenMarked)
		// Front/rear consistency.
		var front, rear *FaceView
		for i := range m.Views {
			v := &m.Views[i]
			if strings.HasPrefix(v.Side, "front") && front == nil {
				front = v
			}
			if strings.HasPrefix(v.Side, "rear") && rear == nil {
				rear = v
			}
		}
		if front != nil && rear != nil {
			m.MirrorCheck = mirrorCheck(*front, *rear)
			if strings.HasPrefix(m.MirrorCheck, "CONFLICT") {
				m.Conflicts = append(m.Conflicts, m.MirrorCheck)
			}
		}
		m.UnusedStatus = "CANDIDATE — confirm every wiring sheet"
		switch {
		case len(m.Conflicts) > 0:
			m.Unused = []string{}
			m.UnusedStatus = "HELD — resolve source and inventory conflicts"
		case len(m.Wired) == 0:
			m.Unused = []string{}
			m.UnusedStatus = "HELD — no wire paths recognized at this connector"
		case !m.AllVerified:
			m.UnusedStatus = "UNVERIFIED — exact inventory not established"
		}
		out = append(out, m)
	}
	return out
}

// dedupeUnused keeps one record per connector/cavity/sheet/source type.
func dedupeUnused(in []UnusedCavity) []UnusedCavity {
	seen := map[string]bool{}
	out := []UnusedCavity{}
	for _, u := range in {
		kind := "row"
		if strings.HasPrefix(u.Source, "note") {
			kind = "note"
		}
		k := u.Connector + "|" + u.Cavity + "|" + itoa(u.Page) + "|" + kind
		if !seen[k] {
			seen[k] = true
			out = append(out, u)
		}
	}
	return out
}
