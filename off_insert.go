package main

import (
	"fmt"
	"sort"
	"strings"
)

// offInsertIssues reports wires that land on a cavity the insert does not have, for example pin 112 on a
// 79-position connector. It is a drawing discrepancy to send back to engineering, not something to correct
// here: the wire's real pin is unknown, so it is also unknown how many of the unwired cavities are unused.
func offInsertIssues(a *Analysis, models []CavityModel) []ViewIssue {
	out := []ViewIssue{}
	for _, m := range models {
		if !m.AllVerified || len(m.All) == 0 {
			continue
		}
		on := map[string]bool{}
		for _, id := range m.All {
			on[id] = true
		}
		off := []string{}
		for _, c := range m.Wired {
			if !on[c] {
				off = append(off, c)
			}
		}
		if len(off) == 0 {
			continue
		}
		sortCavities(off)
		isOff := map[string]bool{}
		for _, c := range off {
			isOff[c] = true
		}
		pages, sigs := map[int]bool{}, []string{}
		for _, w := range a.Wires {
			switch {
			case w.Connector == m.Tag && isOff[w.Cavity]:
				pages[w.Page] = true
				sigs = append(sigs, firstNonEmpty(w.Signal, w.Wire, w.End)+" (pin "+w.Cavity+")")
			case w.End == m.Tag && isOff[w.EndCavity]:
				pages[w.Page] = true
				sigs = append(sigs, firstNonEmpty(w.EndSignal, w.Wire, w.Connector)+" (pin "+w.EndCavity+")")
			}
		}
		pg := []int{}
		for p := range pages {
			pg = append(pg, p)
		}
		sort.Ints(pg)
		candidates := 0
		for _, id := range m.All {
			wired := false
			for _, w := range m.Wired {
				if w == id {
					wired = true
					break
				}
			}
			if !wired {
				candidates++
			}
		}
		low := candidates - len(off)
		if low < 0 {
			low = 0
		}
		head := "pin " + strings.Join(off, ", ") + " is wired on the drawing but the insert has no such cavity"
		if len(off) > 1 {
			head = "pins " + strings.Join(off, ", ") + " are wired on the drawing but the insert has no such cavities"
		}
		msg := fmt.Sprintf("%s (%s, %d cavities): %s. Return to engineering for the correct pin numbers. Until then, %d to %d of the %d cavities with no wire are unused: %d if every one of these wires is real and needs its own cavity, %d if they are errors.",
			m.Tag, m.Part, len(m.All), head, low, candidates, candidates, low, candidates)
		if len(sigs) > 0 {
			msg += " Affected: " + strings.Join(sigs, "; ") + "."
		}
		out = append(out, ViewIssue{Severity: "REVIEW", Category: CatReview, Code: "PIN_NOT_ON_INSERT", Message: msg, Connector: m.Tag, Part: m.Part, Pages: pg, Cavities: off,
			Action: "Send back to engineering: the wire list uses pin numbers that the connector part number cannot have."})
	}
	return out
}
