package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Connector family from the part number. The family decides the shell shape
// drawn on the face view. Cavity positions are drawn only when a layout can be
// stated for the exact part: today that is the standard two-row D-subminiature
// layouts (computed from row counts and pitch). Every other family gets a shell
// outline and a plain cavity list, never invented positions.

type GeoPos struct {
	Label string  `json:"label"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	R     float64 `json:"r"`
}

type FaceGeometry struct {
	Shape     string       `json:"shape"`
	Outline   [][2]float64 `json:"outline,omitempty"` // polygon, same units as Positions, Y down, centred
	Positions []GeoPos     `json:"positions"`
	View      string       `json:"view"` // pin-front, socket-front, pin-rear or socket-rear: the face the positions are drawn as seen
	Face      string       `json:"face"` // the same, in words
	Basis     string       `json:"basis"`
	Status    string       `json:"status"`
}

type ConnectorFamilyInfo struct {
	Part       string               `json:"part"`
	Recognized bool                 `json:"recognized"`
	Family     string               `json:"family"`
	Shape      string               `json:"shape"` // circular, d-sub, micro-d, rectangular, unknown
	ShellSize  string               `json:"shellSize,omitempty"`
	Sex        string               `json:"sex,omitempty"` // PIN, SOCKET or empty when the part number does not say
	Positions  int                  `json:"positions,omitempty"`
	Geometry   *FaceGeometry        `json:"geometry,omitempty"`
	Expected   *ExpectedArrangement `json:"expected,omitempty"` // what the catalogs state this arrangement holds
	Specs      []ConnectorDocRef    `json:"specs"`              // family specification(s) from the connector reference library
	Layouts    []ConnectorDocRef    `json:"layoutStandards"`    // standards that define this family's insert arrangements
	Notes      []string             `json:"notes"`
	Status     string               `json:"status"`
}

const familyStatus = "FAMILY FROM PART-NUMBER PATTERN — UNVERIFIED"

var (
	reDSubPart     = regexp.MustCompile(`^(?:D([EABCD])-?(\d{1,2})([PS])?|DSUB-?(\d{1,2})([PS])?)(?:[-/].*)?$`)
	reM24308       = regexp.MustCompile(`^M24308/`)
	reMicroD       = regexp.MustCompile(`^M83513/`)
	reDeutschDT    = regexp.MustCompile(`^DT[MP]?\d{2}-\d{1,2}[PS]`)
	reMS3xxx       = regexp.MustCompile(`^MS3(?:10[0-8]|11\d|12\d|4\d\d)`)
	reMS5015Arr    = regexp.MustCompile(`^MS3\d{3}[A-Z]{0,3}-?(\d{1,2}S?)-(\d{1,3})[A-Z]{0,3}\d*$`)
	reM83723       = regexp.MustCompile(`^M83723/`)
	reSeriesIIIAny = regexp.MustCompile(`^D38999/2[0-6]([A-Z])([A-HJ])(\d{1,2})([PSHJAB])`)
	re38999        = regexp.MustCompile(`^(?:D|M)38999/`)
)

var shellSeriesIII = map[string]string{"A": "9", "B": "11", "C": "13", "D": "15", "E": "17", "F": "19", "G": "21", "H": "23", "J": "25"}

// Standard-density two-row D-sub contact rows, widest row first. Other counts
// (three-row and high-density shells) are left without positions.
var dsubRows = map[int][2]int{9: {5, 4}, 15: {8, 7}, 25: {13, 12}, 37: {19, 18}}

const (
	dsubPitch    = 2.77 // mm between contacts in a row
	dsubRowSpace = 2.84 // mm between rows
)

func dsubGeometry(n int) *FaceGeometry {
	rows, ok := dsubRows[n]
	if !ok {
		return nil
	}
	g := &FaceGeometry{Shape: "d-sub", View: "pin-front", Face: "male (pin) face, as seen from the mating side", Status: "COMPUTED FROM STANDARD ROW COUNTS — UNVERIFIED",
		Basis: fmt.Sprintf("Standard-density D-subminiature: %d + %d contacts, %.2f mm pitch, %.2f mm between rows; shell outline is schematic. Numbering runs left to right along the wide row, then the narrow row.", rows[0], rows[1], dsubPitch, dsubRowSpace)}
	half := float64(rows[0]-1) * dsubPitch / 2
	top, bot := half+4.2, half+2.0
	g.Outline = [][2]float64{{-top, -4.6}, {top, -4.6}, {bot, 4.6}, {-bot, 4.6}}
	k := 1
	for r, cnt := range rows {
		y := -dsubRowSpace/2 + float64(r)*dsubRowSpace
		x0 := -float64(cnt-1) * dsubPitch / 2
		for i := 0; i < cnt; i++ {
			g.Positions = append(g.Positions, GeoPos{Label: strconv.Itoa(k), X: x0 + float64(i)*dsubPitch, Y: y, R: 0.8})
			k++
		}
	}
	return g
}

// DecodeConnectorFamily reads the family and layout from the part number alone.
func DecodeConnectorFamily(part string) ConnectorFamilyInfo {
	return DecodeConnectorFamilyWith(part, "")
}

// DecodeConnectorFamilyWith lets the user pick a stored insert arrangement by id (for example "15-35")
// when the part number does not identify one. A chosen arrangement is only used if it is stored.
func DecodeConnectorFamilyWith(part, arrangement string) ConnectorFamilyInfo {
	p := strings.ToUpper(strings.TrimSpace(part))
	out := ConnectorFamilyInfo{Part: p, Shape: "unknown", Notes: []string{}, Specs: []ConnectorDocRef{}, Layouts: []ConnectorDocRef{}, Status: "UNRESOLVED"}
	if p == "" {
		out.Notes = append(out.Notes, "No part number: the connector family cannot be determined.")
		if arrangement == "" {
			return out
		}
	}
	fromPart := ""
	set := func(family, shape, specCode string) {
		out.Recognized, out.Family, out.Shape, out.Status = true, family, shape, familyStatus
		if d := lookupConnectorDoc("family-spec", specCode); d != nil {
			out.Specs = append(out.Specs, *d)
			for _, l := range d.Layout {
				if ls := lookupConnectorDoc("layout-standard", l); ls != nil {
					out.Layouts = append(out.Layouts, *ls)
				}
			}
		}
	}
	switch {
	case re38999.MatchString(p):
		set("MIL-DTL-38999 circular", "circular", "MIL-DTL-38999")
		out.Notes = append(out.Notes, "Keyway positions and alternate rotations are not drawn; a stored layout is shown for normal rotation only.")
		if m := reSeriesIIIAny.FindStringSubmatch(p); m != nil {
			out.ShellSize = shellSeriesIII[m[2]]
			n, _ := strconv.Atoi(m[3])
			fromPart = out.ShellSize + "-" + strconv.Itoa(n)
			out.Sex = map[string]string{"P": "PIN", "H": "PIN", "A": "PIN", "S": "SOCKET", "J": "SOCKET", "B": "SOCKET"}[m[4]]
		}
		if rc := ResolveConnectorContacts(p); rc.PositionCount > 0 { // known contact count for the X35 arrangements
			out.Positions = rc.PositionCount
		}
	case reMS3xxx.MatchString(p):
		set("MIL-spec circular (MS3xxx; confirm the governing specification)", "circular", "MIL-DTL-5015 / SAE-AS50151")
		out.Notes = append(out.Notes, "The exact specification behind an MS3 number should be confirmed against the connector standard.")
	case reM83723.MatchString(p):
		set("MIL-DTL-83723 circular", "circular", "MIL-DTL-83723")
	case reMicroD.MatchString(p):
		set("MIL-DTL-83513 micro-D", "micro-d", "MIL-DTL-83513")
	case reM24308.MatchString(p):
		set("MIL-DTL-24308 D-subminiature", "d-sub", "MIL-DTL-24308")
		out.Notes = append(out.Notes, "Contact count and density are not read from the M24308 part number here, so no positions are drawn.")
	case reDeutschDT.MatchString(p):
		set("Deutsch DT / DTM / DTP", "rectangular", "")
	default:
		if m := reDSubPart.FindStringSubmatch(p); m != nil {
			n, sex, shellLetter := 0, "", m[1]
			if m[2] != "" {
				n, _ = strconv.Atoi(m[2])
				sex = m[3]
			} else {
				n, _ = strconv.Atoi(m[4])
				sex = m[5]
			}
			set("D-subminiature", "d-sub", "MIL-DTL-24308")
			out.Positions = n
			out.Sex = map[string]string{"P": "PIN", "S": "SOCKET"}[sex]
			if shellLetter != "" {
				shell := map[string]int{"E": 9, "A": 15, "B": 25, "C": 37, "D": 50}[shellLetter]
				if shell != n {
					out.Notes = append(out.Notes, fmt.Sprintf("CONFLICT: shell letter D%s is the %d-position shell but the part number says %d.", shellLetter, shell, n))
					out.Positions = 0
				}
			}
			if out.Positions > 0 && out.Sex != "" {
				out.Geometry = dsubGeometry(out.Positions)
			} else if out.Positions > 0 {
				out.Notes = append(out.Notes, "Pin or socket is not stated in the part number, so the face cannot be oriented; positions are not drawn.")
			}
		}
	}
	// A stored insert layout is picked from the arrangement in the part number, or one the user chose.
	arr := fromPart
	if arrangement != "" {
		if fromPart != "" && fromPart != arrangement {
			out.Notes = append(out.Notes, "CONFLICT: the part number says insert arrangement "+fromPart+" but "+arrangement+" was chosen; the chosen one is shown.")
		}
		arr = arrangement
	}
	if arr != "" && out.Geometry == nil && out.Shape != "d-sub" {
		g, problems := insertGeometryFor("MIL-STD-1560", arr)
		out.Notes = append(out.Notes, problems...)
		switch {
		case g == nil:
			out.Notes = append(out.Notes, "No stored insert layout for arrangement "+arr+"; cavity positions are not drawn.")
		case out.Positions > 0 && len(g.Positions) != out.Positions:
			out.Notes = append(out.Notes, fmt.Sprintf("CONFLICT: the stored insert layout %s has %d cavities but the part number resolves to %d; no positions are drawn.", arr, len(g.Positions), out.Positions))
		default:
			out.Geometry = g
			if out.Positions == 0 {
				out.Positions = len(g.Positions)
			}
		}
	}
	out.Expected = expectedFor(p, arr)
	if !out.Recognized {
		out.Notes = append(out.Notes, "Connector family not recognized from this part number. No shell shape is drawn.")
	}
	return out
}
