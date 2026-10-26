package main

import (
	"errors"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"strings"
	"sync"

	pdfium_errors "github.com/klippa-app/go-pdfium/errors"
)

// DocExtraction is the single source of truth for one PDF: page-accurate
// layouts in display space (for overlays/evidence) and the recognizer-frame
// pages derived from them.
type DocExtraction struct {
	Engine      string
	EngineError string
	Legacy      bool
	SHA256      string
	Data        PDFData
	Layouts     []PageLayout
	Compare26   *EngineComparison
}

// EngineComparison records what the retired v2.6 text extraction yields on the
// same bytes when run through the SAME v2.7 recognizers. It isolates the
// extraction engine, so a local user can see whether PDFium recovers or loses
// records on a real drawing. (It is not a byte-for-byte rerun of v2.6.)
type EngineComparison struct {
	Connectors int    `json:"connectors"`
	Wires      int    `json:"wires"`
	BOM        int    `json:"bom"`
	Error      string `json:"error,omitempty"`
}

func compareWithV26(b []byte) (c *EngineComparison) {
	c = &EngineComparison{}
	defer func() {
		if r := recover(); r != nil {
			c.Error = fmt.Sprint("v2.6 interpreter crashed: ", r)
		}
	}()
	d, err := extractPDFEnhanced26(b)
	if err != nil {
		c.Error = err.Error()
		return c
	}
	a, err := AnalyzeData(d)
	if err != nil {
		c.Error = err.Error()
		return c
	}
	c.Connectors, c.Wires, c.BOM = len(a.Connectors), len(a.Wires), len(a.BOM)
	return c
}

// ExtractDocument runs PDFium over every page. If PDFium cannot open the file
// at all, the legacy v2.6 interpreter is used and the result is clearly
// labelled so it can never be mistaken for a page-accurate interpretation.
func ExtractDocument(b []byte) (*DocExtraction, error) {
	if len(b) > maxPDFSize {
		return nil, fmt.Errorf("PDF too large")
	}
	if !isPDF(b) {
		return nil, fmt.Errorf("not a PDF")
	}
	sum := sha256.Sum256(b)
	x := &DocExtraction{Engine: EngineName(), SHA256: hex.EncodeToString(sum[:])}
	engineMu.Lock()
	// If the engine itself cannot start, refuse to interpret: the legacy
	// interpreter produces misleading results and must never stand in for a
	// broken installation. (It is only used for a single unreadable file.)
	if _, startErr := startEngine(); startErr != nil {
		engineMu.Unlock()
		return nil, fmt.Errorf("%w: %v", ErrEngineUnavailable, startErr)
	}
	d, err := openPDF(b)
	x.Engine = EngineName() // after start-up, so the execution mode is known
	if err == nil {
		for i := 0; i < d.pages; i++ {
			pl, perr := d.layoutPage(i)
			if perr != nil {
				pl = PageLayout{Page: i + 1, Words: []LayoutWord{}, Images: []ImagePlacement{}, Circles: []Circle{}, Segments: []Segment{}, Notes: []string{"PDFium could not load this page: " + perr.Error()}}
				x.Data.Issues = append(x.Data.Issues, fmt.Sprintf("Page %d could not be loaded by PDFium", i+1))
			}
			x.Layouts = append(x.Layouts, pl)
			x.Data.Pages = append(x.Data.Pages, legacyPage(pl))
		}
		d.close()
	}
	engineMu.Unlock()
	if err != nil && (errors.Is(err, pdfium_errors.ErrPassword) || strings.Contains(err.Error(), "invalid password")) {
		// An encrypted drawing is not "unreadable": the legacy interpreter
		// would only produce garbage from the encrypted streams.
		return nil, ErrPasswordProtected
	}
	if err != nil {
		legacy, lerr := extractPDFEnhanced26(b)
		if lerr != nil {
			return nil, fmt.Errorf("%v; legacy interpreter also failed: %v", err, lerr)
		}
		x.Engine = "LEGACY v2.6 regex interpreter (PDFium failed to open this file)"
		x.EngineError = err.Error()
		x.Legacy = true
		x.Data = legacy
		for _, p := range legacy.Pages {
			x.Layouts = append(x.Layouts, PageLayout{Page: p.Number, Width: p.Width, Height: p.Height, Words: []LayoutWord{}, Images: []ImagePlacement{}, Circles: []Circle{}, Segments: []Segment{}, Notes: []string{"Legacy interpreter: coordinates are NOT page-accurate"}})
		}
	}
	if !x.Legacy {
		x.Compare26 = compareWithV26(b)
	}
	for _, p := range x.Data.Pages {
		if len(p.Words) == 0 {
			x.Data.Issues = append(x.Data.Issues, fmt.Sprintf("Page %d has no machine-readable text", p.Number))
		}
	}
	return x, nil
}

// ErrPasswordProtected means the PDF needs a password to open. The app does
// not ask for or handle document passwords.
var ErrPasswordProtected = errors.New("this PDF is password-protected; open it in your PDF viewer and save an unprotected copy inside your authorized environment, then read that copy")

// ErrEngineUnavailable means the bundled PDF engine did not start on this PC.
var ErrEngineUnavailable = errors.New("PDF ENGINE FAILED TO START — no drawing was interpreted")

// extractPDF keeps the original signature used by the recognizers and tests.
func extractPDF(b []byte) (PDFData, error) {
	x, err := ExtractDocument(b)
	if err != nil {
		return PDFData{}, err
	}
	return x.Data, nil
}

// ---------------------------------------------------------------------------
// In-memory session. The current drawing's bytes, extraction and rendered
// pages live only in this process's memory. Nothing is written to disk.

type session struct {
	mu      sync.Mutex
	data    []byte
	extract *DocExtraction
	renders map[string]*image.RGBA
	order   []string
	gen     int // bumped whenever the drawing changes; stale renders are discarded
}

var current = &session{}

func (s *session) set(b []byte, x *DocExtraction) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = b
	s.extract = x
	s.renders = map[string]*image.RGBA{}
	s.order = nil
	s.gen++
}

func (s *session) get() ([]byte, *DocExtraction) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data, s.extract
}

func (s *session) clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data, s.extract, s.renders, s.order = nil, nil, map[string]*image.RGBA{}, nil
	s.gen++
}

// render returns a cached rendering of page (1-based) at scale.
func (s *session) render(page int, scale float64) (*image.RGBA, error) {
	key := fmt.Sprintf("%d@%.3f", page, scale)
	s.mu.Lock()
	if img, ok := s.renders[key]; ok {
		s.mu.Unlock()
		return img, nil
	}
	data, gen := s.data, s.gen
	s.mu.Unlock()
	if data == nil {
		return nil, fmt.Errorf("no drawing is loaded")
	}
	engineMu.Lock()
	d, err := openPDF(data)
	if err != nil {
		engineMu.Unlock()
		return nil, err
	}
	if page < 1 || page > d.pages {
		d.close()
		engineMu.Unlock()
		return nil, fmt.Errorf("page out of range")
	}
	img, err := d.renderPage(page-1, scale)
	d.close()
	engineMu.Unlock()
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gen != gen {
		return img, nil // the drawing changed while rendering; do not cache
	}
	if s.renders == nil {
		s.renders = map[string]*image.RGBA{}
	}
	s.renders[key] = img
	s.order = append(s.order, key)
	for len(s.order) > 6 { // bound memory: a D-size sheet at 2x is ~40 MB
		delete(s.renders, s.order[0])
		s.order = s.order[1:]
	}
	return img, nil
}

func (s *session) rawImage(page, id int) (image.Image, error) {
	s.mu.Lock()
	data := s.data
	s.mu.Unlock()
	if data == nil {
		return nil, fmt.Errorf("no drawing is loaded")
	}
	engineMu.Lock()
	defer engineMu.Unlock()
	d, err := openPDF(data)
	if err != nil {
		return nil, err
	}
	defer d.close()
	if page < 1 || page > d.pages {
		return nil, fmt.Errorf("page out of range")
	}
	return d.rawImagePixels(page-1, id)
}
