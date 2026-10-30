package main

// PDFium-backed extraction foundation (v2.7).
//
// PDFium (chromium/7323) runs as WebAssembly inside this process through
// go-pdfium + wazero. There is no native DLL, no CGO, no helper process, no
// network access and NO filesystem access: the WebAssembly module is given an
// empty filesystem configuration and receives PDF bytes from memory only.
//
// Coordinate convention used by everything built on this file:
//
//   DISPLAY SPACE = the page exactly as a PDF viewer shows it, after /Rotate,
//   CropBox and all content transforms (cm, form /Matrix, text matrices,
//   mirrors). Units are PDF points. Origin is the TOP-LEFT corner, x grows to
//   the right and y grows DOWN. Every bounding box sent to the UI is in this
//   space, so overlays line up with the rendered page image.
//
// The legacy recognizers (analyze.go, flexible.go, table_scan.go) were written
// for a bottom-left/Y-up frame. They receive Word.X/Word.Y in the same display
// orientation but with Y measured upward (Y = displayHeight - yTop), so the
// existing heuristics ("Y > 65% of height is near the top") keep their meaning
// on rotated, flipped and form-nested pages.

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
)

const pdfiumEngineBase = "PDFium chromium/7323 (WebAssembly via go-pdfium v1.17.3, offline, no filesystem access"

// EngineName reports the engine and the WebAssembly execution mode in use.
func EngineName() string { return pdfiumEngineBase + "; " + engineMode + ")" }

var (
	engineOnce sync.Once
	enginePool pdfium.Pool
	engineInst pdfium.Pdfium
	engineErr  error
	engineMode = "compiled in memory"
	// One WebAssembly instance is not safe for concurrent use. All PDFium
	// calls are serialized through engineMu.
	engineMu sync.Mutex
)

// startEngine initializes PDFium once. Callers must hold engineMu.
func startEngine() (pdfium.Pdfium, error) {
	engineOnce.Do(func() {
		cfg := webassembly.Config{
			MinIdle: 1, MaxIdle: 1, MaxTotal: 1,
			// Empty, non-nil filesystem config: the default go-pdfium setup
			// mounts the host filesystem into the sandbox. We never need it.
			FSConfig: wazero.NewFSConfig(),
			// A Windows GUI-subsystem executable has no console: os.Stdout and
			// os.Stderr are invalid handles, and wazero refuses to start when
			// given them ("GetFileType /dev/stdout: The handle is invalid").
			// PDFium's console output is never needed, so discard it.
			Stdout: io.Discard,
			Stderr: io.Discard,
		}
		if os.Getenv("TRIVEX_PDF_INTERPRETER") == "1" {
			cfg.RuntimeConfig = wazero.NewRuntimeConfigInterpreter()
			engineMode = "interpreter (forced)"
		}
		if os.Getenv("TRIVEX_TEST_ENGINE_FAIL") == "1" { // test hook: simulate a broken engine
			engineErr = errors.New("simulated engine start failure (TRIVEX_TEST_ENGINE_FAIL)")
			return
		}
		enginePool, engineErr = webassembly.Init(cfg)
		if engineErr == nil {
			engineInst, engineErr = enginePool.GetInstance(60 * time.Second)
		}
		if engineErr != nil && cfg.RuntimeConfig == nil {
			// Hardened Windows policies (e.g. Arbitrary Code Guard) can block
			// in-memory native compilation. The interpreter needs no executable
			// memory; it is slower but produces identical results.
			if enginePool != nil {
				enginePool.Close()
			}
			firstErr := engineErr
			cfg.RuntimeConfig = wazero.NewRuntimeConfigInterpreter()
			engineMode = "interpreter (compiler unavailable: " + firstErr.Error() + ")"
			enginePool, engineErr = webassembly.Init(cfg)
			if engineErr == nil {
				engineInst, engineErr = enginePool.GetInstance(60 * time.Second)
			}
		}
	})
	return engineInst, engineErr
}

// WarmEngine compiles the PDFium module ahead of the first PDF so the first
// "Read PDF" click is not delayed. Safe to call from a goroutine.
func WarmEngine() error {
	engineMu.Lock()
	defer engineMu.Unlock()
	_, err := startEngine()
	return err
}

// CloseEngine releases the WebAssembly runtime during application shutdown.
func CloseEngine() {
	engineMu.Lock()
	defer engineMu.Unlock()
	if engineInst != nil {
		engineInst.Close()
		engineInst = nil
	}
	if enginePool != nil {
		enginePool.Close()
		enginePool = nil
	}
	// Once closed the engine is not restarted (sync.Once); late callers must
	// get an explicit error, never a nil instance.
	engineOnce.Do(func() {})
	if engineErr == nil {
		engineErr = errors.New("PDF engine has been shut down")
	}
}

// ---------------------------------------------------------------------------
// Geometry helpers

// affine maps (x,y) -> (A*x + C*y + E, B*x + D*y + F); PDF row-vector form.
type affine struct{ A, B, C, D, E, F float64 }

func identityAffine() affine { return affine{1, 0, 0, 1, 0, 0} }
func (m affine) apply(x, y float64) (float64, float64) {
	return m.A*x + m.C*y + m.E, m.B*x + m.D*y + m.F
}
func (m affine) vec(x, y float64) (float64, float64) { return m.A*x + m.C*y, m.B*x + m.D*y }

// then returns the transform "apply m, then n".
func (m affine) then(n affine) affine {
	return affine{
		A: m.A*n.A + m.B*n.C, B: m.A*n.B + m.B*n.D,
		C: m.C*n.A + m.D*n.C, D: m.C*n.B + m.D*n.D,
		E: m.E*n.A + m.F*n.C + n.E, F: m.E*n.B + m.F*n.D + n.F,
	}
}
func affineFrom32(v [6]float32) affine {
	return affine{float64(v[0]), float64(v[1]), float64(v[2]), float64(v[3]), float64(v[4]), float64(v[5])}
}

// Box is an axis-aligned rectangle in display space (top-left origin, points).
type Box struct {
	X0 float64 `json:"x0"`
	Y0 float64 `json:"y0"`
	X1 float64 `json:"x1"`
	Y1 float64 `json:"y1"`
}

func (b Box) W() float64  { return b.X1 - b.X0 }
func (b Box) H() float64  { return b.Y1 - b.Y0 }
func (b Box) CX() float64 { return (b.X0 + b.X1) / 2 }
func (b Box) CY() float64 { return (b.Y0 + b.Y1) / 2 }
func (b Box) Contains(x, y float64) bool {
	return x >= b.X0 && x <= b.X1 && y >= b.Y0 && y <= b.Y1
}
func (b Box) Intersects(o Box) bool {
	return b.X0 <= o.X1 && o.X0 <= b.X1 && b.Y0 <= o.Y1 && o.Y0 <= b.Y1
}
func (b Box) Union(o Box) Box {
	return Box{math.Min(b.X0, o.X0), math.Min(b.Y0, o.Y0), math.Max(b.X1, o.X1), math.Max(b.Y1, o.Y1)}
}
func boxOfPoints(pts ...[2]float64) Box {
	b := Box{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, p := range pts {
		b.X0, b.Y0 = math.Min(b.X0, p[0]), math.Min(b.Y0, p[1])
		b.X1, b.Y1 = math.Max(b.X1, p[0]), math.Max(b.Y1, p[1])
	}
	return b
}
func roundBox(b Box) Box {
	r := func(v float64) float64 { return math.Round(v*100) / 100 }
	return Box{r(b.X0), r(b.Y0), r(b.X1), r(b.Y1)}
}

// ---------------------------------------------------------------------------
// Extracted layout types (display space)

type LayoutWord struct {
	Text  string  `json:"text"`
	Box   Box     `json:"box"`
	Angle float64 `json:"angle"` // reading direction in display, degrees counter-clockwise, 0 = left-to-right
	Size  float64 `json:"size"`  // approximate em height in points
	// Origin of the first glyph's baseline in display space.
	OX float64 `json:"ox"`
	OY float64 `json:"oy"`
}

type ImagePlacement struct {
	ID          int     `json:"id"`
	Box         Box     `json:"box"`
	PixelWidth  int     `json:"pixelWidth"`
	PixelHeight int     `json:"pixelHeight"`
	Rotation    float64 `json:"rotation"`    // degrees the stored pixels are rotated to appear on the page
	Mirrored    bool    `json:"mirrored"`    // stored pixels are mirrored when displayed
	Orientation string  `json:"orientation"` // plain-language summary of stored vs displayed orientation
	FormDepth   int     `json:"formDepth"`   // nested Form XObject depth (0 = drawn directly on the page)
	matrix      affine  // unit square -> display space
	object      references.FPDF_PAGEOBJECT
}

type Circle struct {
	CX float64 `json:"cx"`
	CY float64 `json:"cy"`
	R  float64 `json:"r"`
	// Mark is how the drawing renders this circle: "solid" (dark fill),
	// "open" (outline or light fill). Its meaning (populated/unused) is
	// never assumed; it is shown to the reviewer.
	Mark string `json:"mark,omitempty"`
}

type CalloutPolygon struct {
	Points [][2]float64 `json:"points"`
	Box    Box          `json:"box"`
}

type Segment [4]float64 // x0,y0,x1,y1 display space

type TextStats struct {
	Chars              int            `json:"chars"`
	Decoded            int            `json:"decoded"`
	UnicodeMapErrors   int            `json:"unicodeMapErrors"`
	Unmapped           int            `json:"unmapped"`
	PrivateUse         int            `json:"privateUse"`
	DirectionShare     map[string]int `json:"directionShare"` // "0","90","180","270","other" -> chars
	Fonts              int            `json:"fonts"`
	DecodedRatio       float64        `json:"decodedRatio"`
	AnalysisRotation   int            `json:"analysisRotation"` // degrees applied to make dominant text upright for recognizers
	OutlinedTextLikely bool           `json:"outlinedTextLikely"`
}

type PageLayout struct {
	LeaderSegments []Segment        `json:"leaderSegments"`
	Polygons       []CalloutPolygon `json:"polygons"`

	Page                  int              `json:"page"`
	Width                 float64          `json:"width"`  // display width, points
	Height                float64          `json:"height"` // display height, points
	Rotation              int              `json:"rotation"`
	MediaBox              [4]float64       `json:"mediaBox"`
	CropBox               [4]float64       `json:"cropBox"`
	Words                 []LayoutWord     `json:"words"`
	Images                []ImagePlacement `json:"images"`
	Circles               []Circle         `json:"circles"`
	Segments              []Segment        `json:"segments"`
	PathObjects           int              `json:"pathObjects"`
	PathSegments          int              `json:"pathSegments"`
	FormObjects           int              `json:"formObjects"`
	TextObjects           int              `json:"textObjects"`
	VectorDetailTruncated bool             `json:"vectorDetailTruncated"`
	Text                  TextStats        `json:"text"`
	Notes                 []string         `json:"notes"`
	toDisplay             affine
}

// ---------------------------------------------------------------------------
// Document session

// pdfDoc is an open PDFium document. Bytes stay in process memory only.
type pdfDoc struct {
	inst  pdfium.Pdfium
	doc   references.FPDF_DOCUMENT
	data  []byte
	pages int
}

func openPDF(b []byte) (*pdfDoc, error) {
	inst, err := startEngine()
	if err != nil {
		return nil, fmt.Errorf("PDFium engine unavailable: %w", err)
	}
	data := append([]byte(nil), b...)
	d, err := inst.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		return nil, fmt.Errorf("PDFium could not open the document: %w", err)
	}
	pc, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: d.Document})
	if err != nil {
		inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: d.Document})
		return nil, err
	}
	return &pdfDoc{inst: inst, doc: d.Document, data: data, pages: pc.PageCount}, nil
}

func (d *pdfDoc) close() {
	if d != nil && d.inst != nil {
		d.inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: d.doc})
	}
}

// withPage loads a page, runs fn and always closes the page.
func (d *pdfDoc) withPage(index int, fn func(page references.FPDF_PAGE) error) error {
	p, err := d.inst.FPDF_LoadPage(&requests.FPDF_LoadPage{Document: d.doc, Index: index})
	if err != nil {
		return err
	}
	defer d.inst.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: p.Page})
	return fn(p.Page)
}

func pageRef(p references.FPDF_PAGE) requests.Page {
	return requests.Page{ByReference: &p}
}

// displayTransform derives the exact page-space -> display-space affine from
// PDFium's own FPDF_PageToDevice, so rotation and CropBox offsets match what
// PDFium (and therefore the rendered image) does.
func (d *pdfDoc) displayTransform(page references.FPDF_PAGE, w, h float64) (affine, error) {
	const k = 64.0 // sub-point precision; FPDF_PageToDevice returns integers
	sx, sy := int(math.Round(w*k)), int(math.Round(h*k))
	probe := func(x, y float64) (float64, float64, error) {
		r, err := d.inst.FPDF_PageToDevice(&requests.FPDF_PageToDevice{Page: pageRef(page), StartX: 0, StartY: 0, SizeX: sx, SizeY: sy, Rotate: enums.FPDF_PAGE_ROTATION_NONE, PageX: x, PageY: y})
		if err != nil {
			return 0, 0, err
		}
		return float64(r.DeviceX) / k, float64(r.DeviceY) / k, nil
	}
	ox, oy, err := probe(0, 0)
	if err != nil {
		return affine{}, err
	}
	const span = 4096.0
	xx, xy, err := probe(span, 0)
	if err != nil {
		return affine{}, err
	}
	yx, yy, err := probe(0, span)
	if err != nil {
		return affine{}, err
	}
	m := affine{A: (xx - ox) / span, B: (xy - oy) / span, C: (yx - ox) / span, D: (yy - oy) / span, E: ox, F: oy}
	// Snap tiny numeric noise so 90-degree rotations stay exact.
	snap := func(v float64) float64 {
		for _, t := range []float64{-1, 0, 1} {
			if math.Abs(v-t) < 2e-3 {
				return t
			}
		}
		return v
	}
	m.A, m.B, m.C, m.D = snap(m.A), snap(m.B), snap(m.C), snap(m.D)
	return m, nil
}

// ---------------------------------------------------------------------------
// Page extraction

const (
	maxSegmentsPerPage   = 40000  // overlay detail sent to the UI
	maxSegmentCallBudget = 400000 // PDFium segment calls per page before truncating vector detail
)

func (d *pdfDoc) layoutPage(index int) (PageLayout, error) {
	pl := PageLayout{Page: index + 1, Words: []LayoutWord{}, Images: []ImagePlacement{}, Circles: []Circle{}, Segments: []Segment{}, Notes: []string{}}
	err := d.withPage(index, func(page references.FPDF_PAGE) error {
		inst := d.inst
		wr, err := inst.FPDF_GetPageWidthF(&requests.FPDF_GetPageWidthF{Page: pageRef(page)})
		if err != nil {
			return err
		}
		hr, err := inst.FPDF_GetPageHeightF(&requests.FPDF_GetPageHeightF{Page: pageRef(page)})
		if err != nil {
			return err
		}
		pl.Width, pl.Height = float64(wr.PageWidth), float64(hr.PageHeight)
		if rr, err := inst.FPDFPage_GetRotation(&requests.FPDFPage_GetRotation{Page: pageRef(page)}); err == nil {
			pl.Rotation = int(rr.PageRotation) * 90
		}
		if mb, err := inst.FPDFPage_GetMediaBox(&requests.FPDFPage_GetMediaBox{Page: pageRef(page)}); err == nil {
			pl.MediaBox = [4]float64{float64(mb.Left), float64(mb.Bottom), float64(mb.Right), float64(mb.Top)}
		}
		if cb, err := inst.FPDFPage_GetCropBox(&requests.FPDFPage_GetCropBox{Page: pageRef(page)}); err == nil {
			pl.CropBox = [4]float64{float64(cb.Left), float64(cb.Bottom), float64(cb.Right), float64(cb.Top)}
		}
		pl.toDisplay, err = d.displayTransform(page, pl.Width, pl.Height)
		if err != nil {
			return err
		}
		if err := d.extractText(page, &pl); err != nil {
			pl.Notes = append(pl.Notes, "Text layer could not be read by PDFium: "+err.Error())
		}
		budget := maxSegmentCallBudget
		if err := d.walkObjects(page, &pl, &budget); err != nil {
			pl.Notes = append(pl.Notes, "Graphic objects could not be fully enumerated: "+err.Error())
		}
		detectOutlinedText(&pl)
		return nil
	})
	return pl, err
}

type glyph struct {
	r          rune
	box        Box     // tight glyph box, display space
	loose      Box     // advance/ascent box, display space (used for spacing)
	ox, oy     float64 // baseline origin, display space
	ux, uy     float64 // unit reading direction, display space (y down)
	em         float64
	angleIndex int    // 0,90,180,270 or -1
	obj        string // identity of the owning text object (one per text-show operation)
}

func (d *pdfDoc) extractText(page references.FPDF_PAGE, pl *PageLayout) error {
	inst := d.inst
	tp, err := inst.FPDFText_LoadPage(&requests.FPDFText_LoadPage{Page: pageRef(page)})
	if err != nil {
		return err
	}
	defer inst.FPDFText_ClosePage(&requests.FPDFText_ClosePage{TextPage: tp.TextPage})
	cnt, err := inst.FPDFText_CountChars(&requests.FPDFText_CountChars{TextPage: tp.TextPage})
	if err != nil {
		return err
	}
	pl.Text.DirectionShare = map[string]int{}
	fonts := map[string]bool{}
	glyphs := make([]glyph, 0, cnt.Count)
	M := pl.toDisplay
	for i := 0; i < cnt.Count; i++ {
		g, err := inst.FPDFText_IsGenerated(&requests.FPDFText_IsGenerated{TextPage: tp.TextPage, Index: i})
		if err == nil && g.IsGenerated {
			continue // PDFium-inserted spaces/newlines; geometry decides breaks
		}
		u, err := inst.FPDFText_GetUnicode(&requests.FPDFText_GetUnicode{TextPage: tp.TextPage, Index: i})
		if err != nil {
			continue
		}
		r := rune(u.Unicode)
		// PDFium reports a hyphen that ends a wrapped line as U+0002. It is a real hyphen on the drawing
		// (a reference designator broken as GSE-/P3), so keep it as one instead of dropping it.
		if r == 2 {
			if hy, herr := inst.FPDFText_IsHyphen(&requests.FPDFText_IsHyphen{TextPage: tp.TextPage, Index: i}); herr == nil && hy.IsHyphen {
				r = '-'
			}
		}
		if r == '\r' || r == '\n' {
			continue
		}
		if unicode.IsSpace(r) {
			continue // real spaces are re-derived from glyph gaps
		}
		pl.Text.Chars++
		if me, err := inst.FPDFText_HasUnicodeMapError(&requests.FPDFText_HasUnicodeMapError{TextPage: tp.TextPage, Index: i}); err == nil && me.HasUnicodeMapError {
			pl.Text.UnicodeMapErrors++
		}
		switch {
		case r == 0 || r == 0xFFFD || r == 0xFFFE:
			pl.Text.Unmapped++
			continue
		case (r >= 0xE000 && r <= 0xF8FF) || r < 0x20:
			pl.Text.PrivateUse++
			continue
		}
		cb, err := inst.FPDFText_GetCharBox(&requests.FPDFText_GetCharBox{TextPage: tp.TextPage, Index: i})
		if err != nil {
			continue
		}
		lb, err := inst.FPDFText_GetLooseCharBox(&requests.FPDFText_GetLooseCharBox{TextPage: tp.TextPage, Index: i})
		if err != nil {
			continue
		}
		org, err := inst.FPDFText_GetCharOrigin(&requests.FPDFText_GetCharOrigin{TextPage: tp.TextPage, Index: i})
		if err != nil {
			continue
		}
		mx, err := inst.FPDFText_GetMatrix(&requests.FPDFText_GetMatrix{TextPage: tp.TextPage, Index: i})
		if err != nil {
			continue
		}
		// go-pdfium hands out a new reference per call, so the owning text
		// object is identified by its bounds (identical for all its glyphs).
		owner := ""
		if to, err := inst.FPDFText_GetTextObject(&requests.FPDFText_GetTextObject{TextPage: tp.TextPage, Index: i}); err == nil {
			if ob, err := inst.FPDFPageObj_GetBounds(&requests.FPDFPageObj_GetBounds{PageObject: to.TextObject}); err == nil {
				owner = fmt.Sprintf("%.3f/%.3f/%.3f/%.3f", ob.Left, ob.Bottom, ob.Right, ob.Top)
			}
		}
		if fi, err := inst.FPDFText_GetFontInfo(&requests.FPDFText_GetFontInfo{TextPage: tp.TextPage, Index: i}); err == nil {
			fonts[fi.FontName] = true
		}
		pl.Text.Decoded++
		corners := func(l, b, rr, t float64) Box {
			x0, y0 := M.apply(l, b)
			x1, y1 := M.apply(rr, b)
			x2, y2 := M.apply(rr, t)
			x3, y3 := M.apply(l, t)
			return boxOfPoints([2]float64{x0, y0}, [2]float64{x1, y1}, [2]float64{x2, y2}, [2]float64{x3, y3})
		}
		tight := corners(cb.Left, cb.Bottom, cb.Right, cb.Top)
		loose := corners(float64(lb.Rect.Left), float64(lb.Rect.Bottom), float64(lb.Rect.Right), float64(lb.Rect.Top))
		ox, oy := M.apply(org.X, org.Y)
		dx, dy := M.vec(float64(mx.Matrix.A), float64(mx.Matrix.B))
		n := math.Hypot(dx, dy)
		if n < 1e-9 {
			dx, dy, n = 1, 0, 1
		}
		dx, dy = dx/n, dy/n
		// display angle, counter-clockwise visually (y axis points down)
		ang := math.Atan2(-dy, dx) * 180 / math.Pi
		if ang < 0 {
			ang += 360
		}
		ai := -1
		for _, a := range []int{0, 90, 180, 270, 360} {
			if math.Abs(ang-float64(a)) <= 4 {
				ai = a % 360
			}
		}
		key := "other"
		if ai >= 0 {
			key = fmt.Sprint(ai)
		}
		pl.Text.DirectionShare[key]++
		em := loose.H()
		if math.Abs(dy) > math.Abs(dx) {
			em = loose.W()
		}
		if em <= 0.1 {
			em = math.Max(tight.H(), tight.W())
		}
		if em <= 0.1 {
			em = 6
		}
		if tight.W() <= 0 && tight.H() <= 0 {
			tight = loose
		}
		glyphs = append(glyphs, glyph{obj: owner, r: r, box: tight, loose: loose, ox: ox, oy: oy, ux: dx, uy: dy, em: em, angleIndex: ai})
	}
	pl.Text.Fonts = len(fonts)
	if pl.Text.Chars > 0 {
		pl.Text.DecodedRatio = math.Round(float64(pl.Text.Decoded)/float64(pl.Text.Chars)*1000) / 1000
	}
	pl.Words = groupGlyphs(glyphs)
	return nil
}

// groupGlyphs clusters glyphs into text fragments purely by geometry: same
// reading direction, same baseline, small along-line gaps. Content-stream
// order is irrelevant, so glyph-by-glyph exporters and out-of-order CAD text
// produce the same fragments as a single Tj string would.
func groupGlyphs(gs []glyph) []LayoutWord {
	// CAD exporters may paint the same glyph twice (e.g. simulated bold).
	// Deduplicate only coincident identical geometry, never repeated characters
	// at distinct positions or differing letters at one position.
	clean := make([]glyph, 0, len(gs))
	buckets := map[[3]int][]int{}
	for _, g := range gs {
		cx, cy := int(math.Floor(g.ox/.2)), int(math.Floor(g.oy/.2))
		dup := false
		for dx := -1; dx <= 1; dx++ {
			for dy := -1; dy <= 1; dy++ {
				for _, i := range buckets[[3]int{int(g.r), cx + dx, cy + dy}] {
					h := clean[i]
					if math.Abs(h.ox-g.ox) <= .15 && math.Abs(h.oy-g.oy) <= .15 && math.Abs(h.box.W()-g.box.W()) <= .15 && math.Abs(h.box.H()-g.box.H()) <= .15 && h.angleIndex == g.angleIndex {
						dup = true
					}
				}
			}
		}
		if !dup {
			k := [3]int{int(g.r), cx, cy}
			buckets[k] = append(buckets[k], len(clean))
			clean = append(clean, g)
		}
	}
	gs = clean

	type line struct {
		ux, uy float64
		base   float64
		em     float64
		items  []int
	}
	lines := []*line{}
	for i, g := range gs {
		nx, ny := -g.uy, g.ux // normal to reading direction
		base := g.ox*nx + g.oy*ny
		var hit *line
		for _, l := range lines {
			if l.ux*g.ux+l.uy*g.uy < 0.996 { // > ~5 degrees apart
				continue
			}
			tol := 0.3 * math.Max(l.em, g.em)
			if math.Abs(l.base-base) <= tol {
				hit = l
				break
			}
		}
		if hit == nil {
			hit = &line{ux: g.ux, uy: g.uy, base: base, em: g.em}
			lines = append(lines, hit)
		}
		hit.items = append(hit.items, i)
	}
	words := []LayoutWord{}
	for _, l := range lines {
		along := func(i int) (float64, float64) {
			b := gs[i].loose
			if b.W() <= 0 && b.H() <= 0 {
				b = gs[i].box
			}
			pts := [][2]float64{{b.X0, b.Y0}, {b.X1, b.Y0}, {b.X0, b.Y1}, {b.X1, b.Y1}}
			lo, hi := math.Inf(1), math.Inf(-1)
			for _, p := range pts {
				v := p[0]*l.ux + p[1]*l.uy
				lo, hi = math.Min(lo, v), math.Max(hi, v)
			}
			return lo, hi
		}
		sort.SliceStable(l.items, func(a, b int) bool {
			ga, gb := gs[l.items[a]], gs[l.items[b]]
			return ga.ox*l.ux+ga.oy*l.uy < gb.ox*l.ux+gb.oy*l.uy
		})
		var cur *LayoutWord
		var curEnd float64
		var curObj string
		var sb strings.Builder
		flush := func() {
			if cur != nil {
				cur.Text = strings.TrimSpace(sb.String())
				if cur.Text != "" {
					cur.Box = roundBox(cur.Box)
					words = append(words, *cur)
				}
			}
			cur = nil
			sb.Reset()
		}
		for _, i := range l.items {
			g := gs[i]
			lo, hi := along(i)
			em := math.Max(g.em, 1)
			if cur != nil {
				gap := lo - curEnd
				// Separate text-show operations with a visible gap stay separate
				// fragments (as CAD tables/schematics emit one string per cell);
				// glyph-by-glyph output (touching objects) is joined.
				newObject := g.obj != "" && curObj != "" && g.obj != curObj
				if gap > 0.75*em || gap < -1.5*em || math.Abs(g.em-cur.Size) > 0.45*math.Max(g.em, cur.Size) || (newObject && gap > 0.15*em) {
					flush()
				} else if gap > 0.18*em {
					sb.WriteByte(' ')
				}
			}
			if cur == nil {
				ang := math.Atan2(-l.uy, l.ux) * 180 / math.Pi
				if ang < 0 {
					ang += 360
				}
				cur = &LayoutWord{Box: g.box, Angle: math.Round(ang*10) / 10, Size: math.Round(g.em*100) / 100, OX: math.Round(g.ox*100) / 100, OY: math.Round(g.oy*100) / 100}
				curEnd = hi
			}
			sb.WriteRune(g.r)
			curObj = g.obj
			cur.Box = cur.Box.Union(g.box)
			if hi > curEnd {
				curEnd = hi
			}
		}
		flush()
	}
	sort.SliceStable(words, func(i, j int) bool {
		if math.Abs(words[i].OY-words[j].OY) > 2 {
			return words[i].OY < words[j].OY
		}
		return words[i].OX < words[j].OX
	})
	return words
}

// walkObjects enumerates page objects, descending into Form XObjects and
// composing their matrices, so image and vector placements are expressed in
// display space exactly as rendered.
func (d *pdfDoc) walkObjects(page references.FPDF_PAGE, pl *PageLayout, budget *int) error {
	inst := d.inst
	n, err := inst.FPDFPage_CountObjects(&requests.FPDFPage_CountObjects{Page: pageRef(page)})
	if err != nil {
		return err
	}
	imageID := 0
	var visit func(obj references.FPDF_PAGEOBJECT, parent affine, depth int)
	visit = func(obj references.FPDF_PAGEOBJECT, parent affine, depth int) {
		if depth > 24 {
			return
		}
		t, err := inst.FPDFPageObj_GetType(&requests.FPDFPageObj_GetType{PageObject: obj})
		if err != nil {
			return
		}
		mr, err := inst.FPDFPageObj_GetMatrix(&requests.FPDFPageObj_GetMatrix{PageObject: obj})
		if err != nil {
			return
		}
		local := affineFrom32([6]float32{mr.Matrix.A, mr.Matrix.B, mr.Matrix.C, mr.Matrix.D, mr.Matrix.E, mr.Matrix.F})
		switch t.Type {
		case enums.FPDF_PAGEOBJ_TEXT:
			pl.TextObjects++
		case enums.FPDF_PAGEOBJ_FORM:
			pl.FormObjects++
			c, err := inst.FPDFFormObj_CountObjects(&requests.FPDFFormObj_CountObjects{PageObject: obj})
			if err != nil {
				return
			}
			full := local.then(parent)
			for i := 0; i < c.Count; i++ {
				ch, err := inst.FPDFFormObj_GetObject(&requests.FPDFFormObj_GetObject{PageObject: obj, Index: uint64(i)})
				if err == nil {
					visit(ch.PageObject, full, depth+1)
				}
			}
		case enums.FPDF_PAGEOBJ_IMAGE:
			imageID++
			full := local.then(parent).then(pl.toDisplay)
			p := func(x, y float64) [2]float64 { a, b := full.apply(x, y); return [2]float64{a, b} }
			box := boxOfPoints(p(0, 0), p(1, 0), p(1, 1), p(0, 1))
			im := ImagePlacement{ID: imageID, Box: roundBox(box), FormDepth: depth, matrix: full, object: obj}
			if sz, err := inst.FPDFImageObj_GetImagePixelSize(&requests.FPDFImageObj_GetImagePixelSize{ImageObject: obj}); err == nil {
				im.PixelWidth, im.PixelHeight = int(sz.Width), int(sz.Height)
			}
			im.Rotation, im.Mirrored, im.Orientation = describeImageOrientation(full)
			pl.Images = append(pl.Images, im)
		case enums.FPDF_PAGEOBJ_PATH:
			pl.PathObjects++
			d.collectPath(obj, local.then(parent).then(pl.toDisplay), pl, budget)
		}
	}
	for i := 0; i < n.Count; i++ {
		o, err := inst.FPDFPage_GetObject(&requests.FPDFPage_GetObject{Page: pageRef(page), Index: i})
		if err == nil {
			visit(o.PageObject, identityAffine(), 0)
		}
	}
	return nil
}

// describeImageOrientation compares the stored pixel grid with how the PDF
// places it. In image space row 0 of the stored pixels is at v=1 (top).
func describeImageOrientation(m affine) (float64, bool, string) {
	exx, exy := m.vec(1, 0)  // stored +x (left->right)
	edx, edy := m.vec(0, -1) // stored +row (top->bottom)
	rot := math.Atan2(exy, exx) * 180 / math.Pi
	rot = math.Mod(math.Round(rot)+360, 360)
	cross := exx*edy - exy*edx
	mirrored := cross < 0
	snap := rot
	for _, a := range []float64{0, 90, 180, 270, 360} {
		if math.Abs(rot-a) <= 2 {
			snap = math.Mod(a, 360)
		}
	}
	var s string
	switch {
	case !mirrored && snap == 0:
		s = "Stored upright; displayed as stored"
	case !mirrored && snap == 180:
		s = "Stored upside-down; the PDF rotates it 180° for display"
	case !mirrored:
		s = fmt.Sprintf("Stored rotated; the PDF rotates it %.0f° for display", snap)
	case mirrored && snap == 0:
		s = "Stored bottom-to-top (rows reversed); the PDF flips it vertically for display. A raw thumbnail looks upside-down and backwards."
	case mirrored && snap == 180:
		s = "Displayed mirrored left-right relative to the stored pixels (e.g. a rear/mirror view). Shown here exactly as on the page."
	default:
		s = fmt.Sprintf("Stored rotated and mirrored; the PDF applies a %.0f° rotation plus a flip for display", snap)
	}
	return snap, mirrored, s
}

func (d *pdfDoc) collectPath(obj references.FPDF_PAGEOBJECT, m affine, pl *PageLayout, budget *int) {
	inst := d.inst
	c, err := inst.FPDFPath_CountSegments(&requests.FPDFPath_CountSegments{PageObject: obj})
	if err != nil || c.Count <= 0 {
		return
	}
	pl.PathSegments += c.Count
	mark := "open"
	if dm, err := inst.FPDFPath_GetDrawMode(&requests.FPDFPath_GetDrawMode{PageObject: obj}); err == nil && dm.FillMode != enums.FPDF_FILLMODE_NONE {
		if fc, err := inst.FPDFPageObj_GetFillColor(&requests.FPDFPageObj_GetFillColor{PageObject: obj}); err == nil {
			luma := 0.299*float64(fc.FillColor.R) + 0.587*float64(fc.FillColor.G) + 0.114*float64(fc.FillColor.B)
			if fc.FillColor.A > 0 && luma < 160 {
				mark = "solid"
			}
		}
	}
	if *budget <= 0 {
		pl.VectorDetailTruncated = true
		return
	}
	*budget -= c.Count * 2
	type pt struct {
		x, y   float64
		kind   enums.FPDF_SEGMENT
		closed bool
	}
	pts := make([]pt, 0, c.Count)
	for i := 0; i < c.Count; i++ {
		s, err := inst.FPDFPath_GetPathSegment(&requests.FPDFPath_GetPathSegment{PageObject: obj, Index: i})
		if err != nil {
			continue
		}
		p, err := inst.FPDFPathSegment_GetPoint(&requests.FPDFPathSegment_GetPoint{PathSegment: s.PathSegment})
		if err != nil {
			continue
		}
		t, err := inst.FPDFPathSegment_GetType(&requests.FPDFPathSegment_GetType{PathSegment: s.PathSegment})
		if err != nil {
			continue
		}
		cl := false
		if cr, err := inst.FPDFPathSegment_GetClose(&requests.FPDFPathSegment_GetClose{PathSegment: s.PathSegment}); err == nil {
			cl = cr.IsClose
		}
		x, y := m.apply(float64(p.X), float64(p.Y))
		pts = append(pts, pt{x, y, t.Type, cl})
	}
	var openPath []pt
	flushOpen := func() {
		if len(openPath) < 2 {
			return
		}
		for _, q := range openPath {
			if q.closed || q.kind == enums.FPDF_SEGMENT_BEZIERTO {
				return
			}
		}
		a, z := openPath[0], openPath[len(openPath)-1]
		if math.Hypot(a.x-z.x, a.y-z.y) < .1 {
			return
		}
		for i := 1; i < len(openPath); i++ {
			a, b := openPath[i-1], openPath[i]
			pl.LeaderSegments = append(pl.LeaderSegments, Segment{r2(a.x), r2(a.y), r2(b.x), r2(b.y)})
		}
	}
	for _, q := range pts {
		if q.kind == enums.FPDF_SEGMENT_MOVETO {
			flushOpen()
			openPath = nil
		}
		openPath = append(openPath, q)
	}
	flushOpen()
	// Line segments for overlay (bezier control points are approximated by chords).
	var prev *pt
	var start *pt
	for i := range pts {
		q := &pts[i]
		if q.kind == enums.FPDF_SEGMENT_MOVETO {
			prev, start = q, q
			continue
		}
		if prev != nil && len(pl.Segments) < maxSegmentsPerPage {
			pl.Segments = append(pl.Segments, Segment{r2(prev.x), r2(prev.y), r2(q.x), r2(q.y)})
		}
		if q.closed && start != nil && len(pl.Segments) < maxSegmentsPerPage {
			pl.Segments = append(pl.Segments, Segment{r2(q.x), r2(q.y), r2(start.x), r2(start.y)})
		}
		prev = q
	}
	if len(pl.Segments) >= maxSegmentsPerPage {
		pl.VectorDetailTruncated = true
	}
	// Circle recognition: a closed sub-path whose points are equidistant from
	// their centroid. Works for 4-bezier circles and for polyline circles.
	var sub []pt
	check := func() {
		pts := sub
		if len(pts) >= 2 {
			a, z := pts[0], pts[len(pts)-1]
			if math.Abs(a.x-z.x) < 0.01 && math.Abs(a.y-z.y) < 0.01 {
				pts = pts[:len(pts)-1] // closing point duplicates the start
			}
		}

		// Preserve closed straight-sided flag-note outlines separately from circles.
		if len(pts) == 3 || len(pts) == 6 {
			closed := len(sub) > 0 && sub[len(sub)-1].closed
			if len(sub) > len(pts) {
				closed = true
			}
			linear := true
			points := [][2]float64{}
			for _, q := range pts {
				if q.kind == enums.FPDF_SEGMENT_BEZIERTO {
					linear = false
				}
				points = append(points, [2]float64{r2(q.x), r2(q.y)})
			}
			if closed && linear {
				pl.Polygons = append(pl.Polygons, CalloutPolygon{Points: points, Box: boxOfPoints(points...)})
			}
		}
		if len(pts) < 4 {
			return
		}
		minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
		for _, q := range pts {
			minX, minY = math.Min(minX, q.x), math.Min(minY, q.y)
			maxX, maxY = math.Max(maxX, q.x), math.Max(maxY, q.y)
		}
		if w, h := maxX-minX, maxY-minY; w <= 0 || math.Abs(w-h) > 0.08*math.Max(w, h) {
			return
		}
		cx, cy := (minX+maxX)/2, (minY+maxY)/2
		rmin, rmax, rsum := math.Inf(1), 0.0, 0.0
		for _, q := range pts {
			r := math.Hypot(q.x-cx, q.y-cy)
			rmin, rmax = math.Min(rmin, r), math.Max(rmax, r)
			rsum += r
		}
		sub = pts
		ravg := rsum / float64(len(sub))
		hasCurve := false
		for _, q := range sub {
			if q.kind == enums.FPDF_SEGMENT_BEZIERTO {
				hasCurve = true
			}
		}
		// Bezier control points sit outside the circle (~1.10r); allow for it.
		tol := 0.06
		if hasCurve {
			tol = 0.16
		}
		if ravg > 0.4 && (rmax-rmin) <= tol*ravg*1.0+0.05 && (hasCurve || len(sub) >= 8) {
			r := ravg
			if hasCurve {
				r = rmin // on-curve endpoints are exactly on the circle
			}
			pl.Circles = append(pl.Circles, Circle{CX: r2(cx), CY: r2(cy), R: r2(r), Mark: mark})
		}
	}
	for i := range pts {
		q := pts[i]
		if q.kind == enums.FPDF_SEGMENT_MOVETO {
			check()
			sub = sub[:0]
		}
		sub = append(sub, q)
		if q.closed {
			check()
			sub = sub[:0]
		}
	}
	check()
}

func r2(v float64) float64 { return math.Round(v*100) / 100 }

// detectOutlinedText flags pages that look like CAD "text as geometry": many
// small filled paths but almost no machine-readable characters. This cannot
// be fixed by a parser; it needs manual region entry or an approved OCR.
func detectOutlinedText(pl *PageLayout) {
	if pl.Text.Decoded < 40 && pl.PathObjects > 1500 {
		pl.Text.OutlinedTextLikely = true
		pl.Notes = append(pl.Notes, "Very little machine-readable text but many vector objects: text may be drawn as outlines/strokes. Use page review to enter values manually.")
	}
	if pl.Text.Chars > 0 && pl.Text.DecodedRatio < 0.7 {
		pl.Notes = append(pl.Notes, fmt.Sprintf("Only %.0f%% of characters map to Unicode; fonts lack a usable ToUnicode/encoding.", pl.Text.DecodedRatio*100))
	}
}

// ---------------------------------------------------------------------------
// Conversion to the legacy recognizer frame

// analysisFrame returns a rotation (0/90/180/270, counter-clockwise) that makes
// the page's dominant reading direction left-to-right. It is applied only to
// recognizer coordinates; UI overlays always use true display space.
func analysisFrame(pl PageLayout) int {
	total := 0
	for _, v := range pl.Text.DirectionShare {
		total += v
	}
	if total < 20 {
		return 0
	}
	best, bestN := "0", pl.Text.DirectionShare["0"]
	for _, k := range []string{"90", "180", "270"} {
		if pl.Text.DirectionShare[k] > bestN {
			best, bestN = k, pl.Text.DirectionShare[k]
		}
	}
	if best == "0" || float64(bestN) < 0.6*float64(total) {
		return 0
	}
	var r int
	fmt.Sscan(best, &r)
	return r
}

// legacyPage converts a display-space layout into the Y-up Page used by the
// recognizers. If the dominant text runs sideways or upside-down, the frame is
// rotated so that text reads left-to-right for the recognizers.
func legacyPage(pl PageLayout) Page {
	rot := analysisFrame(pl)
	w, h := pl.Width, pl.Height
	// rotate display (top-left) coordinates so text at angle `rot` becomes 0.
	conv := func(x, y float64) (float64, float64) {
		switch rot {
		case 90: // text reads bottom-to-top: rotate page clockwise 90
			return h - y, x
		case 180:
			return w - x, h - y
		case 270:
			return y, w - x
		}
		return x, y
	}
	fw, fh := w, h
	if rot == 90 || rot == 270 {
		fw, fh = h, w
	}
	p := Page{Number: pl.Page, Width: fw, Height: fh, Extraction: "PDFium page-accurate text (display orientation)", PageAccurate: true}
	if rot != 0 {
		p.Extraction = fmt.Sprintf("PDFium page-accurate text; analysis frame rotated %d° to make dominant text upright", rot)
	}
	for _, lw := range pl.Words {
		rel := math.Mod(lw.Angle-float64(rot)+360, 360)
		if !(rel <= 4 || rel >= 356) {
			// Rotated labels are kept for designator search but positioned by box centre.
			x, y := conv(lw.Box.CX(), lw.Box.CY())
			p.Words = append(p.Words, Word{Text: lw.Text, X: x, Y: fh - y, Size: lw.Size, X1: x + 0.1, Rotated: true})
			continue
		}
		x, y := conv(lw.OX, lw.OY)
		ax, _ := conv(lw.Box.X0, lw.Box.Y0)
		bx, _ := conv(lw.Box.X1, lw.Box.Y1)
		p.Words = append(p.Words, Word{Text: lw.Text, X: x, Y: fh - y, Size: lw.Size, X1: math.Max(ax, bx)})
	}
	sort.SliceStable(p.Words, func(i, j int) bool {
		if d := p.Words[i].Y - p.Words[j].Y; d > 2 || d < -2 {
			return p.Words[i].Y > p.Words[j].Y
		}
		return p.Words[i].X < p.Words[j].X
	})
	p.Lines = groupLines(p.Words)
	if len(p.Words) == 0 {
		p.Extraction = "NO_MACHINE_READABLE_TEXT (PDFium); use page review"
	}
	return p
}

// ---------------------------------------------------------------------------
// Rendering

// renderPage renders a page in display orientation at the given scale
// (1.0 = 72 dpi) and returns an RGBA image.
func (d *pdfDoc) renderPage(index int, scale float64) (*image.RGBA, error) {
	if scale <= 0 {
		scale = 1.5
	}
	var out *image.RGBA
	err := d.withPage(index, func(page references.FPDF_PAGE) error {
		dpi := int(math.Round(72 * scale))
		if dpi < 18 {
			dpi = 18
		}
		r, err := d.inst.RenderPageInDPI(&requests.RenderPageInDPI{Page: pageRef(page), DPI: dpi, RenderFlags: enums.FPDF_RENDER_FLAG_ANNOT | enums.FPDF_RENDER_FLAG_LCD_TEXT})
		if err != nil {
			return err
		}
		// Copy out of WebAssembly-owned memory before the page closes.
		img := image.NewRGBA(r.Result.Image.Bounds())
		copy(img.Pix, r.Result.Image.Pix)
		// Flatten transparency onto white so PNGs look like the page.
		for i := 3; i < len(img.Pix); i += 4 {
			if a := img.Pix[i]; a != 255 {
				f := float64(a) / 255
				for c := 1; c <= 3; c++ {
					img.Pix[i-c] = uint8(float64(img.Pix[i-c])*f + 255*(1-f))
				}
				img.Pix[i] = 255
			}
		}
		out = img
		return nil
	})
	return out, err
}

// rawImagePixels returns the image object's stored pixels with NO placement
// transform, for side-by-side comparison with the displayed crop.
func (d *pdfDoc) rawImagePixels(pageIndex, imageID int) (image.Image, error) {
	var out image.Image
	err := d.withPage(pageIndex, func(page references.FPDF_PAGE) error {
		pl := PageLayout{Page: pageIndex + 1}
		var err error
		wr, _ := d.inst.FPDF_GetPageWidthF(&requests.FPDF_GetPageWidthF{Page: pageRef(page)})
		hr, _ := d.inst.FPDF_GetPageHeightF(&requests.FPDF_GetPageHeightF{Page: pageRef(page)})
		pl.Width, pl.Height = float64(wr.PageWidth), float64(hr.PageHeight)
		pl.toDisplay, err = d.displayTransform(page, pl.Width, pl.Height)
		if err != nil {
			return err
		}
		budget := 0
		if err := d.walkObjects(page, &pl, &budget); err != nil {
			return err
		}
		for _, im := range pl.Images {
			if im.ID != imageID {
				continue
			}
			bm, err := d.inst.FPDFImageObj_GetBitmap(&requests.FPDFImageObj_GetBitmap{ImageObject: im.object})
			if err != nil {
				return err
			}
			defer d.inst.FPDFBitmap_Destroy(&requests.FPDFBitmap_Destroy{Bitmap: bm.Bitmap})
			out, err = d.bitmapToImage(bm.Bitmap)
			return err
		}
		return errors.New("image not found")
	})
	return out, err
}

func (d *pdfDoc) bitmapToImage(bm references.FPDF_BITMAP) (image.Image, error) {
	inst := d.inst
	w, err := inst.FPDFBitmap_GetWidth(&requests.FPDFBitmap_GetWidth{Bitmap: bm})
	if err != nil {
		return nil, err
	}
	h, err := inst.FPDFBitmap_GetHeight(&requests.FPDFBitmap_GetHeight{Bitmap: bm})
	if err != nil {
		return nil, err
	}
	s, err := inst.FPDFBitmap_GetStride(&requests.FPDFBitmap_GetStride{Bitmap: bm})
	if err != nil {
		return nil, err
	}
	f, err := inst.FPDFBitmap_GetFormat(&requests.FPDFBitmap_GetFormat{Bitmap: bm})
	if err != nil {
		return nil, err
	}
	b, err := inst.FPDFBitmap_GetBuffer(&requests.FPDFBitmap_GetBuffer{Bitmap: bm})
	if err != nil {
		return nil, err
	}
	W, H, S := w.Width, h.Height, s.Stride
	if W <= 0 || H <= 0 || W*H > 40_000_000 {
		return nil, errors.New("unsupported bitmap size")
	}
	buf := b.Buffer
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	bpp := map[enums.FPDF_BITMAP_FORMAT]int{enums.FPDF_BITMAP_FORMAT_GRAY: 1, enums.FPDF_BITMAP_FORMAT_BGR: 3, enums.FPDF_BITMAP_FORMAT_BGRX: 4, enums.FPDF_BITMAP_FORMAT_BGRA: 4}[f.Format]
	if bpp == 0 {
		return nil, errors.New("unsupported bitmap format")
	}
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			i := y*S + x*bpp
			if i+bpp > len(buf) {
				continue
			}
			var c color.RGBA
			if bpp == 1 {
				c = color.RGBA{buf[i], buf[i], buf[i], 255}
			} else {
				c = color.RGBA{buf[i+2], buf[i+1], buf[i], 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img, nil
}

// cropDisplay crops a display-space box from a rendered page image.
func cropDisplay(img *image.RGBA, scale float64, b Box, pad float64) *image.RGBA {
	r := image.Rect(int(math.Floor((b.X0-pad)*scale)), int(math.Floor((b.Y0-pad)*scale)), int(math.Ceil((b.X1+pad)*scale)), int(math.Ceil((b.Y1+pad)*scale))).Intersect(img.Bounds())
	out := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	for y := 0; y < r.Dy(); y++ {
		copy(out.Pix[y*out.Stride:y*out.Stride+r.Dx()*4], img.Pix[(r.Min.Y+y)*img.Stride+r.Min.X*4:(r.Min.Y+y)*img.Stride+r.Max.X*4])
	}
	return out
}

// isPDF accepts the %PDF- header anywhere in the first 1024 bytes, as PDF
// readers do (some generators prepend a few bytes of junk).
func isPDF(b []byte) bool { return bytes.Contains(b[:min(len(b), 1024)], []byte("%PDF-")) }
