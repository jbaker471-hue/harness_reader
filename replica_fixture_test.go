package main

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
)

// A NON-CUI synthetic drawing that reproduces the LAYOUT of a real five-sheet
// D-size harness drawing (D38999 128-position plug to plug, twisted shielded
// pairs drawn as a wiring diagram, ASME parts list built upward, flag notes).
// Every signal name, pin assignment, drawing number and company name is
// invented, and the counts differ from the original. Pin numbers are a seeded random 86 of 128.

type replicaWire struct {
	pinA, pinB int
	awg        string
	sigA, sigB string
	sheet      int
	cableItem  int
}

type replicaDrawing struct {
	pdf    []byte
	wires  []replicaWire
	used   []int
	unused []int
}

func replicaPins() (used, unused []int) {
	r := rand.New(rand.NewSource(20261104))
	perm := r.Perm(128)
	in := map[int]bool{}
	for _, i := range perm[:86] {
		in[i+1] = true
	}
	for i := 1; i <= 128; i++ {
		if in[i] {
			used = append(used, i)
		} else {
			unused = append(unused, i)
		}
	}
	return
}

func buildReplicaDrawing() replicaDrawing { return buildReplicaVariant("") }

// replicaColor: pairs alternate WHT/BLU; the "quad" variant cycles the
// four-conductor colours WHT, BLU, ORA, GRN as printed on real drawings.
func replicaColor(corrupt string, i int) string {
	switch corrupt {
	case "nocolour":
		return ""
	case "oddcolour":
		return []string{"XQ", "ZRP"}[i%2]
	}
	if corrupt == "quad" || strings.HasPrefix(corrupt, "nosheet3") || strings.HasPrefix(corrupt, "noheader3") || corrupt == "typo3" || corrupt == "quadnobom" {
		return []string{"WHT", "BLU", "ORA", "GRN"}[i%4]
	}
	return map[bool]string{true: "WHT", false: "BLU"}[i%2 == 0]
}

// corrupt: "ambiguous" prints a stray second pin label on one sheet-3 line;
// "onesided" leaves the far-end pin off one sheet-4 line.
func buildReplicaVariant(corrupt string) replicaDrawing {
	used, unused := replicaPins()
	r := rand.New(rand.NewSource(7))
	pins := append([]int(nil), used...)
	r.Shuffle(len(pins), func(i, j int) { pins[i], pins[j] = pins[j], pins[i] })
	var wires []replicaWire
	// sheet 3: 17 pairs 24 AWG; sheet 4: 16 pairs 24 AWG + 1 pair 22 AWG; sheet 5: 9 pairs 22 AWG
	plan := []struct {
		sheet, pairs int
		awg          string
		item         int
	}{{3, 17, "24", 4}, {4, 16, "24", 4}, {4, 1, "22", 5}, {5, 9, "22", 5}}
	k := 0
	for _, pl := range plan {
		for i := 0; i < pl.pairs; i++ {
			a, b := pins[k], pins[k+1]
			k += 2
			n := len(wires)/2 + 1
			wires = append(wires,
				replicaWire{a, a, pl.awg, fmt.Sprintf("SIG%02d_P", n), fmt.Sprintf("SIG%02d_P", n), pl.sheet, pl.item},
				replicaWire{b, b, pl.awg, fmt.Sprintf("SIG%02d_N", n), fmt.Sprintf("SIG%02d_N", n), pl.sheet, pl.item})
		}
	}
	const W, H = 2448.0, 1584.0
	w := &pdfWriter{}
	catalog := w.reserve()
	pages := w.reserve()
	helv := w.add("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	var kids []string
	page := func(ops string) {
		cs := w.stream("", []byte(ops))
		pg := w.add(fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 %.0f %.0f] /Resources << /Font << /F1 %d 0 R >> >> /Contents %d 0 R >>", pages, W, H, helv, cs))
		kids = append(kids, fmt.Sprintf("%d 0 R", pg))
	}
	text := func(b *strings.Builder, x, y, size float64, s string) {
		fmt.Fprintf(b, "BT /F1 %.2f Tf 1 0 0 1 %.2f %.2f Tm (%s) Tj ET\n", size, x, y, pdfEsc(s))
	}
	line := func(b *strings.Builder, x0, y0, x1, y1 float64) {
		fmt.Fprintf(b, "%.2f %.2f m %.2f %.2f l S\n", x0, y0, x1, y1)
	}
	rect := func(b *strings.Builder, x, y, ww, hh float64) {
		fmt.Fprintf(b, "%.2f %.2f %.2f %.2f re S\n", x, y, ww, hh)
	}
	frame := func(b *strings.Builder, sheet int) {
		b.WriteString("0.8 w 0 0 0 RG\n")
		rect(b, 30, 30, W-60, H-60)
		text(b, 2050, 1555, 9, "DWG NO SYN-0000")
		text(b, 2150, 1555, 9, fmt.Sprintf("SH %d REV B", sheet))
		text(b, 2050, 60, 9, fmt.Sprintf("SIZE D  CAGE 00000  SYN-0000  REV B  SHEET %d OF 5", sheet))
		text(b, 1500, 60, 7, "SYNTHETIC NON-CUI LAYOUT REPLICA FOR REGRESSION TESTING")
	}
	// ---- sheet 1: notes + parts list (heading at the bottom, rows built upward)
	{
		var b strings.Builder
		frame(&b, 1)
		y := 1480.0
		text(&b, 130, y, 9, "NOTE: UNLESS OTHERWISE SPECIFIED")
		notes := []struct {
			flag bool
			text []string
		}{
			{false, []string{"DIMENSIONS ARE NOMINAL."}},
			{false, []string{"DELETED"}},
			{false, []string{"MARK PARTS WITH BLACK CHARACTERS. TWO LINES ARE", "PERMITTED."}},
			{false, []string{"PUT THE ASSEMBLY LABEL NEAR THE MIDDLE OF THE CABLE."}},
			{false, []string{"ELECTRICAL REQUIREMENTS", "A. EACH CONDUCTOR SHALL MEASURE CONTINUOUS END TO END."}},
			{true, []string{"TERMINATE SHIELDS AT THE BACKSHELL WITH THE BAND, THEN TRIM", "THE REMAINING BRAID"}},
			{true, []string{"DELETED"}},
			{true, []string{"PLACE A MATING-INFORMATION LABEL NEAR EACH BACKSHELL."}},
			{true, []string{"PLACE THE CABLE LABEL WHERE SHOWN."}},
			{true, []string{"EVERY UNUSED CONTACT OPENING IS TO BE POPULATED CONTACTS OR SEALING PLUGS."}},
			{true, []string{"BOND EACH BRAID TO ITS BACKSHELL PER THE ACCESSORY", "INSTRUCTIONS."}},
			{true, []string{"BUILD UP THE CABLE UNDER THE STRAIN RELIEF WITH TAPE AS NEEDED."}},
			{false, []string{"CRIMP ALL CONTACTS WITH THE TOOLING SPECIFIED BY THE CONTACT MAKER."}},
			{true, []string{"CAPTURE THE OUTER BRAID UNDER THE BAND (ITEM 10)."}},
		}
		y -= 34
		for i, n := range notes {
			num := fmt.Sprint(i + 1)
			if n.flag {
				// flag-note triangle with the number inside
				fmt.Fprintf(&b, "%.2f %.2f m %.2f %.2f l %.2f %.2f l h S\n", 120.0, y-3, 140.0, y-3, 130.0, y+14)
				text(&b, 127, y, 8, num)
			} else {
				text(&b, 127, y, 8, num+".")
			}
			for j, t := range n.text {
				text(&b, 170, y-float64(j)*12, 8, t)
			}
			y -= 22 + float64(len(n.text)-1)*12
		}
		type item struct{ no, qty, unit, desc, ref, mfr, pn, remark string }
		items := []item{
			{"1", "1", "EA", "D38999 CONNECTOR 128P PLUG MALE, ALTERNATE KEYED", "RIG-P5", "AMPHENOL OR EQUIVALENT", "D38999/26FJ35PA", ""},
			{"2", "1", "EA", "D38999 CONNECTOR 128P PLUG FEMALE, ALTERNATE KEYED", "BOX-P5", "AMPHENOL OR EQUIVALENT", "D38999/26FJ35SA", ""},
			{"3", "2", "EA", "BANDING BACKSHELL", "", "GLENAIR OR EQUIVALENT", "M85049/20-25N", "USE WITH RIG-P5 AND BOX-P5"},
			{"4", "396", "FT", "CABLE, 24 AWG, 2 COND, SHIELDED, SILVER COATED COPPER", "", "MIL-SPEC", "M27500-24RC2S06", ""},
			{"5", "120", "FT", "CABLE, 22 AWG, 2 COND, SHIELDED, SILVER COATED COPPER", "", "MIL-SPEC", "M27500-22RC2S06", ""},
			{"6", "40", "FT", "LACING CORD, NYLON", "", "MIL-SPEC", "AA52081-C-5", "USE AS REQUIRED"},
			{"7", "18", "FT", "EXPANDABLE SLEEVING 3/4\" DIA", "", "SLEEVE CO", "SLV-075-BK", ""},
			{"8", "18", "FT", "SHIELD BRAID TUBE 1 1/8\" DIA", "", "BRAID CO", "BRD-1125", ""},
			{"9", "24", "IN", "HEAT SHRINK TUBE, WHITE 1 1/2\" DIA", "", "MIL-SPEC", "M23053/5-311-9", "USE AS REQUIRED"},
			{"10", "2", "EA", "BAND, 0.240\" DIA, 14\" LONG", "", "GLENAIR", "600-052", ""},
			{"11", "2", "FT", "HEAT SHRINK TUBE, BLACK 1 1/2\" DIA", "", "MIL-SPEC", "M23053/4-305-0", "USE AS REQUIRED"},
			{"12", "84", "EA", "22AWG SEALING PLUG", "", "AMPHENOL OR EQUIVALENT", "MS27488-22-2", ""},
			{"13", "2", "FT", "TAPE, WHITE, GLASS CLOTH, SILICONE ADHESIVE", "", "TAPE CO", "TAPE-WHT", "USE AS REQUIRED"},
		}
		xs := []float64{1240, 1270, 1310, 1350, 1700, 1760, 1900, 2100, 2260}
		x1 := 2420.0
		hy := 280.0 // heading baseline
		rowH := 24.0
		b.WriteString("0.6 w\n")
		top := hy + 16 + float64(len(items))*rowH
		for _, x := range xs {
			line(&b, x-6, hy-10, x-6, top)
		}
		line(&b, x1, hy-10, x1, top)
		line(&b, xs[0]-6, hy-10, x1, hy-10)
		line(&b, xs[0]-6, hy+16, x1, hy+16)
		heads := [][]string{{"ITEM", "NO"}, {"QTY"}, {"UNIT"}, {"DESCRIPTION"}, {"REF DES"}, {"SPECIFICATION/MANUFACTURER"}, {"MANUFACTURE P/N"}, {"COMPANY P/N"}, {"NOTES OR REMARKS"}}
		for i, h := range heads {
			for j, s := range h {
				text(&b, xs[i], hy+4-float64(j)*8, 6, s)
			}
		}
		text(&b, 1800, hy-24, 9, "PARTS LIST")
		if strings.Contains(corrupt, "nobom") {
			items = nil // the parts list is not readable (e.g. drawn as outlines)
		}
		for i, it := range items {
			ry := hy + 16 + float64(i)*rowH + 8
			line(&b, xs[0]-6, hy+16+float64(i+1)*rowH, x1, hy+16+float64(i+1)*rowH)
			cells := []string{it.no, it.qty, it.unit, it.desc, it.ref, it.mfr, it.pn, it.pn, it.remark}
			for c, s := range cells {
				if s == "" {
					continue
				}
				if c == 4 && strings.Contains(s, "-") { // REF DES wraps after the hyphen
					p := strings.SplitN(s, "-", 2)
					text(&b, xs[c], ry+4, 7, p[0]+"-")
					text(&b, xs[c], ry-5, 7, p[1])
					continue
				}
				text(&b, xs[c], ry, 7, s)
			}
		}
		page(b.String())
	}
	// ---- sheet 2: assembly view
	{
		var b strings.Builder
		frame(&b, 2)
		b.WriteString("0.6 w\n")
		line(&b, 180, 1000, 2280, 1000)
		text(&b, 1150, 1010, 8, "144 -0.0/+1.0")
		rect(&b, 180, 760, 260, 90)
		rect(&b, 2020, 760, 260, 90)
		rect(&b, 440, 780, 1580, 50)
		text(&b, 560, 800, 9, "RIG-P5")
		text(&b, 1860, 800, 9, "BOX-P5")
		text(&b, 1060, 812, 7, "LABEL")
		text(&b, 470, 690, 7, "SLEEVE OVER BACKSHELL")
		for _, c := range [][3]float64{{130, 720, 1}, {130, 680, 12}, {300, 720, 3}, {2330, 720, 2}, {2330, 680, 12}, {860, 900, 6}, {860, 860, 7}} {
			fmt.Fprint(&b, circlePath(c[0], c[1], 14))
			b.WriteString("S\n")
			text(&b, c[0]-4, c[1]-3, 9, fmt.Sprint(c[2]))
		}
		page(b.String())
	}
	// ---- sheets 3-5: wiring diagrams
	faceView := func(b *strings.Builder, cx, cy float64) {
		b.WriteString("0.5 w\n")
		fmt.Fprint(b, circlePath(cx, cy, 150))
		b.WriteString("S\n")
		n := 0
		for row := -6; row <= 6 && n < 128; row++ {
			for col := -6; col <= 6 && n < 128; col++ {
				x, y := cx+float64(col)*20+float64(row%2)*10, cy-float64(row)*20
				if math.Hypot(x-cx, y-cy) > 135 {
					continue
				}
				n++
				fmt.Fprint(b, circlePath(x, y, 5))
				b.WriteString("S\n")
				if n%9 == 1 { // only some cavities are numbered, as on the real view
					text(b, x+6, y+4, 5, fmt.Sprint(n))
				}
			}
		}
	}
	for sheet := 3; sheet <= 5; sheet++ {
		var b strings.Builder
		frame(&b, sheet)
		var ws []replicaWire
		for _, x := range wires {
			if x.sheet == sheet {
				ws = append(ws, x)
			}
		}
		lx, rx := 640.0, 1820.0 // connector block inner edges
		top := 1380.0
		// nosheet3: no designators on sheet 3; noheader3: no designators or part numbers;
		// typo3: sheet 3 labels the left connector with a different designator.
		hide3 := sheet == 3 && (strings.HasPrefix(corrupt, "nosheet3") || strings.HasPrefix(corrupt, "noheader3"))
		noPart3 := sheet == 3 && strings.HasPrefix(corrupt, "noheader3")
		leftTag := "RIG-P5"
		if corrupt == "typo3" && sheet == 3 {
			leftTag = "R1G-P5"
		}
		if !hide3 {
			text(&b, lx-50, top+40, 8, leftTag)
		}
		if !noPart3 {
			text(&b, lx-50, top+30, 7, "D38999/")
			text(&b, lx-50, top+20, 7, "26FJ35PA")
		}
		text(&b, lx-50, top+10, 7, "BENCH")
		if !hide3 {
			text(&b, rx, top+40, 8, "BOX-P5")
		}
		if !noPart3 {
			text(&b, rx, top+30, 7, "D38999/")
			text(&b, rx, top+20, 7, "26FJ35SA")
		}
		text(&b, rx, top+10, 7, "UNIT")
		b.WriteString("0.5 w\n")
		bottom := top - float64(len(ws))*25 - 40
		rect(&b, lx-14, bottom, 14, top-bottom)
		rect(&b, rx, bottom, 14, top-bottom)
		counts := map[int]int{}
		for _, x := range ws {
			counts[x.cableItem]++
		}
		items := []int{}
		for k := range counts {
			items = append(items, k)
		}
		sort.Ints(items)
		bx := 1340.0
		for _, it := range items {
			fmt.Fprint(&b, circlePath(bx, top+30, 10))
			b.WriteString("S\n")
			text(&b, bx-3, top+27, 8, fmt.Sprint(it))
			text(&b, bx+14, top+27, 8, fmt.Sprintf("%dx", counts[it]/2))
			bx += 80
		}
		y := top - 12
		for i, x := range ws {
			// signal names sit a little below the pin/color line, as on the real sheets
			text(&b, lx-150, y-5, 6, x.sigA)
			text(&b, lx+4, y, 6, fmt.Sprintf("%d  %s", x.pinA, replicaColor(corrupt, i)))
			text(&b, 960, y, 6, x.awg+" AWG")
			switch {
			case corrupt == "onesided" && sheet == 4 && i == 7:
				text(&b, rx-38, y, 6, replicaColor(corrupt, i))
			default:
				text(&b, rx-38, y, 6, fmt.Sprintf("%s  %d", replicaColor(corrupt, i), x.pinB))
			}
			if corrupt == "ambiguous" && sheet == 3 && i == 5 {
				text(&b, rx-90, y, 6, "WHT  99")
			}
			text(&b, rx+20, y-5, 6, x.sigB)
			line(&b, lx, y-2, rx, y-2)
			if i%2 == 1 { // pair: twist mark and shield oval
				line(&b, 1210, y+12, 1240, y-2)
				line(&b, 1210, y-2, 1240, y+12)
				fmt.Fprint(&b, circlePath(760, y+5, 7))
				b.WriteString("S\n")
				y -= 14
			}
			y -= 13
		}
		text(&b, lx+30, bottom+6, 5, "SHIELD")
		text(&b, rx-60, bottom+6, 5, "SHIELD")
		faceView(&b, 2200, 800)
		page(b.String())
	}
	w.set(pages, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(kids)))
	w.set(catalog, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pages))
	return replicaDrawing{pdf: w.bytes(catalog), wires: wires, used: used, unused: unused}
}
