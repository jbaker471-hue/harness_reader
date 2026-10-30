package main

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Capture checks read the same fact from more than one independent place on the drawing and say whether the
// readings agree. They never change a value; a disagreement is a reason to look at the sheet, and agreement
// raises confidence without proving the drawing is right.

const (
	CheckAgree     = "AGREE"
	CheckDisagree  = "DISAGREE"
	CheckSingle    = "SINGLE SOURCE"
	CheckNoReading = "NOT AVAILABLE"
)

type CaptureMethod struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Detail string `json:"detail,omitempty"`
}

type CaptureCheck struct {
	ID        string          `json:"id"`
	Title     string          `json:"title"`
	Connector string          `json:"connector,omitempty"`
	Status    string          `json:"status"`
	Message   string          `json:"message"`
	Methods   []CaptureMethod `json:"methods"`
}

var (
	reCondCount   = regexp.MustCompile(`(?i)\b(\d{1,3})\s*[- ]?\s*COND(?:UCTOR)?S?\b`)
	reQtyNumber   = regexp.MustCompile(`^\s*(\d[\d,]*(?:\.\d+)?)\s*$`)
	reSealingPlug = regexp.MustCompile(`(?i)\b(?:WIRE\s+)?SEAL(?:ING)?\s+PLUGS?\b`)
	reConnectorDe = regexp.MustCompile(`(?i)\b(?:CONNECTOR|RECEPTACLE)\b`)
	// not a connector body even when the line says "plug" or "connector"
	reNotConnector = regexp.MustCompile(`(?i)\b(?:SEAL(?:ING)?|CABLE|BACKSHELL|SLEEVE|BOOT|CAP|DUST)\b`)
)

func plainQty(s string) (float64, bool) {
	m := reQtyNumber.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
	return v, err == nil
}

func cableConductors(b BOMRow) int {
	if !strings.Contains(strings.ToUpper(b.Description), "CABLE") {
		return 0
	}
	if m := reCondCount.FindStringSubmatch(b.Description); m != nil {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}

func fmtNum(v float64) string {
	if v == math.Trunc(v) {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

// settle compares the readings that exist. Readings that cannot be made are left out, not counted as disagreement.
func settle(c *CaptureCheck, values []float64, what string) {
	switch {
	case len(values) == 0:
		c.Status, c.Message = CheckNoReading, "No independent reading of "+what+" could be made from this drawing."
	case len(values) == 1:
		c.Status, c.Message = CheckSingle, "Only one reading of "+what+" is available, so nothing confirms it."
	default:
		agree := true
		for _, v := range values[1:] {
			if math.Abs(v-values[0]) > 0.001 {
				agree = false
			}
		}
		if agree {
			c.Status, c.Message = CheckAgree, fmt.Sprintf("%d independent readings of %s agree.", len(values), what)
		} else {
			c.Status, c.Message = CheckDisagree, "The readings of "+what+" disagree. Check the sheets before relying on any of them."
		}
	}
}

func captureChecks(a *Analysis, callouts []GraphicCallout, lengths []HarnessLength) []CaptureCheck {
	out := []CaptureCheck{}

	// 1. How many connectors the drawing has.
	{
		c := CaptureCheck{ID: "CONNECTOR_COUNT", Title: "Number of connectors"}
		vals := []float64{}
		tags := 0
		for _, k := range a.Connectors {
			if k.Tag != "" && k.Tag != "UNASSIGNED" {
				tags++
			}
		}
		if tags > 0 {
			c.Methods = append(c.Methods, CaptureMethod{"Connector designators found", strconv.Itoa(tags), ""})
			vals = append(vals, float64(tags))
		}
		bomQty, bomLines := 0.0, 0
		for _, b := range a.BOM {
			if reConnectorDe.MatchString(b.Description) && !reNotConnector.MatchString(b.Description) && strings.TrimSpace(b.Part) != "" {
				if q, ok := plainQty(b.Qty); ok {
					bomQty += q
					bomLines++
				}
			}
		}
		if bomLines > 0 {
			c.Methods = append(c.Methods, CaptureMethod{"Connector lines in the parts list (quantity)", fmtNum(bomQty), ""})
			vals = append(vals, bomQty)
		}
		ends := map[string]bool{}
		for _, w := range a.Wires {
			for _, t := range []string{w.Connector, w.End} {
				// a far end such as a cable's own plug (ETH1, USB_DN1) is not a harness connector
				if t != "" && t != "UNRESOLVED ENDPOINT" && !strings.HasPrefix(t, "@") && reRefDesignator.MatchString(strings.ToUpper(t)) {
					ends[t] = true
				}
			}
		}
		if len(ends) > 0 {
			c.Methods = append(c.Methods, CaptureMethod{"Distinct connectors named by the wiring", strconv.Itoa(len(ends)), ""})
			vals = append(vals, float64(len(ends)))
		}
		settle(&c, vals, "the connector count")
		out = append(out, c)
	}

	// 2. How many conductors there are: wire rows against cable callouts and against the cable quantity.
	// The two cable-based readings are kept (-1 when unavailable) so the cavity check can use the same pair math.
	calloutConductors, lengthConductors := -1.0, -1.0
	pairCables := false
	{
		c := CaptureCheck{ID: "CONDUCTOR_COUNT", Title: "Number of conductors"}
		vals := []float64{}
		wires := 0
		wiringPages := map[int]bool{}
		for _, w := range a.Wires {
			wires++
			wiringPages[w.Page] = true
		}
		if wires > 0 {
			c.Methods = append(c.Methods, CaptureMethod{"Wire rows read from the wiring sheets", strconv.Itoa(wires), ""})
			vals = append(vals, float64(wires))
		}
		cableItems := map[string]BOMRow{}
		for _, b := range a.BOM {
			if cableConductors(b) > 0 && strings.TrimSpace(b.Item) != "" {
				cableItems[canonicalFind(b.Item)] = b
			}
		}
		// cables called out on the wiring sheets, each with its "19x"-style quantity (none means one)
		cables, conductors, called := 0.0, 0.0, 0
		ambiguousCallout := false
		for _, k := range callouts {
			b, ok := cableItems[canonicalFind(k.Reference)]
			if !ok || !wiringPages[k.Page] {
				continue
			}
			n := 1.0
			// the extractor keeps the digits of a "19x" quantity; two different ones beside one callout are not a reading
			if q := strings.TrimSpace(k.Quantity); q == "AMBIGUOUS" {
				ambiguousCallout = true
				continue
			} else if q != "" {
				if v, err := strconv.ParseFloat(strings.TrimRight(strings.ToLower(q), "x×"), 64); err == nil && v > 0 {
					n = v
				}
			}
			cables += n
			conductors += n * float64(cableConductors(b))
			called++
		}
		if called > 0 && !ambiguousCallout {
			c.Methods = append(c.Methods, CaptureMethod{"Cable callouts on the wiring sheets × conductors per cable", fmtNum(conductors), fmt.Sprintf("%s cables called out", fmtNum(cables))})
			vals = append(vals, conductors)
			calloutConductors = conductors
		}
		// cable quantity from the parts list divided by the length of one cable (unit inferred, so it only counts when it divides evenly)
		if len(cableItems) > 0 {
			for _, L := range lengthCandidatesInches(lengths) {
				total, cablesByLen, ok := 0.0, 0.0, true
				for _, b := range cableItems {
					q, qok := plainQty(b.Qty)
					if !qok || q <= 0 {
						ok = false
						break
					}
					n := q * 12 / L
					if math.Abs(n-math.Round(n)) > 0.02 || n < 1 {
						ok = false
						break
					}
					cablesByLen += math.Round(n)
					total += math.Round(n) * float64(cableConductors(b))
				}
				if ok {
					c.Methods = append(c.Methods, CaptureMethod{"Cable quantity in the parts list ÷ one cable length × conductors per cable", fmtNum(total), fmt.Sprintf("%s cables of %s in (%s ft), quantities read as feet", fmtNum(cablesByLen), fmtNum(L), fmtNum(L/12))})
					vals = append(vals, total)
					lengthConductors = total
					break
				}
			}
		}
		settle(&c, vals, "the conductor count")
		out = append(out, c)

		// Twisted pairs: when every cable is a two-conductor cable, the wiring must come in pairs on every sheet.
		pairCables = len(cableItems) > 0
		for _, b := range cableItems {
			if cableConductors(b) != 2 {
				pairCables = false
			}
		}
		if pairCables && wires > 0 {
			pc := CaptureCheck{ID: "PAIR_STRUCTURE", Title: "Twisted pairs"}
			perPage := map[int]int{}
			for _, w := range a.Wires {
				perPage[w.Page]++
			}
			pages := make([]int, 0, len(perPage))
			for p := range perPage {
				pages = append(pages, p)
			}
			sort.Ints(pages)
			odd := []string{}
			for _, p := range pages {
				pc.Methods = append(pc.Methods, CaptureMethod{fmt.Sprintf("Wire rows on sheet %d", p), strconv.Itoa(perPage[p]), fmtNum(float64(perPage[p])/2) + " pairs"})
				if perPage[p]%2 != 0 {
					odd = append(odd, strconv.Itoa(p))
				}
			}
			if len(odd) == 0 {
				pc.Status, pc.Message = CheckAgree, "Every cable is a two-conductor twisted pair and every sheet has an even number of wire rows, so the wiring divides into pairs."
			} else {
				pc.Status, pc.Message = CheckDisagree, "Every cable is a two-conductor pair but sheet "+strings.Join(odd, ", ")+" has an odd number of wire rows, so a conductor was missed or added."
			}
			out = append(out, pc)
		}
	}

	// 3. How many cavities of each connector are empty: wiring against the sealing-plug quantity and any stated count.
	plugs, plugLines := 0.0, 0
	for _, b := range a.BOM {
		if reSealingPlug.MatchString(b.Description) {
			if q, ok := plainQty(b.Qty); ok {
				plugs += q
				plugLines++
			}
		}
	}
	sized := []Connector{}
	for _, k := range a.Connectors {
		if k.Tag != "" && k.Tag != "UNASSIGNED" && k.CavityCount > 0 {
			sized = append(sized, k)
		}
	}
	sort.SliceStable(sized, func(i, j int) bool { return sized[i].Tag < sized[j].Tag })
	stated := map[string]StatedUsed{}
	for _, s := range a.UsedCounts {
		stated[s.Connector] = s
	}
	for _, k := range sized {
		pins := map[string]bool{}
		off := map[string]bool{}
		note := func(p string) {
			if p == "" {
				return
			}
			// a numbered pin beyond the insert's last position is not a cavity of this connector
			if n, err := strconv.Atoi(p); err == nil && n > k.CavityCount {
				off[p] = true
				return
			}
			pins[p] = true
		}
		for _, w := range a.Wires {
			if w.Connector == k.Tag {
				note(w.Cavity)
			}
			if w.End == k.Tag {
				note(w.EndCavity)
			}
		}
		c := CaptureCheck{ID: "UNUSED_CAVITIES", Title: "Empty cavities", Connector: k.Tag}
		vals := []float64{}
		if len(pins) > 0 {
			c.Methods = append(c.Methods, CaptureMethod{"Cavities in the insert − pins wired", strconv.Itoa(k.CavityCount - len(pins)), fmt.Sprintf("%d positions, %d wired%s", k.CavityCount, len(pins), offNote(len(off)))})
			vals = append(vals, float64(k.CavityCount-len(pins)))
		}
		if s, ok := stated[k.Tag]; ok && !s.Conflict {
			c.Methods = append(c.Methods, CaptureMethod{"Cavities in the insert − used contacts the drawing states", strconv.Itoa(k.CavityCount - s.Count), fmt.Sprintf("%d used, sheet %d", s.Count, s.Page)})
			vals = append(vals, float64(k.CavityCount-s.Count))
		}
		// Pair math: on a point-to-point harness every conductor ends on one pin of each connector, so the
		// conductors counted from the cable callouts or from the cable quantity are the pins used here too.
		if pointToPoint(a, sized) {
			if calloutConductors >= 0 {
				c.Methods = append(c.Methods, CaptureMethod{"Cavities in the insert − conductors from the cable callouts", fmtNum(float64(k.CavityCount) - calloutConductors), fmt.Sprintf("%d positions, %s conductors", k.CavityCount, fmtNum(calloutConductors))})
				vals = append(vals, float64(k.CavityCount)-calloutConductors)
			}
			if lengthConductors >= 0 {
				c.Methods = append(c.Methods, CaptureMethod{"Cavities in the insert − conductors from the cable quantity", fmtNum(float64(k.CavityCount) - lengthConductors), fmt.Sprintf("%d positions, %s conductors", k.CavityCount, fmtNum(lengthConductors))})
				vals = append(vals, float64(k.CavityCount)-lengthConductors)
			}
		}
		// Plugs are shared between the connectors that take them; only an even split is a reading.
		if plugLines > 0 && len(sized) > 0 {
			share := plugs / float64(len(sized))
			if math.Abs(share-math.Round(share)) < 0.001 {
				c.Methods = append(c.Methods, CaptureMethod{"Sealing plugs in the parts list ÷ connectors", fmtNum(share), fmt.Sprintf("%s plugs for %d connectors", fmtNum(plugs), len(sized))})
				vals = append(vals, share)
			}
		}
		settle(&c, vals, "the empty cavities of "+k.Tag)
		out = append(out, c)
	}
	return out
}

// lengthCandidatesInches turns the lengths read from the drawing into inches. A length with no unit is tried as inches.
func lengthCandidatesInches(ls []HarnessLength) []float64 {
	out := []float64{}
	seen := map[float64]bool{}
	for _, h := range ls {
		fields := strings.Fields(h.Text)
		if len(fields) == 0 {
			continue
		}
		v, err := strconv.ParseFloat(fields[0], 64)
		if err != nil || v <= 0 {
			continue
		}
		unit := ""
		if len(fields) > 1 {
			unit = strings.ToUpper(fields[1])
		}
		switch unit {
		case "FT", "FEET", "'":
			v *= 12
		case "", "IN", "INCH", "INCHES", `"`:
		default:
			continue
		}
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func offNote(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprintf(", %d wired pin(s) beyond the insert not counted", n)
}

// pointToPoint is true when every wire runs between the same two connectors, which is the only case where the
// conductor total can stand in for the pins used on each one.
func pointToPoint(a *Analysis, sized []Connector) bool {
	if len(sized) != 2 || len(a.Wires) == 0 {
		return false
	}
	tags := map[string]bool{sized[0].Tag: true, sized[1].Tag: true}
	for _, w := range a.Wires {
		if !tags[w.Connector] || !tags[w.End] || w.Connector == w.End {
			return false
		}
	}
	return true
}
