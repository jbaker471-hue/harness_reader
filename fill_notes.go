package main

import (
	"regexp"
	"sort"
	"strings"
)

// Fill instructions from the drawing notes.
//
// Which cavities are unused comes from the drawing (marked unused, or by
// process of elimination). WHAT goes into them also comes from the drawing:
// notes such as "INSTALL CONTACTS AND SEALING PLUGS IN ALL UNUSED CAVITIES"
// (filler contact + plug), "SEAL UNUSED CAVITIES PER FIND NO. 3" (plug only)
// or "POPULATE UNUSED CAVITIES WITH PINS" (contact only). This reads those
// instructions so the technician does not have to choose a fill policy.

type FillInstruction struct {
	Policy     string   `json:"policy"` // contact+plug | plug-only | contact-only
	Note       string   `json:"note"`   // note number, "" for unnumbered text
	Page       int      `json:"page"`
	Text       string   `json:"text"`
	Connectors []string `json:"connectors"` // empty = applies to every connector
	FindNos    []string `json:"findNos"`
	Parts      []string `json:"parts"` // part numbers named in the note (e.g. MS27488-22-2)
	// PlugPart is the sealing plug the note names, directly or through a find
	// number in the parts list; PlugSource says which.
	PlugPart   string `json:"plugPart"`
	PlugSource string `json:"plugSource"`
}

// resolveFillParts turns "PER FIND NO. 3" into the parts-list part on item 3
// when that part is a sealing plug.
func resolveFillParts(fs []FillInstruction, bom []BOMRow) []FillInstruction {
	isPlug := func(part, desc string) bool {
		p, d := strings.ToUpper(part), strings.ToUpper(desc)
		return strings.HasPrefix(p, "MS27488") || strings.Contains(d, "SEALING PLUG") || strings.Contains(d, "PLUG, SEAL") || strings.Contains(d, "SEAL PLUG")
	}
	for i := range fs {
		for _, p := range fs[i].Parts {
			if strings.HasPrefix(p, "MS27488") {
				fs[i].PlugPart, fs[i].PlugSource = p, "named in the note"
			}
		}
		if fs[i].PlugPart != "" {
			continue
		}
		for _, f := range fs[i].FindNos {
			for _, b := range bom {
				if strings.TrimLeft(strings.TrimSpace(b.Item), "0") == f && isPlug(b.Part, b.Description) {
					fs[i].PlugPart, fs[i].PlugSource = strings.ToUpper(b.Part), "find no. "+f+" in the parts list (sheet "+itoa(b.Page)+")"
				}
			}
		}
	}
	if fs == nil {
		fs = []FillInstruction{}
	}
	return fs
}

var (
	reFillUnused = regexp.MustCompile(`\b(?:UNUSED|SPARE|EMPTY|UNPOPULATED|VACANT|UNWIRED|UNOCCUPIED|NON-?USED|ALL\s+OTHER|REMAINING|OPEN|NOT\s+USED)\b[^.;]{0,40}?\b(?:CONTACT\s+CAVIT(?:Y|IES)|CAVIT(?:Y|IES)|POSITIONS?|HOLES?|LOCATIONS?|CONTACTS?|PINS?|SOCKETS?)\b|\b(?:CAVIT(?:Y|IES)|POSITIONS?|CONTACTS?|HOLES?)\b[^.;]{0,20}?\bNOT\s+(?:USED|WIRED|OCCUPIED)\b`)
	reFillPlug   = regexp.MustCompile(`\b(?:SEALING\s+PLUGS?|SEAL\s+PLUGS?|FILLER\s+PLUGS?|CAVITY\s+PLUGS?|PLUGS?|PLUGGED|SEALED|SEAL|MS27488[-0-9A-Z]*)\b`)
	// a contact named as the thing to install, not the cavity type
	reFillContactAfterVerb = regexp.MustCompile(`\b(?:WITH|USING|INSTALL|INSERT|POPULATE|POPULATED|FILL|FILLED|LOAD|LOADED)\b[^.;]*?\b(?:CONTACTS?|PINS?|SOCKETS?|M39029/[0-9-]+)\b`)
	reFillContactSubject   = regexp.MustCompile(`\b(?:UNUSED|SPARE|FILLER|UNWIRED)\s+(?:CONTACTS?|PINS?|SOCKETS?)\s+(?:SHALL\s+BE\s+|TO\s+BE\s+|ARE\s+|MUST\s+BE\s+)?(?:INSTALLED|INSERTED|POPULATED|LOADED|IN\s?PLACE)`)
	reFillNoContact        = regexp.MustCompile(`\b(?:DO\s+NOT|DON'T|NO|WITHOUT|NOT)\s+(?:INSTALL\s+|INSERT\s+|USE\s+)?(?:ANY\s+)?(?:FILLER\s+)?(?:CONTACTS?|PINS?|SOCKETS?)\b`)
	reFillNoPlug           = regexp.MustCompile(`\b(?:DO\s+NOT|DON'T|NO|WITHOUT|NOT)\s+(?:INSTALL\s+|INSERT\s+|USE\s+)?(?:ANY\s+)?(?:SEALING\s+)?PLUGS?\b`)
	reFillFind             = regexp.MustCompile(`\b(?:FIND|ITEM|F/N)\s*(?:NO\.?|NUMBER|#)?\s*([0-9]{1,3}[A-Z]?)\b`)
	reFillPart             = regexp.MustCompile(`\b(?:MS27488-[0-9A-Z-]+|M39029/[0-9]+-[0-9]+)\b`)
	reConnPlugNoun         = regexp.MustCompile(`\b(?:CONNECTOR,?\s+)?(?:PLUG|RECEPTACLE)\s+((?:J|P|X|TB|CN|PL|SK)-?[0-9]{1,4}[A-Z]?)\b`)
	reFillTag              = regexp.MustCompile(`\b((?:J|P|X|TB|CN|PL|SK)-?[0-9]{1,4}[A-Z]?)\b`)
)

// parseFillInstruction reads one note's text. ok is false when the text is not
// an instruction about unused cavities.
func parseFillInstruction(text string) (FillInstruction, bool) {
	u := strings.Join(strings.Fields(strings.ToUpper(text)), " ")
	// "PLUG" is also a connector noun ("CONNECTOR PLUG P1", "PLUG P1"); that is not a sealing plug.
	u = reConnPlugNoun.ReplaceAllString(u, "CONNECTOR $1")
	if !reFillUnused.MatchString(u) {
		return FillInstruction{}, false
	}
	contact := reFillContactAfterVerb.MatchString(u) || reFillContactSubject.MatchString(u)
	plug := reFillPlug.MatchString(u)
	// "UNUSED CONTACT CAVITIES SHALL BE SEALED": the contact words only name the cavity
	if contact {
		trimmed := reFillUnused.ReplaceAllString(u, " ")
		contact = reFillContactAfterVerb.MatchString(trimmed) || reFillContactSubject.MatchString(u)
	}
	if reFillNoContact.MatchString(u) {
		contact = false
	}
	if reFillNoPlug.MatchString(u) {
		plug = false
	}
	f := FillInstruction{Text: strings.TrimSpace(text), Connectors: []string{}, FindNos: []string{}, Parts: []string{}}
	switch {
	case contact && plug:
		f.Policy = "contact+plug"
	case plug:
		f.Policy = "plug-only"
	case contact:
		f.Policy = "contact-only"
	default:
		return FillInstruction{}, false
	}
	seen := map[string]bool{}
	for _, m := range reFillTag.FindAllStringSubmatch(u, -1) {
		t := strings.ReplaceAll(m[1], "-", "")
		if !seen[t] {
			seen[t] = true
			f.Connectors = append(f.Connectors, t)
		}
	}
	sort.Strings(f.Connectors)
	for _, m := range reFillFind.FindAllStringSubmatch(u, -1) {
		f.FindNos = append(f.FindNos, strings.TrimLeft(m[1], "0"))
	}
	f.Parts = append(f.Parts, reFillPart.FindAllString(u, -1)...)
	return f, true
}

// fillInstructions reads the numbered notes, then any other line on the
// drawing that is a fill instruction on its own.
func fillInstructions(notes []DrawingNote, pages []Page) []FillInstruction {
	out := []FillInstruction{}
	seenText := map[string]bool{}
	for _, n := range notes {
		if f, ok := parseFillInstruction(n.Text); ok {
			f.Note, f.Page = n.ID, n.Page
			out = append(out, f)
			seenText[strings.Join(strings.Fields(strings.ToUpper(n.Text)), " ")] = true
		}
	}
	for _, p := range pages {
		for _, l := range pageLines(p) {
			u := strings.Join(strings.Fields(strings.ToUpper(l.Text)), " ")
			covered := false
			for t := range seenText {
				if strings.Contains(t, u) || strings.Contains(u, t) {
					covered = true
					break
				}
			}
			if covered {
				continue
			}
			if f, ok := parseFillInstruction(l.Text); ok {
				f.Page = p.Number
				out = append(out, f)
				seenText[u] = true
			}
		}
	}
	return out
}
