package main

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// This is a conservative Form-XObject-aware extraction path for CAD-generated PDFs.
// It is not a replacement for MuPDF/PDFium; it processes native text inside
// nested page/form content and deliberately never asserts it understands vector
// drawings or photographs as connector part numbers.
type mat26 [6]float64

func ident26() mat26 { return mat26{1, 0, 0, 1, 0, 0} }
func mul26(x, y mat26) mat26 {
	return mat26{x[0]*y[0] + x[2]*y[1], x[1]*y[0] + x[3]*y[1], x[0]*y[2] + x[2]*y[3], x[1]*y[2] + x[3]*y[3], x[0]*y[4] + x[2]*y[5] + x[4], x[1]*y[4] + x[3]*y[5] + x[5]}
}
func (m mat26) point26(x, y float64) (float64, float64) {
	return m[0]*x + m[2]*y + m[4], m[1]*x + m[3]*y + m[5]
}

var streamEvents26 = regexp.MustCompile(`(?s)\bBT\b.*?\bET\b|/[A-Za-z0-9_.-]+\s+Do\b|(?:[-+\d.]+\s+){6}cm\b|(?:^|\s)[qQ](?:\s|$)`)
var (
	resourceRef26 = regexp.MustCompile(`/Resources\s+(\d+)\s+\d+\s+R`)
	xobj26        = regexp.MustCompile(`(?s)/XObject\s*<<(.*?)>>`)
	xobjItem26    = regexp.MustCompile(`/([A-Za-z0-9_.-]+)\s+(\d+)\s+\d+\s+R`)
	formMatrix26  = regexp.MustCompile(`/Matrix\s*\[([^\]]+)\]`)
	pathCM26      = regexp.MustCompile(`^((?:[-+\d.]+\s+){6})cm$`)
)

// Resources can be inherited from page tree nodes, stored as indirect
// dictionaries, or embedded directly on Form XObjects.
func xobjResource26(obj *indirect, objects map[int]*indirect, depth int) map[string]int {
	out := map[string]int{}
	if obj == nil || depth > 12 {
		return out
	}
	var collect func([]byte)
	collect = func(dict []byte) {
		for _, group := range xobj26.FindAllSubmatch(dict, -1) {
			for _, entry := range xobjItem26.FindAllSubmatch(group[1], -1) {
				n, _ := strconv.Atoi(string(entry[2]))
				out[string(entry[1])] = n
			}
		}
		if r := resourceRef26.FindSubmatch(dict); len(r) > 1 {
			n, _ := strconv.Atoi(string(r[1]))
			if v := objects[n]; v != nil {
				for _, group := range xobj26.FindAllSubmatch(v.dictionary, -1) {
					for _, entry := range xobjItem26.FindAllSubmatch(group[1], -1) {
						n, _ := strconv.Atoi(string(entry[2]))
						out[string(entry[1])] = n
					}
				}
			}
		}
	}
	if m := rePageParent.FindSubmatch(obj.dictionary); len(m) > 1 {
		n, _ := strconv.Atoi(string(m[1]))
		for name, id := range xobjResource26(objects[n], objects, depth+1) {
			out[name] = id
		}
	}
	collect(obj.dictionary)
	return out
}
func matrixFromForm26(b []byte) mat26 {
	m := ident26()
	if mm := formMatrix26.FindSubmatch(b); len(mm) > 1 {
		fields := strings.Fields(string(mm[1]))
		if len(fields) == 6 {
			for i := 0; i < 6; i++ {
				m[i] = f64(fields[i])
			}
		}
	}
	return m
}
func parseCM26(raw string) mat26 {
	var m = ident26()
	raw = strings.TrimSpace(raw)
	if x := pathCM26.FindStringSubmatch(raw); len(x) > 1 {
		f := strings.Fields(x[1])
		if len(f) == 6 {
			for i := 0; i < 6; i++ {
				m[i] = f64(f[i])
			}
		}
	}
	return m
}
func (pg *Page) walkContents26(source []byte, inherited map[string]int, fonts map[string]fontMap26, objects map[int]*indirect, initial mat26, depth int, seen map[int]bool) {
	if depth > 12 {
		return
	}
	local := initial
	stack := []mat26{}
	for _, event := range streamEvents26.FindAllString(string(source), -1) {
		event = strings.TrimSpace(event)
		switch {
		case event == "q":
			stack = append(stack, local)
		case event == "Q":
			if len(stack) > 0 {
				local = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
		case strings.HasSuffix(event, " cm") || strings.HasSuffix(event, "cm") && len(strings.Fields(event)) == 7:
			local = mul26(local, parseCM26(event))
		case strings.HasPrefix(event, "BT") && strings.HasSuffix(event, "ET"):
			// Local coordinates need the active graphics-state transform.
			for _, w := range parseBlockFont26(strings.TrimSuffix(strings.TrimPrefix(event, "BT"), "ET"), fonts) {
				w.X, w.Y = local.point26(w.X, w.Y)
				pg.Words = append(pg.Words, w)
			}
		case strings.HasSuffix(event, "Do"):
			name := strings.TrimPrefix(strings.Fields(event)[0], "/")
			id := inherited[name]
			if id == 0 || seen[id] {
				continue
			}
			obj := objects[id]
			if obj == nil || !obj.hasStream {
				continue
			}
			if !bytes.Contains(obj.dictionary, []byte("/Subtype /Form")) && !bytes.Contains(obj.dictionary, []byte("/Subtype/Form")) {
				continue
			}
			formResources := map[string]int{}
			for k, v := range inherited {
				formResources[k] = v
			}
			for k, v := range xobjResource26(obj, objects, 0) {
				formResources[k] = v
			}
			seen[id] = true
			pg.walkContents26(obj.decoded, formResources, func() map[string]fontMap26 {
				f := map[string]fontMap26{}
				for k, v := range fonts {
					f[k] = v
				}
				for k, v := range fontResources26(obj, objects, 0) {
					f[k] = v
				}
				return f
			}(), objects, mul26(local, matrixFromForm26(obj.dictionary)), depth+1, seen)
			delete(seen, id)
		}
	}
}
func extractPDFEnhanced26(b []byte) (PDFData, error) {
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		return PDFData{}, fmt.Errorf("not a PDF")
	}
	if len(b) > 100<<20 {
		return PDFData{}, fmt.Errorf("PDF too large")
	}
	objects := parseRawObjects(b)
	pages := pdfPageOrder(objects)
	if len(pages) == 0 {
		return PDFData{}, fmt.Errorf("unsupported PDF page tree")
	}
	out := PDFData{Pages: make([]Page, 0, len(pages))}
	for idx, p := range pages {
		pg := Page{Number: idx + 1, Width: 792, Height: 612, Extraction: "native and Form-XObject text (review required)"}
		if box := reBox.FindSubmatch(p.dictionary); len(box) > 2 {
			pg.Width = f64(string(box[1]))
			pg.Height = f64(string(box[2]))
		}
		res := xobjResource26(p, objects, 0)
		for _, id := range contentRefs(p) {
			if obj := objects[id]; obj != nil && obj.hasStream {
				pg.walkContents26(obj.decoded, res, fontResources26(p, objects, 0), objects, ident26(), 0, map[int]bool{})
			}
		}
		sort.SliceStable(pg.Words, func(i, j int) bool {
			if d := pg.Words[i].Y - pg.Words[j].Y; d > 2 || d < -2 {
				return pg.Words[i].Y > pg.Words[j].Y
			}
			return pg.Words[i].X < pg.Words[j].X
		})
		pg.Lines = groupLines(pg.Words)
		if len(pg.Words) == 0 {
			pg.Extraction = "NO_NATIVE_TEXT / unsupported encoding or raster"
			out.Issues = append(out.Issues, fmt.Sprintf("Page %d has no decodable text; inspect locally", idx+1))
		}
		out.Pages = append(out.Pages, pg)
	}
	return out, nil
}


