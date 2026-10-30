package main

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

type ViewConnector struct {
	ContactCandidates     []ConnectorContact `json:"ContactCandidates"`
	ContactMappingStatus  string             `json:"ContactMappingStatus"`
	ContactMappingWarning string             `json:"ContactMappingWarning"`
	Name                  string
	Part                  string
	Side                  string
	Cavities              int
	Status                string
	Source                string
	Warning               string
	Page                  int
	Pages                 []int
	Family                string
	Arrangement           string
	Labels                []string
}
type ViewCandidate struct {
	Connector string
	Number    string
	Color     string
	Page      int
	Signal    string
}
type ViewBOM struct {
	PartReference string `json:"partReference,omitempty"`
	Item          string `json:"item"`
	Qty           string `json:"qty"`
	Part          string `json:"part"`
	Description   string `json:"description"`
	Vendor        string `json:"vendor"`
	Reference     string `json:"reference"`
	Page          int    `json:"Page"`
}
type ViewIssue struct {
	Severity  string
	Category  string
	Code      string
	Message   string
	Connector string `json:",omitempty"`
	Part      string `json:",omitempty"`
	Pages     []int
	Cavities  []string
	Expected  string `json:",omitempty"`
	Observed  string `json:",omitempty"`
	Action    string
}

// PageInfo is the display-space geometry the UI needs for the page viewer.
// Word text is NOT included here; it is fetched per page on demand.
type PageInfo struct {
	Page     int              `json:"page"`
	Width    float64          `json:"width"`
	Height   float64          `json:"height"`
	Rotation int              `json:"rotation"`
	Images   []ImagePlacement `json:"images"`
}

type ViewAnalysis struct {
	Callouts []GraphicCallout `json:"graphicalCallouts"`

	AppVersion     string             `json:"appVersion"`
	Engine         string             `json:"engine"`
	EngineError    string             `json:"engineError,omitempty"`
	Interpretation string             `json:"interpretation"`
	Compare26      *EngineComparison  `json:"compareV26,omitempty"`
	GraphicPages   []GraphicPage      `json:"graphicPages"`
	PageInfo       []PageInfo         `json:"pageInfo"`
	Diagnostics    []PageDiagnostic   `json:"diagnostics"`
	Endpoints      []EndpointEvidence `json:"endpointEvidence"`
	InsertViews    []InsertDiagram    `json:"insertDiagrams"`
	CavityModels   []CavityModel      `json:"cavityModels"`
	Unused         []UnusedCavity     `json:"unusedCavities"`
	Pages          int                `json:"pages"`
	Words          int                `json:"words"`
	Sha256         string             `json:"sha256"`
	PageWords      []int              `json:"pageWords"`
	Connectors     []ViewConnector    `json:"connectors"`
	Candidates     []ViewCandidate    `json:"candidates"`
	Wires          []WireRow          `json:"wires"`
	Branches       []string           `json:"branches"`
	Boms           []ViewBOM          `json:"boms"`
	Issues         []ViewIssue        `json:"issues"`
	Warnings       []string           `json:"warnings"`
	PageSummary    []struct {
		Page       int
		Words      int
		CavityRows int
		BOMRows    int
		Category   string
	} `json:"pageSummary"`
	PdfText          string          `json:"pdfText"`
	AssemblyFeatures []string        `json:"assemblyFeatures"`
	HarnessLengths   []HarnessLength `json:"harnessLengths"`
	CaptureChecks    []CaptureCheck  `json:"captureChecks"`
}

func viewAnalysis(b []byte) (ViewAnalysis, error) {
	x, err := ExtractDocument(b)
	if err != nil {
		return ViewAnalysis{}, err
	}
	return viewFromExtraction(x)
}

func viewFromExtraction(x *DocExtraction) (ViewAnalysis, error) {
	a, endpoints, diagrams, err := AnalyzeExtraction(x)
	if err != nil {
		return ViewAnalysis{}, err
	}
	v := ViewAnalysis{AppVersion: AppVersion, Engine: x.Engine, EngineError: x.EngineError, Pages: a.Pages, Words: a.Words, Sha256: x.SHA256, Connectors: []ViewConnector{}, Candidates: []ViewCandidate{}, Wires: a.Wires, Branches: []string{}, Boms: []ViewBOM{}, Issues: []ViewIssue{}, Warnings: []string{}, PageWords: []int{}, PageInfo: []PageInfo{}, Endpoints: endpoints, InsertViews: diagrams, AssemblyFeatures: []string{}, PageSummary: []struct {
		Page       int
		Words      int
		CavityRows int
		BOMRows    int
		Category   string
	}{}}
	if v.Endpoints == nil {
		v.Endpoints = []EndpointEvidence{}
	}
	if v.InsertViews == nil {
		v.InsertViews = []InsertDiagram{}
	}
	v.Callouts = graphicalCallouts(x, a.BOM)
	v.Diagnostics = pageDiagnostics(x, &a, diagrams)
	v.CavityModels = buildCavityModels(&a, diagrams)
	v.Unused = a.Unused
	if v.Unused == nil {
		v.Unused = []UnusedCavity{}
	}
	v.Compare26 = x.Compare26
	if c := x.Compare26; c != nil && c.Error == "" && (c.Connectors > len(a.Connectors) || c.Wires > len(a.Wires) || c.BOM > len(a.BOM)) {
		a.Findings = append([]Finding{{Severity: "WARNING", Code: "ENGINE_COMPARISON", Category: CatReview, Pages: []int{}, Message: fmt.Sprintf("The retired v2.6 text extraction, run through the same v2.7 recognizers, yields more records of some type on this file (v2.6 extraction %d/%d/%d vs PDFium %d/%d/%d connectors/conductors/BOM). The legacy extraction has known false positives on rotated pages, but review the sheets for records PDFium extraction missed and report which record TYPE differs.", c.Connectors, c.Wires, c.BOM, len(a.Connectors), len(a.Wires), len(a.BOM))}}, a.Findings...)
	}
	v.GraphicPages = graphicPagesFromLayouts(x.Layouts, diagrams)
	for _, pl := range x.Layouts {
		v.PageInfo = append(v.PageInfo, PageInfo{Page: pl.Page, Width: pl.Width, Height: pl.Height, Rotation: pl.Rotation, Images: pl.Images})
	}
	switch {
	case x.Legacy:
		v.Interpretation = "UNRELIABLE — legacy engine"
	case len(a.Connectors)+len(a.Wires)+len(a.BOM) == 0:
		v.Interpretation = "FAILED — no engineering records recognized"
	case len(a.Connectors) == 0 || len(a.Wires) == 0 || len(a.BOM) == 0:
		v.Interpretation = "PARTIAL — some record types not recognized"
	default:
		v.Interpretation = "CANDIDATES RECOGNIZED — DRAFT, review required"
	}
	for _, p := range a.PageSummary {
		v.PageWords = append(v.PageWords, p.Words)
		v.PageSummary = append(v.PageSummary, struct {
			Page       int
			Words      int
			CavityRows int
			BOMRows    int
			Category   string
		}{p.Page, p.Words, p.CavityRows, p.BOMRows, p.Category})
	}
	for _, c := range a.Connectors {
		sex := ""
		if strings.HasSuffix(c.Part, "PA") || strings.HasSuffix(c.Part, "P") {
			sex = "PIN"
		} else if strings.HasSuffix(c.Part, "SA") || strings.HasSuffix(c.Part, "S") {
			sex = "SOCKET"
		}
		if c.Part == "NOT RESOLVED" {
			sex = ""
		}
		p := 0
		if len(c.Pages) > 0 {
			p = c.Pages[0]
		}
		arrangement := ""
		if strings.HasPrefix(c.Part, "D38999/") && len(c.Part) > 13 {
			arrangement = c.Part[10:13]
		}
		resolved := ResolveConnectorContacts(c.Part)
		// The 38999 contact letter precedes the keying letter (…35SN = socket,
		// normal key); trust the decoded part number over a suffix guess.
		if resolved.ContactSex != "" {
			sex = resolved.ContactSex
		} else if m := reSeries3.FindStringSubmatch(c.Part); m != nil {
			sex = map[string]string{"P": "PIN", "S": "SOCKET", "H": "PIN", "J": "SOCKET", "A": "PIN (less contacts)", "B": "SOCKET (less contacts)"}[m[5]]
		}
		v.Connectors = append(v.Connectors, ViewConnector{ContactCandidates: resolved.Contacts, ContactMappingStatus: resolved.Status, ContactMappingWarning: resolved.Warning, Name: c.Tag, Part: c.Part, Side: sex, Cavities: c.CavityCount, Status: "catalog-count-only", Source: "Catalog count inferred; exact cavity layout not yet verified", Warning: c.LayoutStatus, Page: p, Pages: c.Pages, Family: c.Family, Arrangement: arrangement, Labels: []string{}})
	}
	branch := map[string]bool{}
	for _, w := range a.Wires {
		v.Candidates = append(v.Candidates, ViewCandidate{w.Connector, w.Cavity, w.Color, w.Page, w.Signal})
		if w.End != "" && w.End != "UNRESOLVED ENDPOINT" {
			branch[w.End] = true
		}
	}
	for end := range branch {
		v.Branches = append(v.Branches, end)
	}
	sort.Strings(v.Branches)
	for _, bom := range a.BOM {
		v.Boms = append(v.Boms, ViewBOM{Item: bom.Item, Qty: bom.Qty, Part: bom.Part, Description: bom.Description, Vendor: bom.Vendor, Reference: bom.Reference, Page: bom.Page, PartReference: bom.PartReference})
	}
	for _, f := range a.Findings {
		x := ViewIssue{Severity: f.Severity, Category: f.Category, Code: f.Code, Message: f.Message, Pages: f.Pages, Action: "Review original source sheet and obtain approved engineering resolution before production release.", Cavities: []string{}}
		switch f.Category {
		case CatExtraction:
			x.Action = "PDF recognition is incomplete. This is NOT proof of an error in the customer drawing. Open the sheet in page review and select or enter the missing records; do not use an inferred cavity list."
		case CatReview:
			x.Action = "Confirm against the rendered sheet. Review items do not by themselves indicate a drawing error."
		}
		if f.Code == "CAVITY_OUT_OF_RANGE" {
			for _, c := range a.Connectors {
				if strings.Contains(f.Message, c.Part) {
					x.Connector = c.Tag
					x.Part = c.Part
					x.Expected = fmt.Sprintf("Maximum %d contacts, but exact cavity IDs require verified insert map", c.CavityCount)
				}
			}
			for _, w := range a.Wires {
				if w.Connector != x.Connector {
					continue
				}
				for _, c := range a.Connectors {
					if c.Tag != x.Connector || c.CavityCount == 0 {
						continue
					}
					var n int
					fmt.Sscanf(w.Cavity, "%d", &n)
					if n > c.CavityCount {
						x.Cavities = append(x.Cavities, w.Cavity)
					}
				}
			}
			if len(x.Cavities) > 0 {
				x.Observed = "Source assigns cavities " + strings.Join(x.Cavities, ", ")
			}
		}
		v.Issues = append(v.Issues, x)
	}
	v.Issues = append(v.Issues, offInsertIssues(&a, v.CavityModels)...)
	v.Issues = append(v.Issues, drawingWeaknessIssues(&a)...)
	// ALL drawing-derived records are preliminary and must not bypass source verification.
	v.Warnings = append(v.Warnings, "Engine: "+x.Engine)
	v.Warnings = append(v.Warnings, "Interpretation: "+v.Interpretation)
	v.Warnings = append(v.Warnings, "Page-accurate PDFium extraction. Candidates are UNVERIFIED until each drawing sheet is checked.")
	v.Warnings = append(v.Warnings, "Exact-part guide inventories are used when available. Count-only matches still need a verified cavity-ID map. All unused lists require complete wiring review and conflict resolution.")
	if len(a.BOM) == 0 {
		v.Warnings = append(v.Warnings, "BOM not recognized; select it in page review or reconcile manually.")
	}
	if len(a.Wires) == 0 {
		v.Warnings = append(v.Warnings, "COVERAGE GAP: no wiring rows recognized.")
	}
	if len(a.Connectors) == 0 {
		v.Warnings = append(v.Warnings, "No trustworthy connector identification.")
	}
	// Local-only exact contact lookup, not full source text; prevents extraneous drawing content in exported saved projects.
	d := x.Data
	ids := []string{}
	re := regexp.MustCompile(`(?i)(?:M39029/[0-9]{1,3}-[0-9]{1,4}|M85049/(?:20|38|128)-[0-9]{1,3}[A-Z]?)`)
	for _, p := range d.Pages {
		for _, w := range p.Words {
			ids = append(ids, re.FindAllString(strings.ToUpper(w.Text), -1)...)
		}
	}
	v.PdfText = strings.Join(uniqueStrings(ids), " ")
	v.HarnessLengths = harnessLengths(d.Pages)
	v.CaptureChecks = captureChecks(&a, v.Callouts, v.HarnessLengths)
	for _, c := range v.CaptureChecks {
		if c.Status == CheckDisagree {
			v.Issues = append(v.Issues, ViewIssue{Severity: "REVIEW", Category: CatReview, Code: "CAPTURE_READINGS_DISAGREE", Message: c.Title + ": " + c.Message, Connector: c.Connector, Pages: []int{}, Cavities: []string{}, Action: "Compare the readings listed under Capture checks with the sheet."})
		}
	}
	// Whitelisted, local-only process categories; do not return source drawing text.
	terms := map[string]bool{}
	for _, p := range d.Pages {
		for _, w := range p.Words {
			s := strings.ToUpper(w.Text)
			for k, hints := range map[string][]string{
				"SHIELD_PREPARATION": {"SHIELD", "BRAID", "DRAIN"},
				"SOLDERING":          {"SOLDER", "TINNED"},
				"WIRE_STRIPPING":     {"STRIP", "JACKET"},
				"HEAT_SHRINK":        {"SHRINK", "SLEEVING", "BOOT"},
				"LABELING":           {"LABEL", "MARKER"},
				"TERMINAL_CRIMPING":  {"CRIMP", "RING TONGUE"},
			} {
				for _, hint := range hints {
					if strings.Contains(s, hint) {
						terms[k] = true
					}
				}
			}
		}
	}
	for k := range terms {
		v.AssemblyFeatures = append(v.AssemblyFeatures, k)
	}
	sort.Strings(v.AssemblyFeatures)
	return v, nil
}

func graphicPagesFromLayouts(ls []PageLayout, diagrams []InsertDiagram) []GraphicPage {
	out := []GraphicPage{}
	for _, pl := range ls {
		g := GraphicPage{Page: pl.Page, VectorSegments: pl.PathSegments, Rectangles: 0, Curves: len(pl.Circles), ImageObjects: len(pl.Images), FormObjects: pl.FormObjects}
		g.GraphicsPresent = g.ImageObjects > 0 || g.FormObjects > 0 || g.VectorSegments > 0
		views := 0
		for _, d := range diagrams {
			if d.Page == pl.Page {
				views++
			}
		}
		switch {
		case !g.GraphicsPresent:
			g.Assessment = "No drawable graphics detected; inspect source page"
		case views > 0:
			g.Assessment = fmt.Sprintf("%d vector insert/face view candidate(s); open page review", views)
		case g.ImageObjects > 0:
			g.Assessment = "Placed pictures present; identity is not determined automatically"
		default:
			g.Assessment = "Vector drawing content; inspect connector views, wires and callouts in page review"
		}
		out = append(out, g)
	}
	return out
}
func uniqueStrings(ss []string) []string {
	m := map[string]bool{}
	o := []string{}
	for _, s := range ss {
		if !m[s] {
			o = append(o, s)
			m[s] = true
		}
	}
	return o
}

// libraryCSV returns a schema-validated reference library. A schema error is
// returned to the caller (and shown in the UI) instead of skipping rows.
func libraryCSV(name string) ([][]string, error) { return readLibrary(name) }
func splitToolRoutes(s string) [][]string {
	var out [][]string
	for _, x := range strings.Split(s, " OR ") {
		path := []string{}
		for _, v := range strings.Split(x, " + ") {
			if s := strings.TrimSpace(v); s != "" {
				path = append(path, s)
			}
		}
		if len(path) > 0 {
			out = append(out, path)
		}
	}
	return out
}
func crimpReference() ([]any, error) {
	rows, err := libraryCSV("DMC_EXPANDED_REFERENCE.csv")
	if err != nil {
		return nil, err
	}
	out := []any{}
	for _, r := range rows[1:] {
		if len(r) < 11 {
			continue
		}
		out = append(out, map[string]any{"part": r[0], "aliases": splitAliases(r[1]), "sex": r[2], "contactSize": r[3], "wireRange": r[5], "routes": splitToolRoutes(r[6]), "insertion": r[7], "removal": r[8], "source": r[9], "status": r[10]})
	}
	return out, nil
}
func splitAliases(s string) []string {
	if s == "" {
		return []string{}
	}
	f := strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == '|' || r == ',' })
	out := []string{}
	for _, x := range f {
		if v := strings.TrimSpace(x); v != "" {
			out = append(out, v)
		}
	}
	return out
}
func specialtyReference() (map[string]any, error) {
	out := []any{}
	rows, err := libraryCSV("SPECIALTY_CONTACTS_REFERENCE.csv")
	if err != nil {
		return nil, err
	}
	for _, r := range rows[1:] {
		if len(r) < 8 {
			continue
		}
		steps := []any{}
		for _, segment := range strings.Split(r[7], "; ") {
			parts := strings.SplitN(segment, ": ", 2)
			if len(parts) != 2 {
				continue
			}
			steps = append(steps, map[string]any{"name": parts[0], "alternatives": splitToolRoutes(parts[1]), "notice": "Confirm all operations against manufacturer instructions."})
		}
		out = append(out, map[string]any{"part": r[0], "aliases": []string{}, "family": r[1], "kind": r[2], "sex": r[3], "wireRange": r[4], "source": r[5], "status": r[6], "manufacturer": "Manufacturer source", "steps": steps, "cable": "Check cable specification", "notice": "DRAFT: contact-specific process needs verification", "insertion": "UNRESOLVED", "removal": "UNRESOLVED"})
	}
	rows, err = libraryCSV("MANUFACTURER_CONTACT_REFERENCE.csv")
	if err != nil {
		return nil, err
	}
	for _, r := range rows[1:] {
		if len(r) < 10 {
			continue
		}
		steps := []any{}
		for _, seg := range strings.Split(r[6], "; ") {
			a := strings.SplitN(seg, ": ", 2)
			if len(a) != 2 {
				continue
			}
			steps = append(steps, map[string]any{"name": a[0], "alternatives": splitToolRoutes(a[1]), "notice": "Review manufacturer application specification."})
		}
		out = append(out, map[string]any{"part": r[1], "aliases": splitAliases(r[2]), "family": r[3], "kind": "Manufacturer contact", "sex": r[4], "wireRange": r[5], "source": r[8], "status": r[7], "manufacturer": r[0], "steps": steps, "cable": "Manufacturer-specific", "notice": r[9], "insertion": "UNRESOLVED", "removal": "UNRESOLVED"})
	}
	return map[string]any{"contacts": out, "guidance": []any{}}, nil
}

func accessoryReference() ([]any, error) {
	rows, err := libraryCSV("ACCESSORY_REFERENCE.csv")
	if err != nil {
		return nil, err
	}
	out := []any{}
	for _, r := range rows[1:] {
		if len(r) < 6 {
			continue
		}
		out = append(out, map[string]any{
			"kind":        r[0],
			"matches":     splitAliases(r[1]),
			"class":       r[2],
			"requirement": r[3],
			"source":      r[4],
			"status":      r[5],
			"specification": func() string {
				if len(r) > 6 {
					return r[6]
				}
				return ""
			}(),
			"alternates": func() []string {
				if len(r) > 7 {
					return splitAliases(r[7])
				}
				return []string{}
			}(),
			"alternateSource": func() string {
				if len(r) > 8 {
					return r[8]
				}
				return ""
			}(),
			"alternateStatus": func() string {
				if len(r) > 9 {
					return r[9]
				}
				return ""
			}(),
		})
	}
	return out, nil
}

func wireCableReference() ([]any, error) {
	rows, err := libraryCSV("WIRE_CABLE_REFERENCE.csv")
	if err != nil {
		return nil, err
	}
	out := []any{}
	for _, r := range rows[1:] {
		out = append(out, map[string]any{
			"pattern": r[0], "family": r[1], "construction": r[2],
			"preparation": r[3], "termination": r[4], "test": r[5],
			"source": r[6], "status": r[7],
		})
	}
	return out, nil
}

func toolEquivalenceReference() ([]any, error) {
	rows, err := libraryCSV("TOOL_EQUIVALENCE_REFERENCE.csv")
	if err != nil {
		return nil, err
	}
	out := []any{}
	for _, r := range rows[1:] {
		out = append(out, map[string]any{
			"specification": r[0], "operation": r[1], "primary": r[2],
			"alternates": splitToolRoutes(r[3]), "conditions": r[4],
			"sourcePrimary": r[5], "sourceAlternate": r[6], "status": r[7],
		})
	}
	return out, nil
}
func noUnknownCitation() io.Reader { return strings.NewReader("") }
