package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Developer tool, skipped unless REGISTER_DIR names the local reference library. It reads the
// arrangement text (id, service rating, contact count and size) from the retrieved catalogs by word
// position and writes assets/CONNECTOR_ARRANGEMENT_REGISTER.csv. It reads no coordinates.
//
//	REGISTER_DIR=C:\...\reference_library REGISTER_OUT=assets\CONNECTOR_ARRANGEMENT_REGISTER.csv go test -run TestBuildArrangementRegister .
//
// An arrangement is written only when its cells read cleanly (a rating, a whole-number count and known
// contact sizes directly beneath its id); anything else is listed as skipped, never guessed.

type regRow struct {
	Family, ID, Shell, Contacts, Sizes, Rating, Source, Page, Status, Note string
}

type regSkip struct{ Source, Page, ID, Why, Family string }

var (
	reRegID     = regexp.MustCompile(`^\d{1,2}[A-Z]{0,2}-\d{1,3}[A-Z]?\*{0,3}$`)
	reRegRating = regexp.MustCompile(`^(?:[A-F]|I{1,3}|Inst\.)\*{0,3}$`)
	reRegInt    = regexp.MustCompile(`^\d{1,3}$`)
	reRegSize   = regexp.MustCompile(`^(?:0|00|2|4|6|8|10|12|16|20|22D?)$`)
	reLeadInt   = regexp.MustCompile(`^\d+`)
)

func regWords(t *testing.T, path string) (*DocExtraction, [][]LayoutWord) {
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	x, err := ExtractDocument(b)
	if err != nil {
		t.Fatal(err)
	}
	pages := [][]LayoutWord{}
	for _, pl := range x.Layouts {
		pages = append(pages, pl.Words)
	}
	return x, pages
}

// verticalBlocks reads the stack printed beneath each arrangement id: a rating row, then a row of contact
// counts and a row of contact sizes. A mixed arrangement prints one count/size column per contact size
// (for example 2 x #8 beside 3 x #12); a single-size arrangement prints one column.
func verticalBlocks(words []LayoutWord, family, source string, page int, view string) ([]regRow, []regSkip) {
	var rows []regRow
	var skips []regSkip
	type tok struct {
		t      string
		cx, y0 float64
		y1     float64
	}
	ws := []tok{}
	for _, w := range words {
		t := strings.TrimSpace(w.Text)
		// the catalogs print a vertical tab index (III, HD, Dualok, SJT ...) in the outer margin; it is not table content
		if t != "" && w.Box.X1 > 48 && w.Box.X0 < 564 {
			ws = append(ws, tok{t, w.Box.CX(), w.Box.Y0, w.Box.Y1})
		}
	}
	reRatingTok := regexp.MustCompile(`^[A-Za-z0-9,;:=. \-*]+$`)
	reCountTok := regexp.MustCompile(`^(\d{1,3})([†‡*]*)$`)
	for _, id := range ws {
		if !reRegID.MatchString(id.t) {
			continue
		}
		skip := func(why string) { skips = append(skips, regSkip{source, strconv.Itoa(page), id.t, why, family}) }
		var col []tok
		for _, w := range ws {
			if w.y0 > id.y1-1 && w.y0 < id.y1+70 && math.Abs(w.cx-id.cx) <= 42 {
				col = append(col, w)
			}
		}
		sort.Slice(col, func(a, b int) bool { return col[a].y0 < col[b].y0 })
		// group into printed rows
		var lines [][]tok
		last := id.y1
		for _, w := range col {
			if reRegID.MatchString(w.t) || strings.Contains(w.t, "Rotation") || w.t == "of" {
				break
			}
			if w.y0-last > 16 {
				break
			}
			if n := len(lines); n > 0 && math.Abs(w.y0-lines[n-1][0].y0) <= 2.5 {
				lines[n-1] = append(lines[n-1], w)
			} else {
				lines = append(lines, []tok{w})
			}
			last = w.y1
		}
		for _, l := range lines {
			sort.Slice(l, func(a, b int) bool { return l[a].cx < l[b].cx })
		}
		if len(lines) == 0 {
			skip("no cells beneath the id")
			continue
		}
		join := func(l []tok) string {
			p := []string{}
			for _, w := range l {
				p = append(p, w.t)
			}
			return strings.Join(p, " ")
		}
		isCountRow := func(l []tok) bool {
			for _, w := range l {
				if !reCountTok.MatchString(w.t) {
					return false
				}
			}
			return true
		}
		i, rating := 0, ""
		if !isCountRow(lines[0]) {
			ok := true
			hasLetter := false
			for _, w := range lines[0] {
				if !reRatingTok.MatchString(w.t) {
					ok = false
				}
				if regexp.MustCompile(`[A-Za-z]`).MatchString(w.t) {
					hasLetter = true
				}
			}
			if !ok || !hasLetter {
				skip("rating row unclear: " + join(lines[0]))
				continue
			}
			rating, i = join(lines[0]), 1
		}
		if len(lines) != i+2 {
			skip(fmt.Sprintf("expected a count row and a size row; found %d row(s) after the rating: %s", len(lines)-i, join(lines[len(lines)-1])))
			continue
		}
		counts, sizes := lines[i], lines[i+1]
		if !isCountRow(counts) || len(counts) != len(sizes) || len(counts) > 6 {
			skip("count and size rows do not pair: " + join(counts) + " / " + join(sizes))
			continue
		}
		total, parts, foot := 0, []string{}, false
		okRow := true
		for k := range counts {
			if math.Abs(counts[k].cx-sizes[k].cx) > 6 || !reRegSize.MatchString(sizes[k].t) {
				okRow = false
				break
			}
			m := reCountTok.FindStringSubmatch(counts[k].t)
			n, _ := strconv.Atoi(m[1])
			if n < 1 || n > 200 {
				okRow = false
				break
			}
			if m[2] != "" {
				foot = true
			}
			total += n
			if len(counts) == 1 {
				parts = append(parts, sizes[k].t)
			} else {
				parts = append(parts, fmt.Sprintf("%dx%s", n, sizes[k].t))
			}
		}
		if !okRow {
			skip("count or size cell not recognized: " + join(counts) + " / " + join(sizes))
			continue
		}
		clean := strings.TrimRight(id.t, "*")
		shell := reLeadInt.FindString(clean)
		if n, err := strconv.Atoi(shell); err == nil {
			shell = strconv.Itoa(n)
		}
		note := view
		if strings.HasSuffix(id.t, "*") {
			note += "; the source marks this arrangement with " + id.t[len(clean):] + " (footnote not read)"
		}
		if foot {
			note += "; a count carries a footnote mark (footnote not read)"
		}
		if len(counts) > 1 {
			note += "; mixed contact sizes, shown as quantity x size, total is the sum"
		}
		rows = append(rows, regRow{family, clean, shell, strconv.Itoa(total), strings.Join(parts, ";"), rating, source, strconv.Itoa(page), "EXTRACTED FROM CATALOG TEXT — UNREVIEWED", note})
	}
	return rows, skips
}

func TestBuildArrangementRegister(t *testing.T) {
	dir, out := os.Getenv("REGISTER_DIR"), os.Getenv("REGISTER_OUT")
	if dir == "" || out == "" {
		t.Skip("set REGISTER_DIR and REGISTER_OUT")
	}
	var rows []regRow
	var skips []regSkip
	view := "front face of pin insert or rear face of socket insert, as printed"
	add := func(file, family, label string, from, to int, v string) {
		_, pages := regWords(t, filepath.Join(dir, file))
		for p := from; p <= to && p <= len(pages); p++ {
			r, s := verticalBlocks(pages[p-1], family, label, p, v)
			rows, skips = append(rows, r...), append(skips, s...)
		}
	}
	add("digikey-amphenol-5015-matrix.pdf", "MIL-DTL-5015 (Amphenol Matrix crimp rear release)", "Amphenol MIL-DTL-5015 Matrix catalog (DigiKey copy)", 5, 13, view)
	add("amphenol-high-power-5015-data.pdf", "Amphenol high-power 5015", "Amphenol high-power connectors catalog", 14, 15, view)
	add("fiber_catalog-1128958.pdf", "CF38999 fiber-optic (MIL-DTL-38999 Series III)", "Amphenol fiber-optic catalog", 10, 11, "front face of pin inserts, as printed")

	// MIL-DTL-26482 arrangement ids (ids only: the retrieved sheet is drawings)
	_, p26 := regWords(t, filepath.Join(dir, "glenair-26482-contact-arrangements.pdf"))
	re26 := regexp.MustCompile(`^\d{1,2}-\d{1,3}\*?$`)
	seen := map[string]bool{}
	for _, w := range p26[0] {
		id := strings.TrimRight(strings.TrimSpace(w.Text), "*")
		if re26.MatchString(strings.TrimSpace(w.Text)) && !seen[id] {
			seen[id] = true
			shell := strings.SplitN(id, "-", 2)[0]
			rows = append(rows, regRow{"MIL-DTL-26482 Series II (MIL-STD-1669)", id, shell, "", "", "", "Glenair MIL-STD-1669 insert arrangements sheet", "1", "ID ONLY — UNREVIEWED", "The sheet lists the arrangement id and draws it; counts and sizes are not in the retrieved text"})
		}
	}

	// MIL-DTL-28840: shell size and contact count pairs ("Shell Size 11 ... 7 Contacts")
	_, p28 := regWords(t, filepath.Join(dir, "glenair-28840-arrangements.pdf"))
	var shells, counts []float64
	var sv, cv []string
	for _, w := range p28[0] {
		tx := strings.TrimSpace(w.Text)
		if m := regexp.MustCompile(`^Shell Size (\d+)$`).FindStringSubmatch(tx); m != nil {
			sv = append(sv, m[1])
			shells = append(shells, w.Box.CX())
		}
		if m := regexp.MustCompile(`^(\d+) Contacts$`).FindStringSubmatch(tx); m != nil {
			cv = append(cv, m[1])
			counts = append(counts, w.Box.CX())
		}
	}
	if len(sv) == len(cv) && len(sv) > 0 {
		for i, s := range sv {
			rows = append(rows, regRow{"MIL-DTL-28840 (MIL-STD-1698)", "shell " + s + " standard insert", s, cv[i], "", "", "Glenair MIL-DTL-28840 standard insert arrangements sheet", "1", "EXTRACTED FROM CATALOG TEXT — UNREVIEWED", "One standard insert per shell size; the contact size is not stated in the retrieved text"})
		}
	} else {
		skips = append(skips, regSkip{"Glenair 28840", "1", "", fmt.Sprintf("shell size and contact count labels did not pair (%d and %d)", len(sv), len(cv)), ""})
	}
	_, _ = shells, counts

	// duplicates inside one source: keep the first, and drop both when two readings disagree
	type key struct{ f, id string }
	byKey := map[key]regRow{}
	conflict := map[key]bool{}
	var uniq []regRow
	for _, r := range rows {
		k := key{r.Family, r.ID}
		if prev, ok := byKey[k]; ok {
			if prev.Contacts != r.Contacts || prev.Sizes != r.Sizes || prev.Rating != r.Rating {
				conflict[k] = true
			}
			continue
		}
		byKey[k] = r
		uniq = append(uniq, r)
	}
	final := []regRow{}
	for _, r := range uniq {
		if conflict[key{r.Family, r.ID}] {
			skips = append(skips, regSkip{r.Source, r.Page, r.ID, "the id is printed twice with different cells", r.Family})
			continue
		}
		final = append(final, r)
	}
	// An arrangement id that was seen but whose cells could not be read is kept as a visible row, so the register
	// lists everything found and says which ones still need a person.
	have := map[key]bool{}
	for _, r := range final {
		have[key{r.Family, r.ID}] = true
	}
	for _, s := range skips {
		k := key{s.Family, strings.TrimRight(s.ID, "*")}
		if s.ID == "" || have[k] {
			continue
		}
		have[k] = true
		id := strings.TrimRight(s.ID, "*")
		shell := reLeadInt.FindString(id)
		if n, err := strconv.Atoi(shell); err == nil {
			shell = strconv.Itoa(n)
		}
		final = append(final, regRow{s.Family, id, shell, "", "", "", s.Source, s.Page, "SEEN — CELLS NOT READ", s.Why})
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	w.Write([]string{"family", "arrangement_id", "shell_size", "contacts", "contact_sizes", "service_rating", "source", "source_page", "status", "note"})
	for _, r := range final {
		w.Write([]string{r.Family, r.ID, r.Shell, r.Contacts, r.Sizes, r.Rating, r.Source, r.Page, r.Status, r.Note})
	}
	w.Flush()
	known := map[string]map[string]bool{}
	inReg := map[string]int{}
	for _, r := range final {
		if known[r.Source] == nil {
			known[r.Source] = map[string]bool{}
		}
		known[r.Source][r.ID] = true
		inReg[r.Source]++
	}
	for _, s := range skips {
		if s.ID != "" {
			if known[s.Source] == nil {
				known[s.Source] = map[string]bool{}
			}
			known[s.Source][s.ID] = true
		}
	}
	for src, ids := range known {
		fmt.Printf("SOURCE %s: ids seen %d, in register %d\n", src, len(ids), inReg[src])
	}
	fmt.Printf("register rows=%d skipped=%d\n", len(final), len(skips))
	for _, s := range skips {
		fmt.Printf("SKIP p%s %-8s %s | %s\n", s.Page, s.ID, s.Why, s.Source)
	}
}
