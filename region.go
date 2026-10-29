package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"math"
	"regexp"
	"sort"
	"strings"

)

// AppVersion is the single version string shown by the backend and the UI.
const AppVersion = "2.8.1-candidate"

// ---------------------------------------------------------------------------
// Region review: the user boxes an area of a rendered sheet (BOM, pinout,
// connector view). The backend returns what is machine-readable inside it,
// reconstructed as a grid, plus graphics that overlap it. The UI then lets
// the user assign column roles and import rows into the SAME connector, net
// and BOM workflows, with page/region provenance.

type RegionRequest struct {
	Page int    `json:"page"`
	Box  Box    `json:"box"`
	Kind string `json:"kind"` // bom | pinout | connector | text
}

type RegionGraphic struct {
	Kind   string `json:"kind"`
	Box    Box    `json:"box"`
	Detail string `json:"detail"`
}

type RegionResult struct {
	Page       int             `json:"page"`
	Box        Box             `json:"box"`
	Kind       string          `json:"kind"`
	Fragments  int             `json:"fragments"`
	Frame      int             `json:"frame"`
	Lines      []string        `json:"lines"`
	Grid       [][]string      `json:"grid"`
	Roles      []string        `json:"roles"`
	HeaderRow  int             `json:"headerRow"`
	Tags       []string        `json:"tags"`
	Parts      []string        `json:"parts"`
	AutoWires  []WireRow       `json:"autoWires"`
	AutoBOM    []BOMRow        `json:"autoBom"`
	Graphics   []RegionGraphic `json:"graphics"`
	Note       string          `json:"note"`
}

var reGenericPN = regexp.MustCompile(`(?i)\b(?:D38999/[0-9A-Z-]{7,22}|M[0-9]{4,5}/[0-9A-Z-]{2,20}|MS[0-9]{4,5}[A-Z0-9-]{2,20}|[A-Z]{1,4}[0-9]{2,}[-/][0-9A-Z-/]{2,24}|[0-9]{3,}-[0-9A-Z-]{2,20})\b`)

func analyzeRegion(x *DocExtraction, req RegionRequest) (RegionResult, error) {
	if req.Page < 1 || req.Page > len(x.Layouts) {
		return RegionResult{}, fmt.Errorf("page out of range")
	}
	pl := x.Layouts[req.Page-1]
	b := req.Box
	if b.X1 < b.X0 {
		b.X0, b.X1 = b.X1, b.X0
	}
	if b.Y1 < b.Y0 {
		b.Y0, b.Y1 = b.Y1, b.Y0
	}
	res := RegionResult{Page: req.Page, Box: roundBox(b), Kind: req.Kind, Lines: []string{}, Grid: [][]string{}, Roles: []string{}, Tags: []string{}, Parts: []string{}, AutoWires: []WireRow{}, AutoBOM: []BOMRow{}, Graphics: []RegionGraphic{}}
	sub := PageLayout{Page: pl.Page, Width: pl.Width, Height: pl.Height, Rotation: pl.Rotation, Words: []LayoutWord{}}
	sub.Text.DirectionShare = map[string]int{}
	for _, w := range pl.Words {
		if b.Contains(w.Box.CX(), w.Box.CY()) {
			sub.Words = append(sub.Words, w)
			k := "other"
			for _, a := range []float64{0, 90, 180, 270} {
				if math.Abs(math.Mod(w.Angle-a+360, 360)) <= 4 || math.Abs(math.Mod(w.Angle-a+360, 360)-360) <= 4 {
					k = fmt.Sprint(a)
				}
			}
			sub.Text.DirectionShare[k] += len([]rune(w.Text))
		}
	}
	res.Fragments = len(sub.Words)
	for _, im := range pl.Images {
		if im.Box.Intersects(b) {
			res.Graphics = append(res.Graphics, RegionGraphic{"embedded picture", im.Box, im.Orientation})
		}
	}
	circles := 0
	for _, c := range pl.Circles {
		if b.Contains(c.CX, c.CY) {
			circles++
		}
	}
	if circles > 0 {
		res.Graphics = append(res.Graphics, RegionGraphic{"vector circles", b, fmt.Sprintf("%d circle(s) inside the selection (possible insert/cavity view; count is heuristic)", circles)})
	}
	if len(sub.Words) == 0 {
		res.Note = "No machine-readable text inside this selection. The content is a picture or lettering drawn as graphics. Read it from the rendered page and enter it manually; the entry will keep this sheet/region as its source."
		return res, nil
	}
	res.Frame = analysisFrame(sub)
	if res.Frame == 0 {
		// analysisFrame needs >=20 chars; for small selections use the majority direction.
		best, n := "0", sub.Text.DirectionShare["0"]
		for _, k := range []string{"90", "180", "270"} {
			if sub.Text.DirectionShare[k] > n {
				best, n = k, sub.Text.DirectionShare[k]
			}
		}
		fmt.Sscan(best, &res.Frame)
	}
	// Build the recognizer-frame page for this selection only.
	sub.Text.DirectionShare = map[string]int{fmt.Sprint(res.Frame): 1000}
	page := legacyPage(sub)
	lines := pageLines(page)
	for _, l := range lines {
		res.Lines = append(res.Lines, l.Text)
	}
	res.Grid, res.Roles, res.HeaderRow = gridFromLines(lines)
	seenT, seenP := map[string]bool{}, map[string]bool{}
	for _, w := range sub.Words {
		u := strings.ToUpper(w.Text)
		for _, tok := range strings.Fields(u) {
			tok = strings.Trim(tok, ":;,.()[]")
			if reLabelToken.MatchString(tok) && !seenT[tok] {
				seenT[tok] = true
				res.Tags = append(res.Tags, strings.ReplaceAll(tok, "-", ""))
			}
		}
		for _, pn := range reGenericPN.FindAllString(u, -1) {
			if !seenP[pn] && !reLabelToken.MatchString(pn) {
				seenP[pn] = true
				res.Parts = append(res.Parts, pn)
			}
		}
	}
	switch req.Kind {
	case "pinout":
		res.AutoWires = pinoutTableRows(page)
		if len(res.AutoWires) == 0 {
			res.AutoWires = findStructuredWireRows(page)
		}
		if res.AutoWires == nil {
			res.AutoWires = []WireRow{}
		}
	case "bom":
		// The user has asserted this is a parts list, so no title is required.
		page.Words = append(page.Words, Word{Text: "BILL OF MATERIALS", X: 0, Y: page.Height + 50, Size: 1})
		page.Height += 60
		if r := flexibleBOMRows(page); len(r) > 0 {
			res.AutoBOM = r
		} else if r := findTextBOMRows(page); len(r) > 0 {
			res.AutoBOM = r
		}
	}
	if res.Note == "" {
		res.Note = "Text read from the selection. Assign column roles, check every value against the rendered page, then import. Imported rows remain DRAFT."
	}
	return res, nil
}

// gridFromLines reconstructs columns from left edges. Columns start where a
// heading or a recurring left edge sits; widely spaced cells split.
func gridFromLines(lines []evidenceLine) ([][]string, []string, int) {
	if len(lines) == 0 {
		return [][]string{}, []string{}, -1
	}
	xs := []float64{}
	size := 0.0
	n := 0
	for _, l := range lines {
		for _, w := range l.Words {
			xs = append(xs, w.X)
			size += w.Size
			n++
		}
	}
	sort.Float64s(xs)
	em := math.Max(size/float64(max(n, 1)), 3)
	anchors := []float64{}
	for i, x := range xs {
		if i == 0 || x-xs[i-1] > 1.2*em {
			anchors = append(anchors, x)
		}
	}
	// Merge anchors used by only one word if they sit inside another column's text.
	if len(anchors) > 24 {
		anchors = anchors[:24]
	}
	col := func(x float64) int {
		idx := 0
		for i, a := range anchors {
			if x >= a-0.6*em {
				idx = i
			}
		}
		return idx
	}
	grid := [][]string{}
	for _, l := range lines {
		row := make([]string, len(anchors))
		for _, w := range l.Words {
			c := col(w.X)
			if row[c] != "" {
				row[c] += " "
			}
			row[c] += strings.TrimSpace(w.Text)
		}
		grid = append(grid, row)
	}
	// Drop columns that are empty in every row.
	keep := []int{}
	for c := range anchors {
		for _, r := range grid {
			if strings.TrimSpace(r[c]) != "" {
				keep = append(keep, c)
				break
			}
		}
	}
	out := [][]string{}
	for _, r := range grid {
		nr := []string{}
		for _, c := range keep {
			nr = append(nr, r[c])
		}
		out = append(out, nr)
	}
	// The heading is the first row in which at least two cells are known
	// column names; titles above it are kept but not used for roles.
	roles := make([]string, len(keep))
	header := -1
	for ri, r := range out {
		cand := make([]string, len(r))
		n := 0
		for i, h := range r {
			if cand[i] = suggestRole(h); cand[i] != "" {
				n++
			}
		}
		if n >= 2 {
			roles, header = cand, ri
			break
		}
	}
	return out, roles, header
}

func suggestRole(h string) string {
	n := normHeader(h)
	if k := normalizedHeader(h); k != "" {
		return map[string]string{"item": "item", "qty": "qty", "part": "part", "description": "description", "vendor": "vendor", "ref": "ref"}[k]
	}
	for _, hw := range pinHeaderWords {
		for _, c := range hw.words {
			if n == c {
				return string(hw.role)
			}
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Picture gallery from the RENDERED page, i.e. exactly as a PDF viewer shows it.

type GalleryImage struct {
	Page        int     `json:"page"`
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Box         Box     `json:"box"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	PixelWidth  int     `json:"pixelWidth"`
	PixelHeight int     `json:"pixelHeight"`
	Rotation    float64 `json:"rotation"`
	Mirrored    bool    `json:"mirrored"`
	Orientation string  `json:"orientation"`
	MIME        string  `json:"mime"`
	Data        string  `json:"data"`
	Caption     string  `json:"caption"`
}

func encodePNG(img image.Image, maxDim int) ([]byte, error) {
	b := img.Bounds()
	if maxDim > 0 && (b.Dx() > maxDim || b.Dy() > maxDim) {
		img = downscale(img, maxDim)
	}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func galleryFromSession(s *session) ([]GalleryImage, error) {
	_, x := s.get()
	if x == nil {
		return nil, fmt.Errorf("no drawing is loaded")
	}
	out := []GalleryImage{}
	total := 0
	for _, pl := range x.Layouts {
		if len(pl.Images) == 0 {
			continue
		}
		const scale = 2.0
		page, err := s.render(pl.Page, scale)
		if err != nil {
			return out, err
		}
		for _, im := range pl.Images {
			if im.Box.W() < 2 || im.Box.H() < 2 {
				continue
			}
			crop := cropDisplay(page, scale, im.Box, 0)
			data, err := encodePNG(crop, 640)
			if err != nil {
				continue
			}
			if total+len(data) > 24_000_000 || len(out) >= 64 {
				return out, nil
			}
			total += len(data)
			out = append(out, GalleryImage{Page: pl.Page, ID: im.ID, Name: fmt.Sprintf("Picture %d", im.ID), Box: im.Box, Width: int(math.Round(im.Box.W())), Height: int(math.Round(im.Box.H())), PixelWidth: im.PixelWidth, PixelHeight: im.PixelHeight, Rotation: im.Rotation, Mirrored: im.Mirrored, Orientation: im.Orientation, MIME: "image/png", Data: base64.StdEncoding.EncodeToString(data), Caption: "Shown as placed on the rendered page. Identity not determined (connector, logo or other artwork)."})
		}
	}
	return out, nil
}

// downscale is an area-averaging reducer (no external imaging dependency).
func downscale(src image.Image, maxDim int) *image.RGBA {
	b := src.Bounds()
	f := float64(maxDim) / math.Max(float64(b.Dx()), float64(b.Dy()))
	W, H := max(1, int(float64(b.Dx())*f)), max(1, int(float64(b.Dy())*f))
	dst := image.NewRGBA(image.Rect(0, 0, W, H))
	for y := 0; y < H; y++ {
		y0, y1 := b.Min.Y+int(float64(y)/f), b.Min.Y+int(float64(y+1)/f)
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < W; x++ {
			x0, x1 := b.Min.X+int(float64(x)/f), b.Min.X+int(float64(x+1)/f)
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, a, n uint32
			for yy := y0; yy < y1 && yy < b.Max.Y; yy++ {
				for xx := x0; xx < x1 && xx < b.Max.X; xx++ {
					cr, cg, cb, ca := src.At(xx, yy).RGBA()
					r, g, bl, a, n = r+cr, g+cg, bl+cb, a+ca, n+1
				}
			}
			if n == 0 {
				continue
			}
			i := dst.PixOffset(x, y)
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = uint8(r/n>>8), uint8(g/n>>8), uint8(bl/n>>8), uint8(a/n>>8)
		}
	}
	return dst
}
