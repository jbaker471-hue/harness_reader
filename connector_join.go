package main

import (
	"regexp"
	"strings"
)

// One connector is sometimes read as two records: a short designator (P2) that carries the part number and the
// position count but no wires, and a longer designator ending in it (W1-P2) that carries the wires but no part.
// joinSplitConnectors makes them one connector, so every view of the drawing counts its positions once.
//
// It joins only when the match is certain from the records themselves:
//   - the short record has a resolved part and a position count, and no wire names it;
//   - exactly one wired record's designator ends in the short designator after a separator;
//   - that wired record has no part and no position count of its own;
//   - its wired cavities fit within the short record's positions.
//
// Anything else (two wired records ending in the same short name, a wired record with its own part) is left alone.

var reTagSplit = regexp.MustCompile(`[^A-Z0-9]+`)

func tagTail(tag string) string {
	parts := []string{}
	for _, p := range reTagSplit.Split(strings.ToUpper(strings.TrimSpace(tag)), -1) {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-1]
}

func partResolved(part string) bool {
	p := strings.ToUpper(strings.TrimSpace(part))
	return p != "" && p != "NOT RESOLVED"
}

// joinSplitConnectors returns the designators that were joined, as pairs of short name and the wired name it joined.
func joinSplitConnectors(a *Analysis) [][2]string {
	up := func(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }
	cavities := map[string]map[string]bool{}
	note := func(tag, cavity string) {
		t := up(tag)
		if t == "" {
			return
		}
		if cavities[t] == nil {
			cavities[t] = map[string]bool{}
		}
		cavities[t][up(cavity)] = true
	}
	for _, w := range a.Wires {
		note(w.Connector, w.Cavity)
		note(w.End, w.EndCavity)
	}
	wired := func(tag string) int { return len(cavities[up(tag)]) }

	joined := [][2]string{}
	taken := map[int]bool{} // index of a wired record that already took a short record
	drop := map[int]bool{}  // index of a short record that was joined
	for di := range a.Connectors {
		d := &a.Connectors[di]
		if up(d.Tag) == "" || !partResolved(d.Part) || d.CavityCount <= 0 || wired(d.Tag) > 0 {
			continue
		}
		match := -1
		count := 0
		for mi := range a.Connectors {
			if mi == di || drop[mi] {
				continue
			}
			m := &a.Connectors[mi]
			if tagTail(m.Tag) == up(d.Tag) && wired(m.Tag) > 0 {
				match = mi
				count++
			}
		}
		if count != 1 || taken[match] {
			continue
		}
		m := &a.Connectors[match]
		if partResolved(m.Part) || m.CavityCount > 0 || wired(m.Tag) > d.CavityCount {
			continue
		}
		m.Part = d.Part
		m.CavityCount = d.CavityCount
		if m.Family == "" {
			m.Family = d.Family
		}
		if m.LayoutStatus == "" {
			m.LayoutStatus = d.LayoutStatus
		}
		seen := map[int]bool{}
		for _, p := range m.Pages {
			seen[p] = true
		}
		for _, p := range d.Pages {
			if !seen[p] {
				seen[p] = true
				m.Pages = append(m.Pages, p)
			}
		}
		taken[match] = true
		drop[di] = true
		joined = append(joined, [2]string{d.Tag, m.Tag})
	}
	if len(drop) == 0 {
		return joined
	}
	kept := a.Connectors[:0:0]
	for i, c := range a.Connectors {
		if !drop[i] {
			kept = append(kept, c)
		}
	}
	a.Connectors = kept
	return joined
}
