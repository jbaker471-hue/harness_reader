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
	ID   string  `json:"id"`
	Text string  `json:"text"`
	Page int     `json:"page"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
}

var (
	reNotesHeading = regexp.MustCompile(`^(?:GENERAL\s+)?NOTES?\s*[:.]|^(?:GENERAL\s+)?NOTES?$`)
	reNoteNumber   = regexp.MustCompile(`^(\d{1,2})[.)]?$`)
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

// drawingNotes reads the numbered notes under a NOTES heading. A page with no such heading gives none: the
// numbers in a parts list or a callout are never taken for notes.
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
		if h < 0 {
			continue
		}
		margin := segs[h].x0
		var cur *DrawingNote
		textX := 0.0
		lastY := segs[h].y
		pitch := 0.0
		flush := func() {
			if cur != nil {
				cur.Text = strings.Join(strings.Fields(cur.Text), " ")
				out = append(out, *cur)
				cur = nil
			}
		}
		for _, s := range segs[h+1:] {
			// another column or the sheet's own tables: only segments that start at the note margin or indented under it
			if s.x0 < margin-12 || s.x0 > margin+0.45*p.Width {
				continue
			}
			gap := lastY - s.y
			if pitch > 0 && gap > 3.5*pitch {
				break
			}
			first := s.words[0]
			if m := reNoteNumber.FindStringSubmatch(strings.TrimSpace(first.Text)); m != nil && abs(first.X-margin) <= 14 && len(s.words) > 0 {
				flush()
				cur = &DrawingNote{ID: m[1], Page: p.Number, X: first.X, Y: s.y}
				textX = first.X + 8
				if len(s.words) > 1 {
					textX = s.words[1].X
					rest := noteSeg{words: s.words[1:]}
					cur.Text = rest.text()
				}
			} else if cur != nil && s.x0 >= textX-6 && s.x0 <= textX+40 {
				// a wrapped line starts under the note text; anything further right is another block
				cur.Text += " " + s.text()
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
