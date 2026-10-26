package main

import (
	"bytes"
	"regexp"
	"strconv"
)

// GraphicPage reports independent non-text PDF drawing evidence. Graphic
// operators can establish that a drawing contains lines, illustrations,
// and placed image/form objects, but never establish connector identity alone.
type GraphicPage struct {
	Page              int    `json:"page"`
	VectorSegments    int    `json:"vectorSegments"`
	Rectangles        int    `json:"rectangles"`
	Curves            int    `json:"curves"`
	ImageObjects      int    `json:"imageObjects"`
	FormObjects       int    `json:"formObjects"`
	UnresolvedObjects int    `json:"unresolvedObjects"`
	ContentStreams    int    `json:"contentStreams"`
	GraphicsPresent   bool   `json:"graphicsPresent"`
	Assessment        string `json:"assessment"`
}

var (
	reXObjectResource  = regexp.MustCompile(`(?s)/XObject\s*<<(.*?)>>`)
	reXObjectReference = regexp.MustCompile(`/([A-Za-z][A-Za-z0-9_\-.]*)\s+(\d+)\s+\d+\s+R`)
	rePDFDo            = regexp.MustCompile(`/([A-Za-z][A-Za-z0-9_\-.]*)\s+Do\b`)
	rePDFPathLine      = regexp.MustCompile(`(?:^|\s)(?:l|m|S|s)(?:\s|$)`)
	rePDFPathRect      = regexp.MustCompile(`(?:^|\s)re(?:\s|$)`)
	rePDFPathCurve     = regexp.MustCompile(`(?:^|\s)(?:c|v|y)(?:\s|$)`)
	rePDFInlineImage   = regexp.MustCompile(`(?:^|\s)BI(?:\s|$)`)
	rePageParent       = regexp.MustCompile(`/Parent\s+(\d+)\s+\d+\s+R`)
)

func contentRefs(p *indirect) []int {
	out := []int{}
	if m := reContents.FindSubmatch(p.dictionary); len(m) > 1 {
		n, _ := strconv.Atoi(string(m[1]))
		out = append(out, n)
	}
	if m := reContentsArr.FindSubmatch(p.dictionary); len(m) > 1 {
		for _, v := range reRef.FindAllSubmatch(m[1], -1) {
			n, _ := strconv.Atoi(string(v[1]))
			out = append(out, n)
		}
	}
	return out
}
func graphicInfoFromPDF(b []byte) []GraphicPage {
	objects := parseRawObjects(b)
	pages := pdfPageOrder(objects)
	out := make([]GraphicPage, 0, len(pages))
	for i, p := range pages {
		g := GraphicPage{Page: i + 1}
		// PDF page resources are often inherited from the parent page tree.
		references := map[string]int{}
		var inheritResources func(*indirect, int)
		inheritResources = func(obj *indirect, depth int) {
			if obj == nil || depth > 8 {
				return
			}
			for _, match := range reXObjectResource.FindAllSubmatch(obj.dictionary, -1) {
				for _, entry := range reXObjectReference.FindAllSubmatch(match[1], -1) {
					id, _ := strconv.Atoi(string(entry[2]))
					references[string(entry[1])] = id
				}
			}
			if m := rePageParent.FindSubmatch(obj.dictionary); len(m) > 1 {
				n, _ := strconv.Atoi(string(m[1]))
				inheritResources(objects[n], depth+1)
			}
		}
		inheritResources(p, 0)
		for _, n := range contentRefs(p) {
			obj := objects[n]
			if obj == nil || !obj.hasStream {
				continue
			}
			g.ContentStreams++
			// These counts are weak structural evidence, not a reconstructed wire map.
			g.VectorSegments += len(rePDFPathLine.FindAll(obj.decoded, -1))
			g.Rectangles += len(rePDFPathRect.FindAll(obj.decoded, -1))
			g.Curves += len(rePDFPathCurve.FindAll(obj.decoded, -1))
			g.ImageObjects += len(rePDFInlineImage.FindAll(obj.decoded, -1))
			for _, call := range rePDFDo.FindAllSubmatch(obj.decoded, -1) {
				name := string(call[1])
				target := objects[references[name]]
				if target == nil {
					g.UnresolvedObjects++
					continue
				}
				switch {
				case bytes.Contains(target.dictionary, []byte(`/Subtype /Image`)) || bytes.Contains(target.dictionary, []byte(`/Subtype/Image`)):
					g.ImageObjects++
				case bytes.Contains(target.dictionary, []byte(`/Subtype /Form`)) || bytes.Contains(target.dictionary, []byte(`/Subtype/Form`)):
					g.FormObjects++
					// Form XObjects can contain entire vector illustrations and text.
					if target.hasStream {
						g.VectorSegments += len(rePDFPathLine.FindAll(target.decoded, -1))
						g.Rectangles += len(rePDFPathRect.FindAll(target.decoded, -1))
						g.Curves += len(rePDFPathCurve.FindAll(target.decoded, -1))
					}
				default:
					g.UnresolvedObjects++
				}
			}
		}
		g.GraphicsPresent = g.ImageObjects > 0 || g.FormObjects > 0 || g.VectorSegments > 0 || g.Rectangles > 0 || g.Curves > 0 || g.UnresolvedObjects > 0
		if !g.GraphicsPresent {
			g.Assessment = "No drawable graphics detected by structural scan; inspect source page"
		} else if g.ImageObjects > 0 || g.FormObjects > 0 {
			g.Assessment = "Placed image/form content detected; visual review required to identify connectors"
		} else if g.VectorSegments > 2 || g.Curves > 0 {
			g.Assessment = "Vector drawing content detected; inspect connector views, wires and callouts visually"
		} else {
			g.Assessment = "Drawing primitives detected; inspect original page"
		}

		out = append(out, g)
	}
	return out
}
