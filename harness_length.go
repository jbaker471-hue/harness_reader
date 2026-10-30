package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// HarnessLength is an overall harness length stated on the drawing (usually in
// a note or title block on one of the first sheets). It carries only the
// number, unit, sheet and a fixed label, never the text of the line it came
// from, so saved projects do not carry extra drawing content.
type HarnessLength struct {
	Page      int    `json:"page"`
	Label     string `json:"label"`
	Text      string `json:"text"` // normalized, for example "48 IN"; the unit is empty when the drawing gives none
	Tolerance string `json:"tolerance,omitempty"`
	// Corroborated is set when the cable quantity in the parts list and the cable callouts both support this length.
	Corroborated bool   `json:"corroborated,omitempty"`
	Basis        string `json:"basis,omitempty"`
}

const maxHarnessLengthPage = 3

var (
	reLengthLabel = regexp.MustCompile(`\b(HARNESS|ASSEMBLY|CABLE|OVERALL|FINISHED|REQUIRED|REQD|CUT|WIRE)\s+LENGTH\b|^LENGTH\b|\bLENGTH\s+(?:REQD|REQUIRED)\b`)
	// Lengths of things that are not the harness.
	reLengthExclude = regexp.MustCompile(`\b(STRIP|STRIPPED|STRIPPING|SLEEV|SLEEVING|SHRINK|TWIST|LAY|BARREL|CRIMP|SOLDER|BRAID|SPLICE|TAIL|PIGTAIL|BREAKOUT|BRANCH|LEG|TIE|LACING|MARKER|LABEL|SCREW|BOLT|THREAD|BAND|HEIGHT|WIDTH)\b`)
	reLengthValue   = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*(?:(FT|FEET|')\s*[-\s]?\s*(\d+(?:\.\d+)?)\s*(IN(?:CH(?:ES)?)?|")?|(IN(?:CH(?:ES)?)?|"|FT|FEET|'|MM|CM|M)\b)?`)
	reLengthLead    = regexp.MustCompile(`^(?:[\s:=\-()]|\.{2,})*(?:(?:REF|APPROX|MIN|MAX|NOM)\.?(?:[\s:=\-()]|\.{2,})*)?`)
	// "CUT 5.0FT (+- 2IN) OF CABLE": an assembly step that states the cut length of the cable or wire without the word LENGTH.
	reCutOf     = regexp.MustCompile(`\bCUT\s+(\d+(?:\.\d+)?\s*(?:FT|FEET|'|IN(?:CH(?:ES)?)?|")?)(?:\s*\([^)]{0,24}\))?\s*OF\s+(?:CABLE|WIRE|HARNESS|ASSEMBLY)\b`)
	reLengthTol = regexp.MustCompile(`(?:\+\s*/\s*-|\+-|±)\s*(\d+(?:\.\d+)?)`)
)

func normalizeLengthMatch(m []string) string {
	n, _ := strconv.ParseFloat(m[1], 64)
	if m[2] != "" && m[3] != "" { // feet and inches, for example 4 FT 6 IN
		in, _ := strconv.ParseFloat(m[3], 64)
		return strconv.FormatFloat(n*12+in, 'f', -1, 64) + " IN"
	}
	unit := strings.ToUpper(m[5])
	if unit == "" && m[2] != "" { // a bare "4 FT"
		unit = strings.ToUpper(m[2])
	}
	switch unit {
	case "IN", "INCH", "INCHES", `"`:
		unit = "IN"
	case "FT", "FEET", "'":
		unit = "FT"
	}
	return strings.TrimSpace(m[1] + " " + unit)
}

func harnessLengths(pages []Page) []HarnessLength {
	out := []HarnessLength{}
	seen := map[string]bool{}
	add := func(page int, label, text, tol string) {
		k := fmt.Sprint(page, "|", label, "|", text)
		if text == "" || seen[k] {
			return
		}
		seen[k] = true
		out = append(out, HarnessLength{Page: page, Label: label, Text: text, Tolerance: tol})
	}
	for _, p := range pages {
		if p.Number > maxHarnessLengthPage {
			continue
		}
		lines := pageLines(p)
		for i, l := range lines {
			t := strings.ToUpper(strings.Join(strings.Fields(l.Text), " "))
			loc := reLengthLabel.FindStringSubmatchIndex(t)
			if loc == nil {
				continue
			}
			if loc[2] < 0 && reLengthExclude.MatchString(t) { // a bare LENGTH must not be a sleeve, strip or similar length
				continue
			}
			if loc[2] >= 0 && reLengthExclude.MatchString(t[:loc[0]]) {
				continue
			}
			label := "LENGTH"
			if loc[2] >= 0 {
				label = t[loc[2]:loc[3]] + " LENGTH"
			}
			rest := t[loc[1]:]
			rest = rest[len(reLengthLead.FindString(rest)):]
			// A label alone on its line takes its value from the next line.
			if rest == "" && i+1 < len(lines) {
				rest = strings.ToUpper(strings.Join(strings.Fields(lines[i+1].Text), " "))
			}
			// Only a number straight after the label counts; "PER NOTE 3" or ".25" do not.
			if rest == "" || rest[0] < '0' || rest[0] > '9' {
				continue
			}
			m := reLengthValue.FindStringSubmatch(rest)
			if m == nil {
				continue
			}
			tol := ""
			if tm := reLengthTol.FindStringSubmatch(rest); tm != nil {
				tol = "±" + tm[1]
			}
			add(p.Number, label, normalizeLengthMatch(m), tol)
		}
	}
	// The cut length of a cable is often stated in an assembly step on any sheet. It is reported once per distinct value.
	seenCut := map[string]bool{}
	for _, p := range pages {
		for _, l := range pageLines(p) {
			t := strings.ToUpper(strings.Join(strings.Fields(l.Text), " "))
			m := reCutOf.FindStringSubmatchIndex(t)
			if m == nil {
				continue
			}
			v := reLengthValue.FindStringSubmatch(t[m[2]:m[3]])
			if v == nil {
				continue
			}
			text := normalizeLengthMatch(v)
			tol := ""
			if tm := reLengthTol.FindStringSubmatch(t[m[3]:]); tm != nil {
				tol = "±" + tm[1]
			}
			if text == "" || seenCut[text+"|"+tol] {
				continue
			}
			seenCut[text+"|"+tol] = true
			add(p.Number, "CUT", text, tol)
		}
	}
	out = append(out, harnessDimensions(pages)...)
	return out
}
