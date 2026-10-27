package main

import (
	"strings"
	"testing"
)

func TestGraphicalCalloutReferences(t *testing.T) {
	p := PageLayout{Page: 1, Circles: []Circle{{CX: 30, CY: 30, R: 10}, {CX: 30, CY: 30, R: 10}}, Words: []LayoutWord{{Text: "27", Box: Box{26, 27, 34, 33}}}}
	x := &DocExtraction{Layouts: []PageLayout{p}}
	cs := graphicalCallouts(x, []BOMRow{{Item: "27", Part: "INVENTED-PART"}})
	if len(cs) != 1 || len(cs[0].Matches) != 1 || cs[0].Kind != "BOM reference candidate" {
		t.Fatalf("%+v", cs)
	}
	x.Legacy = true
	if len(graphicalCallouts(x, nil)) != 0 {
		t.Fatal("legacy geometry accepted")
	}
	x.Legacy = false
	x.Layouts[0].Words[0].Text = "27 28"
	if len(graphicalCallouts(x, nil)) != 0 {
		t.Fatal("ambiguous text accepted")
	}
}
func TestFragmentedFlagAndOpenGap(t *testing.T) {
	pts := [][2]float64{{20, 20}, {30, 20}, {35, 30}, {30, 40}, {20, 40}, {15, 30}}
	p := PageLayout{Page: 1, Words: []LayoutWord{{Text: "23", Box: Box{22, 27, 28, 33}}}}
	for i, a := range pts {
		b := pts[(i+1)%len(pts)]
		p.Segments = append(p.Segments, Segment{a[0], a[1], b[0], b[1]})
	}
	if len(fragmentedFlags(p)) != 1 {
		t.Fatal("split flag not recognized")
	}
	p.Segments = p.Segments[:5]
	if len(fragmentedFlags(p)) != 0 {
		t.Fatal("open gap closed without evidence")
	}
}
func TestLeaderDoesNotFollowCrossingOrBranch(t *testing.T) {
	p := PageLayout{LeaderSegments: []Segment{{20, 15, 40, 15}, {40, 15, 50, 30}, {30, 10, 30, 20}}}
	target, status := calloutLeaders(p, Box{10, 10, 20, 20})
	if len(target) != 1 || target[0] != [2]float64{50, 30} || strings.Contains(status, "HELD") {
		t.Fatalf("%v %s", target, status)
	}
	p.LeaderSegments = append(p.LeaderSegments, Segment{40, 15, 60, 15})
	_, status = calloutLeaders(p, Box{10, 10, 20, 20})
	if !strings.Contains(status, "HELD") {
		t.Fatal("branch guessed")
	}
	p.VectorDetailTruncated = true
	target, status = calloutLeaders(p, Box{})
	if len(target) != 0 || !strings.Contains(status, "HELD") {
		t.Fatal("truncation ignored")
	}
}
func TestCoincidentGlyphsPreserveRealRepeats(t *testing.T) {
	gs := []glyph{}
	for _, x := range []float64{10, 10, 16, 16} {
		gs = append(gs, glyph{r: '1', box: Box{x, 10, x + 5, 18}, loose: Box{x, 10, x + 6, 18}, ox: x, oy: 18, ux: 1, em: 8})
	}
	ws := groupGlyphs(gs)
	if len(ws) != 1 || ws[0].Text != "11" {
		t.Fatalf("%+v", ws)
	}
}
func TestGraphicalCalloutPDFium(t *testing.T) {
	b := onePagePDF("20 20 m 30 20 l S 30 20 m 35 30 l S 35 30 m 30 40 l S 30 40 m 20 40 l S 20 40 m 15 30 l S 15 30 m 20 20 l S BT /F1 7 Tf 1 0 0 1 21 27 Tm (23) Tj ET", "[0 0 200 200]")
	x, e := ExtractDocument(b)
	if e != nil {
		t.Fatal(e)
	}
	cs := graphicalCallouts(x, nil)
	if len(cs) != 1 || cs[0].Shape != "hexagon" || cs[0].Reference != "23" {
		t.Fatalf("%+v", cs)
	}
}

func TestFlagDefinitionReconciliation(t *testing.T) {
	poly := func(x, y float64) CalloutPolygon {
		return CalloutPolygon{Points: [][2]float64{{x, y}, {x + 10, y}, {x + 15, y + 10}, {x + 10, y + 20}, {x, y + 20}, {x - 5, y + 10}}, Box: Box{x - 5, y, x + 15, y + 20}}
	}
	p := PageLayout{Page: 1, Polygons: []CalloutPolygon{poly(20, 20), poly(20, 100)}, Words: []LayoutWord{{Text: "23", Box: Box{22, 27, 28, 33}}, {Text: "Secure test sleeve with approved material", Box: Box{45, 27, 220, 33}}, {Text: "23", Box: Box{22, 107, 28, 113}}}}
	cs := graphicalCallouts(&DocExtraction{Layouts: []PageLayout{p}}, nil)
	if len(cs) != 2 || cs[1].DefinitionPage != 1 || len(cs[1].Matches) != 1 {
		t.Fatalf("%+v", cs)
	}
}
