package main

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// Cross-source, page-grounded evidence. Nothing in this file changes a part
// number, contact sex or cavity assignment. It records WHERE each clue is on
// the rendered page and which clues agree, so a reviewer can confirm them.

type LabelHit struct {
	Page int    `json:"page"`
	Box  Box    `json:"box"`
	Text string `json:"text"`
}

type GraphicHit struct {
	Page     int     `json:"page"`
	Box      Box     `json:"box"`
	Kind     string  `json:"kind"`
	Detail   string  `json:"detail"`
	Distance float64 `json:"distance"`
}

type EndpointEvidence struct {
	Tag         string       `json:"tag"`
	Part        string       `json:"part"`
	Labels      []LabelHit   `json:"labels"`
	BOMRows     []string     `json:"bomRows"`
	WiringRows  int          `json:"wiringRows"`
	WiringPages []int        `json:"wiringPages"`
	Graphics    []GraphicHit `json:"graphics"`
	Sources     []string     `json:"sources"`
	Notes       []string     `json:"notes"`
	Status      string       `json:"status"`
}

type InsertDiagram struct {
	Page     int    `json:"page"`
	Box      Box    `json:"box"`
	Outline  Circle `json:"outline"`
	Cavities int    `json:"cavityCircles"`
	Labels   int    `json:"labelsInside"`
	NearTag  string `json:"nearTag"`
	Note     string `json:"note"`
	// Cavity positions exactly as drawn (display space, relative to the
	// outline centre) with the label printed at each one. ViewSide is taken
	// from the drawing's own caption, never assumed.
	Positions []CavityPos `json:"positions"`
	Labeled   int         `json:"labeled"`
	ViewSide  string      `json:"viewSide"`
}

type CavityPos struct {
	Mark  string  `json:"mark"`
	Label string  `json:"label"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	R     float64 `json:"r"`
}

var (
	reCavityLabel = regexp.MustCompile(`^[A-Za-z]{0,2}[0-9]{0,3}[A-Za-z]?$`)
	reFrontView   = regexp.MustCompile(`(?i)\b(?:FRONT|MATING|ENGAGING|FACE VIEW|MATING FACE|ENGAGEMENT)\b`)
	reRearView    = regexp.MustCompile(`(?i)\b(?:REAR|WIRE (?:SIDE|ENTRY)|TERMINATION SIDE|BACK VIEW|INSERTION SIDE)\b`)
)

type PageDiagnostic struct {
	Page               int     `json:"page"`
	Rotation           int     `json:"rotation"`
	DisplayOrientation string  `json:"displayOrientation"`
	AnalysisFrame      int     `json:"analysisFrame"`
	TextFragments      int     `json:"textFragments"`
	Characters         int     `json:"characters"`
	DecodedPercent     int     `json:"decodedPercent"`
	Pictures           int     `json:"pictures"`
	PicturesFlipped    int     `json:"picturesStoredFlipped"`
	VectorObjects      int     `json:"vectorObjects"`
	Circles            int     `json:"circles"`
	InsertDiagrams     int     `json:"insertDiagramCandidates"`
	OutlinedTextLikely bool    `json:"outlinedTextLikely"`
	WiringRows         int     `json:"wiringRows"`
	BOMRows            int     `json:"bomRows"`
	Status             string  `json:"status"`
	Notes              []string `json:"notes"`
}

var reLabelToken = regexp.MustCompile(`^(?:J|P|X|TB|CN|CON|PL|SK)-?[0-9]{1,4}[A-Z]?$`)

func labelHits(layouts []PageLayout) map[string][]LabelHit {
	out := map[string][]LabelHit{}
	for _, pl := range layouts {
		for _, w := range pl.Words {
			toks := strings.Fields(strings.ToUpper(w.Text))
			if len(toks) == 0 || len(toks) > 5 {
				continue // long notes mention designators without locating them
			}
			for _, t := range toks {
				t = strings.Trim(t, ":;,.()[]{}<>\"'")
				if reLabelToken.MatchString(t) {
					t = strings.ReplaceAll(t, "-", "")
					if len(out[t]) < 40 {
						out[t] = append(out[t], LabelHit{Page: pl.Page, Box: w.Box, Text: w.Text})
					}
				}
			}
		}
	}
	return out
}

func boxDistance(a, b Box) float64 {
	dx := math.Max(0, math.Max(a.X0-b.X1, b.X0-a.X1))
	dy := math.Max(0, math.Max(a.Y0-b.Y1, b.Y0-a.Y1))
	return math.Hypot(dx, dy)
}

func insertDiagrams(layouts []PageLayout, labels map[string][]LabelHit) []InsertDiagram {
	out := []InsertDiagram{}
	for _, pl := range layouts {
		cs := mergeConcentric(pl.Circles)
		sort.Slice(cs, func(i, j int) bool { return cs[i].R > cs[j].R })
		accepted := []Circle{}
		for _, big := range cs {
			if big.R < 12 {
				break
			}
			skip := false
			for _, a := range accepted {
				if math.Hypot(a.CX-big.CX, a.CY-big.CY) < a.R {
					skip = true
				}
			}
			if skip {
				continue
			}
			inner := 0
			for _, c := range cs {
				if c.R >= big.R*0.35 || c.R < 0.5 {
					continue
				}
				if math.Hypot(c.CX-big.CX, c.CY-big.CY) <= big.R-c.R*0.5 {
					inner++
				}
			}
			if inner < 3 {
				continue
			}
			accepted = append(accepted, big)
			box := Box{big.CX - big.R, big.CY - big.R, big.CX + big.R, big.CY + big.R}
			lab := 0
			for _, w := range pl.Words {
				if len([]rune(w.Text)) <= 4 && math.Hypot(w.Box.CX()-big.CX, w.Box.CY()-big.CY) < big.R {
					lab++
				}
			}
			d := InsertDiagram{Page: pl.Page, Box: roundBox(box), Outline: big, Cavities: inner, Labels: lab, Positions: []CavityPos{}, ViewSide: "not stated on the drawing"}
			// Pair each cavity circle with the nearest short label printed in
			// or right beside it (greedy, closest pairs first, each label once).
			type cand struct {
				ci, wi int
				d      float64
			}
			var inners []Circle
			for _, c := range cs {
				if c.R < big.R*0.35 && c.R >= 0.5 && math.Hypot(c.CX-big.CX, c.CY-big.CY) <= big.R-c.R*0.5 {
					inners = append(inners, c)
				}
			}
			var cands []cand
			for ci, c := range inners {
				for wi, w := range pl.Words {
					t := strings.TrimSpace(w.Text)
					if t == "" || len([]rune(t)) > 4 || !reCavityLabel.MatchString(t) {
						continue
					}
					// Cavity numbers are printed on the insert face; a connector
					// tag or note next to the view is not a cavity label.
					if math.Hypot(w.Box.CX()-big.CX, w.Box.CY()-big.CY) > big.R*1.1 {
						continue
					}
					if dd := math.Hypot(w.Box.CX()-c.CX, w.Box.CY()-c.CY); dd <= 2.4*c.R+2 {
						cands = append(cands, cand{ci, wi, dd})
					}
				}
			}
			sort.Slice(cands, func(a, b int) bool { return cands[a].d < cands[b].d })
			usedC, usedW := map[int]bool{}, map[int]bool{}
			cavLabel := map[int]string{}
			for _, c := range cands {
				if usedC[c.ci] || usedW[c.wi] {
					continue
				}
				usedC[c.ci], usedW[c.wi] = true, true
				cavLabel[c.ci] = strings.TrimSpace(pl.Words[c.wi].Text)
			}
			for ci, c := range inners {
				d.Positions = append(d.Positions, CavityPos{Mark: c.Mark, Label: cavLabel[ci], X: r2(c.CX - big.CX), Y: r2(c.CY - big.CY), R: r2(c.R)})
				if cavLabel[ci] != "" {
					d.Labeled++
				}
			}
			// The NEAREST caption within ~1.5 radii decides the view side
			// (front and rear views are often drawn side by side); otherwise
			// the side stays unknown.
			capDist := math.Inf(1)
			for _, w := range pl.Words {
				dist := boxDistance(w.Box, box)
				if dist > big.R*1.5+20 || dist >= capDist || len([]rune(w.Text)) <= 4 {
					continue
				}
				switch {
				case reRearView.MatchString(w.Text):
					d.ViewSide, capDist = "rear (wire side) — as captioned on the drawing", dist
				case reFrontView.MatchString(w.Text):
					d.ViewSide, capDist = "front (mating face) — as captioned on the drawing", dist
				}
			}
			best := math.Inf(1)
			for tag, hits := range labels {
				for _, h := range hits {
					if h.Page != pl.Page {
						continue
					}
					if dist := boxDistance(h.Box, box); dist < best && dist < big.R+100 {
						best, d.NearTag = dist, tag
					}
				}
			}
			d.Note = fmt.Sprintf("Vector circular view with %d cavity-like circles and %d short labels inside (heuristic; review the face/rear view on the page)", inner, lab)
			out = append(out, d)
		}
	}
	return out
}

// mergeConcentric collapses circles drawn more than once at the same place
// (a filled disc plus its outline stroke, or a ring with a dot inside) into
// one, so every cavity is counted once. The larger circle is kept; its mark
// becomes "solid" when either copy is a solid disc of nearly the same size,
// and "ring+dot" for a solid dot inside an open ring.
func mergeConcentric(in []Circle) []Circle {
	cs := append([]Circle(nil), in...)
	sort.Slice(cs, func(i, j int) bool { return cs[i].R > cs[j].R })
	out := []Circle{}
	for _, c := range cs {
		merged := false
		for k := range out {
			o := &out[k]
			if math.Hypot(o.CX-c.CX, o.CY-c.CY) > 0.25*o.R || c.R < 0.3*o.R {
				continue
			}
			if c.Mark == "solid" && o.Mark != "solid" {
				if c.R >= 0.8*o.R {
					o.Mark = "solid"
				} else {
					o.Mark = "ring+dot"
				}
			}
			merged = true
			break
		}
		if !merged {
			out = append(out, c)
		}
	}
	return out
}

// BuildEvidence correlates labels, BOM, wiring and graphics per endpoint.
func BuildEvidence(x *DocExtraction, a *Analysis) ([]EndpointEvidence, []InsertDiagram, []Finding) {
	labels := labelHits(x.Layouts)
	diagrams := insertDiagrams(x.Layouts, labels)
	findings := []Finding{}
	byTag := map[string]*EndpointEvidence{}
	get := func(tag string) *EndpointEvidence {
		tag = strings.ToUpper(strings.ReplaceAll(tag, "-", ""))
		if e, ok := byTag[tag]; ok {
			return e
		}
		e := &EndpointEvidence{Tag: tag, Labels: []LabelHit{}, BOMRows: []string{}, WiringPages: []int{}, Graphics: []GraphicHit{}, Sources: []string{}, Notes: []string{}}
		byTag[tag] = e
		return e
	}
	for _, c := range a.Connectors {
		if c.Tag == "" || c.Tag == "UNASSIGNED" {
			continue
		}
		e := get(c.Tag)
		if c.Part != "" && c.Part != "NOT RESOLVED" {
			e.Part = c.Part
		}
	}
	for _, b := range a.BOM {
		for _, t := range reRefDesignator.FindAllString(strings.ToUpper(b.Reference), -1) {
			e := get(t)
			e.BOMRows = append(e.BOMRows, fmt.Sprintf("sheet %d item %s: %s %s", b.Page, b.Item, b.Part, b.Description))
		}
	}
	for _, w := range a.Wires {
		for _, t := range []string{w.Connector, w.End} {
			if t == "" || t == "UNASSIGNED" || t == "UNRESOLVED ENDPOINT" || !reRefDesignator.MatchString(t) {
				continue
			}
			e := get(t)
			e.WiringRows++
			e.WiringPages = uniqInts(append(e.WiringPages, w.Page))
		}
	}
	// Only add designators seen on the drawing when they are also used by
	// another source; a lone "P3" in a note is not an endpoint by itself.
	for tag, hits := range labels {
		e, ok := byTag[tag]
		if !ok {
			continue
		}
		e.Labels = append(e.Labels, hits...)
	}
	for _, e := range byTag {
		seen := map[string]bool{}
		for _, h := range e.Labels {
			pl := x.Layouts[h.Page-1]
			for _, im := range pl.Images {
				if dist := boxDistance(h.Box, im.Box); dist < 150 {
					k := fmt.Sprintf("img%d/%d", h.Page, im.ID)
					if !seen[k] {
						seen[k] = true
						e.Graphics = append(e.Graphics, GraphicHit{Page: h.Page, Box: im.Box, Kind: "embedded picture", Detail: im.Orientation, Distance: math.Round(dist)})
					}
				}
			}
		}
		for _, d := range diagrams {
			if d.NearTag != e.Tag {
				continue
			}
			e.Graphics = append(e.Graphics, GraphicHit{Page: d.Page, Box: d.Box, Kind: "vector insert view", Detail: d.Note})
		}
		sort.Slice(e.Graphics, func(i, j int) bool { return e.Graphics[i].Distance < e.Graphics[j].Distance })
		if len(e.Graphics) > 8 {
			e.Graphics = e.Graphics[:8]
		}
		if len(e.Labels) > 0 {
			e.Sources = append(e.Sources, "drawing label")
		}
		if e.Part != "" {
			e.Sources = append(e.Sources, "part number")
		}
		if len(e.BOMRows) > 0 {
			e.Sources = append(e.Sources, "BOM reference")
		}
		if e.WiringRows > 0 {
			e.Sources = append(e.Sources, "wiring/pinout rows")
		}
		if len(e.Graphics) > 0 {
			e.Sources = append(e.Sources, "nearby graphic")
		}
		// Graphic cavity-count corroboration (never authoritative).
		for _, c := range a.Connectors {
			if c.Tag != e.Tag || c.CavityCount == 0 {
				continue
			}
			for _, d := range diagrams {
				if d.NearTag != e.Tag {
					continue
				}
				if d.Cavities == c.CavityCount {
					e.Notes = append(e.Notes, fmt.Sprintf("Insert view on sheet %d shows %d cavity-like circles, equal to the catalog position count (heuristic agreement).", d.Page, d.Cavities))
				} else {
					e.Notes = append(e.Notes, fmt.Sprintf("Insert view on sheet %d shows %d cavity-like circles; catalog lists %d positions. Review the view (count is heuristic).", d.Page, d.Cavities, c.CavityCount))
					findings = append(findings, Finding{Severity: "REVIEW", Code: "GRAPHIC_COUNT_REVIEW", Category: CatReview, Message: fmt.Sprintf("%s: vector insert view on sheet %d has %d cavity-like circles but the catalog count for %s is %d. Heuristic only; confirm visually.", e.Tag, d.Page, d.Cavities, c.Part, c.CavityCount), Pages: []int{d.Page}})
				}
			}
		}
		e.Status = fmt.Sprintf("%d independent source type(s) — DRAFT, review required", len(e.Sources))
		if len(e.Sources) <= 1 {
			e.Status = "Single source only — unconfirmed"
		}
		if len(e.Labels) > 12 {
			e.Labels = e.Labels[:12]
		}
	}
	out := []EndpointEvidence{}
	for _, e := range byTag {
		out = append(out, *e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tag < out[j].Tag })
	return out, diagrams, findings
}

// Finding categories separate "the app could not read it" from "the drawing
// disagrees with itself" from "a human must confirm".
const (
	CatExtraction = "EXTRACTION_INCOMPLETE"
	CatConflict   = "DRAWING_CONFLICT"
	CatReview     = "REVIEW"
)

func categoryFor(code string) string {
	switch code {
	case "CAVITY_OUT_OF_RANGE", "DUPLICATE_CAVITY":
		return CatConflict
	case "NO_CONNECTORS", "NO_CAVITY_ROWS", "NO_NATIVE_TEXT", "WIRING_PAGE_UNINTERPRETED", "PDF_PARSE_INCOMPLETE", "TEXT_NOT_DECODABLE", "TEXT_AS_GRAPHICS", "BOM_NOT_FOUND", "LEGACY_ENGINE", "PAGE_NOT_LOADED":
		return CatExtraction
	}
	return CatReview
}

// interpretationFindings adds document-level interpretation status. A drawing
// with content but no records is a FAILED interpretation, never an empty one.
func interpretationFindings(x *DocExtraction, a *Analysis) []Finding {
	f := []Finding{}
	if x.Legacy {
		f = append(f, Finding{Severity: "ERROR", Code: "LEGACY_ENGINE", Message: "PDFium could not open this file, so the legacy v2.6 interpreter was used. Its coordinates are not page-accurate. Results cannot be trusted; review every sheet.", Pages: []int{}})
	}
	contentPages := []int{}
	lowText, outlined := []int{}, []int{}
	for _, pl := range x.Layouts {
		if len(pl.Words) > 15 || len(pl.Images) > 0 || pl.PathObjects > 40 {
			contentPages = append(contentPages, pl.Page)
		}
		if pl.Text.Chars >= 20 && pl.Text.DecodedRatio < 0.7 {
			lowText = append(lowText, pl.Page)
		}
		if pl.Text.OutlinedTextLikely {
			outlined = append(outlined, pl.Page)
		}
	}
	if len(a.Connectors)+len(a.Wires)+len(a.BOM) == 0 && len(contentPages) > 0 {
		f = append(f, Finding{Severity: "ERROR", Code: "PDF_PARSE_INCOMPLETE", Message: fmt.Sprintf("INTERPRETATION FAILED: %d sheet(s) contain drawing content, but no connector, conductor or BOM record was recognized. This is NOT an empty drawing. Use page review to select the BOM, pinout tables and connector views.", len(contentPages)), Pages: contentPages})
	}
	if len(lowText) > 0 {
		f = append(f, Finding{Severity: "ERROR", Code: "TEXT_NOT_DECODABLE", Message: "Some characters on these sheets have no Unicode mapping (font without a usable ToUnicode/encoding). Part numbers and pin labels there may be missing or wrong.", Pages: lowText})
	}
	if len(outlined) > 0 {
		f = append(f, Finding{Severity: "ERROR", Code: "TEXT_AS_GRAPHICS", Message: "These sheets have almost no machine-readable text but many vector objects: lettering is probably drawn as outlines. Enter values from the rendered page in page review.", Pages: outlined})
	}
	unresolved := []int{}
	for _, w := range a.Wires {
		if w.End == "UNRESOLVED ENDPOINT" || w.Connector == "UNASSIGNED" {
			unresolved = append(unresolved, w.Page)
		}
	}
	if len(unresolved) > 0 {
		f = append(f, Finding{Severity: "WARNING", Code: "ENDPOINT_UNRESOLVED", Message: fmt.Sprintf("%d conductor candidate(s) have an unassigned source connector or an unresolved far end. Resolve them against the drawing.", len(unresolved)), Pages: uniqInts(unresolved)})
	}
	return f
}

func pageDiagnostics(x *DocExtraction, a *Analysis, diagrams []InsertDiagram) []PageDiagnostic {
	out := []PageDiagnostic{}
	for i, pl := range x.Layouts {
		d := PageDiagnostic{Page: pl.Page, Rotation: pl.Rotation, AnalysisFrame: analysisFrame(pl), TextFragments: len(pl.Words), Characters: pl.Text.Chars, Pictures: len(pl.Images), VectorObjects: pl.PathObjects, Circles: len(pl.Circles), OutlinedTextLikely: pl.Text.OutlinedTextLikely, Notes: append([]string{}, pl.Notes...)}
		if pl.Width >= pl.Height {
			d.DisplayOrientation = "landscape"
		} else {
			d.DisplayOrientation = "portrait"
		}
		if pl.Text.Chars > 0 {
			d.DecodedPercent = int(math.Round(pl.Text.DecodedRatio * 100))
		}
		for _, im := range pl.Images {
			if im.Mirrored || im.Rotation != 0 {
				d.PicturesFlipped++
			}
		}
		for _, g := range diagrams {
			if g.Page == pl.Page {
				d.InsertDiagrams++
			}
		}
		if i < len(a.PageSummary) {
			d.WiringRows, d.BOMRows = a.PageSummary[i].CavityRows, a.PageSummary[i].BOMRows
		}
		switch {
		case x.Legacy:
			d.Status = "LEGACY ENGINE — not page-accurate"
		case pl.Text.OutlinedTextLikely:
			d.Status = "NO READABLE TEXT — manual page review"
		case pl.Text.Chars > 0 && pl.Text.DecodedRatio < 0.7:
			d.Status = "TEXT PARTLY UNDECODABLE"
		case d.WiringRows+d.BOMRows > 0:
			d.Status = "RECORDS RECOGNIZED — review"
		case len(pl.Words) > 15 || len(pl.Images) > 0:
			d.Status = "CONTENT, NO RECORDS — use page review"
		default:
			d.Status = "LITTLE CONTENT"
		}
		if i < len(x.Data.Pages) {
			ts := genericTables(x.Data.Pages[i])
			for _, t := range ts {
				d.Notes = append(d.Notes, "Table read: "+tableSummary(t)+".")
			}
			if len(ts) == 0 && len(pl.Words) > 15 {
				d.Notes = append(d.Notes, "No table heading recognized on this sheet. If it has a parts list or pin table, select it in Page review and tick \"Remember these headings\".")
			}
		}
		if d.AnalysisFrame != 0 {
			d.Notes = append(d.Notes, fmt.Sprintf("Dominant text runs at %d° on the displayed page; recognizers read it in a rotated frame.", d.AnalysisFrame))
		}
		out = append(out, d)
	}
	return out
}
