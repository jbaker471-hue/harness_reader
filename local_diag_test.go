package main

// LOCAL STRUCTURE DIAGNOSTIC — never part of an automated run. Skipped unless
// TRIVEX_DIAG_DIR names a folder of drawings on this PC. It writes a report to
// TRIVEX_DIAG_OUT on the PC. The report holds ONLY counts, shape classes and
// relative positions: no text, designators, part numbers, file names or
// checksums. Drawings are numbered in an order that does not reveal names.
// The user reviews the report on the PC before anyone else reads it.

import (
	"regexp"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// shapeClass is a one-word class for a printed item, never its text.
func shapeClass(t string) string {
	t = strings.TrimSpace(t)
	u := strings.ToUpper(t)
	switch {
	case t == "":
		return "empty"
	case strings.EqualFold(t, "AWG"):
		return "AWG"
	case reGaugeLabel.MatchString(t):
		return "GAUGE+AWG"
	case reDigitPin.MatchString(t):
		return "num"
	case schematicColor(u) != "":
		return "COLOUR"
	case reRefDesignator.FindString(u) == u:
		return "DESIGNATOR"
	case reSplitPNHead.MatchString(u):
		return "PARTHEAD"
	case reSplitPNTail.MatchString(u):
		return "PARTTAIL"
	}
	return tokenShape(t)
}

func TestLocalStructureDiagnostic(t *testing.T) {
	dir, outPath := os.Getenv("TRIVEX_DIAG_DIR"), os.Getenv("TRIVEX_DIAG_OUT")
	if dir == "" || outPath == "" {
		t.Skip("local diagnostic: set TRIVEX_DIAG_DIR and TRIVEX_DIAG_OUT")
	}
	var files []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && p != dir {
			return filepath.SkipDir // this folder only
		}
		if err == nil && !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".pdf") {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	if len(files) > 25 {
		files = files[:25]
	}
	out := &strings.Builder{}
	pf := func(f string, v ...any) { fmt.Fprintf(out, f+"\n", v...) }
	pf("STRUCTURE REPORT: counts, shape classes and relative positions only. No drawing text.")
	pf("drawings found: %d", len(files))
	for di, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			pf("\n### drawing %d: could not be read", di+1)
			continue
		}
		x, err := ExtractDocument(b)
		if err != nil {
			pf("\n### drawing %d: text extraction failed", di+1)
			continue
		}
		v, err := viewFromExtraction(x)
		if err != nil {
			pf("\n### drawing %d: analysis failed", di+1)
			continue
		}
		pf("\n### drawing %d: %d sheets, engine %s", di+1, len(x.Data.Pages), x.Engine)
		withLen := 0
		for _, w := range v.Wires {
			if strings.TrimSpace(w.Length) != "" {
				withLen++
			}
		}
		knownPos := 0
		for _, c := range v.Connectors {
			if c.Cavities > 0 {
				knownPos++
			}
		}
		lab := 0
		for _, pg := range x.Data.Pages {
			for _, l := range gaugeLines(pg) {
				lab += gaugeLabelCount(l)
			}
		}
		pf("WIRES: %d gauge labels on the wiring sheets, %d wire rows read", lab, len(v.Wires))
		{
			co, tol, up := 0, 0, 0
			for _, h := range v.HarnessLengths {
				if h.Corroborated {
					co++
				}
				if h.Tolerance != "" {
					tol++
				}
				if h.Upper != "" {
					up++
				}
			}
			pf("LENGTHS: harness length candidates %d (corroborated %d); with a tolerance %d, cut at the top %d", len(v.HarnessLengths), co, tol, up)
			// each candidate: its label (fixed vocabulary), whether it has a unit
			// and a tolerance, and its size relative to the first candidate
			cin := func(h HarnessLength) float64 {
				c := lengthCandidatesInches([]HarnessLength{h})
				if len(c) == 0 {
					return 0
				}
				return c[0]
			}
			for k, h := range v.HarnessLengths {
				fs := strings.Fields(h.Text)
				unit := "none"
				if len(fs) > 1 {
					unit = fs[1]
				}
				rel := 0.0
				if c0 := cin(v.HarnessLengths[0]); c0 > 0 {
					rel = cin(h) / c0
				}
				pf("LENGTHCAND: %d label %s, sheet %d, unit %s, tolerance %v, cut-at-top %v, size vs first %.3f, confirmed %v", k+1, h.Label, h.Page, unit, h.Tolerance != "", h.Upper != "", rel, h.Corroborated)
			}
			// cables in the parts list and their callouts on the wiring sheets
			wp := map[int]bool{}
			for _, w := range v.Wires {
				wp[w.Page] = true
			}
			for k, b := range v.Boms {
				br := BOMRow{Item: b.Item, Qty: b.Qty, Description: b.Description, Unit: b.Unit}
				up := strings.ToUpper(b.Description + " " + b.Part)
				isCable, isWire := cableConductors(br) > 0, strings.Contains(up, "WIRE") || strings.Contains(up, "M22759") || strings.Contains(up, "M27500") || strings.Contains(up, "CABLE")
				if !isCable && !isWire {
					continue
				}
				calls, callsW, amb := 0, 0, false
				for _, c := range v.Callouts {
					if canonicalFind(c.Reference) != canonicalFind(b.Item) || b.Item == "" {
						continue
					}
					calls++
					if wp[c.Page] {
						callsW++
					}
					if c.Quantity == "AMBIGUOUS" {
						amb = true
					}
				}
				q, qok := plainQty(b.Qty)
				ratios := []string{}
				for _, h := range v.HarnessLengths {
					if L := cin(h); L > 0 && qok {
						qf := q
						if f := lengthUnitIn(b.Unit); f > 0 {
							qf = q * f / 12
						}
						ratios = append(ratios, fmt.Sprintf("%.2f", qf*12/L))
					}
				}
				u := b.Unit
				if u == "" {
					u = "none"
				}
				pf("LENGTHBOM: line %d cable %v (conductors %d) wire %v, unit %s, numeric qty %v, callouts %d (on wiring sheets %d, ambiguous %v), qty / candidate length [%s]", k+1, isCable, cableConductors(br), isWire, u, qok, calls, callsW, amb, strings.Join(ratios, " "))
			}
			// the user's stated value for drawing 1: 216 IN +1, cut at 217 IN
			if di == 0 {
				nom, cut := false, false
				for _, h := range v.HarnessLengths {
					if h.Text == "216 IN" || h.Text == "216" {
						nom = true
						cut = cut || h.Upper == "217 IN"
					}
				}
				pf("LENGTHCHECK: nominal matches the stated value %v, cut at the top of the tolerance matches %v", nom, cut)
			}
		}
		{
			fe, rg := 0, 0
			for _, w := range v.Wires {
				switch w.Method {
				case "traced-wire-to-ferrule":
					fe++
				case "traced-wire-to-ring":
					rg++
				}
			}
			pf("TERMS: %d ferrule ends, %d ring terminal ends", fe, rg)
		}
		{
			got, _ := readAnswers(v)
			keys := []string{}
			for k := range got {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for ci, k := range keys {
				a := got[k]
				per := map[string]int{}
				for _, w := range v.Wires {
					if strings.EqualFold(w.Connector, k) {
						per[w.Cavity]++
					}
					if strings.EqualFold(w.End, k) {
						per[w.EndCavity]++
					}
				}
				double := 0
				for _, n := range per {
					if n > 1 {
						double++
					}
				}
				pf("ASSIGN: connector %d: positions %d, wired %d, not used %d, pins with 2+ wires %d", ci+1, a.Positions, a.Wired, a.Unused, double)
				{
					// Where each connector's position count came from, and whether the parts list agrees. Counts and one-word classes only.
					class := "none"
					for _, m := range v.CavityModels {
						if strings.EqualFold(m.Tag, k) {
							src := m.AllSource
							switch {
							case strings.HasPrefix(src, "from the drawing: pins") && strings.Contains(src, "marks no pin NC"):
								class = "wired-pins-no-NC"
							case strings.HasPrefix(src, "from the drawing"):
								class = "wired-plus-NC"
							case strings.HasPrefix(src, "face view"):
								class = "face-view"
							case strings.HasPrefix(src, "catalog position count"):
								class = "stated-count"
							case m.AllVerified:
								class = "guide-inventory"
							}
						}
					}
					stated := map[int]bool{}
					part := ""
					for _, c := range v.Connectors {
						if strings.EqualFold(c.Name, k) {
							part = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(c.Part), " ", ""))
						}
					}
					if part != "" {
						for _, bm := range v.Boms {
							if strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(bm.Part), " ", "")) == part {
								if n, ok := positionsFromDescription(bm.Description); ok {
									stated[n] = true
								}
							}
						}
					}
					statedText := "none"
					for n := range stated {
						statedText = fmt.Sprintf("%d", n)
					}
					{
						// Is an NC list printed on the sheets where this connector's wires are, even though none was read for it? Counts only.
						wirePages := map[int]bool{}
						for _, w := range v.Wires {
							if strings.EqualFold(w.Connector, k) || strings.EqualFold(w.End, k) {
								wirePages[w.Page] = true
							}
						}
						ncw, lists := 0, 0
						for _, pg := range x.Data.Pages {
							if !wirePages[pg.Number] {
								continue
							}
							for _, w := range pg.Words {
								tx := strings.TrimSpace(w.Text)
								if u := strings.ToUpper(tx); u == "NC" || u == "N/C" || u == "N.C." || u == "(NC)" {
									ncw++
								}
								if expandPinList(tx) != nil {
									lists++
								}
							}
						}
						pf("NCSEEN: connector %d: sheets with its wires %d, NC words on them %d, pin-list words on them %d", ci+1, len(wirePages), ncw, lists)
					}
					pf("POSCHK: connector %d: positions %d, source %s, parts-list count %s", ci+1, a.Positions, class, statedText)
					if len(stated) == 1 && statedText != fmt.Sprintf("%d", a.Positions) && a.Positions > 0 {
						hi := 0
						for _, m := range v.CavityModels {
							if strings.EqualFold(m.Tag, k) {
								for _, id := range m.Wired {
									if n, err := strconv.Atoi(id); err == nil && n > hi {
										hi = n
									}
								}
							}
						}
						pf("POSCONF: connector %d: positions %d, parts-list count %s, wired pins %d, highest wired pin %d", ci+1, a.Positions, statedText, a.Wired, hi)
					}
				}
				if a.Positions == 0 {
					// Why no position count: counts and yes/no only, never a designator or a part number.
					checks, pass, nc := 0, 0, 0
					for _, ta := range v.TableAudits {
						if strings.EqualFold(ta.Title, "pin numbers "+k) {
							checks++
							if ta.Status == "COMPLETE" {
								pass++
							}
							nc += len(ta.NC)
						}
					}
					numeric, other, maxPin := map[int]bool{}, 0, 0
					unknownWhy := false
					for _, m := range v.CavityModels {
						if strings.EqualFold(m.Tag, k) {
							for _, id := range m.Wired {
								if n, err := strconv.Atoi(id); err == nil && n > 0 {
									numeric[n] = true
									if n > maxPin {
										maxPin = n
									}
								} else {
									other++
								}
							}
							unknownWhy = strings.HasPrefix(m.AllSource, "unknown")
						}
					}
					partKnown, famKnown := false, false
					partShape, famShape := "none", "none"
					for _, c := range v.Connectors {
						if strings.EqualFold(c.Name, k) {
							partKnown = strings.TrimSpace(c.Part) != "" && !strings.EqualFold(strings.TrimSpace(c.Part), "NOT RESOLVED")
							famKnown = c.Family != ""
							if partKnown {
								partShape = tokenShape(c.Part)
								// the decoder's own one-word shape vocabulary, never the part number
								famShape = DecodeConnectorFamily(c.Part).Shape
							}
						}
					}
					pinShapes := map[string]int{}
					for _, m := range v.CavityModels {
						if strings.EqualFold(m.Tag, k) {
							for _, id := range m.Wired {
								pinShapes[tokenShape(id)]++
							}
						}
					}
					shapeKeys := make([]string, 0, len(pinShapes))
					for s := range pinShapes {
						shapeKeys = append(shapeKeys, s)
					}
					sort.Strings(shapeKeys)
					shapeList := []string{}
					for _, s := range shapeKeys {
						shapeList = append(shapeList, fmt.Sprintf("%s=%d", s, pinShapes[s]))
					}
					yn := func(b bool) string {
						if b {
							return "yes"
						}
						return "no"
					}
					for _, m := range v.CavityModels {
						if strings.EqualFold(m.Tag, k) && m.AtLeast > 0 {
							pf("NOPOSL: connector %d: at least %d positions", ci+1, m.AtLeast)
						}
					}
					// Does the drawing state a position count anywhere for this connector? Numbers and a fixed unit word only.
					{
						reCount := regexp.MustCompile(`(?i)\b(\d{1,3})\s*-?\s*(POSITIONS?|POS|PINS?|WAYS?|WAY|CAVITIES|CAVITY|CKTS?|CIRCUITS?|CONTACTS?|PIN|P|C)\b`)
						rePerSize := regexp.MustCompile(`(?i)\b(\d{1,3})\s*#\s*\d{1,2}[A-Z]?\b`)
						unitWord := func(s string) string {
							s = strings.ToUpper(s)
							switch {
							case strings.HasPrefix(s, "POS"):
								return "POS"
							case strings.HasPrefix(s, "PIN"):
								return "PIN"
							case strings.HasPrefix(s, "WAY"):
								return "WAY"
							case strings.HasPrefix(s, "CAV"):
								return "CAV"
							case strings.HasPrefix(s, "CK"), strings.HasPrefix(s, "CIR"):
								return "CKT"
							case strings.HasPrefix(s, "CON"):
								return "CONTACT"
							}
							return "P"
						}
						part := ""
						for _, c := range v.Connectors {
							if strings.EqualFold(c.Name, k) {
								part = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(c.Part), " ", ""))
							}
						}
						found := map[string]bool{}
						if part != "" {
							for _, bm := range v.Boms {
								if strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(bm.Part), " ", "")) == part {
									for _, m := range reCount.FindAllStringSubmatch(bm.Description, -1) {
										found["parts-list description: "+m[1]+" "+unitWord(m[2])] = true
									}
									for _, m := range rePerSize.FindAllStringSubmatch(bm.Description, -1) {
										found["parts-list description: "+m[1]+" per-size"] = true
									}
								}
							}
						}
						// the same wording printed within reach of the connector's own name on a sheet
						for _, pg := range x.Data.Pages {
							var tagWords []Word
							for _, w := range pg.Words {
								if strings.EqualFold(strings.TrimSpace(w.Text), k) {
									tagWords = append(tagWords, w)
								}
							}
							for _, ln := range pageLines(pg) {
								for _, tw := range tagWords {
									if math.Abs(ln.Y-tw.Y) > 70 {
										continue
									}
									near := false
									for _, w := range ln.Words {
										if math.Abs(w.X-tw.X) < 260 {
											near = true
										}
									}
									if !near {
										continue
									}
									for _, m := range reCount.FindAllStringSubmatch(ln.Text, -1) {
										found["printed near the connector: "+m[1]+" "+unitWord(m[2])] = true
									}
									for _, m := range rePerSize.FindAllStringSubmatch(ln.Text, -1) {
										found["printed near the connector: "+m[1]+" per-size"] = true
									}
								}
							}
						}
						keys := make([]string, 0, len(found))
						for s := range found {
							keys = append(keys, s)
						}
						sort.Strings(keys)
						if len(keys) == 0 {
							pf("NOPOSC: connector %d: no position count stated", ci+1)
						}
						for _, s := range keys {
							pf("NOPOSC: connector %d: %s", ci+1, s)
						}
					}
					pf("NOPOSX: connector %d: part shape [%s], decoder shape [%s], wired pin shapes [%s]", ci+1, partShape, famShape, strings.Join(shapeList, "; "))
					pf("NOPOS: connector %d: pin-number checks %d, passing %d, NC pins %d, wired pins numeric %d, wired pins not numeric %d, highest wired pin %d, gaps below it %d, part number present %s, family recognised %s, gap-held %s", ci+1, checks, pass, nc, len(numeric), other, maxPin, maxPin-len(numeric), yn(partKnown), yn(famKnown), yn(unknownWhy))
				}
			}
			for _, is := range v.Issues {
				if is.Code != "CONNECTOR_PART_MISMATCH" {
					continue
				}
				class := "other"
				switch {
				case strings.Contains(is.Message, "Both have insert"):
					class = "same insert, different connector part"
				case strings.Contains(is.Message, "mating halves"):
					class = "mating halves"
				case strings.Contains(is.Message, "The inserts differ"):
					class = "different inserts"
				}
				other := "no"
				if strings.Contains(is.Message, "is also the parts-list part of") {
					other = "yes"
				} else if strings.Contains(is.Message, "designator was not read") {
					other = "yes, designator not read"
				}
				ci := 0
				for i, k := range keys {
					if strings.EqualFold(k, is.Connector) {
						ci = i + 1
					}
				}
				sheet := 0
				if len(is.Pages) > 0 {
					sheet = is.Pages[0]
				}
				pf("PARTMIS: connector %d on sheet %d: %s; header part belongs to another connector in the parts list: %s", ci, sheet, class, other)
				{
					// where the two part numbers differ, as positions after the slash and character classes only
					ex := normPart(strings.TrimPrefix(is.Expected, "Parts list: "))
					ob := normPart(strings.TrimPrefix(is.Observed, "Wiring sheet header: "))
					cut := func(s string) string {
						if i := strings.Index(s, "/"); i >= 0 {
							return s[i+1:]
						}
						return s
					}
					cls := func(b byte) string {
						switch {
						case b >= '0' && b <= '9':
							return "digit"
						case b >= 'A' && b <= 'Z':
							return "letter"
						}
						return "symbol"
					}
					e2, o2 := cut(ex), cut(ob)
					diffs := []string{}
					for i := 0; i < len(e2) && i < len(o2); i++ {
						if e2[i] != o2[i] {
							diffs = append(diffs, fmt.Sprintf("%d:%s/%s", i+1, cls(e2[i]), cls(o2[i])))
						}
					}
					pf("PARTDIFF: connector %d on sheet %d: lengths after the slash %d and %d, differing positions [%s]", ci, sheet, len(e2), len(o2), strings.Join(diffs, " "))
				}
			}
			{
				// per sheet and reader: how many wire rows, and how many of their ends carry a single-letter or numeric label
				type mk struct {
					page   int
					method string
				}
				cnt, lab := map[mk]int{}, map[mk]int{}
				for _, w := range v.Wires {
					k := mk{w.Page, w.Method}
					cnt[k]++
					if i := strings.LastIndex(w.EndSignal, ": "); i >= 0 {
						if _, ok := pinAtEnd(strings.TrimSpace(w.EndSignal[i+2:])); ok {
							lab[k]++
						}
					}
				}
				ks := make([]mk, 0, len(cnt))
				for k := range cnt {
					ks = append(ks, k)
				}
				sort.Slice(ks, func(i, j int) bool {
					if ks[i].page != ks[j].page {
						return ks[i].page < ks[j].page
					}
					return ks[i].method < ks[j].method
				})
				for _, k := range ks {
					pf("METHODS: sheet %d: %s rows %d, far-end label is pin-shaped %d", k.page, strings.ReplaceAll(k.method, "-", " "), cnt[k], lab[k])
				}
			}
			// every number printed with a length unit on the first three sheets: unit class, size class and what shares its line
			{
				reNU := regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(INCHES|INCH|IN\.?|"|FT\.?|FEET|'|MM|CM)$`)
				reNum := regexp.MustCompile(`^\d+(?:\.\d+)?$`)
				for _, pg := range x.Data.Pages {
					if pg.Number > 3 {
						continue
					}
					lineWords := map[float64]int{}
					for _, l := range pageLines(pg) {
						lineWords[l.Y] = len(strings.Fields(l.Text))
					}
					for _, s := range noteSegments(pg) {
						txt := strings.ToUpper(strings.Join(strings.Fields(s.text()), " "))
						toks := strings.Fields(txt)
						for i, tk := range toks {
							cand := tk
							if i+1 < len(toks) && reNum.MatchString(tk) {
								cand = tk + " " + toks[i+1]
							}
							m := reNU.FindStringSubmatch(cand)
							if m == nil {
								continue
							}
							v, _ := strconv.ParseFloat(m[1], 64)
							unit := "inch"
							switch {
							case strings.HasPrefix(m[2], "F"), m[2] == "'":
								unit = "foot"
							case m[2] == "MM", m[2] == "CM":
								unit = "metric"
							}
							size := "under 12"
							if v >= 12 {
								size = "12 or more"
							}
							n := 0
							for y, c := range lineWords {
								if math.Abs(y-s.y) < 3.6 {
									n = c
								}
							}
							pf("NUMUNIT: sheet %d: unit %s, value %s, words in its text group %d, words on its whole line %d, excluded word in group %v, height on sheet %.1f", pg.Number, unit, size, len(toks), n, reLengthExclude.MatchString(txt), s.y/pg.Height)
						}
					}
				}
			}
			// parts-list rows that are connector parts: which read connector (by index) their reference names, never the text
			for bi, bm := range v.Boms {
				if !rePN.MatchString(strings.ToUpper(bm.Part)) {
					continue
				}
				ref := strings.ToUpper(strings.TrimSpace(bm.Reference))
				who := "none"
				if ref != "" {
					who = "a designator that is not a read connector"
					for i, k := range keys {
						if strings.EqualFold(k, ref) {
							who = fmt.Sprintf("connector %d", i+1)
						}
					}
				}
				hdr := "no"
				for _, is := range v.Issues {
					if is.Code == "CONNECTOR_PART_MISMATCH" && normPart(strings.TrimPrefix(is.Observed, "Wiring sheet header: ")) == normPart(bm.Part) {
						hdr = "yes"
					}
				}
				own := "none"
				for i, k := range keys {
					for _, c := range v.Connectors {
						if strings.EqualFold(c.Name, k) && normPart(c.Part) == normPart(bm.Part) {
							own = fmt.Sprintf("connector %d", i+1)
						}
					}
				}
				pf("BOMCONN: parts-list row %d: reference names %s; part is held by %s; part is the mismatching header part: %s; reference words %d", bi+1, who, own, hdr, len(strings.Fields(ref)))
			}
			// every part-number header on the wiring sheets: how many designators sit near it, and where (em, relative)
			for _, pg := range x.Data.Pages {
				for _, h := range pg.Words {
					ht := strings.TrimSpace(strings.ToUpper(h.Text))
					if !reSplitPNHead.MatchString(ht) {
						continue
					}
					em := math.Max(h.Size, 4)
					near := []string{}
					for _, w := range pg.Words {
						tx := strings.ToUpper(strings.TrimSpace(w.Text))
						if reRefDesignator.FindString(tx) != tx || tx == "" {
							continue
						}
						dx, dy := (w.X-h.X)/em, (w.Y-h.Y)/em
						if math.Abs(dx) <= 30 && math.Abs(dy) <= 8 {
							near = append(near, fmt.Sprintf("%+.0f,%+.0f", dx, dy))
						}
					}
					sort.Strings(near)
					pf("HDRNEAR: sheet %d: header at %.2f,%.2f: designators within 30 em across and 8 em up or down: %d [%s]", pg.Number, h.X/pg.Width, h.Y/pg.Height, len(near), strings.Join(near, " "))
				}
			}
			done, total := 0, 0
			for _, t := range v.TableAudits {
				if t.Title == "wire gauge labels" {
					total++
					if t.Status == "COMPLETE" {
						done++
					}
				}
			}
			pf("ASSIGN: wiring sheets with every gauge label read %d of %d", done, total)
			pc, pa, pmiss, pdup := 0, 0, 0, 0
			for _, t := range v.TableAudits {
				if !strings.HasPrefix(t.Title, "pin numbers ") {
					continue
				}
				pc++
				if t.Status == "COMPLETE" {
					pa++
				}
				for _, m := range t.Missed {
					if strings.Contains(m.Reason, "no wire") {
						pmiss++
					} else {
						pdup++
					}
				}
			}
			pf("ASSIGN: pin-number checks %d, agreeing %d; printed pin numbers with no wire %d; pins with two or more wires %d", pc, pa, pmiss, pdup)
			idx := map[string]int{}
			for ci, k := range keys {
				idx[k] = ci + 1
			}
			for _, t := range v.TableAudits {
				if !strings.HasPrefix(t.Title, "pin numbers ") || t.Status == "COMPLETE" {
					continue
				}
				tag := strings.ToUpper(strings.TrimPrefix(t.Title, "pin numbers "))
				pf("PINCHECK: connector %d on sheet %d: %d pin numbers printed beside it, %d with a wire, %d problem(s)", idx[tag], t.Page, t.RuledRows, t.RowsRead, len(t.Missed))
				// where each problem number sits relative to the nearest NC word (em), structure only
				pg := x.Data.Pages[t.Page-1]
				var its []Word
				for _, l := range pageLines(pg) {
					its = append(its, schematicWords(l)...)
				}
				for _, m := range t.Missed {
					pin := m.Text[strings.LastIndex(m.Text, ":")+1:]
					for _, w := range its {
						if strings.TrimSpace(w.Text) != pin {
							continue
						}
						best, bdx, bdy := math.Inf(1), 0.0, 0.0
						for _, n := range its {
							u := strings.ToUpper(strings.TrimSpace(n.Text))
							if u != "NC" && u != "N/C" && u != "N.C." && u != "(NC)" {
								continue
							}
							em := math.Max(w.Size, 4)
							dx, dy := ((n.X+n.right())/2-(w.X+w.right())/2)/em, (n.Y-w.Y)/em
							if d := math.Hypot(dx, dy); d < best {
								best, bdx, bdy = d, dx, dy
							}
						}
						pf("PINCHECK: problem number: %s; nearest NC at dx %.1f em, dy %.1f em", m.Reason, bdx, bdy)
						{
							// what is drawn and printed at this pin: counts only
							em := math.Max(w.Size, 4)
							cx := (w.X + w.right()) / 2
							hNear, hLong, gauge, others := 0, 0, 0, 0
							for _, s := range pg.Segs {
								if math.Abs(s[1]-s[3]) > 0.3*em || math.Abs((s[1]+s[3])/2-w.Y) > 1.2*em {
									continue
								}
								x0, x1 := math.Min(s[0], s[2]), math.Max(s[0], s[2])
								if x1 < cx-8*em || x0 > cx+8*em {
									continue
								}
								hNear++
								if x1-x0 > 10*em {
									hLong++
								}
							}
							for _, n := range its {
								if math.Abs(n.Y-w.Y) > 1.0*em || (n.X == w.X && n.Y == w.Y) {
									continue
								}
								if reGaugeLabel.MatchString(n.Text) || strings.EqualFold(n.Text, "AWG") {
									gauge++
								} else {
									others++
								}
							}
							pf("PINDRAWN: horizontal lines at the pin %d (longer than 10 em: %d), gauge labels on its line %d, other items on its line %d, pin item shape [%s]", hNear, hLong, gauge, others, tokenShape(w.Text))
						}
					}
				}
			}
		}
		pf("RESULT: connectors %d (positions known %d) · wire rows %d (with a length %d) · parts-list rows %d · findings %d", len(v.Connectors), knownPos, len(v.Wires), withLen, len(v.Boms), len(v.Issues))
		methods := map[string]int{}
		for _, w := range v.Wires {
			methods[w.Method]++
		}
		pf("wire rows by reader: %v", methods)
		codes := map[string]int{}
		for _, i := range v.Issues {
			codes[i.Code]++
		}
		pf("finding codes: %v", codes)
		for _, f := range v.Issues {
			if f.Code != "CONNECTOR_PART_MISMATCH" {
				continue
			}
			cls := "other"
			switch {
			case strings.Contains(f.Message, "cavity layout is the same"):
				cls = "same insert, different part (e.g. shell, finish or key)"
			case strings.Contains(f.Message, "mating halves"):
				cls = "mating halves (pin vs socket)"
			case strings.Contains(f.Message, "inserts differ"):
				cls = "different inserts"
			}
			other := strings.Contains(f.Message, "also the parts-list part of") || strings.Contains(f.Message, "designator was not read")
			pf("MISMATCH: %s; header part belongs to another parts-list connector %v", cls, other)
		}
		for i, p := range x.Data.Pages {
			pl := x.Layouts[i]
			single, rot, multi := 0, 0, 0
			sizes := map[int]int{}
			for _, w := range p.Words {
				n := len([]rune(strings.TrimSpace(w.Text)))
				if n == 1 {
					single++
				}
				if len(strings.Fields(w.Text)) > 1 {
					multi++
				}
				if w.Rotated {
					rot++
				}
				sizes[int(math.Round(w.Size))]++
			}
			pct := func(n int) float64 {
				if len(p.Words) == 0 {
					return 0
				}
				return 100 * float64(n) / float64(len(p.Words))
			}
			pf("\n  sheet %d: %.0fx%.0f rot=%d accurate=%v · fragments %d (single-char %.0f%%, multi-word %.0f%%, rotated %.0f%%) · font sizes %v · horizontal rules %d", p.Number, p.Width, p.Height, pl.Rotation, p.PageAccurate, len(p.Words), pct(single), pct(multi), pct(rot), sizes, len(p.HRules))
			for _, d := range bomProbe(p) {
				pf("    bom probe: %s", d)
			}
			for _, d := range lengthProbe(p) {
				pf("    length probe: %s", d)
			}
			shapes := map[string]int{}
			for _, w := range p.Words {
				for _, f := range strings.Fields(w.Text) {
					shapes[shapeClass(f)]++
				}
			}
			pf("    item shapes on the sheet: %v", shapes)
			gl := gaugeLines(p)
			labels := 0
			for _, l := range gl {
				labels += gaugeLabelCount(l)
			}
			awgWords := 0
			for _, w := range p.Words {
				if strings.Contains(strings.ToUpper(w.Text), "AWG") {
					awgWords++
				}
			}
			pf("    fragments containing AWG: %d · lines with a gauge label: %d (labels %d)", awgWords, len(gl), labels)
			colour := pairedSchematicRows(p)
			pos, why := positionalSchematicRows(p, colour, nil)
			tw, twhy := tracedWireRows(p, append(append([]WireRow{}, colour...), pos...), nil)
			if labels > 0 && len(colour)+len(pos)+len(tw) < labels {
				pf("    name check: %s", nameMatchStats(x.Data.Pages, i))
				pf("    part check: %s", partMatchStats(x.Data, p))
				_, _, tb := traceSheet(p, nil, nil)
				for _, blk := range tb {
					if blk.tag != "" {
						continue
					}
					cx, top, _ := blockSpan(blk)
					if math.IsNaN(cx) {
						continue
					}
					type it struct {
						d float64
						s string
					}
					var near []it
					for _, v := range p.Words {
						em := 6.0
						if len(blk.pins) > 0 {
							em = math.Max(blk.pins[0].Size, 4)
						}
						dx, dy := (v.X-cx)/em, (v.Y-top)/em
						if dy < 0 || dy > 40 || math.Abs(dx) > 30 {
							continue
						}
						rot := ""
						if v.Rotated {
							rot = "(rotated)"
						}
						near = append(near, it{math.Hypot(dx, dy), fmt.Sprintf("%s%s@%.0f,%.0f", shapeClass(v.Text), rot, dx, dy)})
					}
					sort.Slice(near, func(a, b int) bool { return near[a].d < near[b].d })
					ps := []string{}
					for k := 0; k < len(near) && k < 8; k++ {
						ps = append(ps, near[k].s)
					}
					pf("    unnamed block at %.2f with %d ends: above it %s", cx/p.Width, len(blk.pins), strings.Join(ps, " "))
					// the three nearest designator-shaped items anywhere on the sheet
					var ds []it
					for _, v := range p.Words {
						u := strings.ToUpper(strings.TrimSpace(v.Text))
						if u == "" || (reRefDesignator.FindString(u) != u && !(reBlockName.MatchString(u) && schematicColor(u) == "" && !reSplitPNTail.MatchString(u))) {
							continue
						}
						em := 6.0
						if len(blk.pins) > 0 {
							em = math.Max(blk.pins[0].Size, 4)
						}
						kind := "DESIGNATOR"
						if reRefDesignator.FindString(u) != u {
							kind = "NAME"
						}
						rot := ""
						if v.Rotated {
							rot = "(rotated)"
						}
						dx, dy := (v.X-cx)/em, (v.Y-top)/em
						ds = append(ds, it{math.Hypot(dx, dy), fmt.Sprintf("%s%s@%.0f,%.0f", kind, rot, dx, dy)})
					}
					sort.Slice(ds, func(a, b int) bool { return ds[a].d < ds[b].d })
					dps := []string{}
					for k := 0; k < len(ds) && k < 3; k++ {
						dps = append(dps, ds[k].s)
					}
					pf("    unnamed block at %.2f nearest names: %s", cx/p.Width, strings.Join(dps, " "))
				}
				for _, d := range nameDebug(p) {
					pf("    names: %s", d)
				}
				for _, d := range endPatterns(p) {
					pf("    ends: %s", d)
				}
				for _, d := range farEndItems(p, 3) {
					pf("    far end: %s", d)
				}
				for _, d := range traceDebug(p, 3) {
					pf("    trace: %s", d)
				}
				for _, d := range heldEndShapes(p) {
					pf("    held end: %s", d)
				}
				for _, d := range bareEndShapes(p) {
					pf("    bare end: %s", d)
				}
				for _, d := range sameNameShapes(p) {
					pf("    same-name pair: %s", d)
				}
			}
			pf("    colour reader rows %d · column reader rows %d · traced rows %d · drawn segments %d · column reader says: %q · tracer says: %q", len(colour), len(pos), len(tw), len(p.Segs), why, twhy)
			for _, tb := range genericTables(p) {
				roles := []string{}
				for _, c := range tb.Cols {
					roles = append(roles, c.role)
				}
				pf("    table kind=%s rows=%d up=%v columns=%v", tb.Kind, len(tb.Rows), tb.Up, roles)
			}
			// the shape of a few wire lines, with relative positions (0 = left edge, 1 = right edge)
			shown := 0
			for _, l := range gl {
				if shown >= 4 {
					break
				}
				shown++
				parts := []string{}
				for _, w := range schematicWords(l) {
					parts = append(parts, fmt.Sprintf("%s@%.2f", shapeClass(w.Text), w.X/math.Max(p.Width, 1)))
				}
				pf("    wire line %d: %s", shown, strings.Join(parts, " "))
			}
			// if there are AWG fragments but no gauge lines, show how those fragments look
			if awgWords > 0 && len(gl) == 0 {
				n := 0
				for _, l := range pageLines(p) {
					if !strings.Contains(strings.ToUpper(l.Text), "AWG") || n >= 4 {
						continue
					}
					n++
					parts := []string{}
					for _, w := range l.Words {
						cls := []string{}
						for _, f := range strings.Fields(w.Text) {
							cls = append(cls, shapeClass(f))
						}
						parts = append(parts, "["+strings.Join(cls, " ")+"]")
					}
					pf("    line with AWG (fragments in brackets): %s", strings.Join(parts, " "))
				}
			}
		}
	}
	os.WriteFile(outPath, []byte(out.String()), 0o600)
}

// nameMatchStats tests, by counts only, whether a wire can be matched to its
// far end by a name printed on its line (signal name or wire number) that is
// printed again beside a pin elsewhere on the sheet or the drawing.
func nameMatchStats(pages []Page, pi int) string {
	p := pages[pi]
	edge := 0.03 * p.Width
	isName := func(w Word) bool {
		t := strings.TrimSpace(w.Text)
		u := strings.ToUpper(t)
		if len(t) < 2 || w.X < edge || w.X > p.Width-edge || strings.EqualFold(t, "AWG") || schematicColor(u) != "" || schematicGauge.MatchString(t) {
			return false
		}
		_, pin := pinAtEnd(t)
		return !pin
	}
	besidePin := func(pg Page, w Word) bool {
		for _, v := range pg.Words {
			if _, ok := pinAtEnd(strings.TrimSpace(v.Text)); ok && math.Abs(v.Y-w.Y) < 2 && math.Abs(v.X-w.X) < 6*math.Max(w.Size, 4) && v.X != w.X {
				return true
			}
		}
		return false
	}
	labels, withName, onceSheetPin, onceDrawingPin, many, none := 0, 0, 0, 0, 0, 0
	for _, l := range gaugeLines(p) {
		labels++
		var names []Word
		for _, w := range schematicWords(l) {
			if isName(w) {
				names = append(names, w)
			}
		}
		if len(names) == 0 {
			continue
		}
		withName++
		bestSheet, bestDrawing, anyMany := false, false, false
		for _, nm := range names {
			sheetHits, drawHits, sheetPin, drawPin := 0, 0, 0, 0
			for qi, pg := range pages {
				for _, w := range pg.Words {
					for _, f := range strings.Fields(w.Text) {
						if f != nm.Text || (qi == pi && math.Abs(w.Y-l.Y) < 2) {
							continue
						}
						ww := Word{Text: f, X: w.X, Y: w.Y, Size: w.Size}
						if qi == pi {
							sheetHits++
							if besidePin(pg, ww) {
								sheetPin++
							}
						}
						drawHits++
						if besidePin(pg, ww) {
							drawPin++
						}
					}
				}
			}
			if sheetHits == 1 && sheetPin == 1 {
				bestSheet = true
			}
			if drawHits == 1 && drawPin == 1 {
				bestDrawing = true
			}
			if drawHits > 1 {
				anyMany = true
			}
		}
		switch {
		case bestSheet:
			onceSheetPin++
		case bestDrawing:
			onceDrawingPin++
		case anyMany:
			many++
		default:
			none++
		}
	}
	return fmt.Sprintf("%d labels; %d with a name item on the line; name printed once more beside a pin on this sheet %d, on another sheet %d; printed several times %d; not printed elsewhere %d", labels, withName, onceSheetPin, onceDrawingPin, many, none)
}

// partMatchStats compares, by counts only, the part numbers printed on a sheet
// with the parts the drawing's connector headers name with a designator.
func partMatchStats(d PDFData, p Page) string {
	tags, _ := headerPartTags(d)
	known := map[string]bool{}
	for part := range tags {
		known[normPart(part)] = true
	}
	slashed, slashedKnown, heads, tails, joinedKnown, tailSuffixKnown := 0, 0, 0, 0, 0, 0
	for _, w := range p.Words {
		t := strings.ToUpper(strings.TrimSpace(w.Text))
		switch {
		case reSplitPNHead.MatchString(t):
			heads++
		case reSplitPNTail.MatchString(t):
			tails++
			for k := range known {
				if strings.HasSuffix(k, normPart(t)) {
					tailSuffixKnown++
					break
				}
			}
		case strings.Contains(t, "/") && strings.ContainsAny(t, "0123456789"):
			slashed++
			if known[normPart(t)] {
				slashedKnown++
			}
		}
	}
	for part := range headerPartNumbers(p) {
		_ = part
	}
	for _, v := range headerPartNumbers(p) {
		if known[normPart(v)] {
			joinedKnown++
		}
	}
	return fmt.Sprintf("parts with a designator on the drawing %d; on this sheet: one-piece part numbers %d (%d equal a known part), split heads %d, split tails %d (%d end a known part), joined head+tail under a designator equal to a known part %d", len(known), slashed, slashedKnown, heads, tails, tailSuffixKnown, joinedKnown)
}

// farEndItems describes, by shape and position only, what is printed around
// the far end of the first n traced wire lines: the 8 nearest items within
// 30 em, as shape@dx,dy in em (dx right of the end, dy above it).
func farEndItems(p Page, n int) []string {
	var out []string
	g := newSegGraph(p.Segs)
	var items []Word
	for _, l := range pageLines(p) {
		items = append(items, schematicWords(l)...)
	}
	for _, l := range pageLines(p) {
		if len(out) >= n {
			break
		}
		ws := schematicWords(l)
		for j, w := range ws {
			if !(j+1 < len(ws) && schematicGauge.MatchString(w.Text) && strings.EqualFold(ws[j+1].Text, "AWG")) || len(out) >= n {
				continue
			}
			em := lineEm(l)
			bi, bd := -1, math.Inf(1)
			for i, sg := range g.segs {
				x0, x1 := math.Min(sg[0], sg[2]), math.Max(sg[0], sg[2])
				dy := l.Y - (sg[1]+sg[3])/2
				if math.Abs(sg[1]-sg[3]) > 0.8 || w.X < x0-em || w.X > x1+em || dy < -0.6*em || dy > 1.6*em {
					continue
				}
				if d := math.Abs(dy - 0.3*em); d < bd {
					bi, bd = i, d
				}
			}
			if bi < 0 {
				continue
			}
			sg := g.segs[bi]
			ex, ey, _, kind := g.followInfo(bi, sg[2], sg[3])
			if sg[2] < sg[0] {
				ex, ey, _, kind = g.followInfo(bi, sg[0], sg[1])
			}
			type it struct {
				d float64
				s string
			}
			var near []it
			for _, v := range items {
				dx, dy := ((v.X+v.right())/2-ex)/em, (v.Y-ey)/em
				if d := math.Hypot(dx, dy); d <= 30 {
					rot := ""
					if v.Rotated {
						rot = "(rotated)"
					}
					sc := shapeClass(v.Text)
					if sc == "hyphenated code" {
						ps := []string{}
						for _, part := range strings.FieldsFunc(v.Text, func(r rune) bool { return r == '-' || r == '_' }) {
							ps = append(ps, shapeClass(part))
						}
						sc += "{" + strings.Join(ps, "+") + "}<" + abstractPattern(v.Text) + ">"
					}
					near = append(near, it{d, fmt.Sprintf("%s%s@%.0f,%.0f", sc, rot, dx, dy)})
				}
			}
			for _, v := range p.Words { // rotated text is not in the lines
				if !v.Rotated {
					continue
				}
				dx, dy := (v.X-ex)/em, (v.Y-ey)/em
				if d := math.Hypot(dx, dy); d <= 30 {
					near = append(near, it{d, fmt.Sprintf("%s(rotated)@%.0f,%.0f", shapeClass(v.Text), dx, dy)})
				}
			}
			sort.Slice(near, func(a, b int) bool { return near[a].d < near[b].d })
			parts := []string{}
			for k := 0; k < len(near) && k < 8; k++ {
				parts = append(parts, near[k].s)
			}
			out = append(out, fmt.Sprintf("end@%.2f,%.2f (%s): %s", ex/p.Width, ey/p.Height, kind, strings.Join(parts, " ")))
		}
	}
	return out
}

// abstractPattern reduces text to its form: each run of letters becomes A,
// each run of digits 9, other characters are kept. No letters or digits of
// the original survive.
func abstractPattern(t string) string {
	var b strings.Builder
	last := byte(0)
	for _, r := range strings.TrimSpace(t) {
		var c byte
		switch {
		case r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z':
			c = 'A'
		case r >= '0' && r <= '9':
			c = '9'
		case r == ' ':
			c = '_'
		case r < 128:
			c = byte(r)
		default:
			c = '?'
		}
		if (c == 'A' || c == '9') && c == last {
			continue
		}
		b.WriteByte(c)
		last = c
	}
	return b.String()
}

// endPatterns: for every traced wire line, the abstract pattern of what is
// printed at the wire's own height near each end (within 25 em outward and 3
// em inward), as pattern@dx (em, outward positive). No text survives.
func endPatterns(p Page) []string {
	var out []string
	g := newSegGraph(p.Segs)
	var items []Word
	for _, l := range pageLines(p) {
		items = append(items, schematicWords(l)...)
	}
	for _, l := range pageLines(p) {
		ws := schematicWords(l)
		for j, w := range ws {
			if !(j+1 < len(ws) && schematicGauge.MatchString(w.Text) && strings.EqualFold(ws[j+1].Text, "AWG")) {
				continue
			}
			em := lineEm(l)
			bi, bd := -1, math.Inf(1)
			for i, sg := range g.segs {
				x0, x1 := math.Min(sg[0], sg[2]), math.Max(sg[0], sg[2])
				dy := l.Y - (sg[1]+sg[3])/2
				if math.Abs(sg[1]-sg[3]) > 0.8 || w.X < x0-em || w.X > x1+em || dy < -0.6*em || dy > 1.6*em {
					continue
				}
				if d := math.Abs(dy - 0.3*em); d < bd {
					bi, bd = i, d
				}
			}
			if bi < 0 {
				out = append(out, "no line")
				continue
			}
			sg := g.segs[bi]
			ax, ay, _, _ := g.followInfo(bi, sg[0], sg[1])
			bx, by, _, _ := g.followInfo(bi, sg[2], sg[3])
			if ax > bx {
				ax, ay, bx, by = bx, by, ax, ay
			}
			side := func(x, y, dir float64) string {
				type it struct {
					d float64
					s string
				}
				var near []it
				for _, v := range items {
					if math.Abs(v.Y-y) > 1.0*em {
						continue
					}
					d := ((v.X+v.right())/2 - x) * dir / em
					if d < -3 || d > 25 {
						continue
					}
					near = append(near, it{d, fmt.Sprintf("%s@%.0f", abstractPattern(v.Text), d)})
				}
				sort.Slice(near, func(a, b int) bool { return near[a].d < near[b].d })
				ps := []string{}
				for k := 0; k < len(near) && k < 5; k++ {
					ps = append(ps, near[k].s)
				}
				return strings.Join(ps, " ")
			}
			out = append(out, fmt.Sprintf("left end@%.2f [%s] right end@%.2f [%s]", ax/p.Width, side(ax, ay, -1), bx/p.Width, side(bx, by, 1)))
		}
	}
	return out
}

// heldEndShapes summarises, as counts only, the strokes drawn at each wire
// end the tracer held: short horizontal, vertical and slanted strokes within
// 6 em along and 3 em across, their extent in em, and where it sits.
func heldEndShapes(p Page) []string {
	cnt := map[string]int{}
	traceHeldHook = func(reason string, x, y, dir, em float64, segs []Segment) {
		h, v, sl, long, longH, longV := 0, 0, 0, 0, 0, 0
		bx0, by0, bx1, by1 := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
		for _, s := range segs {
			if math.Abs((s[0]+s[2])/2-x) > 6*em || math.Abs((s[1]+s[3])/2-y) > 3*em {
				continue
			}
			lw, lh := math.Abs(s[2]-s[0]), math.Abs(s[3]-s[1])
			if math.Hypot(lw, lh) > 2*em {
				long++
				switch {
				case lh <= 0.15*em:
					longH++
				case lw <= 0.15*em:
					longV++
				}
				continue
			}
			switch {
			case lh <= 0.15*em:
				h++
			case lw <= 0.15*em:
				v++
			default:
				sl++
			}
			bx0, bx1 = math.Min(bx0, math.Min(s[0], s[2])), math.Max(bx1, math.Max(s[0], s[2]))
			by0, by1 = math.Min(by0, math.Min(s[1], s[3])), math.Max(by1, math.Max(s[1], s[3]))
		}
		ext := "none"
		if h+v+sl > 0 {
			ext = fmt.Sprintf("%.0fx%.0f em, centre %+.0f em outward", (bx1-bx0)/em, (by1-by0)/em, ((bx0+bx1)/2-x)*dir/em)
		}
		near := "none"
		bd := math.Inf(1)
		for _, w := range p.Words {
			if _, ok := pinAtEnd(w.Text); !ok || w.Rotated {
				continue
			}
			dx, dy := ((w.X+w.right())/2-x)*dir/em, (w.Y-y)/em
			if math.Abs(dx) > 12 || math.Abs(dy) > 4 {
				continue
			}
			if d := math.Abs(dx) + 2*math.Abs(dy); d < bd {
				bd, near = d, fmt.Sprintf("dx %+.0f dy %+.0f", dx, dy)
			}
		}
		// the nearest number on the same line, either way, up to 60 em
		along := "none"
		ad := math.Inf(1)
		for _, w := range p.Words {
			if _, ok := pinAtEnd(w.Text); !ok || w.Rotated || math.Abs(w.Y-y) > 1.2*em {
				continue
			}
			dx := ((w.X+w.right())/2 - x) * dir / em
			if math.Abs(dx) <= 60 && math.Abs(dx) < ad {
				ad, along = math.Abs(dx), fmt.Sprintf("%+.0f", dx)
			}
		}
		cnt[fmt.Sprintf("%s: short h%d v%d slant%d, long %d (h%d v%d), extent %s, nearest number %s, on the line %s", reason, h, v, sl, long, longH, longV, ext, near, along)]++
	}
	defer func() { traceHeldHook = nil }()
	traceSheet(p, nil, nil)
	out := []string{}
	for k, n := range cnt {
		out = append(out, fmt.Sprintf("%dx %s", n, k))
	}
	sort.Strings(out)
	return out
}

// bareEndShapes says, for each kept one-ended wire, where the nearest pin-shaped item is from its unnumbered end
// (em across, outward positive, and em up), and how many drawn strokes touch the end. Counts and offsets only.
func bareEndShapes(p Page) []string {
	cnt := map[string]int{}
	traceBareHook = func(x, y, dir, em float64) {
		near, bd := "none within 40 em", math.Inf(1)
		for _, w := range p.Words {
			if _, ok := pinAtEnd(w.Text); !ok || w.Rotated {
				continue
			}
			dx, dy := ((w.X+w.right())/2-x)*dir/em, (w.Y-y)/em
			if math.Abs(dx) > 40 || math.Abs(dy) > 6 {
				continue
			}
			if d := math.Abs(dx) + 3*math.Abs(dy); d < bd {
				bd, near = d, fmt.Sprintf("dx %+.0f dy %+.0f, %s", dx, dy, tokenShape(w.Text))
			}
		}
		touch := 0
		for _, s := range p.Segs {
			if math.Hypot(s[0]-x, s[1]-y) <= 0.5*em || math.Hypot(s[2]-x, s[3]-y) <= 0.5*em {
				touch++
			}
		}
		// further along the same height, outward: the nearest pin-shaped item up to 400 em, and how much of the gap is drawn
		far, fd := "none", math.Inf(1)
		for _, w := range p.Words {
			if _, ok := pinAtEnd(w.Text); !ok || w.Rotated || math.Abs(w.Y-y) > 1.2*em {
				continue
			}
			dx := ((w.X+w.right())/2 - x) * dir
			if dx > 0 && dx < 400*em && dx < fd {
				fd = dx
			}
		}
		if !math.IsInf(fd, 1) {
			covered := 0.0
			for _, s := range p.Segs {
				if math.Abs(s[1]-s[3]) > 0.2*em || math.Abs((s[1]+s[3])/2-y) > 0.6*em {
					continue
				}
				a0, a1 := (math.Min(s[0], s[2])-x)*dir, (math.Max(s[0], s[2])-x)*dir
				if a0 > a1 {
					a0, a1 = a1, a0
				}
				a0, a1 = math.Max(a0, 0), math.Min(a1, fd)
				if a1 > a0 {
					covered += a1 - a0
				}
			}
			far = fmt.Sprintf("%.0f em outward, %.0f%% of the gap drawn at that height", math.Round(fd/em/10)*10, math.Round(100*math.Min(covered/fd, 1)/10)*10)
		}
		vert := 0
		for _, s := range p.Segs {
			if (math.Hypot(s[0]-x, s[1]-y) <= 0.5*em || math.Hypot(s[2]-x, s[3]-y) <= 0.5*em) && math.Abs(s[0]-s[2]) <= 0.2*em {
				vert++
			}
		}
		cnt[fmt.Sprintf("nearest pin-shaped item %s; strokes touching the end %d (vertical %d); same height further out: %s", near, touch, vert, far)]++
	}
	defer func() { traceBareHook = nil }()
	traceSheet(p, nil, nil)
	out := []string{}
	for k, n := range cnt {
		out = append(out, fmt.Sprintf("%dx %s", n, k))
	}
	sort.Strings(out)
	return out
}

// sameNameShapes classes, as counts only, the wires whose two ends landed on blocks given the same connector
// name: whether the ends share a column (one block wired to itself) or sit in different columns, and how far
// apart they are in em.
func sameNameShapes(p Page) []string {
	cnt := map[string]int{}
	traceSameHook = func(ax, ay, bx, by, em float64) {
		col := "different columns"
		if math.Abs(ax-bx) <= 3 {
			col = "same column"
		}
		cnt[fmt.Sprintf("%s, %.0f em apart across, %.0f em apart down", col, math.Round(math.Abs(ax-bx)/em/10)*10, math.Round(math.Abs(ay-by)/em/5)*5)]++
	}
	defer func() { traceSameHook = nil }()
	traceSheet(p, nil, nil)
	out := []string{}
	for k, n := range cnt {
		out = append(out, fmt.Sprintf("%dx %s", n, k))
	}
	sort.Strings(out)
	return out
}

// bomProbe reports, as role names and counts only, the parts-list evidence on
// a sheet: heading words by role (flat and rotated), parts-list title words,
// heading words grouped by printed line, and what each BOM reader returned.
func bomProbe(p Page) []string {
	out := []string{}
	flat, rot := map[string]int{}, map[string]int{}
	title, titleRot := 0, 0
	for _, w := range p.Words {
		if reBOMWord.MatchString(w.Text) {
			if w.Rotated {
				titleRot++
			} else {
				title++
			}
		}
		r := headerRole(w.Text)
		if r == "" {
			continue
		}
		if w.Rotated {
			rot[r]++
		} else {
			flat[r]++
		}
	}
	roles := func(m map[string]int) string {
		ks := []string{}
		for k, n := range m {
			ks = append(ks, fmt.Sprintf("%s=%d", k, n))
		}
		sort.Strings(ks)
		return strings.Join(ks, ",")
	}
	out = append(out, fmt.Sprintf("title words %d (rotated %d); heading words flat [%s] rotated [%s]", title, titleRot, roles(flat), roles(rot)))
	// printed lines holding two or more parts-list heading roles
	for _, l := range pageLines(p) {
		rs := map[string]bool{}
		for i := range l.Words {
			for step := 1; step <= 4 && i+step <= len(l.Words); step++ {
				ss := []string{}
				for _, x := range l.Words[i : i+step] {
					ss = append(ss, x.Text)
				}
				switch r := headerRole(strings.Join(ss, " ")); r {
				case "item", "qty", "part", "partref", "desc", "cage", "vendor", "um", "note":
					rs[r] = true
				}
			}
		}
		if len(rs) >= 2 {
			ks := []string{}
			for k := range rs {
				ks = append(ks, k)
			}
			sort.Strings(ks)
			// each word of the heading line in order: its role, or its abstract shape
			seq := []string{}
			for _, w := range l.Words {
				// cells below this heading word, within its span: how many look
				// like part numbers (letters and digits, 6+ characters)
				pn, cells := 0, 0
				for _, v := range p.Words {
					if v.Rotated || v.Y >= l.Y || v.Y < l.Y-60*math.Max(w.Size, 4) || v.right() < w.X-2*w.Size || v.X > w.right()+2*w.Size {
						continue
					}
					cells++
					t := strings.TrimSpace(v.Text)
					if len(t) >= 6 && strings.ContainsAny(t, "0123456789") && strings.ContainsAny(strings.ToUpper(t), "ABCDEFGHIJKLMNOPQRSTUVWXYZ") {
						pn++
					}
				}
				runs := []string{}
				n := 0
				for _, r := range w.Text {
					if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
						n++
						continue
					}
					if n > 0 {
						runs = append(runs, fmt.Sprint(n))
						n = 0
					}
					runs = append(runs, "/")
				}
				if n > 0 {
					runs = append(runs, fmt.Sprint(n))
				}
				shape := strings.Join(runs, "")
				if r := headerRole(w.Text); r != "" {
					shape = r
				}
				seq = append(seq, fmt.Sprintf("%s(cells %d, pn-like %d)", shape, cells, pn))
			}
			out = append(out, fmt.Sprintf("heading line at %.2f,%.2f words %d roles [%s] sequence [%s]", l.Words[0].X/p.Width, l.Y/p.Height, len(l.Words), strings.Join(ks, ","), strings.Join(seq, " | ")))
		}
	}
	// every heading the table reader tries: its columns' roles, how the
	// columns split, each part's kind and the rows read below and above
	if p.PageAccurate {
		lines := pageLines(p)
		for i := range lines {
			cols, band, ok := buildHeader(lines, i)
			if !ok {
				continue
			}
			rs := []string{}
			for _, c := range cols {
				r := c.role
				if r == "" {
					r = "?"
				}
				rs = append(rs, r)
			}
			hasPart := false
			for _, c := range cols {
				if c.role == "part" {
					hasPart = true
				}
			}
			if !hasPart {
				continue
			}
			bandSet := map[int]bool{}
			minI, maxI := len(lines), -1
			for _, j := range band {
				bandSet[j] = true
				if j < minI {
					minI = j
				}
				if j > maxI {
					maxI = j
				}
			}
			em := lineEm(lines[i])
			segs := []string{}
			for _, seg := range splitColumns(cols, em) {
				kind := tableKind(seg)
				d, u := 0, 0
				if kind != "" {
					dr, _ := readRows(lines, maxI+1, 1, bandSet, kind, seg, em, lines[maxI].Y)
					ur, _ := readRows(lines, minI-1, -1, bandSet, kind, seg, em, lines[minI].Y)
					d, u = len(dr), len(ur)
				}
				sr := []string{}
				for _, c := range seg {
					r := c.role
					if r == "" {
						r = "?"
					}
					sr = append(sr, r)
				}
				segs = append(segs, fmt.Sprintf("{%s kind=%s down=%d up=%d}", strings.Join(sr, " "), kind, d, u))
			}
			out = append(out, fmt.Sprintf("table try at %.2f band %d cols [%s] parts %s", lines[i].Y/p.Height, len(band), strings.Join(rs, " "), strings.Join(segs, " ")))
			// the lines just above and below the heading: how many words, how
			// many are heading words, and the shapes of the others
			for _, j := range []int{i - 3, i - 2, i - 1, i + 1, i + 2, i + 3} {
				if j < 0 || j >= len(lines) {
					continue
				}
				hw, other := 0, []string{}
				for _, w := range lines[j].Words {
					if headerish(w.Text) {
						hw++
					} else {
						other = append(other, abstractPattern(w.Text))
					}
				}
				if len(other) > 6 {
					other = append(other[:6], "...")
				}
				out = append(out, fmt.Sprintf("  near heading: line %+d at %+.1f em: words %d heading %d others [%s]", j-i, (lines[j].Y-lines[i].Y)/em, len(lines[j].Words), hw, strings.Join(other, " ")))
			}
		}
	}
	for _, t := range genericTables(p) {
		if t.Kind != "bom" {
			continue
		}
		cs := []string{}
		for _, c := range t.Cols {
			r := c.role
			if r == "" {
				r = "?"
			}
			cs = append(cs, r+"("+abstractPattern(c.label)+")")
		}
		out = append(out, "bom columns: "+strings.Join(cs, " "))
		for k, r := range t.Rows {
			up := strings.ToUpper(r["desc"] + " " + r["part"])
			if !strings.Contains(up, "WIRE") && !strings.Contains(up, "CABLE") && !strings.Contains(up, "M22759") && !strings.Contains(up, "M27500") {
				continue
			}
			out = append(out, fmt.Sprintf("bom wire row %d: qty %s, um %s, item %s", k+1, abstractPattern(r["qty"]), abstractPattern(r["um"]), abstractPattern(r["item"])))
		}
	}
	g := 0
	for _, t := range genericTables(p) {
		if t.Kind == "bom" {
			g += len(t.Rows)
		}
	}
	out = append(out, fmt.Sprintf("readers: fixed %d, flexible %d, text %d, table %d", len(bomRows(p)), len(flexibleBOMRows(p)), len(findTextBOMRows(p)), g))
	return out
}

// lengthProbe counts, per sheet, the things a wire length could come from:
// lines with a length label word, numbers carrying a length unit, bare
// decimal numbers, and long horizontal/vertical strokes with short ticks at
// both ends (dimension lines). Counts only.
func lengthProbe(p Page) []string {
	label, unit, dec := 0, 0, 0
	reUnit := regexp.MustCompile(`^\d+(\.\d+)?\s*(IN|INCH|INCHES|"|FT|'|MM|CM)$`)
	reDec := regexp.MustCompile(`^\d+\.\d+$`)
	for _, l := range pageLines(p) {
		if reLengthLabel.MatchString(strings.ToUpper(l.Text)) {
			label++
		}
	}
	for _, w := range p.Words {
		t := strings.ToUpper(strings.TrimSpace(w.Text))
		switch {
		case reUnit.MatchString(t):
			unit++
		case reDec.MatchString(t):
			dec++
		}
	}
	return []string{fmt.Sprintf("length-label lines %d, numbers with a unit %d, bare decimals %d", label, unit, dec)}
}
