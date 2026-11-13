package main

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// DrawingNote is a numbered note read from the NOTES block of a sheet, with the exact words it carries.
// It is a candidate until a person has compared it with the sheet.
type DrawingNote struct {
	ID         string                 `json:"id"`
	Text       string                 `json:"text"`
	Page       int                    `json:"page"`
	X          float64                `json:"x"`
	Y          float64                `json:"y"`
	Box        *Box                   `json:"box,omitempty"`
	References []DrawingNoteReference `json:"references"`
}

// DrawingNoteReference is an explicit textual callout such as "SEE NOTE 7". Bare numbers are
// intentionally excluded because they may be BOM balloons, dimensions, wire IDs or cavity labels.
type DrawingNoteReference struct {
	Page   int    `json:"page"`
	Box    Box    `json:"box"`
	Text   string `json:"text"`
	Method string `json:"method"`
}

var (
	reNotesHeading = regexp.MustCompile(`^(?:(?:GENERAL|DRAWING)\s+)?NOTES?\s*[:.]|^(?:(?:GENERAL|DRAWING)\s+)?NOTES?$`)
	reNoteNumber   = regexp.MustCompile(`^(\d{1,2})[.)]?$`)
	reInlineNote   = regexp.MustCompile(`^(\d{1,2})[.)]\s*(.+)$`)
	reExplicitNote = regexp.MustCompile(`(?i)\bNOTES?\s*(?:NO\.?\s*)?([0-9]{1,2}(?:\s*(?:,|AND|&)\s*[0-9]{1,2})*)\b`)
	reReferenceID  = regexp.MustCompile(`[0-9]{1,2}`)
)

type noteSeg struct {
	y, x0, x1 float64
	words     []Word
}

// noteSegments splits every text line where the gap between words is a column break, so a parts list that
// shares a baseline with a note does not run into it.
func noteSegments(p Page) []noteSeg {
	lines := pageLines(p)
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].Y > lines[j].Y })
	var out []noteSeg
	for _, l := range lines {
		ws := make([]Word, 0, len(l.Words))
		for _, w := range l.Words {
			if !w.Rotated && strings.TrimSpace(w.Text) != "" {
				ws = append(ws, w)
			}
		}
		sort.SliceStable(ws, func(i, j int) bool { return ws[i].X < ws[j].X })
		var cur *noteSeg
		for _, w := range ws {
			sz := w.Size
			if sz <= 0 {
				sz = 6
			}
			if cur != nil && w.X-cur.x1 <= 2.5*sz {
				cur.words = append(cur.words, w)
				if r := w.right(); r > cur.x1 {
					cur.x1 = r
				}
				continue
			}
			if cur != nil {
				out = append(out, *cur)
			}
			cur = &noteSeg{y: l.Y, x0: w.X, x1: w.right(), words: []Word{w}}
		}
		if cur != nil {
			out = append(out, *cur)
		}
	}
	return out
}

func (s noteSeg) text() string {
	parts := make([]string, 0, len(s.words))
	for _, w := range s.words {
		parts = append(parts, strings.TrimSpace(w.Text))
	}
	return strings.Join(parts, " ")
}

func displayWordBox(p Page, w Word) Box {
	sz := w.Size
	if sz <= 0 {
		sz = 6
	}
	return Box{X0: mathMax(0, w.X-2), Y0: mathMax(0, p.Height-w.Y-sz), X1: mathMin(p.Width, w.right()+2), Y1: mathMin(p.Height, p.Height-w.Y+0.3*sz)}
}

func displaySegmentBox(p Page, words []Word) (Box, bool) {
	var b Box
	for i, w := range words {
		wb := displayWordBox(p, w)
		if i == 0 {
			b = wb
		} else {
			b = b.Union(wb)
		}
	}
	return b, len(words) > 0
}

func mathMin(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func mathMax(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func noteStartCandidate(s noteSeg) (int, bool) {
	if len(s.words) < 2 {
		return 0, false
	}
	m := reNoteNumber.FindStringSubmatch(strings.TrimSpace(s.words[0].Text))
	if m == nil {
		return 0, false
	}
	// These are common quantity/unit cells at the left edge of a BOM. A true note can still be
	// "DELETED", but an item number followed by EA, A/R, QTY or PCS is not prose.
	second := strings.ToUpper(strings.Trim(strings.TrimSpace(s.words[1].Text), ".,:;()"))
	switch second {
	case "EA", "EACH", "A/R", "AR", "QTY", "PCS", "PC":
		return 0, false
	}
	rest := strings.TrimSpace(noteSeg{words: s.words[1:]}.text())
	letters := 0
	for _, r := range rest {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			letters++
		}
	}
	if letters < 4 {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	return n, err == nil
}

// headinglessNoteBlock recognizes a genuine construction-note column when the CAD author omitted
// a NOTES heading. It requires at least three vertically adjacent, increasing numbered prose rows
// at the same left margin. Isolated callouts and BOM quantity rows therefore do not qualify.
func headinglessNoteBlock(p Page, segs []noteSeg) (int, float64, bool) {
	type candidate struct {
		index, id int
		x, y      float64
	}
	cs := []candidate{}
	for i, s := range segs {
		if s.x0 >= 0.5*p.Width {
			continue
		}
		if id, ok := noteStartCandidate(s); ok {
			cs = append(cs, candidate{i, id, s.x0, s.y})
		}
	}
	bestStart, bestCount := -1, 0
	for i := range cs {
		count, lastID, lastY := 1, cs[i].id, cs[i].y
		for j := i + 1; j < len(cs); j++ {
			if abs(cs[j].x-cs[i].x) > 14 || cs[j].y >= lastY || lastY-cs[j].y > 0.12*p.Height {
				continue
			}
			if cs[j].id <= lastID {
				continue
			}
			count++
			lastID, lastY = cs[j].id, cs[j].y
		}
		if count > bestCount {
			bestStart, bestCount = cs[i].index, count
		}
	}
	if bestCount < 3 {
		return 0, 0, false
	}
	return bestStart, segs[bestStart].x0, true
}

// drawingNotes reads numbered notes under a NOTES heading, or a strongly shaped headingless note column.
// Numbers in a parts list or isolated callouts are never taken for notes.
func drawingNotes(pages []Page) []DrawingNote {
	out := []DrawingNote{}
	for _, p := range pages {
		if !p.PageAccurate || p.Width <= 0 {
			continue
		}
		segs := noteSegments(p)
		h := -1
		for i, s := range segs {
			if s.x0 < 0.5*p.Width && reNotesHeading.MatchString(strings.ToUpper(s.text())) {
				h = i
				break
			}
		}
		start := h + 1
		margin := 0.0
		if h >= 0 {
			margin = segs[h].x0
		} else {
			var ok bool
			start, margin, ok = headinglessNoteBlock(p, segs)
			if !ok {
				continue
			}
		}
		var cur *DrawingNote
		var curBox Box
		hasCurBox := false
		textX := 0.0
		lastY := p.Height
		if h >= 0 {
			lastY = segs[h].y
		} else if start < len(segs) {
			lastY = segs[start].y + 1
		}
		pitch := 0.0
		flush := func() {
			if cur != nil {
				cur.Text = strings.Join(strings.Fields(cur.Text), " ")
				if hasCurBox {
					b := curBox
					cur.Box = &b
				}
				out = append(out, *cur)
				cur = nil
				hasCurBox = false
			}
		}
		for _, s := range segs[start:] {
			// another column or the sheet's own tables: only segments that start at the note margin or indented under it
			if s.x0 < margin-12 || s.x0 > margin+0.45*p.Width {
				continue
			}
			gap := lastY - s.y
			if pitch > 0 && gap > 6*pitch && gap > 0.12*p.Height {
				break
			}
			first := s.words[0]
			lineText := strings.TrimSpace(s.text())
			if m := reInlineNote.FindStringSubmatch(lineText); m != nil && abs(s.x0-margin) <= 14 {
				flush()
				cur = &DrawingNote{ID: m[1], Text: strings.TrimSpace(m[2]), Page: p.Number, X: s.x0, Y: s.y, References: []DrawingNoteReference{}}
				curBox, hasCurBox = displaySegmentBox(p, s.words)
				textX = s.x0 + 8
			} else if m := reNoteNumber.FindStringSubmatch(strings.TrimSpace(first.Text)); m != nil && abs(first.X-margin) <= 14 && len(s.words) > 0 {
				flush()
				cur = &DrawingNote{ID: m[1], Page: p.Number, X: first.X, Y: s.y, References: []DrawingNoteReference{}}
				curBox, hasCurBox = displaySegmentBox(p, s.words)
				textX = first.X + 8
				if len(s.words) > 1 {
					textX = s.words[1].X
					rest := noteSeg{words: s.words[1:]}
					cur.Text = rest.text()
				}
			} else if cur != nil && s.x0 >= textX-6 && s.x0 <= textX+40 {
				// a wrapped line starts under the note text; anything further right is another block
				cur.Text += " " + s.text()
				if b, ok := displaySegmentBox(p, s.words); ok {
					if hasCurBox {
						curBox = curBox.Union(b)
					} else {
						curBox, hasCurBox = b, true
					}
				}
			} else {
				continue
			}
			if g := lastY - s.y; g > 0 && (pitch == 0 || g < pitch) {
				pitch = g
			}
			lastY = s.y
		}
		flush()
	}
	// Index only explicit NOTE/NOTES + number wording. A standalone numeric symbol is not enough
	// evidence to distinguish a construction note from a BOM balloon or other drawing callout.
	byID := map[string][]int{}
	for i := range out {
		byID[out[i].ID] = append(byID[out[i].ID], i)
	}
	for _, p := range pages {
		if !p.PageAccurate {
			continue
		}
		for _, l := range pageLines(p) {
			text := strings.Join(strings.Fields(l.Text), " ")
			matches := reExplicitNote.FindAllStringSubmatch(text, -1)
			if len(matches) == 0 {
				continue
			}
			box, ok := displaySegmentBox(p, l.Words)
			if !ok {
				continue
			}
			for _, m := range matches {
				for _, id := range reReferenceID.FindAllString(m[1], -1) {
					for _, i := range byID[id] {
						r := DrawingNoteReference{Page: p.Number, Box: box, Text: text, Method: "Explicit NOTE-number text"}
						duplicate := false
						for _, prior := range out[i].References {
							if prior.Page == r.Page && prior.Box == r.Box {
								duplicate = true
							}
						}
						if !duplicate {
							out[i].References = append(out[i].References, r)
						}
					}
				}
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Page != out[j].Page {
			return out[i].Page < out[j].Page
		}
		a, _ := strconv.Atoi(out[i].ID)
		b, _ := strconv.Atoi(out[j].ID)
		return a < b
	})
	return out
}
