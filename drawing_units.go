package main

import (
	"regexp"
	"sort"
	"strings"
)

// DrawingUnit is the measurement unit the drawing itself states in a note or title block, for example
// "UNLESS OTHERWISE SPECIFIED, DIMENSIONS ARE IN INCHES". It is read from the sheet text, never assumed.
type DrawingUnit struct {
	Unit string `json:"unit"` // IN, FT, MM, CM or M
	Text string `json:"text"` // the drawing's own words
	Page int    `json:"page"`
}

var reUnitStatement = regexp.MustCompile(`(?i)\b(?:DIM(?:ENSION)?S?|UNITS?|MEASUREMENTS?|LENGTHS?)\b[^.;:]{0,40}?\b(?:ARE\s+|ARE\s+GIVEN\s+|GIVEN\s+|SHOWN\s+|STATED\s+|=\s*|:\s*)?\bIN\s+(INCHES|INCH|IN\.|MILLIMETERS|MILLIMETRES|MILLIMETER|MILLIMETRE|MM|CENTIMETERS|CENTIMETRES|CM|METERS|METRES|METER|METRE|FEET|FOOT|FT)\b`)
var reUnitColon = regexp.MustCompile(`(?i)\bUNITS?\s*[:=]\s*(INCHES|INCH|MILLIMETERS|MILLIMETRES|MM|CENTIMETERS|CENTIMETRES|CM|METERS|METRES|FEET|FT)\b`)

func unitCode(word string) string {
	switch strings.ToUpper(strings.TrimRight(word, ".")) {
	case "INCHES", "INCH", "IN":
		return "IN"
	case "MILLIMETERS", "MILLIMETRES", "MILLIMETER", "MILLIMETRE", "MM":
		return "MM"
	case "CENTIMETERS", "CENTIMETRES", "CM":
		return "CM"
	case "METERS", "METRES", "METER", "METRE":
		return "M"
	case "FEET", "FOOT", "FT":
		return "FT"
	}
	return ""
}

// drawingUnitStatement returns the unit the drawing states, or nil when it states none or states two different ones.
func drawingUnitStatement(pages []Page) *DrawingUnit {
	found := []DrawingUnit{}
	for _, p := range pages {
		lines := pageLines(p)
		// title-block notes wrap, so a statement can sit across two neighbouring lines as well as on one
		sort.SliceStable(lines, func(i, j int) bool { return lines[i].Y > lines[j].Y })
		texts := []string{}
		for _, l := range lines {
			texts = append(texts, strings.ToUpper(strings.Join(strings.Fields(l.Text), " ")))
		}
		for i, t := range texts {
			cands := []string{t}
			if i+1 < len(texts) {
				cands = append(cands, t+" "+texts[i+1])
			}
			for _, c := range cands {
				m := reUnitStatement.FindStringSubmatch(c)
				if m == nil {
					m = reUnitColon.FindStringSubmatch(c)
				}
				if m == nil {
					continue
				}
				if u := unitCode(m[len(m)-1]); u != "" {
					found = append(found, DrawingUnit{Unit: u, Text: strings.TrimSpace(m[0]), Page: p.Number})
					break
				}
			}
		}
	}
	if len(found) == 0 {
		return nil
	}
	for _, f := range found[1:] {
		if f.Unit != found[0].Unit {
			return nil
		}
	}
	return &found[0]
}

// applyDrawingUnit gives a printed dimension that carries no unit the unit the drawing states.
func applyDrawingUnit(ls []HarnessLength, du *DrawingUnit) []HarnessLength {
	if du == nil {
		return ls
	}
	out := make([]HarnessLength, len(ls))
	copy(out, ls)
	for i, h := range out {
		if h.Label == "DIMENSION" && len(strings.Fields(h.Text)) == 1 {
			out[i].Text = strings.TrimSpace(h.Text) + " " + du.Unit
			out[i].UnitNote = "unit from the drawing: " + du.Text
		}
	}
	return out
}
