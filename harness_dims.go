package main

import (
	"regexp"
	"strconv"
	"strings"
)

// harnessDimensions offers dimensions printed on the first sheets with a tolerance (for example "36.00 ±1.0")
// as candidates for an overall length. They carry no label and no unit, so they are never selected
// automatically: the user decides whether one of them is the length the cut list should use.
var reDimTol = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(?:±|\+/-|\+-)\s*(\d+(?:\.\d+)?)`)

// an unequal tolerance such as "216 -0.0/+1.0" or "216 +1.0/-0.0"
var reDimAsym = regexp.MustCompile(`(\d+(?:\.\d+)?)\s+(?:[-−]\s*(\d+(?:\.\d+)?)\s*/\s*\+\s*(\d+(?:\.\d+)?)|\+\s*(\d+(?:\.\d+)?)\s*/\s*[-−]\s*(\d+(?:\.\d+)?))`)

var reDimInches = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(?:INCHES|INCH|IN\.?|")$`)

const minDimensionCandidate = 12 // smaller dimensions are strip lengths, sleeve overlaps and similar

func harnessDimensions(pages []Page) []HarnessLength {
	out := []HarnessLength{}
	seen := map[string]bool{}
	for _, p := range pages {
		if p.Number > maxHarnessLengthPage {
			continue
		}
		for _, l := range pageLines(p) {
			t := strings.ToUpper(strings.Join(strings.Fields(l.Text), " "))
			if reLengthExclude.MatchString(t) || strings.Contains(t, "SHEET") || strings.Contains(t, "REV") {
				continue
			}
			for _, m := range reDimAsym.FindAllStringSubmatch(t, -1) {
				v, err := strconv.ParseFloat(m[1], 64)
				if err != nil || v < minDimensionCandidate {
					continue
				}
				minus, plus := m[2], m[3]
				if m[4] != "" {
					plus, minus = m[4], m[5]
				}
				k := m[1] + "|-" + minus + "/+" + plus
				if seen[k] {
					continue
				}
				seen[k] = true
				out = append(out, HarnessLength{Page: p.Number, Label: "DIMENSION", Text: m[1], Tolerance: "-" + minus + "/+" + plus})
			}
			for _, m := range reDimTol.FindAllStringSubmatch(t, -1) {
				v, err := strconv.ParseFloat(m[1], 64)
				if err != nil || v < minDimensionCandidate {
					continue
				}
				k := m[1] + "|" + m[2]
				if seen[k] {
					continue
				}
				seen[k] = true
				out = append(out, HarnessLength{Page: p.Number, Label: "DIMENSION", Text: m[1], Tolerance: "±" + m[2]})
			}
		}
	}
	// A length printed on its own with an inch unit and no tolerance (for example 216" or 216 IN). It must stand
	// alone: the whole text group is the number and its unit, on a line with few other words, so a quantity and
	// unit in a parts list row is not taken for a length.
	for _, p := range pages {
		if p.Number > maxHarnessLengthPage || !p.PageAccurate {
			continue
		}
		words := map[float64]int{}
		for _, l := range pageLines(p) {
			words[l.Y] = len(strings.Fields(l.Text))
		}
		for _, s := range noteSegments(p) {
			t := strings.ToUpper(strings.Join(strings.Fields(s.text()), " "))
			// the number and its unit, alone or with one more word beside it (REF, MAX, a symbol)
			toks := strings.Fields(t)
			var m []string
			if len(toks) <= 3 {
				for i, tk := range toks {
					if m = reDimInches.FindStringSubmatch(tk); m != nil {
						break
					}
					if i+1 < len(toks) {
						if m = reDimInches.FindStringSubmatch(tk + " " + toks[i+1]); m != nil {
							break
						}
					}
				}
			}
			if m == nil || reLengthExclude.MatchString(t) {
				continue
			}
			busy := false
			for y, n := range words {
				if abs(y-s.y) < 3.6 && n > 6 {
					busy = true
				}
			}
			v, err := strconv.ParseFloat(m[1], 64)
			if busy || err != nil || v < minDimensionCandidate {
				continue
			}
			k := m[1] + "|IN"
			if seen[k] || seen[m[1]+"|"] {
				continue
			}
			seen[k] = true
			out = append(out, HarnessLength{Page: p.Number, Label: "DIMENSION", Text: m[1] + " IN"})
		}
	}
	return out
}
