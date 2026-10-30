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
	return out
}
