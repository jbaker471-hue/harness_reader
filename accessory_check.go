package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Connector accessory / BOM consistency check (v2.8).
//
// For every connector, work out what the connector needs (backshell size,
// contacts, sealing plugs for unused cavities, bands and their tools) from
// SOURCED reference data, and compare it with the BOM that is currently in
// the workspace (extracted, CSV-imported or typed). Nothing is guessed: a
// requirement without a verified rule is reported as UNRESOLVED.

type AccConnector struct {
	Tag      string   `json:"tag"`
	Part     string   `json:"part"`
	Occupied []string `json:"occupied"` // documented populated cavities (may be empty = unknown)
	Capacity int      `json:"capacity"`
}

type AccBOM struct {
	Item        string `json:"item"`
	Part        string `json:"part"`
	Qty         string `json:"qty"`
	Description string `json:"description"`
}

type AccRequest struct {
	Connectors []AccConnector `json:"connectors"`
	BOM        []AccBOM       `json:"bom"`
}

type AccCheck struct {
	Connector string   `json:"connector"`
	Part      string   `json:"part"`
	Check     string   `json:"check"`
	Status    string   `json:"status"` // OK | MISMATCH | MISSING | VERIFY | UNRESOLVED | INFO
	Expected  string   `json:"expected"`
	Found     string   `json:"found"`
	Detail    string   `json:"detail"`
	Tooling   []string `json:"tooling"`
	Source    string   `json:"source"`
}

type accRef struct {
	kind, class, requirement, source, specification, alternates, alternateSource, alternateStatus string
	match                                                                                         []string
}

func loadAccessoryRefs() ([]accRef, error) {
	rows, err := readLibrary("ACCESSORY_REFERENCE.csv")
	if err != nil {
		return nil, err
	}
	out := []accRef{}
	for _, r := range rows[1:] {
		ref := accRef{kind: r[0], match: strings.Split(r[1], "|"), class: r[2], requirement: r[3], source: r[4]}
		if len(r) > 9 {
			ref.specification, ref.alternates, ref.alternateSource, ref.alternateStatus = r[6], r[7], r[8], r[9]
		}
		out = append(out, ref)
	}
	return out, nil
}

var (
	reSeries3              = regexp.MustCompile(`^D38999/(2[0-9])([A-Z])([A-HJ])([0-9]{2})([PSHJAB])([NABCDE])?$`)
	shellByCode            = map[string]int{"A": 9, "B": 11, "C": 13, "D": 15, "E": 17, "F": 19, "G": 21, "H": 23, "J": 25}
	reBackshell            = regexp.MustCompile(`^M85049/(38|39)(S|-)?([0-9]{1,2})([ANWS])?$`)
	reAnyM85049            = regexp.MustCompile(`^(?:M|AS|SAE-AS)85049/`)
	reContactPN            = regexp.MustCompile(`^M39029/([0-9]{1,3})-([0-9]{1,4})$`)
	reCrimpRingBackshellPN = regexp.MustCompile(`^(?:M|AS|SAE-AS)85049/(?:20|26-[123])(?:-|$)`)
	reClampBackshellPN     = regexp.MustCompile(`^(?:M|AS|SAE-AS)85049/(?:38|39|118|120|121|123|124|126)(?:-|$)`)
	reTinelBackshellPN     = regexp.MustCompile(`^(?:TXR|STXR|SETR)[A-Z0-9-]*$`)
)

func backshellTerminationStyle(b AccBOM) string {
	p, d := cleanPN(b.Part), strings.ToUpper(b.Description)
	// An exact, sourced family wins over a loose or misleading BOM description.
	if reCrimpRingBackshellPN.MatchString(p) || strings.Contains(d, "CRIMP RING") || strings.Contains(d, "MAGEFORM") || strings.Contains(d, "MAGNEFORM") {
		if strings.Contains(d, "PRE-TERMINATED BRAID") || strings.Contains(d, "PRETERMINATED BRAID") || strings.Contains(d, "FACTORY ATTACHED BRAID") || strings.Contains(d, "FACTORY-ATTACHED BRAID") {
			return "factory-crimp-ring"
		}
		return "crimp-ring"
	}
	if reTinelBackshellPN.MatchString(p) || strings.Contains(d, "TINEL-LOCK") || strings.Contains(d, "TINEL LOCK") || strings.Contains(d, "TINEL RING") {
		return "tinel"
	}
	if reClampBackshellPN.MatchString(p) || strings.Contains(d, "CLAMP BACKSHELL") || strings.Contains(d, "CABLE CLAMP") || strings.Contains(d, "STRAIN RELIEF CLAMP") || strings.Contains(d, "SADDLE CLAMP") {
		return "clamp"
	}
	return ""
}

// SeriesIIIShell returns the shell size of a MIL-DTL-38999 Series III/IV part.
func SeriesIIIShell(part string) (int, string, bool) {
	m := reSeries3.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(part)))
	if len(m) == 0 {
		return 0, "", false
	}
	return shellByCode[m[3]], m[3], true
}

func cleanPN(s string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
}

// qtyNumber returns the numeric quantity, or -1 for A/R / unknown.
func qtyNumber(q string) int {
	q = strings.ToUpper(strings.TrimSpace(q))
	if n, err := strconv.Atoi(q); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(q, 64); err == nil {
		return int(f)
	}
	return -1
}

func CheckAccessories(req AccRequest) ([]AccCheck, error) {
	refs, err := loadAccessoryRefs()
	if err != nil {
		return nil, err
	}
	refOf := func(kind string) []accRef {
		o := []accRef{}
		for _, r := range refs {
			if r.kind == kind {
				o = append(o, r)
			}
		}
		return o
	}
	out := []AccCheck{}
	bom := req.BOM
	for i := range bom {
		bom[i].Part = cleanPN(bom[i].Part)
	}
	isBackshellDesc := regexp.MustCompile(`(?i)BACKSHELL|STRAIN RELIEF|ADAPTER,? (?:SHIELD|CABLE)|SHIELD TERMINATION`)

	// Connector shell sizes present in the drawing.
	shells := map[int][]string{}
	type conn struct {
		AccConnector
		shell int
		res   ContactResolution
	}
	conns := []conn{}
	for _, c := range req.Connectors {
		c.Part = cleanPN(c.Part)
		cc := conn{AccConnector: c, res: ResolveConnectorContacts(c.Part)}
		if s, _, ok := SeriesIIIShell(c.Part); ok {
			cc.shell = s
			shells[s] = append(shells[s], c.Tag)
		}
		conns = append(conns, cc)
	}

	// --- Backshells -------------------------------------------------------
	bsRef := refOf("backshell")
	bsSource := ""
	if len(bsRef) > 0 {
		bsSource = bsRef[0].source
	}
	type bsRow struct {
		b    AccBOM
		size int
	}
	var bsRows, otherBS []bsRow
	for _, b := range bom {
		if m := reBackshell.FindStringSubmatch(b.Part); m != nil {
			n, _ := strconv.Atoi(m[3])
			bsRows = append(bsRows, bsRow{b, n})
		} else if reAnyM85049.MatchString(b.Part) || isBackshellDesc.MatchString(b.Description) {
			otherBS = append(otherBS, bsRow{b, 0})
		}
	}
	for _, r := range bsRows {
		if len(shells) > 0 && len(shells[r.size]) == 0 {
			sizes := []string{}
			for s := range shells {
				sizes = append(sizes, strconv.Itoa(s))
			}
			sort.Strings(sizes)
			out = append(out, AccCheck{Connector: "(BOM item " + r.b.Item + ")", Part: r.b.Part, Check: "Backshell size", Status: "MISMATCH", Expected: "Accessory shell size equal to a connector shell size (" + strings.Join(sizes, ", ") + ")", Found: fmt.Sprintf("%s = size %d", r.b.Part, r.size), Detail: "No Series III connector on this drawing has this shell size.", Source: bsSource})
		}
	}
	for _, c := range conns {
		if c.shell == 0 {
			continue
		}
		found := []string{}
		for _, r := range bsRows {
			if r.size == c.shell {
				found = append(found, r.b.Part)
			}
		}
		chk := AccCheck{Connector: c.Tag, Part: c.Part, Check: "Backshell", Expected: fmt.Sprintf("Shell size %d accessory, e.g. M85049/38 (straight) or /39 (90°) size %d, or the backshell the drawing specifies", c.shell, c.shell), Source: bsSource,
			Tooling: []string{"Connector holding tool for MIL-DTL-38999 Series III shell " + strconv.Itoa(c.shell) + " (P/N per tool manufacturer)", "Backshell assembly wrench / strap wrench; torque per M85049 and backshell manufacturer instructions"}}
		switch {
		case len(found) > 0:
			chk.Status, chk.Found = "OK", strings.Join(found, ", ")
			chk.Detail = "Backshell size matches the connector shell size."
		case len(otherBS) > 0:
			names := []string{}
			for _, r := range otherBS {
				names = append(names, r.b.Part)
			}
			chk.Status, chk.Found = "VERIFY", strings.Join(names, ", ")
			chk.Detail = "A backshell/accessory is listed, but its size cannot be verified from the reference data. Confirm it fits shell size " + strconv.Itoa(c.shell) + "."
		default:
			chk.Status, chk.Found = "MISSING", "none in BOM"
			chk.Detail = "No backshell for this connector is in the BOM. Confirm whether the drawing requires one (it may be specified in notes or another document)."
		}
		chk.Tooling[0] = strings.Replace(chk.Tooling[0], "(P/N", "— UNRESOLVED P/N (select", 1) + ")"
		out = append(out, chk)
	}

	// Contact and sealing-plug quantities are calculated in MakeBuildPlan,
	// using the selected fill policy. Do not duplicate those checks here.

	// --- Backshell termination styles and their tooling --------------------
	const crimpRingSource = "https://www.glenair.com/mil-spec/pdf/b/as85049-20.pdf"
	const clampSource = "https://cdn.glenair.com/space-mechanisms/pdf/d/as85049-118.pdf"
	const tinelSource = "https://www.te.com/en/industries/defense-military/insights/tinel-lock-backshells-for-military-applications.html"
	for _, b := range bom {
		switch backshellTerminationStyle(b) {
		case "crimp-ring":
			out = append(out, AccCheck{Connector: "(BOM item " + b.Item + ")", Part: b.Part, Check: "Crimp-ring termination tooling", Status: "UNRESOLVED", Expected: "Dedicated crimp-ring installation tool/die qualified for the exact backshell and ring P/N", Found: b.Part + " recognized as a crimp-ring backshell/ring", Detail: "This is not a shield-band operation. Resolve the exact manufacturer-approved crimp-ring machine/tool, die or forming process from the approved application specification before release.", Tooling: []string{"UNRESOLVED — special crimp-ring tool/die and process required"}, Source: crimpRingSource})
		case "factory-crimp-ring":
			out = append(out, AccCheck{Connector: "(BOM item " + b.Item + ")", Part: b.Part, Check: "Factory crimp-ring termination", Status: "INFO", Expected: "No field crimp unless the drawing separately requires one", Found: "Description identifies factory-preterminated braid", Detail: "Verify the supplied assembly and do not create a field crimp-tool requirement solely from the factory attachment.", Source: crimpRingSource})
		case "clamp":
			out = append(out, AccCheck{Connector: "(BOM item " + b.Item + ")", Part: b.Part, Check: "Backshell clamp driver", Status: "VERIFY", Expected: "Correctly sized small flat-blade driver matching the clamp screw slot; torque per drawing/manufacturer instructions", Found: b.Part + " recognized as a clamp/strain-relief backshell", Detail: "Do not use an oversized driver or infer one universal blade size. Confirm the clamp screw size, slot width, tightening sequence and torque for the exact backshell.", Tooling: []string{"Small correctly sized flat-blade driver + approved torque method (exact blade/torque unresolved)"}, Source: clampSource})
		case "tinel":
			out = append(out, AccCheck{Connector: "(BOM item " + b.Item + ")", Part: b.Part, Check: "Tinel-Lock ring installation", Status: "INFO", Expected: "TE AD-5000 Tinel-Lock resistance-heating installation tool and applicable electrodes/procedure", Found: b.Part + " recognized as Tinel-Lock/Tinel ring", Detail: "Confirm the exact ring, electrode configuration and current TE application procedure before use.", Tooling: []string{"TE AD-5000 Tinel-Lock installation tool"}, Source: tinelSource})
		}
	}

	// --- Bands and their tools --------------------------------------------
	bandingBackshell := regexp.MustCompile(`(?i)\bBAND(?:ING|ED)?\b.*\bBACKSHELL\b|\bBACKSHELL\b.*\bBAND(?:ING|ED)?\b`)
	bandingBackshellPN := regexp.MustCompile(`^M85049/(?:82|83|84|85|86|87|88|89|90)(?:-|$)`)
	bandingBackshells := []AccBOM{}
	recognizedBands := 0
	for _, b := range bom {
		if backshellTerminationStyle(b) != "crimp-ring" && backshellTerminationStyle(b) != "factory-crimp-ring" && (bandingBackshell.MatchString(b.Description) || bandingBackshellPN.MatchString(b.Part)) {
			bandingBackshells = append(bandingBackshells, b)
		}
		for _, r := range refOf("band") {
			for _, m := range r.match {
				if b.Part == m || strings.HasPrefix(b.Part, m+"-") && !strings.HasPrefix(m, "600-0") {
					recognizedBands++
					tooling := []string{r.requirement}
					detail := "Bands must be installed with the matching tool; tool tension per manufacturer instructions."
					if r.alternates != "" {
						tooling = append(tooling, r.alternates+" — alternate under "+r.specification)
						detail += " The alternate is listed because both tools map to " + r.specification + "; confirm the exact band/application, current qualification, calibration and OEM process before substitution."
					}
					out = append(out, AccCheck{Connector: "(BOM item " + b.Item + ")", Part: b.Part, Check: "Shield termination band", Status: "INFO", Expected: r.class, Found: b.Part + " qty " + b.Qty, Detail: detail, Tooling: tooling, Source: strings.Trim(strings.Join([]string{r.source, r.alternateSource}, " | "), " |")})
				}
			}
		}
	}
	if len(bandingBackshells) > 0 && recognizedBands == 0 {
		for _, b := range bandingBackshells {
			out = append(out, AccCheck{Connector: "(BOM item " + b.Item + ")", Part: b.Part, Check: "Shield termination band/tool", Status: "UNRESOLVED", Expected: "Exact supported band P/N and its matching installation tool", Found: "Banded backshell without a recognized band P/N", Detail: "Do not select a banding tool from the backshell alone. Resolve the exact band from the approved BOM/drawing, then use its manufacturer-specified tool.", Tooling: []string{"UNRESOLVED — exact band P/N required"}})
		}
	}
	return out, nil
}

// isOppositeSex compares M39029 slash numbers for the common Series III pairs.
func isOppositeSex(want, got string) bool {
	sex := map[string]string{"56": "S", "57": "S", "106": "S", "58": "P", "107": "P"}
	a, b := sex[want], sex[got]
	return a != "" && b != "" && a != b
}
