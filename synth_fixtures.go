package main

// Deterministic, NON-CUI synthetic drawings used by the unit tests and by the
// in-app "orientation self-test". Every variant draws the SAME logical harness
// drawing; only the PDF encoding differs (page /Rotate, flipped coordinate
// systems, nested Form XObjects, glyph-by-glyph text, image storage order).
// A correct extractor must return identical engineering records for all of
// them, and must show embedded pictures the way a PDF viewer shows them.

import (
	"bytes"
	"fmt"
	"math"
	"strings"
)

type synthVariant struct {
	Name        string
	Description string
	// Display expectations
	DisplayW, DisplayH float64
	TextAngle          float64 // expected dominant reading direction in display
	ImageMirrored      bool    // the drawing itself shows the picture mirrored
	RawFlipped         bool    // stored pixels differ from displayed orientation
}

var synthVariants = []synthVariant{
	{Name: "upright", Description: "Plain landscape page, no rotation", DisplayW: 792, DisplayH: 612},
	{Name: "rotate90", Description: "Portrait MediaBox with /Rotate 90; content pre-rotated so the viewer shows it upright", DisplayW: 792, DisplayH: 612},
	{Name: "rotate180", Description: "/Rotate 180 with content stored upside-down", DisplayW: 792, DisplayH: 612},
	{Name: "rotate270", Description: "Portrait MediaBox with /Rotate 270", DisplayW: 792, DisplayH: 612},
	{Name: "topdown", Description: "CAD/Visio-style Y-down coordinate system (1 0 0 -1 0 H cm), text with flipped Tm, image rows stored bottom-up", DisplayW: 792, DisplayH: 612, RawFlipped: true},
	{Name: "nestedforms", Description: "Everything inside nested, reused Form XObjects whose /Matrix values each rotate 180°", DisplayW: 792, DisplayH: 612},
	{Name: "nestedmixed", Description: "/Rotate 90 page; inner Form /Matrix translates, outer Form /Matrix rotates (non-commuting transforms)", DisplayW: 792, DisplayH: 612},
	{Name: "glyphs", Description: "Every character is a separate positioned text object, emitted in reverse order", DisplayW: 792, DisplayH: 612},
	{Name: "mirrored", Description: "Picture intentionally drawn mirrored (rear-view convention); must stay mirrored", DisplayW: 792, DisplayH: 612, ImageMirrored: true, RawFlipped: true},
	{Name: "sideways", Description: "Portrait page WITHOUT /Rotate: the drawing appears sideways in every viewer", DisplayW: 612, DisplayH: 792, TextAngle: 90, RawFlipped: true},
}

type synthText struct {
	x, y, size float64
	s          string
}

// The logical drawing in design coordinates: 792 x 612, Y up.
func synthDesign() (page1 []synthText, page2 []synthText) {
	t := func(x, y, s float64, str string) synthText { return synthText{x, y, s, str} }
	page1 = []synthText{
		t(40, 580, 10, "NON-CUI SYNTHETIC ORIENTATION TEST / SHEET 1"),
		t(60, 520, 9, "J12"), t(110, 520, 9, "D38999/26FG35SA"),
		t(60, 470, 8, "FROM"), t(110, 470, 8, "PIN"), t(160, 470, 8, "SIGNAL"), t(260, 470, 8, "TO"), t(310, 470, 8, "PIN"),
		t(60, 455, 8, "J12"), t(110, 455, 8, "1"), t(160, 455, 8, "VBUS"), t(260, 455, 8, "J21"), t(310, 455, 8, "3"),
		t(60, 440, 8, "J12"), t(110, 440, 8, "2"), t(160, 440, 8, "RETURN"), t(260, 440, 8, "J21"), t(310, 440, 8, "4"),
		t(60, 425, 8, "J12"), t(110, 425, 8, "3"), t(160, 425, 8, "TXD_P"), t(260, 425, 8, "J21"), t(310, 425, 8, "5"),
		t(60, 410, 8, "J12"), t(110, 410, 8, "4"), t(160, 410, 8, "TXD_N"), t(260, 410, 8, "J21"), t(310, 410, 8, "6"),
		t(500, 300, 8, "CONNECTOR P2 PINOUT"),
		t(500, 285, 7, "PIN"), t(540, 285, 7, "SIGNAL"), t(610, 285, 7, "WIRE"), t(660, 285, 7, "COLOR"), t(710, 285, 7, "DESTINATION"),
		t(500, 272, 7, "A"), t(540, 272, 7, "+28V"), t(610, 272, 7, "W101"), t(660, 272, 7, "RED"), t(710, 272, 7, "TB1-4"),
		t(500, 260, 7, "B"), t(540, 260, 7, "28V_RTN"), t(610, 260, 7, "W102"), t(660, 260, 7, "BLK"), t(710, 260, 7, "TB1-5"),
		t(500, 248, 7, "C"), t(540, 248, 7, "CHASSIS"), t(610, 248, 7, "W103"), t(660, 248, 7, "GRN"), t(710, 248, 7, "TB1-6"),
		t(130, 205, 8, "J12 FACE VIEW"),
	}
	// small cavity numbers inside the insert diagram
	for i := 0; i < 6; i++ {
		a := float64(i) * math.Pi / 3
		page1 = append(page1, t(150+22*math.Cos(a)-2, 140+22*math.Sin(a)-2.5, 5, fmt.Sprint(i+1)))
	}
	page2 = []synthText{
		t(40, 580, 10, "NON-CUI SYNTHETIC ORIENTATION TEST / SHEET 2"),
		t(60, 520, 10, "BILL OF MATERIALS"),
		t(60, 500, 8, "ITEM"), t(110, 500, 8, "QTY"), t(160, 500, 8, "PART NUMBER"), t(300, 500, 8, "DESCRIPTION"), t(480, 500, 8, "REF DES"),
		t(60, 485, 8, "1"), t(110, 485, 8, "1"), t(160, 485, 8, "D38999/26FG35SA"), t(300, 485, 8, "CONNECTOR PLUG"), t(480, 485, 8, "J12"),
		t(60, 470, 8, "2"), t(110, 470, 8, "79"), t(160, 470, 8, "M39029/56-348"), t(300, 470, 8, "CONTACT SOCKET 22D"), t(480, 470, 8, "J12"),
		t(60, 455, 8, "3"), t(110, 455, 8, "A/R"), t(160, 455, 8, "M23053/5-104-0"), t(300, 455, 8, "HEAT SHRINK SLEEVE"), t(480, 455, 8, ""),
	}
	return
}

// Quadrant test pattern: TL red, TR green, BL blue, BR yellow (as stored, row 0 = top).
func synthImagePixels(w, h int, bottomUp bool) []byte {
	out := make([]byte, 0, w*h*3)
	for row := 0; row < h; row++ {
		y := row
		if bottomUp {
			y = h - 1 - row
		}
		for x := 0; x < w; x++ {
			top, left := y < h/2, x < w/2
			var c [3]byte
			switch {
			case top && left:
				c = [3]byte{220, 30, 30}
			case top && !left:
				c = [3]byte{30, 180, 60}
			case !top && left:
				c = [3]byte{40, 60, 220}
			default:
				c = [3]byte{235, 200, 20}
			}
			out = append(out, c[:]...)
		}
	}
	return out
}

type pdfWriter struct{ objs [][]byte }

func (p *pdfWriter) reserve() int { p.objs = append(p.objs, nil); return len(p.objs) }
func (p *pdfWriter) set(n int, body string) { p.objs[n-1] = []byte(body) }
func (p *pdfWriter) add(body string) int  { n := p.reserve(); p.set(n, body); return n }
func (p *pdfWriter) stream(dict string, data []byte) int {
	n := p.reserve()
	var b bytes.Buffer
	fmt.Fprintf(&b, "<< %s /Length %d >>\nstream\n", dict, len(data))
	b.Write(data)
	b.WriteString("\nendstream")
	p.objs[n-1] = b.Bytes()
	return n
}
func (p *pdfWriter) bytes(root int) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n%\xe2\xe3\xcf\xd3\n")
	offs := make([]int, len(p.objs))
	for i, o := range p.objs {
		offs[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n", i+1)
		b.Write(o)
		b.WriteString("\nendobj\n")
	}
	x := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(p.objs)+1)
	for _, o := range offs {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root %d 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(p.objs)+1, root, x)
	return b.Bytes()
}

func pdfEsc(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`)
	return r.Replace(s)
}

// circlePath emits a 4-bezier circle in design coordinates.
func circlePath(cx, cy, r float64) string {
	k := 0.5523 * r
	return fmt.Sprintf("%.2f %.2f m %.2f %.2f %.2f %.2f %.2f %.2f c %.2f %.2f %.2f %.2f %.2f %.2f c %.2f %.2f %.2f %.2f %.2f %.2f c %.2f %.2f %.2f %.2f %.2f %.2f c h S\n",
		cx+r, cy,
		cx+r, cy+k, cx+k, cy+r, cx, cy+r,
		cx-k, cy+r, cx-r, cy+k, cx-r, cy,
		cx-r, cy-k, cx-k, cy-r, cx, cy-r,
		cx+k, cy-r, cx+r, cy-k, cx+r, cy)
}

// designOps returns content-stream operators for one sheet in design
// coordinates (Y up). glyphMode emits every character separately, in reverse.
func designOps(texts []synthText, withGraphics bool, font string, glyphMode bool, imageCM string) string {
	var b strings.Builder
	b.WriteString("0.2 w 0 0 0 RG\n")
	b.WriteString("30 30 732 552 re S\n")
	if withGraphics {
		b.WriteString(circlePath(150, 140, 40))
		for i := 0; i < 6; i++ {
			a := float64(i) * math.Pi / 3
			b.WriteString(circlePath(150+22*math.Cos(a), 140+22*math.Sin(a), 5))
		}
		b.WriteString("120 140 m 60 140 l S\n")
		b.WriteString("q " + imageCM + " cm /Im1 Do Q\n")
	}
	if glyphMode {
		type g struct {
			x, y, s float64
			c       string
		}
		var gs []g
		for _, t := range texts {
			for i, c := range t.s {
				gs = append(gs, g{t.x + float64(i)*0.6*t.size, t.y, t.size, string(c)})
			}
		}
		for i := len(gs) - 1; i >= 0; i-- {
			if gs[i].c == " " {
				continue
			}
			fmt.Fprintf(&b, "BT /%s %.2f Tf 1 0 0 1 %.2f %.2f Tm (%s) Tj ET\n", font, gs[i].s, gs[i].x, gs[i].y, pdfEsc(gs[i].c))
		}
		return b.String()
	}
	for _, t := range texts {
		if t.s == "" {
			continue
		}
		fmt.Fprintf(&b, "BT /%s %.2f Tf 1 0 0 1 %.2f %.2f Tm (%s) Tj ET\n", font, t.size, t.x, t.y, pdfEsc(t.s))
	}
	return b.String()
}

// topdownOps emits the same drawing the way Y-down CAD exporters do.
func topdownOps(texts []synthText, withGraphics bool) string {
	var b strings.Builder
	b.WriteString("1 0 0 -1 0 612 cm\n0.2 w 0 0 0 RG\n30 30 732 552 re S\n")
	flipY := func(y float64) float64 { return 612 - y }
	if withGraphics {
		// circles are symmetric: draw in flipped coordinates directly
		b.WriteString(circlePath(150, flipY(140), 40))
		for i := 0; i < 6; i++ {
			a := float64(i) * math.Pi / 3
			b.WriteString(circlePath(150+22*math.Cos(a), flipY(140+22*math.Sin(a)), 5))
		}
		b.WriteString(fmt.Sprintf("120 %.2f m 60 %.2f l S\n", flipY(140), flipY(140)))
		// Image rows are stored bottom-up and placed with a POSITIVE height in
		// the flipped system, so the viewer shows them upright.
		fmt.Fprintf(&b, "q 120 0 0 80 500 %.2f cm /Im1 Do Q\n", flipY(380+80))
	}
	for _, t := range texts {
		if t.s == "" {
			continue
		}
		fmt.Fprintf(&b, "BT /F1 %.2f Tf 1 0 0 -1 %.2f %.2f Tm (%s) Tj ET\n", t.size, t.x, flipY(t.y), pdfEsc(t.s))
	}
	return b.String()
}

// BuildSyntheticVariant returns a 2-page PDF for the named variant.
func BuildSyntheticVariant(name string) ([]byte, error) {
	var v *synthVariant
	for i := range synthVariants {
		if synthVariants[i].Name == name {
			v = &synthVariants[i]
		}
	}
	if v == nil {
		return nil, fmt.Errorf("unknown synthetic variant %q", name)
	}
	p1, p2 := synthDesign()
	w := &pdfWriter{}
	catalog := w.reserve()
	pages := w.reserve()
	helv := w.add("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	cour := w.add("<< /Type /Font /Subtype /Type1 /BaseFont /Courier /Encoding /WinAnsiEncoding >>")
	const iw, ih = 48, 32
	img := w.stream(fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8", iw, ih), synthImagePixels(iw, ih, v.RawFlipped && v.Name == "topdown"))
	fontRes := fmt.Sprintf("/Font << /F1 %d 0 R /F2 %d 0 R >>", helv, cour)
	imageCM := "120 0 0 80 500 380"
	if v.ImageMirrored {
		imageCM = "-120 0 0 80 620 380" // mirrored left-right on the page
	}
	font := "F1"
	if v.Name == "glyphs" {
		font = "F2"
	}
	mediaLandscape := "[0 0 792 612]"
	mediaPortrait := "[0 0 612 792]"
	var kids []string
	for sheet, texts := range [][]synthText{p1, p2} {
		gfx := sheet == 0
		ops := designOps(texts, gfx, font, v.Name == "glyphs", imageCM)
		media, rotate, wrap := mediaLandscape, 0, ""
		switch v.Name {
		case "rotate90":
			media, rotate, wrap = mediaPortrait, 90, "0 1 -1 0 612 0"
		case "rotate180":
			rotate, wrap = 180, "-1 0 0 -1 792 612"
		case "rotate270":
			media, rotate, wrap = mediaPortrait, 270, "0 -1 1 0 0 792"
		case "sideways":
			media, wrap = mediaPortrait, "0 1 -1 0 612 0"
		case "topdown":
			ops = topdownOps(texts, gfx)
		}
		resources := fmt.Sprintf("<< %s /XObject << /Im1 %d 0 R >> >>", fontRes, img)
		var content string
		if v.Name == "nestedforms" {
			inner := w.stream(fmt.Sprintf("/Type /XObject /Subtype /Form /BBox [0 0 792 612] /Matrix [-1 0 0 -1 792 612] /Resources %s", resources),
				[]byte(ops))
			outer := w.stream(fmt.Sprintf("/Type /XObject /Subtype /Form /BBox [0 0 792 612] /Matrix [-1 0 0 -1 792 612] /Resources << /XObject << /FmA %d 0 R >> >>", inner),
				[]byte("q /FmA Do Q\n"))
			resources = fmt.Sprintf("<< /XObject << /FmB %d 0 R >> >>", outer)
			content = "q /FmB Do Q\n"
		} else if v.Name == "nestedmixed" {
			media, rotate = mediaPortrait, 90
			inner := w.stream(fmt.Sprintf("/Type /XObject /Subtype /Form /BBox [-200 -200 1000 1000] /Matrix [1 0 0 1 100 50] /Resources %s", resources),
				[]byte("q 1 0 0 1 -100 -50 cm\n"+ops+"Q\n"))
			outer := w.stream(fmt.Sprintf("/Type /XObject /Subtype /Form /BBox [-200 -200 1000 1000] /Matrix [0 1 -1 0 612 0] /Resources << /XObject << /FmA %d 0 R >> >>", inner),
				[]byte("q /FmA Do Q\n"))
			resources = fmt.Sprintf("<< /XObject << /FmB %d 0 R >> >>", outer)
			content = "q /FmB Do Q\n"
		} else if wrap != "" {
			content = "q " + wrap + " cm\n" + ops + "Q\n"
		} else {
			content = ops
		}
		cs := w.stream("", []byte(content))
		pg := w.add(fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox %s /Rotate %d /Resources %s /Contents %d 0 R >>", pages, media, rotate, resources, cs))
		kids = append(kids, fmt.Sprintf("%d 0 R", pg))
	}
	w.set(pages, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(kids)))
	w.set(catalog, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pages))
	return w.bytes(catalog), nil
}

// BuildSyntheticOutlinedText returns a sheet whose "lettering" is drawn as
// thousands of tiny filled vector strokes (as CAD exporters do with SHX
// fonts) plus a picture, and NO machine-readable text at all. A correct
// auditor must report a FAILED interpretation, never an empty drawing.
func BuildSyntheticOutlinedText() []byte {
	w := &pdfWriter{}
	catalog := w.reserve()
	pages := w.reserve()
	img := w.stream("/Type /XObject /Subtype /Image /Width 48 /Height 32 /ColorSpace /DeviceRGB /BitsPerComponent 8", synthImagePixels(48, 32, false))
	var b strings.Builder
	b.WriteString("0 0 0 rg\n")
	for row := 0; row < 40; row++ {
		for col := 0; col < 45; col++ {
			// one stroke per "glyph" as its own path object
			fmt.Fprintf(&b, "%.1f %.1f 2.2 4.5 re f\n", 60+float64(col)*6.5, 560-float64(row)*11)
		}
	}
	b.WriteString("q 120 0 0 80 500 380 cm /Im1 Do Q\n")
	cs := w.stream("", []byte(b.String()))
	pg := w.add(fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 792 612] /Resources << /XObject << /Im1 %d 0 R >> >> /Contents %d 0 R >>", pages, img, cs))
	w.set(pages, fmt.Sprintf("<< /Type /Pages /Kids [%d 0 R] /Count 1 >>", pg))
	w.set(catalog, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pages))
	return w.bytes(catalog)
}

// BuildSyntheticASME returns a NON-CUI 3-sheet harness drawing laid out the
// way many defense drawings are (ASME Y14.24/Y14.34 conventions):
//   - sheet 1: connector table (REF DES / FIND NO. / DESCRIPTION) and a
//     per-connector pin table whose headings are stacked on two lines;
//   - sheet 2: wire list with grouped headings FROM -> REF DES | PIN and
//     TO -> REF DES | PIN, plus WIRE NO. / SIGNAL NAME / GAGE / COLOR;
//   - sheet 3: parts list built UPWARD from the title block: the heading row
//     is at the BOTTOM, headings are two lines (QTY/REQD, PART OR/IDENTIFYING
//     NO., ...), FIND NO. is the right-most column, and "PARTS LIST" sits
//     under the heading. It includes a sealing plug and a protective cap that
//     must NOT become connectors.
func BuildSyntheticASME() []byte {
	type T = synthText
	t := func(x, y, s float64, str string) T { return T{x, y, s, str} }
	sheet1 := []T{
		t(40, 750, 10, "NON-CUI SYNTHETIC ASME-STYLE HARNESS / SHEET 1 OF 3"),
		t(80, 690, 9, "CONNECTOR TABLE"),
		t(80, 672, 7, "REF"), t(80, 664, 7, "DES"), t(130, 672, 7, "FIND"), t(130, 664, 7, "NO."), t(180, 668, 7, "DESCRIPTION"),
		t(80, 650, 7, "P1"), t(135, 650, 7, "1"), t(180, 650, 7, "CONNECTOR, PLUG, 128 POS"),
		t(80, 638, 7, "J2"), t(135, 638, 7, "5"), t(180, 638, 7, "CONNECTOR, RECEPTACLE, 6 POS"),
		t(600, 690, 9, "P1 PIN-OUT"),
		t(600, 672, 7, "CONT"), t(600, 664, 7, "NO."), t(650, 672, 7, "SIGNAL"), t(650, 664, 7, "NAME"), t(740, 672, 7, "WIRE"), t(740, 664, 7, "NO."), t(800, 668, 7, "TO"),
		t(600, 650, 7, "1"), t(650, 650, 7, "PWR_28V"), t(740, 650, 7, "W1"), t(800, 650, 7, "J2-A"),
		t(600, 638, 7, "2"), t(650, 638, 7, "PWR_RTN"), t(740, 638, 7, "W2"), t(800, 638, 7, "J2-B"),
		t(600, 626, 7, "3"), t(650, 626, 7, "CAN_H"), t(740, 626, 7, "W3"), t(800, 626, 7, "J2-C"),
		t(600, 614, 7, "128"), t(650, 614, 7, "SPARE"), t(740, 614, 7, "-"), t(800, 614, 7, "-"),
		t(90, 420, 9, "P1"), t(400, 420, 9, "J2"),
		t(40, 120, 7, "NOTES:"), t(40, 108, 7, "1. ALL CONTACTS PER M39029. SEAL UNUSED CAVITIES PER FIND NO. 3."),
	}
	sheet2 := []T{
		t(40, 750, 10, "NON-CUI SYNTHETIC ASME-STYLE HARNESS / SHEET 2 OF 3"),
		t(80, 690, 9, "WIRE LIST"),
		t(170, 676, 7, "FROM"), t(290, 676, 7, "TO"),
		t(80, 664, 7, "WIRE NO."), t(150, 664, 7, "REF DES"), t(200, 664, 7, "PIN"), t(270, 664, 7, "REF DES"), t(320, 664, 7, "PIN"), t(370, 664, 7, "SIGNAL NAME"), t(470, 664, 7, "GAGE"), t(510, 664, 7, "COLOR"),
		t(80, 650, 7, "W1"), t(155, 650, 7, "P1"), t(202, 650, 7, "1"), t(275, 650, 7, "J2"), t(322, 650, 7, "A"), t(370, 650, 7, "PWR_28V"), t(472, 650, 7, "22"), t(510, 650, 7, "WHT"),
		t(80, 638, 7, "W2"), t(155, 638, 7, "P1"), t(202, 638, 7, "2"), t(275, 638, 7, "J2"), t(322, 638, 7, "B"), t(370, 638, 7, "PWR_RTN"), t(472, 638, 7, "22"), t(510, 638, 7, "BLK"),
		t(80, 626, 7, "W3"), t(155, 626, 7, "P1"), t(202, 626, 7, "3"), t(275, 626, 7, "J2"), t(322, 626, 7, "C"), t(370, 626, 7, "CAN_H"), t(472, 626, 7, "24"), t(510, 626, 7, "WHT/GRN"),
		t(80, 614, 7, "W4"), t(155, 614, 7, "P1"), t(202, 614, 7, "4"), t(275, 614, 7, "J2"), t(322, 614, 7, "D"), t(370, 614, 7, "CAN_L"), t(472, 614, 7, "24"), t(510, 614, 7, "WHT/BLU"),
	}
	sheet3 := []T{
		t(40, 750, 10, "NON-CUI SYNTHETIC ASME-STYLE HARNESS / SHEET 3 OF 3"),
		// rows stacked upward (item 1 nearest the heading)
		t(620, 298, 7, "A/R"), t(670, 298, 7, "81349"), t(730, 298, 7, "M23053/5-104-0"), t(880, 298, 7, "SLEEVING, HEAT SHRINK"), t(1120, 298, 7, "6"),
		t(620, 286, 7, "1"), t(670, 286, 7, "77820"), t(730, 286, 7, "D38999/26FA35PN"), t(880, 286, 7, "CONNECTOR, RECEPTACLE"), t(1120, 286, 7, "5"),
		t(620, 274, 7, "1"), t(670, 274, 7, "77820"), t(730, 274, 7, "D38999/32F25R"), t(880, 274, 7, "CAP, PROTECTIVE"), t(1120, 274, 7, "4"),
		t(620, 262, 7, "AR"), t(670, 262, 7, "96906"), t(730, 262, 7, "MS27488-22-2"), t(880, 262, 7, "PLUG, SEALING"), t(1120, 262, 7, "3"),
		t(620, 250, 7, "128"), t(670, 250, 7, "81349"), t(730, 250, 7, "M39029/56-348"), t(880, 250, 7, "CONTACT, ELECTRICAL, SOCKET"), t(1120, 250, 7, "2"),
		t(620, 238, 7, "1"), t(670, 238, 7, "77820"), t(730, 238, 7, "D38999/26FJ35SN"), t(880, 238, 7, "CONNECTOR, PLUG"), t(1120, 238, 7, "1"),
		// two-line heading at the BOTTOM of the table
		t(620, 222, 7, "QTY"), t(620, 214, 7, "REQD"), t(670, 222, 7, "CAGE"), t(670, 214, 7, "CODE"), t(730, 222, 7, "PART OR"), t(730, 214, 7, "IDENTIFYING NO."), t(880, 222, 7, "NOMENCLATURE"), t(880, 214, 7, "OR DESCRIPTION"), t(1120, 222, 7, "FIND"), t(1120, 214, 7, "NO."),
		t(840, 200, 8, "PARTS LIST"),
		// title block below
		t(620, 160, 7, "CAGE CODE"), t(700, 160, 7, "SIZE"), t(760, 160, 7, "DWG NO."), t(620, 140, 9, "00000"), t(700, 140, 9, "B"), t(760, 140, 9, "SYN-0001"), t(1000, 140, 7, "SCALE NONE"), t(1080, 140, 7, "SHEET 3 OF 3"),
	}
	w := &pdfWriter{}
	catalog := w.reserve()
	pages := w.reserve()
	helv := w.add("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	var kids []string
	for _, texts := range [][]T{sheet1, sheet2, sheet3} {
		var b strings.Builder
		b.WriteString("0.4 w 0 0 0 RG 20 20 1184 752 re S\n")
		for _, x := range texts {
			fmt.Fprintf(&b, "BT /F1 %.2f Tf 1 0 0 1 %.2f %.2f Tm (%s) Tj ET\n", x.size, x.x, x.y, pdfEsc(x.s))
		}
		cs := w.stream("", []byte(b.String()))
		pg := w.add(fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 1224 792] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>", pages, helv, cs))
		kids = append(kids, fmt.Sprintf("%d 0 R", pg))
	}
	w.set(pages, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(kids)))
	w.set(catalog, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pages))
	return w.bytes(catalog)
}

// BuildSyntheticUnknownHeadings uses heading words that are NOT in the
// built-in vocabulary, to prove that a format can be taught once and then
// read automatically.
func BuildSyntheticUnknownHeadings() []byte {
	var b strings.Builder
	cell := func(x, y float64, s string) {
		fmt.Fprintf(&b, "BT /F1 7 Tf 1 0 0 1 %.1f %.1f Tm (%s) Tj ET\n", x, y, pdfEsc(s))
	}
	cell(60, 740, "NON-CUI SYNTHETIC — HOUSE-STANDARD HEADINGS")
	// parts list
	for i, h := range []string{"LINE", "AMOUNT", "IDENT", "NOUN"} {
		cell([]float64{60, 110, 170, 300}[i], 700, h)
	}
	rows := [][]string{{"1", "1", "D38999/26WD35SN", "CONNECTOR, PLUG"}, {"2", "22", "M39029/56-348", "CONTACT, SOCKET"}, {"3", "1", "M85049/38S15W", "CLAMP, STRAIN RELIEF"}}
	for r, row := range rows {
		for i, v := range row {
			cell([]float64{60, 110, 170, 300}[i], 686-float64(r)*12, v)
		}
	}
	// wire table
	for i, h := range []string{"CKT ID", "ORIGIN", "CNTC", "TERMINUS", "CNTC", "FUNC"} {
		cell([]float64{60, 120, 180, 230, 290, 340}[i], 600, h)
	}
	wr := [][]string{{"C1", "P4", "1", "J9", "7", "DATA_P"}, {"C2", "P4", "2", "J9", "8", "DATA_N"}, {"C3", "P4", "3", "J9", "9", "SHLD"}}
	for r, row := range wr {
		for i, v := range row {
			cell([]float64{60, 120, 180, 230, 290, 340}[i], 586-float64(r)*12, v)
		}
	}
	w := &pdfWriter{}
	cat, pages := w.reserve(), w.reserve()
	f := w.add("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	cs := w.stream("", []byte(b.String()))
	pg := w.add(fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>", pages, f, cs))
	w.set(pages, fmt.Sprintf("<< /Type /Pages /Kids [%d 0 R] /Count 1 >>", pg))
	w.set(cat, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pages))
	return w.bytes(cat)
}

// BuildSyntheticFaceViews draws connector P3 (8 cavities) twice: front
// (mating face) and rear (wire side). Wired cavities are solid circles,
// unused are open. A pin table marks 4 SPARE and 7 N/C; a note lists the
// unused cavities. With mirrored=false the rear view is (wrongly) drawn
// identical to the front, which must be reported.
func BuildSyntheticFaceViews(mirrored bool) []byte {
	var b strings.Builder
	txt := func(x, y, s float64, t string) {
		fmt.Fprintf(&b, "BT /F1 %.1f Tf 1 0 0 1 %.1f %.1f Tm (%s) Tj ET\n", s, x, y, pdfEsc(t))
	}
	wired := map[int]bool{1: true, 2: true, 3: true, 5: true, 6: true}
	view := func(cx, cy float64, sign float64, caption string) {
		b.WriteString("0 0 0 RG 0.6 w\n" + circlePath(cx, cy, 40))
		for k := 0; k < 8; k++ {
			a := float64(k) * math.Pi / 4
			x, y := cx+sign*22*math.Cos(a), cy+22*math.Sin(a)
			if wired[k+1] {
				b.WriteString("0 0 0 rg\n" + strings.Replace(circlePath(x, y, 4), " h S\n", " h f\n", 1))
			} else {
				b.WriteString(circlePath(x, y, 4))
			}
			txt(x+5, y+3, 5, fmt.Sprint(k+1))
		}
		txt(cx-45, cy-55, 7, caption)
	}
	txt(40, 740, 10, "NON-CUI SYNTHETIC FACE VIEWS")
	view(150, 450, 1, "P3 FRONT VIEW (MATING FACE)")
	sign := -1.0
	if !mirrored {
		sign = 1
	}
	view(350, 450, sign, "P3 REAR VIEW (WIRE SIDE)")
	txt(60, 330, 8, "P3 PIN-OUT")
	for i, h := range []string{"CAV", "SIGNAL", "WIRE", "TO"} {
		txt([]float64{60, 100, 170, 220}[i], 315, 7, h)
	}
	rows := [][]string{{"1", "CAN_H", "W31", "J7-1"}, {"2", "CAN_L", "W32", "J7-2"}, {"3", "PWR", "W33", "J7-3"}, {"4", "SPARE", "-", "-"}, {"5", "RTN", "W35", "J7-5"}, {"6", "CHASSIS", "W36", "J7-6"}, {"7", "N/C", "-", "-"}}
	for r, row := range rows {
		for i, v := range row {
			txt([]float64{60, 100, 170, 220}[i], 302-float64(r)*12, 7, v)
		}
	}
	txt(60, 180, 7, "NOTES:")
	txt(60, 168, 7, "1. UNUSED CAVITIES P3: 4, 7, 8. FILL WITH CONTACT AND SEALING PLUG.")
	w := &pdfWriter{}
	cat, pages := w.reserve(), w.reserve()
	f := w.add("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	cs := w.stream("", []byte(b.String()))
	pg := w.add(fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>", pages, f, cs))
	w.set(pages, fmt.Sprintf("<< /Type /Pages /Kids [%d 0 R] /Count 1 >>", pg))
	w.set(cat, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pages))
	return w.bytes(cat)
}
