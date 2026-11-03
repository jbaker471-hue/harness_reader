package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Checks that look for weaknesses in the drawing itself rather than in the extraction. Every finding is a REVIEW
// item to confirm against the sheet; none of them changes a value.

var (
	reColorParen = regexp.MustCompile(`\(([A-Za-z][A-Za-z/ ]{1,23})\)`)
	reParen      = regexp.MustCompile(`\([^)]*\)`)
	reNonAlnum   = regexp.MustCompile(`[^A-Z0-9]`)
)

func signalKey(s string) string {
	return reNonAlnum.ReplaceAllString(strings.ToUpper(reParen.ReplaceAllString(s, "")), "")
}

// swapTxRx exchanges TX and RX so that a deliberate transmit/receive crossover is not reported as a rename.
func swapTxRx(k string) string {
	k = strings.NewReplacer("TXD", "\x01", "RXD", "\x02", "TX", "\x03", "RX", "\x04").Replace(k)
	return strings.NewReplacer("\x01", "RXD", "\x02", "TXD", "\x03", "RX", "\x04", "TX").Replace(k)
}

func drawingWeaknessIssues(a *Analysis) []ViewIssue {
	out := []ViewIssue{}
	add := func(code, msg, action string, conn string, pages map[int]bool) {
		pg := []int{}
		for p := range pages {
			pg = append(pg, p)
		}
		sort.Ints(pg)
		out = append(out, ViewIssue{Severity: "REVIEW", Category: CatReview, Code: code, Message: msg, Connector: conn, Pages: pg, Cavities: []string{}, Action: action})
	}

	// 1. A colour written in brackets that is not a recognised colour (for example a typo): the colour is left blank, never guessed.
	typos := map[string][]string{}
	typoPages := map[string]map[int]bool{}
	for _, w := range a.Wires {
		if strings.TrimSpace(w.Color) != "" {
			continue
		}
		for _, s := range []string{w.EndSignal, w.Signal} {
			if m := reColorParen.FindStringSubmatch(s); m != nil {
				txt := strings.TrimSpace(m[1])
				typos[txt] = append(typos[txt], firstNonEmpty(w.Signal, w.EndSignal))
				if typoPages[txt] == nil {
					typoPages[txt] = map[int]bool{}
				}
				typoPages[txt][w.Page] = true
				break
			}
		}
	}
	keys := make([]string, 0, len(typos))
	for k := range typos {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		add("WIRE_COLOR_NOT_RECOGNIZED", fmt.Sprintf("The colour text \"%s\" written for %s is not a recognised colour, possibly a typo. The colour was left blank, not guessed.", k, strings.Join(dedupeStrings(typos[k]), ", ")),
			"Confirm the colour on the sheet and ask engineering to correct the spelling.", "", typoPages[k])
	}

	// 2. The signal name changes across a connection (other than a TX/RX crossover).
	type grp struct {
		page int
		far  string
	}
	changed := map[grp][][2]string{}
	compared := map[grp]int{}
	for _, w := range a.Wires {
		if strings.TrimSpace(w.Signal) == "" || strings.TrimSpace(w.EndSignal) == "" || w.End == "" || w.End == "UNRESOLVED ENDPOINT" {
			continue
		}
		g := grp{w.Page, w.End}
		compared[g]++
		l, r := signalKey(w.Signal), signalKey(w.EndSignal)
		if l == "" || r == "" || l == r || l == swapTxRx(r) {
			continue
		}
		changed[g] = append(changed[g], [2]string{strings.TrimSpace(reParen.ReplaceAllString(w.Signal, "")), strings.TrimSpace(reParen.ReplaceAllString(w.EndSignal, ""))})
	}
	gs := make([]grp, 0, len(changed))
	for g := range changed {
		gs = append(gs, g)
	}
	sort.Slice(gs, func(i, j int) bool {
		if gs[i].page != gs[j].page {
			return gs[i].page < gs[j].page
		}
		return gs[i].far < gs[j].far
	})
	for _, g := range gs {
		ex := []string{}
		for i, p := range changed[g] {
			if i == 3 {
				break
			}
			ex = append(ex, p[0]+" becomes "+p[1])
		}
		add("SIGNAL_NAME_CHANGES", fmt.Sprintf("Sheet %d, to %s: the signal name differs between the two ends on %d of %d wires (%s). A rename can be intentional, but it also hides a mix-up.", g.page, g.far, len(changed[g]), compared[g], strings.Join(ex, "; ")),
			"Confirm the names are meant to differ.", "", map[int]bool{g.page: true})
	}

	// 3. One cavity carries wires to different places. Two wires on one contact can be legitimate, so this is a question, not an error.
	type pin struct{ conn, cav string }
	far := map[pin]map[string]bool{}
	pinPages := map[pin]map[int]bool{}
	note := func(p pin, other string, page int) {
		if p.conn == "" || p.cav == "" || p.conn == "UNASSIGNED" || strings.HasPrefix(p.conn, "@") {
			return
		}
		if far[p] == nil {
			far[p], pinPages[p] = map[string]bool{}, map[int]bool{}
		}
		far[p][other] = true
		pinPages[p][page] = true
	}
	for _, w := range a.Wires {
		note(pin{w.Connector, w.Cavity}, w.End+":"+w.EndCavity, w.Page)
		note(pin{w.End, w.EndCavity}, w.Connector+":"+w.Cavity, w.Page)
	}
	pins := make([]pin, 0)
	for p, s := range far {
		if len(s) > 1 {
			pins = append(pins, p)
		}
	}
	sort.Slice(pins, func(i, j int) bool { return pins[i].conn+":"+pins[i].cav < pins[j].conn+":"+pins[j].cav })
	for _, p := range pins {
		to := []string{}
		for o := range far[p] {
			to = append(to, o)
		}
		sort.Strings(to)
		add("PIN_USED_TWICE", fmt.Sprintf("%s pin %s goes to more than one place: %s. Two wires in one contact are sometimes intended, but it is also a common drawing mistake.", p.conn, p.cav, strings.Join(to, ", ")),
			"Confirm against the sheet that this pin is meant to carry more than one wire.", p.conn, pinPages[p])
	}
	return out
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
