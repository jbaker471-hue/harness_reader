package main

import (
	"strconv"
	"strings"
	"testing"
)

func TestConnectorFamilyFromPartNumber(t *testing.T) {
	cases := map[string]string{
		"D38999/26FD35PN": "circular", "MS3106A18-1S": "circular", "M83723/71G1210N": "circular",
		"M83513/01-A01N": "micro-d", "DT06-2S": "rectangular", "M24308/2-1": "d-sub", "DB25P": "d-sub", "DE9S": "d-sub",
		"ZZZ-123": "unknown", "": "unknown",
	}
	for pn, shape := range cases {
		if got := DecodeConnectorFamily(pn); got.Shape != shape {
			t.Fatalf("%q shape = %q, want %q (%+v)", pn, got.Shape, shape, got)
		}
	}
	if got := DecodeConnectorFamily("ZZZ-123"); got.Recognized || got.Geometry != nil || len(got.Notes) == 0 {
		t.Fatalf("unknown family must not be guessed: %+v", got)
	}
}

func TestSeriesIIIReadsShellSizeSexCountAndStoredLayout(t *testing.T) {
	d := DecodeConnectorFamily("D38999/26FD35PN")
	if d.ShellSize != "15" || d.Positions != 37 || d.Sex != "PIN" || d.Geometry == nil || len(d.Geometry.Positions) != 37 {
		t.Fatalf("%+v", d)
	}
	if !strings.Contains(d.Status, "UNVERIFIED") {
		t.Fatalf("family must not read as verified: %q", d.Status)
	}
}

func TestDSubStandardLayouts(t *testing.T) {
	for _, c := range []struct {
		pn     string
		n, top int
	}{{"DE9P", 9, 5}, {"DA15S", 15, 8}, {"DB25P", 25, 13}, {"DC37P", 37, 19}} {
		d := DecodeConnectorFamily(c.pn)
		if d.Geometry == nil || len(d.Geometry.Positions) != c.n || d.Positions != c.n {
			t.Fatalf("%s: %+v", c.pn, d)
		}
		seen := map[string]bool{}
		topRow := 0
		for _, p := range d.Geometry.Positions {
			if seen[p.Label] {
				t.Fatalf("%s duplicate label %s", c.pn, p.Label)
			}
			seen[p.Label] = true
			if p.Y < 0 {
				topRow++
			}
		}
		if topRow != c.top {
			t.Fatalf("%s top row has %d, want %d", c.pn, topRow, c.top)
		}
		if !strings.Contains(d.Geometry.Status, "UNVERIFIED") {
			t.Fatalf("computed layout must stay unverified")
		}
	}
}

func TestDSubWithoutSexOrWithWrongShellDrawsNoPositions(t *testing.T) {
	if d := DecodeConnectorFamily("DB25"); d.Geometry != nil {
		t.Fatalf("no sex stated, orientation unknown: %+v", d)
	}
	d := DecodeConnectorFamily("DE25P")
	if d.Geometry != nil || !strings.Contains(strings.Join(d.Notes, " "), "CONFLICT") {
		t.Fatalf("shell letter conflict not flagged: %+v", d)
	}
	if d := DecodeConnectorFamily("DD50P"); d.Geometry != nil {
		t.Fatalf("three-row shell layout is not modelled: %+v", d)
	}
}

func TestConnectorLibraryRowsAreSourceLinkedAndNeverVerified(t *testing.T) {
	rows, err := connectorDocTable()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 50 {
		t.Fatalf("only %d rows", len(rows))
	}
	seen := map[string]bool{}
	for _, r := range rows {
		k := r.Kind + "|" + r.Code
		if seen[k] {
			t.Fatalf("duplicate row %s", k)
		}
		seen[k] = true
		if strings.Contains(strings.ToUpper(r.Status), "VERIFIED") && !strings.Contains(r.Status, "UNVERIFIED") {
			t.Fatalf("supplied row claims verification: %+v", r)
		}
		if len(r.Sources) == 0 && !strings.Contains(r.Status, "NO SOURCE URL") {
			t.Fatalf("row without a source or an explicit no-source status: %+v", r)
		}
		for _, s := range r.Sources {
			if !strings.HasPrefix(s, "https://") {
				t.Fatalf("source is not https: %q", s)
			}
		}
		for _, l := range r.Layout {
			if lookupConnectorDoc("layout-standard", l) == nil {
				t.Fatalf("%s names layout standard %q that is not in the library", r.Code, l)
			}
		}
	}
	if lookupConnectorDoc("layout-standard", "MIL-STD-1560") == nil || lookupConnectorDoc("tooling-spec", "SAE-AS81969") == nil {
		t.Fatal("priority 1 documents missing")
	}
}

func TestFamilyCarriesSpecAndLayoutStandard(t *testing.T) {
	d := DecodeConnectorFamily("D38999/26FD35PN")
	if len(d.Specs) != 1 || d.Specs[0].Code != "MIL-DTL-38999" || len(d.Layouts) != 1 || d.Layouts[0].Code != "MIL-STD-1560" || len(d.Layouts[0].Sources) == 0 {
		t.Fatalf("%+v", d)
	}
	if d.Geometry == nil || d.Geometry.View != "pin-front" || !strings.Contains(d.Geometry.Status, "UNREVIEWED") || !strings.Contains(d.Geometry.Basis, "page 21") {
		t.Fatalf("stored layout must carry its view, source page and unreviewed status: %+v", d.Geometry)
	}
	if d := DecodeConnectorFamily("M83723/71G1210N"); len(d.Layouts) != 2 {
		t.Fatalf("83723 maps to two layout standards: %+v", d.Layouts)
	}
}

func geoRow(arr, cav, x, y, view, src, page, status string) []string {
	return []string{arr, "MIL-STD-1560", "9", cav, x, y, "22D", view, "normal", src, page, "fig", status}
}

func TestInsertGeometryRowsAreChecked(t *testing.T) {
	good := [][]string{geoRow("9-35", "1", "0", "-3", "pin-front", "S", "5", "REVIEWED"), geoRow("9-35", "2", "3", "0", "pin-front", "S", "5", "REVIEWED")}
	g, p := buildInsertGeometry(good, "MIL-STD-1560", "9-35")
	if g == nil || len(p) != 0 || len(g.Positions) != 2 || g.View != "pin-front" || !strings.Contains(g.Basis, "page 5") {
		t.Fatalf("%+v %v", g, p)
	}
	if g, p := buildInsertGeometry(good, "MIL-STD-1560", "11-35"); g != nil || len(p) != 0 {
		t.Fatalf("other arrangement must be absent, not an error: %+v %v", g, p)
	}
	for name, rows := range map[string][][]string{
		"duplicate":  {geoRow("9-35", "1", "0", "0", "pin-front", "S", "5", "R"), geoRow("9-35", "1", "1", "1", "pin-front", "S", "5", "R")},
		"bad number": {geoRow("9-35", "1", "x", "0", "pin-front", "S", "5", "R")},
		"bad view":   {geoRow("9-35", "1", "0", "0", "top", "S", "5", "R")},
		"no source":  {geoRow("9-35", "1", "0", "0", "pin-front", "", "5", "R")},
		"mixed view": {geoRow("9-35", "1", "0", "0", "pin-front", "S", "5", "R"), geoRow("9-35", "2", "1", "0", "socket-front", "S", "5", "R")},
		"no status":  {geoRow("9-35", "1", "0", "0", "pin-front", "S", "5", "")},
	} {
		if g, p := buildInsertGeometry(rows, "MIL-STD-1560", "9-35"); g != nil || len(p) == 0 {
			t.Fatalf("%s: a bad row must reject the whole arrangement: %+v %v", name, g, p)
		}
	}
}

func TestShippedInsertGeometryLibraryIsValid(t *testing.T) {
	rows, err := readLibrary("CONNECTOR_INSERT_GEOMETRY.csv")
	if err != nil {
		t.Fatal(err)
	}
	arr := map[string]bool{}
	for _, r := range rows[1:] {
		arr[r[1]+"|"+r[0]] = true
	}
	for k := range arr {
		p := strings.SplitN(k, "|", 2)
		if g, probs := buildInsertGeometry(rows[1:], p[0], p[1]); g == nil || len(probs) != 0 {
			t.Fatalf("shipped arrangement %s is invalid: %v", k, probs)
		}
	}
}

func TestAmphenolTablesLoadedCorrectly(t *testing.T) {
	// Spot values read by eye from the printed Contact Hole Locations tables (inches x 25.4, Y flipped to point down).
	for _, c := range []struct {
		arr, cav string
		x, y     float64
		n        int
	}{{"25-35", "1", -12.167, -7.087, 128}, {"25-35", "99", 6.325, 0, 128}, {"25-35", "104", 6.325, 12.065, 128}, {"15-35", "1", 1.143, -6.655, 37}, {"25-61", "PP", 0, 0, 61}, {"25-61", "k", 0, 10.211, 61}, {"25-61", "A", 4.978, -12.7, 61}, {"25-61", "a", -1.727, -11.532, 61}} {
		g, p := insertGeometryFor("MIL-STD-1560", c.arr)
		if g == nil || len(p) != 0 || len(g.Positions) != c.n {
			t.Fatalf("%s: %+v %v", c.arr, g, p)
		}
		found := false
		for _, q := range g.Positions {
			if q.Label == c.cav {
				found = true
				if d := q.X - c.x; d > 0.002 || d < -0.002 {
					t.Fatalf("%s/%s x = %v, want %v", c.arr, c.cav, q.X, c.x)
				}
				if d := q.Y - c.y; d > 0.002 || d < -0.002 {
					t.Fatalf("%s/%s y = %v, want %v", c.arr, c.cav, q.Y, c.y)
				}
			}
		}
		if !found {
			t.Fatalf("%s cavity %s missing", c.arr, c.cav)
		}
	}
	// every loaded series III x35 arrangement matches its reviewed cavity inventory, and none overlap
	for shell, letter := range map[string]string{"15": "D", "17": "E", "21": "G", "23": "H", "25": "J"} {
		g, _ := insertGeometryFor("MIL-STD-1560", shell+"-35")
		inv, _ := guideInventory("D38999/26F" + letter + "35PN")
		if g == nil || len(g.Positions) != len(inv.IDs) {
			t.Fatalf("%s-35 has %v positions, inventory %d", shell, g, len(inv.IDs))
		}
		have := map[string]bool{}
		for _, q := range g.Positions {
			have[q.Label] = true
		}
		for _, id := range inv.IDs {
			if !have[id] {
				t.Fatalf("%s-35 lacks inventory cavity %s", shell, id)
			}
		}
		for i := range g.Positions {
			for j := i + 1; j < len(g.Positions); j++ {
				dx, dy := g.Positions[i].X-g.Positions[j].X, g.Positions[i].Y-g.Positions[j].Y
				if dx*dx+dy*dy < 1.2*1.2 {
					t.Fatalf("%s-35 contacts %s and %s overlap", shell, g.Positions[i].Label, g.Positions[j].Label)
				}
			}
		}
	}
	// arrangements that did not validate are absent rather than partial
	for _, arr := range []string{"19-11"} {
		if g, _ := insertGeometryFor("MIL-STD-1560", arr); g != nil {
			t.Fatalf("%s must not be loaded", arr)
		}
	}
}

func TestInsertLayoutLibraryIsIndependentOfPartNumbers(t *testing.T) {
	all, err := listInsertLayouts()
	if err != nil {
		t.Fatal(err)
	}
	loaded, missing := 0, 0
	for _, l := range all {
		if l.Loaded {
			loaded++
			if l.View != "pin-front" || l.Contacts == 0 || !strings.Contains(l.Status, "UNREVIEWED") {
				t.Fatalf("loaded layout lacks view, count or honest status: %+v", l)
			}
		} else {
			missing++
			if l.Reason == "" || l.Status != "NOT LOADED" {
				t.Fatalf("an arrangement that was not loaded must say why: %+v", l)
			}
		}
	}
	if loaded != 8 || missing != 16 || len(all) != 24 {
		t.Fatalf("catalog coverage changed: %d loaded, %d not loaded, %d total", loaded, missing, len(all))
	}
}

func TestArrangementFromAnyPartNumberOrChoice(t *testing.T) {
	// the arrangement is read from the part number whatever it is, not from a fixed list of parts
	if d := DecodeConnectorFamily("D38999/24WJ61SN"); d.ShellSize != "25" || d.Geometry == nil || len(d.Geometry.Positions) != 61 || d.Sex != "SOCKET" {
		t.Fatalf("25-61 from its part number: %+v", d)
	}
	if d := DecodeConnectorFamily("D38999/26WB35SA"); d.ShellSize != "11" || d.Geometry != nil || !strings.Contains(strings.Join(d.Notes, " "), "No stored insert layout for arrangement 11-35") {
		t.Fatalf("an arrangement with no stored layout must say so: %+v", d)
	}
	// a layout can be chosen when the part number says nothing, and a clash with the part number is reported
	d := DecodeConnectorFamilyWith("D38999/26FD35PN", "17-26")
	if d.Geometry != nil || strings.Count(strings.Join(d.Notes, " "), "CONFLICT") != 2 {
		t.Fatalf("%+v", d)
	}
	if d := DecodeConnectorFamilyWith("D38999/26FD35PN", "15-35"); d.Geometry == nil || len(d.Geometry.Positions) != 37 {
		t.Fatalf("matching choice should draw: %+v", d)
	}
	d = DecodeConnectorFamilyWith("", "25-35")
	if d.Geometry == nil || len(d.Geometry.Positions) != 128 || d.Recognized {
		t.Fatalf("a chosen layout works without a part number and does not invent a family: %+v", d)
	}
	if d := DecodeConnectorFamilyWith("D38999/26FD35PN", "99-99"); d.Geometry != nil {
		t.Fatalf("unknown arrangement must not draw: %+v", d)
	}
}

func TestFamilyCoverageAuditNeverClaimsCompleteAndAgreesWithStoredLayouts(t *testing.T) {
	cov, err := familyCoverage()
	if err != nil {
		t.Fatal(err)
	}
	layouts, _ := listInsertLayouts()
	stored, known := 0, len(layouts)
	for _, l := range layouts {
		if l.Loaded {
			stored++
		}
	}
	if len(cov) < 10 {
		t.Fatalf("only %d families audited", len(cov))
	}
	allowed := map[string]bool{"NOT STARTED": true, "MANUAL DOWNLOAD REQUIRED": true, "DOCUMENTS RETRIEVED - NOT EXTRACTED": true, "PARTIAL": true, "INCOMPLETE": true}
	for _, c := range cov {
		if !allowed[c.AuditStatus] {
			t.Fatalf("%s: audit status %q is not allowed; no family is complete without an arrangement-level audit", c.Family, c.AuditStatus)
		}
		if c.Note == "" {
			t.Fatalf("%s: audit row needs a note", c.Family)
		}
		if c.AuditStatus == "MANUAL DOWNLOAD REQUIRED" && c.ManualRequired == "None" {
			t.Fatalf("%s: says manual download but lists none", c.Family)
		}
	}
	// the 38999 row must agree with the stored library, so the audit cannot drift from the data
	var row *FamilyCoverage
	for i := range cov {
		if strings.HasPrefix(cov[i].Family, "MIL-DTL-38999 Series I") {
			row = &cov[i]
		}
	}
	if row == nil || row.ArrangementsStored != strconv.Itoa(stored) || row.ArrangementsKnown != strconv.Itoa(known) {
		t.Fatalf("38999 coverage row %+v disagrees with the library (%d stored of %d)", row, stored, known)
	}
}

func TestArrangementRegisterIsHonestAndAgreesWithTheCoordinateCatalog(t *testing.T) {
	reg, err := arrangementRegister()
	if err != nil {
		t.Fatal(err)
	}
	if len(reg) < 250 {
		t.Fatalf("register has only %d rows", len(reg))
	}
	allowed := map[string]bool{"EXTRACTED FROM CATALOG TEXT — UNREVIEWED": true, "ID ONLY — UNREVIEWED": true, "SEEN — CELLS NOT READ": true}
	seen := map[string]bool{}
	read, unread := 0, 0
	for _, r := range reg {
		if !allowed[r.Status] {
			t.Fatalf("%s %s: status %q; a register row is never verified", r.Family, r.ID, r.Status)
		}
		k := r.Family + "|" + r.ID
		if seen[k] {
			t.Fatalf("duplicate register row %s", k)
		}
		seen[k] = true
		if r.Source == "" || r.SourcePage == "" {
			t.Fatalf("%s %s: no source page", r.Family, r.ID)
		}
		switch r.Status {
		case "EXTRACTED FROM CATALOG TEXT — UNREVIEWED":
			read++
			if r.Contacts < 1 || r.Contacts > 200 {
				t.Fatalf("%s %s: implausible contact count %d", r.Family, r.ID, r.Contacts)
			}
		case "SEEN — CELLS NOT READ":
			unread++
			if r.Contacts != 0 || r.ContactSize != "" || r.Note == "" {
				t.Fatalf("%s %s: an unread arrangement must carry no invented values and must say why", r.Family, r.ID)
			}
		case "ID ONLY — UNREVIEWED":
			if r.Contacts != 0 {
				t.Fatalf("%s %s: id-only row has a count", r.Family, r.ID)
			}
		}
	}
	if read < 200 || unread > 10 {
		t.Fatalf("register mix changed: %d read, %d unread", read, unread)
	}
	// a mixed-size arrangement prints quantity x size per group; its contact count must be the sum of the groups
	mixed := 0
	for _, r := range reg {
		if r.Status != "EXTRACTED FROM CATALOG TEXT — UNREVIEWED" || !strings.Contains(r.ContactSize, "x") {
			continue
		}
		mixed++
		sum := 0
		for _, g := range strings.Split(r.ContactSize, ";") {
			q, _, ok := strings.Cut(g, "x")
			n, err := strconv.Atoi(q)
			if !ok || err != nil || n < 1 {
				t.Fatalf("%s %s: malformed size group %q", r.Family, r.ID, g)
			}
			sum += n
		}
		if sum != r.Contacts {
			t.Fatalf("%s %s: groups %q add to %d, not %d", r.Family, r.ID, r.ContactSize, sum, r.Contacts)
		}
	}
	if mixed < 50 {
		t.Fatalf("only %d mixed-size arrangements read", mixed)
	}
	// arrangements whose contact count is well known from the standard, read independently from the catalog text
	known := map[string][2]string{"16S-1": {"7", "16"}, "20-29": {"17", "16"}, "24-28": {"24", "16"}, "28-12": {"26", "16"}}
	for _, r := range reg {
		if w, ok := known[r.ID]; ok && strings.HasPrefix(r.Family, "MIL-DTL-5015") {
			if strconv.Itoa(r.Contacts) != w[0] || r.ContactSize != w[1] {
				t.Fatalf("%s reads %d x %s, want %s x %s", r.ID, r.Contacts, r.ContactSize, w[0], w[1])
			}
			delete(known, r.ID)
		}
	}
	if len(known) != 0 {
		t.Fatalf("well-known arrangements missing from the register: %v", known)
	}
	// Independent check: fiber-optic CF38999 inserts reuse the 38999 patterns, so wherever the Amphenol 38999 catalog also lists
	// the arrangement its contact count and size must agree with what was read from the fiber catalog.
	cov, _ := listInsertLayouts()
	byID := map[string]InsertLayoutInfo{}
	for _, l := range cov {
		byID[l.ID] = l
	}
	matched := 0
	for _, r := range reg {
		if !strings.HasPrefix(r.Family, "CF38999") || r.Status != "EXTRACTED FROM CATALOG TEXT — UNREVIEWED" {
			continue
		}
		id := strings.TrimLeft(r.ID, "0")
		if l, ok := byID[id]; ok {
			matched++
			if l.Contacts != r.Contacts || l.ContactSize != r.ContactSize {
				t.Fatalf("fiber %s reads %d x %s but the 38999 catalog lists %d x %s", r.ID, r.Contacts, r.ContactSize, l.Contacts, l.ContactSize)
			}
		}
	}
	if matched < 8 {
		t.Fatalf("only %d fiber arrangements could be cross-checked", matched)
	}
}
