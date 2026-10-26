package main

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// Recognition probe: a CONTENT-FREE description of how each sheet is laid out,
// so a recognition failure on a controlled drawing can be diagnosed without
// any drawing content leaving the PC. Only words from a fixed public heading
// vocabulary are printed; every other word becomes "▢". Part numbers are
// reduced to counts per public specification family. No values, signal names,
// designator numbers, file names or checksums are included.

var probeVocab = func() map[string]bool {
	m := map[string]bool{}
	for k := range headerVocab {
		for _, t := range strings.Fields(k) {
			m[t] = true
		}
	}
	for _, t := range strings.Fields("OR OF AND THE A B C NO NUMBER NAME DES REQD CODE IDENTIFYING LIST PARTS PART TABLE CHART SCHEDULE WIRING DIAGRAM WIRE HARNESS ASSEMBLY ASSY DETAIL VIEW SECTION FACE REAR FRONT MATING INSERT ARRANGEMENT PIN PINOUT PIN-OUT CONTACT CONNECTOR CONNECTORS PLUG RECEPTACLE JACK SOCKET END NOTES NOTE SHEET OF REV REVISION SCALE SIZE DWG DRAWING TITLE DATE APPROVED CHECKED DRAWN ENGINEER CONTRACT ZONE LTR DESCRIPTION SEE BACKSHELL SHIELD SHIELDED TWISTED PAIR GROUND GND SPLICE TERMINAL LUG BRANCH BREAKOUT LENGTH DIM REF TYP MAX MIN INCHES MM TOLERANCES UNLESS OTHERWISE SPECIFIED MARKER LABEL SLEEVE BOOT CABLE CABLES BUNDLE FROM TO SIGNAL CIRCUIT CKT COLOR GAGE GAUGE AWG TYPE QTY ITEM FIND CAGE NOMENCLATURE MATERIAL SPECIFICATION UNIT UM PER DASH -1 -2 -3") {
		m[t] = true
	}
	return m
}()

var (
	reProbeDesignator = regexp.MustCompile(`^(?:J|P|X|TB|CN|CON|PL|SK|W|E|S|SP)[-_]?[0-9]{1,4}[A-Z]?$`)
	reProbeNumber     = regexp.MustCompile(`^[0-9]{1,4}[A-Z]?$`)
	probeFamilies     = []struct{ name, re string }{
		{"D38999 connector", `D38999/(?:2[0-9]|1[0-9])`},
		{"D38999 cap/accessory", `D38999/3[0-9]`},
		{"M39029 contact", `M39029/`},
		{"MS27488 sealing plug", `MS27488`},
		{"M85049 backshell", `M85049|AS85049`},
		{"M22759/M27500 wire/cable", `M22759|M27500`},
		{"M23053/M81044 sleeving/wire", `M23053|M81044`},
		{"MS/NAS hardware", `\b(?:MS|NAS|AN)[0-9]{3,5}`},
		{"other dashed/slashed part-like", `\b[A-Z0-9]{2,}[-/][A-Z0-9-/]{2,}\b`},
	}
)

func probeToken(tok string) string {
	u := strings.ToUpper(strings.Trim(tok, ".,:;()[]"))
	if u == "" {
		return ""
	}
	if probeVocab[u] {
		return u
	}
	switch {
	case reProbeDesignator.MatchString(u):
		return "‹desig›"
	case reProbeNumber.MatchString(u):
		return "‹n›"
	}
	return "▢"
}

func probeText(s string) string {
	out := []string{}
	for _, t := range strings.Fields(s) {
		if p := probeToken(t); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " ")
}

// alignedRun counts consecutive lines below/above line i whose fragments line
// up with at least 3 of line i's left edges (a table-shaped block).
func alignedRun(lines []evidenceLine, i, step int) int {
	edges := []float64{}
	for _, w := range lines[i].Words {
		edges = append(edges, w.X)
	}
	n := 0
	for j := i + step; j >= 0 && j < len(lines) && n < 200; j += step {
		hits := 0
		for _, w := range lines[j].Words {
			for _, e := range edges {
				if math.Abs(w.X-e) < 6 {
					hits++
					break
				}
			}
		}
		if hits < 2 {
			break
		}
		n++
	}
	return n
}

func RecognitionProbe(x *DocExtraction) string {
	var b strings.Builder
	fmt.Fprintf(&b, "TRIVEX RECOGNITION PROBE v%s — structure only, no drawing content\n", AppVersion)
	fmt.Fprintf(&b, "Legend: UPPERCASE = generic heading word, ▢ = any other word (masked), ‹n› = short number, ‹desig› = designator-like (J/P/X…), counts only otherwise.\n")
	a, _ := AnalyzeData(x.Data)
	fmt.Fprintf(&b, "Engine: %s · sheets %d · records connectors/conductors/BOM %d/%d/%d\n", map[bool]string{true: "LEGACY", false: "PDFium"}[x.Legacy], len(x.Data.Pages), len(a.Connectors), len(a.Wires), len(a.BOM))
	for pi, p := range x.Data.Pages {
		pl := PageLayout{}
		if pi < len(x.Layouts) {
			pl = x.Layouts[pi]
		}
		lines := pageLines(p)
		fmt.Fprintf(&b, "\n== Sheet %d: %d text fragments in %d lines; rotation %d; frame %d; pictures %d; vector objects %d; segments %d; circles %d\n", p.Number, len(p.Words), len(lines), pl.Rotation, analysisFrame(pl), len(pl.Images), pl.PathObjects, pl.PathSegments, len(pl.Circles))
		desig, nums, rotated := 0, 0, 0
		fam := map[string]int{}
		for _, w := range p.Words {
			if w.Rotated {
				rotated++
			}
			for _, t := range strings.Fields(strings.ToUpper(w.Text)) {
				t = strings.Trim(t, ".,:;()[]")
				if reProbeDesignator.MatchString(t) {
					desig++
				} else if reProbeNumber.MatchString(t) {
					nums++
				}
			}
			u := strings.ToUpper(w.Text)
			for _, f := range probeFamilies {
				if regexp.MustCompile(f.re).MatchString(u) {
					fam[f.name]++
					break
				}
			}
		}
		fmt.Fprintf(&b, "   designator-like tokens %d · short numbers %d · fragments not upright %d\n", desig, nums, rotated)
		if len(fam) > 0 {
			keys := []string{}
			for k := range fam {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			parts := []string{}
			for _, k := range keys {
				parts = append(parts, fmt.Sprintf("%s ×%d", k, fam[k]))
			}
			fmt.Fprintf(&b, "   part-number families: %s\n", strings.Join(parts, "; "))
		}
		for _, t := range genericTables(p) {
			fmt.Fprintf(&b, "   RECOGNIZED: %s\n", tableSummary(t))
		}
		// Heading-like lines and table-shaped blocks, masked.
		shown := 0
		for i, l := range lines {
			if shown >= 25 {
				fmt.Fprintf(&b, "   … more heading-like lines omitted\n")
				break
			}
			hw := 0
			for _, w := range l.Words {
				if headerish(w.Text) {
					hw++
				}
			}
			below, above := alignedRun(lines, i, 1), alignedRun(lines, i, -1)
			if hw >= 1 && (below >= 2 || above >= 2) || hw >= 2 {
				cells := []string{}
				for _, w := range l.Words {
					cells = append(cells, probeText(w.Text))
				}
				fmt.Fprintf(&b, "   heading-like line (%d cells, %d aligned rows below, %d above, y %.0f%% from top): | %s |\n", len(l.Words), below, above, 100*(1-l.Y/math.Max(p.Height, 1)), strings.Join(cells, " | "))
				shown++
			}
		}
		// Largest table-shaped blocks even without known headings.
		type blk struct{ i, n, cols int }
		var blocks []blk
		for i := range lines {
			if len(lines[i].Words) >= 3 {
				if n := alignedRun(lines, i, 1); n >= 4 {
					blocks = append(blocks, blk{i, n, len(lines[i].Words)})
				}
			}
		}
		sort.Slice(blocks, func(a, c int) bool { return blocks[a].n > blocks[c].n })
		for k, bl := range blocks {
			if k >= 4 {
				break
			}
			cells := []string{}
			for _, w := range lines[bl.i].Words {
				cells = append(cells, probeText(w.Text))
			}
			fmt.Fprintf(&b, "   table-shaped block: %d aligned rows × %d cols, first line: | %s |\n", bl.n+1, bl.cols, strings.Join(cells, " | "))
		}
	}
	return b.String()
}
