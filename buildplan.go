package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Build plan (v2.8): the answer to "what goes in the kit and which crimp tool
// do I set up". Per connector:
//
//   - which cavities are wired and which are unused (from the drawing),
//   - the contact part number and how many: one per wired cavity, plus one
//     filler contact per unused cavity when the fill policy says so,
//   - sealing plugs for every unused cavity,
//   - for every wired contact, the crimp tool frame and positioner from the
//     DMC reference, checked against each wire's gauge,
//   - the insertion / removal tools,
//
// and, across the whole assembly, the kit totals compared with the BOM.
// Every value carries its source; anything without a sourced rule is
// UNRESOLVED, never guessed.

type PlanWire struct {
	Cavity string `json:"cavity"`
	AWG    string `json:"awg"`
	Wire   string `json:"wire"`
}

type PlanConnector struct {
	Tag         string     `json:"tag"`
	Part        string     `json:"part"`
	All         []string   `json:"all"`         // cavity inventory; empty = unknown
	AllSource   string     `json:"allSource"`   // where the inventory came from
	AllVerified bool       `json:"allVerified"` // inventory taken from the drawing or a verified layout
	Unused      []string   `json:"unused"`      // drawing-declared unused (used when the inventory is unknown)
	Wired       []PlanWire `json:"wired"`
	Contact     string     `json:"contact"` // optional user-entered contact P/N (overrides the catalog candidate)
}

type PlanRequest struct {
	Policy     string          `json:"policy"` // "contact+plug" (default) or "plug-only"
	BuildQty   int             `json:"buildQty"`
	Connectors []PlanConnector `json:"connectors"`
	BOM        []AccBOM        `json:"bom"`
}

type CrimpSetup struct {
	AWG       string   `json:"awg"`
	Cavities  []string `json:"cavities"`
	Frame     string   `json:"frame"`
	Locator   string   `json:"locator"`
	Routes    []string `json:"routes"` // every sourced frame + positioner route
	WireRange string   `json:"wireRange"`
	Status    string   `json:"status"` // OK | OUT OF RANGE | AWG UNKNOWN | UNRESOLVED
	Detail    string   `json:"detail"`
}

type KitLine struct {
	Part    string `json:"part"`
	Purpose string `json:"purpose"`
	Qty     int    `json:"qty"` // per assembly; -1 = cannot be counted yet
	Note    string `json:"note"`
}

type ConnectorPlan struct {
	Tag            string       `json:"tag"`
	Part           string       `json:"part"`
	Contact        string       `json:"contact"`
	ContactSex     string       `json:"contactSex"`
	ContactSize    string       `json:"contactSize"`
	ContactSource  string       `json:"contactSource"`
	ContactStatus  string       `json:"contactStatus"`
	Positions      int          `json:"positions"`
	PositionSource string       `json:"positionSource"`
	Wired          []string     `json:"wired"`
	Unused         []string     `json:"unused"`
	UnusedComplete bool         `json:"unusedComplete"`
	Kit            []KitLine    `json:"kit"`
	Crimp          []CrimpSetup `json:"crimp"`
	Insertion      string       `json:"insertion"`
	Removal        string       `json:"removal"`
	ToolSource     string       `json:"toolSource"`
	Issues         []string     `json:"issues"`
}

type KitTotal struct {
	Part      string   `json:"part"`
	Purpose   string   `json:"purpose"`
	Need      int      `json:"need"` // per assembly; -1 = unknown
	NeedBuild int      `json:"needBuild"`
	Used      []string `json:"usedBy"`
	BOMQty    string   `json:"bomQty"`
	Status    string   `json:"status"` // OK | SHORT | NOT IN BOM | WRONG PART | VERIFY | UNRESOLVED
	Detail    string   `json:"detail"`
}

type BuildPlan struct {
	Policy     string          `json:"policy"`
	BuildQty   int             `json:"buildQty"`
	Connectors []ConnectorPlan `json:"connectors"`
	Totals     []KitTotal      `json:"totals"`
	Notes      []string        `json:"notes"`
}

type dmcRecord struct {
	part, sex, size, wireRange, insertion, removal, source, status string
	aliases                                                        []string
	routes                                                         [][]string
}

func loadDMC() (map[string][]dmcRecord, error) {
	rows, err := readLibrary("DMC_EXPANDED_REFERENCE.csv")
	if err != nil {
		return nil, err
	}
	out := map[string][]dmcRecord{}
	for _, r := range rows[1:] {
		if len(r) < 11 {
			continue
		}
		rec := dmcRecord{part: cleanPN(r[0]), aliases: splitAliases(r[1]), sex: strings.ToUpper(r[2]), size: strings.ToUpper(r[3]), wireRange: r[5], routes: splitToolRoutes(strings.ToUpper(r[6])), insertion: r[7], removal: r[8], source: "DMC contact/tool wall chart (" + r[9] + ")", status: r[10]}
		out[rec.part] = append(out[rec.part], rec)
		for _, a := range rec.aliases {
			out[cleanPN(a)] = append(out[cleanPN(a)], rec)
		}
	}
	return out, nil
}

var reAWG = regexp.MustCompile(`(?i)(?:^|[^0-9])(\d{1,2})(?:\s*(?:AWG|GA|GAUGE))?(?:[^0-9]|$)`)

// awgNumber reads a gauge such as "22", "#24", "22 AWG", "M22759/16-22" is NOT
// read (a specification suffix is not a gauge field). Returns -1 when unknown.
func awgNumber(s string) int {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "#"))
	if s == "" || strings.Contains(s, "/") {
		return -1
	}
	m := reAWG.FindStringSubmatch(s)
	if m == nil {
		return -1
	}
	n, _ := strconv.Atoi(m[1])
	if n < 0 || n > 40 {
		return -1
	}
	return n
}

// wireRangeBounds parses "22-28" (AWG; a larger number is a smaller wire).
func wireRangeBounds(r string) (int, int, bool) {
	p := strings.Split(strings.TrimSpace(r), "-")
	if len(p) == 1 {
		n, err := strconv.Atoi(p[0])
		return n, n, err == nil
	}
	if len(p) != 2 {
		return 0, 0, false
	}
	a, e1 := strconv.Atoi(p[0])
	b, e2 := strconv.Atoi(p[1])
	if e1 != nil || e2 != nil {
		return 0, 0, false
	}
	if a > b {
		a, b = b, a
	}
	return a, b, true
}

// sealPlugFor returns the sourced sealing plug for a contact size.
func sealPlugFor(size string, refs []accRef) (string, string, bool) {
	for _, r := range refs {
		if r.kind != "sealplug" {
			continue
		}
		// The reference row covers size 22 / 22D cavities only.
		if strings.HasPrefix(strings.ToUpper(size), "22") {
			return r.match[0], r.source, true
		}
	}
	return "", "", false
}

func uniqSorted(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sortCavities(out)
	return out
}

func MakeBuildPlan(req PlanRequest) (BuildPlan, error) {
	dmc, err := loadDMC()
	if err != nil {
		return BuildPlan{}, err
	}
	refs, err := loadAccessoryRefs()
	if err != nil {
		return BuildPlan{}, err
	}
	policy := req.Policy
	if policy != "plug-only" {
		policy = "contact+plug"
	}
	if req.BuildQty < 1 {
		req.BuildQty = 1
	}
	plan := BuildPlan{Policy: policy, BuildQty: req.BuildQty, Connectors: []ConnectorPlan{}, Totals: []KitTotal{}, Notes: []string{}}
	type total struct {
		purpose string
		need    int
		unknown bool
		used    []string
	}
	totals := map[string]*total{}
	addTotal := func(part, purpose, tag string, qty int) {
		t := totals[part]
		if t == nil {
			t = &total{purpose: purpose}
			totals[part] = t
		}
		if qty < 0 {
			t.unknown = true
		} else {
			t.need += qty
		}
		t.used = append(t.used, tag)
	}

	for _, c := range req.Connectors {
		c.Part = cleanPN(c.Part)
		cp := ConnectorPlan{Tag: c.Tag, Part: c.Part, Wired: []string{}, Unused: []string{}, Kit: []KitLine{}, Crimp: []CrimpSetup{}, Issues: []string{}}
		// --- contact identity ---------------------------------------------
		res := ResolveConnectorContacts(c.Part)
		cp.Positions = res.PositionCount
		if cp.Positions > 0 {
			cp.PositionSource = res.Source
		}
		if override := cleanPN(c.Contact); override != "" {
			cp.Contact, cp.ContactStatus, cp.ContactSource = override, "ENTERED BY USER — confirm against the drawing / connector spec", "user entry"
		} else if res.Status == "manufacturer-catalog-candidate" && len(res.Contacts) > 0 {
			k := res.Contacts[0]
			cp.Contact, cp.ContactSex, cp.ContactSize = k.Part, k.Sex, k.ContactSize
			cp.ContactStatus, cp.ContactSource = "CATALOG CANDIDATE — confirm the contact actually specified/supplied", k.Source
		} else {
			cp.ContactStatus = "UNRESOLVED"
			cp.ContactSource = res.Warning
			cp.Issues = append(cp.Issues, "Contact P/N not resolved for "+c.Part+". Enter the contact the drawing or connector specification calls for; no crimp tool can be selected without it.")
		}
		var recs []dmcRecord
		if cp.Contact != "" {
			recs = dmc[cp.Contact]
			if len(recs) > 0 {
				if cp.ContactSex == "" {
					cp.ContactSex = recs[0].sex
				}
				if cp.ContactSize == "" {
					cp.ContactSize = recs[0].size
				}
				if res.ContactSex != "" && recs[0].sex != "" && recs[0].sex != res.ContactSex {
					cp.Issues = append(cp.Issues, fmt.Sprintf("Contact %s is a %s but %s takes %s contacts.", cp.Contact, recs[0].sex, c.Part, res.ContactSex))
				}
			} else {
				cp.Issues = append(cp.Issues, "Contact "+cp.Contact+" is not in the local DMC tooling reference; crimp tooling UNRESOLVED.")
			}
		}

		// --- wired / unused -------------------------------------------------
		wiredAWG := map[string][]string{}
		for _, w := range c.Wired {
			if w.Cavity == "" {
				continue
			}
			cp.Wired = append(cp.Wired, w.Cavity)
			wiredAWG[w.Cavity] = append(wiredAWG[w.Cavity], strings.TrimSpace(w.AWG))
		}
		cp.Wired = uniqSorted(cp.Wired)
		all := uniqSorted(c.All)
		inventoryConflict := len(all) != len(c.All)
		if len(all) > 0 {
			in := map[string]bool{}
			for _, a := range all {
				in[a] = true
			}
			isWired := map[string]bool{}
			for _, w := range cp.Wired {
				isWired[w] = true
				if !in[w] {
					inventoryConflict = true
					cp.Issues = append(cp.Issues, "Cavity "+w+" is wired but is not in the cavity inventory ("+c.AllSource+").")
				}
			}
			for _, a := range all {
				if !isWired[a] {
					cp.Unused = append(cp.Unused, a)
				}
			}
			cp.UnusedComplete = c.AllVerified && !inventoryConflict
			if cp.Positions == 0 {
				cp.Positions, cp.PositionSource = len(all), c.AllSource
			} else if cp.Positions != len(all) {
				cp.UnusedComplete = false
				cp.Issues = append(cp.Issues, fmt.Sprintf("Cavity inventory has %d cavities but the catalog lists %d positions.", len(all), cp.Positions))
			}
			if !c.AllVerified {
				cp.Issues = append(cp.Issues, "Cavity inventory is ASSUMED ("+c.AllSource+"); confirm the cavity IDs against the insert arrangement before kitting.")
			}
		} else {
			cp.Unused = uniqSorted(c.Unused)
			if cp.Positions > 0 {
				cp.Issues = append(cp.Issues, "No cavity inventory: unused cavities could only be counted, not listed.")
			} else {
				cp.Issues = append(cp.Issues, "Cavity count unknown: unused cavities cannot be determined. Import the insert layout or enter the capacity.")
			}
		}

		wiredSet := map[string]bool{}
		inventorySet := map[string]bool{}
		for _, w := range cp.Wired { wiredSet[w] = true }
		for _, a := range all { inventorySet[a] = true }
		for _, u := range c.Unused {
			if wiredSet[u] || (len(all) > 0 && !inventorySet[u]) {
				cp.UnusedComplete = false
				cp.Issues = append(cp.Issues, "Conflicting unused-cavity declaration: " + u + ". Resolve before kitting.")
			}
		}
		if inventoryConflict {
			cp.Issues = append(cp.Issues, "Duplicate or out-of-inventory cavity evidence; quantities remain unresolved.")
		}
		unusedCount := len(cp.Unused)
		countKnown := cp.UnusedComplete
		if !countKnown && cp.Positions > 0 {
			unusedCount = cp.Positions - len(cp.Wired)
			// Catalog position count alone does not establish which cavities are unused.
			countKnown = false
			if unusedCount < 0 {
				cp.Issues = append(cp.Issues, fmt.Sprintf("%d wired cavities exceed the %d catalog positions.", len(cp.Wired), cp.Positions))
			}
		}

		// --- kit ----------------------------------------------------------
		if cp.Contact != "" {
			if len(cp.Wired) > 0 {
				cp.Kit = append(cp.Kit, KitLine{Part: cp.Contact, Purpose: "contacts to crimp (wired cavities)", Qty: len(cp.Wired)})
				addTotal(cp.Contact, "contact", c.Tag, len(cp.Wired))
			}
			if policy == "contact+plug" {
				q, note := unusedCount, ""
				if !countKnown {
					q, note = -1, "unused count unknown"
				}
				if q != 0 {
					cp.Kit = append(cp.Kit, KitLine{Part: cp.Contact, Purpose: "filler contacts for unused cavities (not crimped)", Qty: q, Note: note})
					addTotal(cp.Contact, "contact", c.Tag, q)
				}
			}
		} else {
			cp.Kit = append(cp.Kit, KitLine{Part: "UNRESOLVED CONTACT", Purpose: "contacts", Qty: -1, Note: "enter the contact P/N"})
		}
		if plug, src, ok := sealPlugFor(cp.ContactSize, refs); ok {
			q, note := unusedCount, "source: "+src
			if !countKnown {
				q, note = -1, "unused count unknown"
			}
			if q != 0 {
				cp.Kit = append(cp.Kit, KitLine{Part: plug, Purpose: "sealing plugs for unused cavities", Qty: q, Note: note})
				addTotal(plug, "sealing plug", c.Tag, q)
			}
		} else if unusedCount != 0 {
			cp.Kit = append(cp.Kit, KitLine{Part: "UNRESOLVED SEALING PLUG", Purpose: "sealing plugs for unused cavities", Qty: -1, Note: "no sourced sealing plug for contact size " + firstNonEmpty(cp.ContactSize, "unknown")})
		}

		// --- crimp tooling ------------------------------------------------
		if len(recs) > 0 {
			r := recs[0]
			cp.Insertion, cp.Removal, cp.ToolSource = firstNonEmpty(r.insertion, "UNRESOLVED"), firstNonEmpty(r.removal, "UNRESOLVED"), r.source+" · "+r.status
			routes := []string{}
			seenRoute := map[string]bool{}
			for _, rec := range recs {
				for _, rt := range rec.routes {
					k := strings.Join(rt, " + ")
					if !seenRoute[k] {
						seenRoute[k] = true
						routes = append(routes, k)
					}
				}
			}
			frame, loc := "UNRESOLVED", "UNRESOLVED"
			if len(recs[0].routes) > 0 && len(recs[0].routes[0]) >= 2 {
				frame, loc = recs[0].routes[0][0], strings.Join(recs[0].routes[0][1:], " + ")
			}
			lo, hi, okRange := wireRangeBounds(r.wireRange)
			byAWG := map[string][]string{}
			for _, cav := range cp.Wired {
				gs := uniqueStrings(wiredAWG[cav])
				for _, g := range gs {
					byAWG[g] = append(byAWG[g], cav)
				}
			}
			keys := []string{}
			for k := range byAWG {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, g := range keys {
				s := CrimpSetup{AWG: g, Cavities: uniqSorted(byAWG[g]), Frame: frame, Locator: loc, Routes: routes, WireRange: r.wireRange}
				n := awgNumber(g)
				switch {
				case g == "" || n < 0:
					s.AWG = "not documented"
					s.Status = "AWG UNKNOWN"
					s.Detail = "Wire gauge is not documented for these cavities; confirm it is within " + r.wireRange + " AWG before crimping."
				case !okRange:
					s.Status = "UNRESOLVED"
					s.Detail = "The reference wire range '" + r.wireRange + "' could not be read."
				case n < lo || n > hi:
					s.Status = "OUT OF RANGE"
					s.Detail = fmt.Sprintf("%d AWG is outside the %s AWG range of %s. Do not crimp; obtain an engineering resolution.", n, r.wireRange, cp.Contact)
				default:
					s.Status = "OK"
					s.Detail = fmt.Sprintf("%d AWG is within %s AWG for %s. Set the crimp frame per the positioner data plate for this contact and gauge.", n, r.wireRange, cp.Contact)
				}
				cp.Crimp = append(cp.Crimp, s)
			}
		} else if cp.Contact != "" {
			cp.Insertion, cp.Removal = "UNRESOLVED", "UNRESOLVED"
		}
		for _, cr := range cp.Crimp {
			if cr.Status == "OUT OF RANGE" {
				cp.Issues = append(cp.Issues, "Cavities "+strings.Join(cr.Cavities, ", ")+": "+cr.Detail)
			}
		}
		plan.Connectors = append(plan.Connectors, cp)
	}

	// --- kit totals vs BOM -------------------------------------------------
	parts := []string{}
	for p := range totals {
		parts = append(parts, p)
	}
	sort.Strings(parts)
	bom := req.BOM
	for i := range bom {
		bom[i].Part = cleanPN(bom[i].Part)
	}
	for _, p := range parts {
		t := totals[p]
		kt := KitTotal{Part: p, Purpose: t.purpose, Need: t.need, NeedBuild: t.need * req.BuildQty, Used: uniqueStrings(t.used)}
		if t.unknown {
			kt.Need, kt.NeedBuild = -1, -1
		}
		qty, rows, known := 0, []string{}, true
		aliases := map[string]bool{p: true}
		for _, rec := range dmc[p] {
			aliases[rec.part] = true
			for _, a := range rec.aliases {
				aliases[cleanPN(a)] = true
			}
		}
		var wrong []string
		for _, b := range bom {
			switch {
			case aliases[b.Part] || (t.purpose == "sealing plug" && strings.HasPrefix(b.Part, p+"-")):
				rows = append(rows, b.Part+" ×"+b.Qty)
				if q := qtyNumber(b.Qty); q >= 0 {
					qty += q
				} else {
					known = false
				}
			case t.purpose == "contact" && reContactPN.MatchString(b.Part):
				if w, g := reContactPN.FindStringSubmatch(p), reContactPN.FindStringSubmatch(b.Part); w != nil && g != nil && isOppositeSex(w[1], g[1]) {
					wrong = append(wrong, b.Part)
				}
			case t.purpose == "sealing plug" && strings.HasPrefix(b.Part, "MS27488"):
				wrong = append(wrong, b.Part)
			}
		}
		kt.BOMQty = strings.Join(rows, "; ")
		switch {
		case len(rows) == 0 && len(wrong) > 0:
			kt.Status, kt.BOMQty = "WRONG PART", strings.Join(wrong, ", ")
			kt.Detail = "The BOM lists " + strings.Join(wrong, ", ") + " instead of " + p + " (wrong contact sex or plug size)."
		case len(rows) == 0:
			kt.Status, kt.BOMQty = "NOT IN BOM", "—"
			kt.Detail = p + " is needed but is not in the BOM. It may be supplied with the connector or called out elsewhere; confirm before kitting."
		case !known:
			kt.Status = "VERIFY"
			kt.Detail = "BOM quantity is not numeric (A/R); kit the quantity shown in Need."
		case t.unknown:
			kt.Status = "VERIFY"
			kt.Detail = fmt.Sprintf("BOM lists %d; the need cannot be counted until every connector's unused cavities are known.", qty)
		case qty < t.need:
			kt.Status = "SHORT"
			kt.Detail = fmt.Sprintf("BOM lists %d per assembly; the plan needs %d.", qty, t.need)
		default:
			kt.Status = "OK"
			kt.Detail = fmt.Sprintf("BOM lists %d per assembly; the plan needs %d.", qty, t.need)
		}
		plan.Totals = append(plan.Totals, kt)
	}
	if policy == "contact+plug" {
		plan.Notes = append(plan.Notes, "Fill policy: every unused cavity gets an uncrimped contact AND a sealing plug. Change the policy if your drawing or process specification says plugs only.")
	} else {
		plan.Notes = append(plan.Notes, "Fill policy: unused cavities get a sealing plug only (no filler contact). Confirm this against the drawing notes and process specification.")
	}
	plan.Notes = append(plan.Notes, "Crimp frame and positioner come from the DMC wall chart reference (chart-extracted, unverified). Confirm against the current DMC chart and the positioner data plate before production.")
	return plan, nil
}
