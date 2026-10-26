package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type Word struct {
	Text string  `json:"text"`
	X    float64 `json:"x"`
	Y    float64 `json:"y"`
	Size float64 `json:"size"`
	// v2.7.2: right edge in the recognizer frame (0 when unknown, e.g. legacy
	// engine) and whether the fragment runs across the reading direction.
	X1      float64 `json:"x1,omitempty"`
	Rotated bool    `json:"rotated,omitempty"`
}

// right returns the fragment's right edge, estimating it when unknown.
func (w Word) right() float64 {
	if w.X1 > w.X {
		return w.X1
	}
	sz := w.Size
	if sz <= 0 {
		sz = 6
	}
	return w.X + float64(len([]rune(w.Text)))*0.55*sz
}
type Page struct {
	Number     int      `json:"number"`
	Width      float64  `json:"width"`
	Height     float64  `json:"height"`
	Words      []Word   `json:"words"`
	Lines      []string `json:"lines"`
	Extraction string   `json:"extraction"`
	// PageAccurate is true only for PDFium display-space pages. Column-based
	// table readers require it: on legacy (possibly mirrored) coordinates they
	// can swap columns and produce wrong pin mappings.
	PageAccurate bool `json:"-"`
}
type PDFData struct {
	Pages  []Page   `json:"pages"`
	Issues []string `json:"issues"`
}

var (
	reBTBlock = regexp.MustCompile(`(?s)\bBT\b(.*?)\bET\b`)
	reTm      = regexp.MustCompile(`([-+]?\d+(?:\.\d+)?)\s+([-+]?\d+(?:\.\d+)?)\s+([-+]?\d+(?:\.\d+)?)\s+([-+]?\d+(?:\.\d+)?)\s+([-+]?\d+(?:\.\d+)?)\s+([-+]?\d+(?:\.\d+)?)\s+Tm\b`)
	reTd      = regexp.MustCompile(`([-+]?\d+(?:\.\d+)?)\s+([-+]?\d+(?:\.\d+)?)\s+T[dD]\b`)
	reTf      = regexp.MustCompile(`/\S+\s+([-+]?\d+(?:\.\d+)?)\s+Tf\b`)
	reShow    = regexp.MustCompile(`(?s)(\[(?:\([^)]*(?:\\.[^)]*)*\)|[^]])*\]\s*TJ|\((?:\\.|[^\\)])*\)\s*Tj|<[0-9A-Fa-f\s]+>\s*Tj)`)
	reStr     = regexp.MustCompile(`(?s)\((?:\\.|[^\\)])*\)|<[0-9A-Fa-f\s]+>`)
	reBox     = regexp.MustCompile(`/MediaBox\s*\[\s*[-\d.]+\s+[-\d.]+\s+([\d.]+)\s+([\d.]+)`)
)

func f64(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }
func pdfDecodeString(s string) string {
	if strings.HasPrefix(s, "<") {
		raw := strings.Join(strings.Fields(strings.Trim(s, "<>")), "")
		b, err := hex.DecodeString(raw)
		if err == nil {
			return string(b)
		}
		return ""
	}
	if len(s) < 2 {
		return ""
	}
	s = s[1 : len(s)-1]
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '\n':
				continue
			case '\r':
				if i+1 < len(s) && s[i+1] == '\n' {
					i++
				}
				continue
			default:
				if s[i] >= '0' && s[i] <= '7' {
					num := int(s[i] - '0')
					for j := 0; j < 2 && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '7'; j++ {
						i++
						num = num*8 + int(s[i]-'0')
					}
					out = append(out, byte(num))
				} else {
					out = append(out, s[i])
				}
			}
		} else {
			out = append(out, s[i])
		}
	}
	// Preserve useful ASCII/Latin-1. Unknown binary glyphs must not silently become numbers.
	var runes []rune
	for _, c := range out {
		if c >= 32 && c <= 126 {
			runes = append(runes, rune(c))
		} else if c == 9 || c == 10 {
			runes = append(runes, ' ')
		} else if c >= 160 {
			runes = append(runes, rune(c))
		}
	}
	return string(runes)
}

// parseBlock interprets text positioning sequentially. The previous parser
// searched the entire BT/ET block for its final Tm, which incorrectly assigned
// that position to earlier and later Tj/TJ operations whenever Td/TD/T* moved
// text. Multiple CAD exporters use one Tm followed by many relative moves.
func parseBlock(b string) []Word {
	x, y, size, leading := 0.0, 0.0, 8.0, 0.0
	words := []Word{}
	// Keep the existing conservative literal/hex string decoder; this is NOT
	// OCR and does not claim to interpret custom-font encodings/ToUnicode maps.
	reEvent := regexp.MustCompile(`(?s)([-+]?\d+(?:\.\d+)?\s+[-+]?\d+(?:\.\d+)?\s+[-+]?\d+(?:\.\d+)?\s+[-+]?\d+(?:\.\d+)?\s+[-+]?\d+(?:\.\d+)?\s+[-+]?\d+(?:\.\d+)?\s+Tm\b|[-+]?\d+(?:\.\d+)?\s+[-+]?\d+(?:\.\d+)?\s+T[dD]\b|[-+]?\d+(?:\.\d+)?\s+TL\b|/\S+\s+[-+]?\d+(?:\.\d+)?\s+Tf\b|T\*|\[(?:\([^)]*(?:\\.[^)]*)*\)|[^]])*\]\s*TJ|\((?:\\.|[^\\)])*\)\s*Tj|<[0-9A-Fa-f\s]+>\s*Tj)`)
	for _, loc := range reEvent.FindAllStringIndex(b, -1) {
		event := b[loc[0]:loc[1]]
		if m := reTm.FindStringSubmatch(event); len(m) > 6 {
			x, y = f64(m[5]), f64(m[6])
			continue
		}
		if m := reTd.FindStringSubmatch(event); len(m) > 2 {
			dx, dy := f64(m[1]), f64(m[2])
			x += dx
			y += dy
			if strings.HasSuffix(event, "TD") {
				leading = -dy
			}
			continue
		}
		if strings.TrimSpace(event) == "T*" {
			y -= leading
			continue
		}
		if m := reTf.FindStringSubmatch(event); len(m) > 1 {
			size = f64(m[1])
			continue
		}
		if strings.HasSuffix(strings.TrimSpace(event), "TL") {
			fields := strings.Fields(event)
			if len(fields) > 0 {
				leading = f64(fields[0])
			}
			continue
		}
		parts := []string{}
		for _, str := range reStr.FindAllString(event, -1) {
			parts = append(parts, pdfDecodeString(str))
		}
		value := strings.TrimSpace(strings.Join(parts, ""))
		if value == "" || len(value) > 1500 {
			continue
		}
		good := 0
		for _, r := range value {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				good++
			}
		}
		if good > 0 {
			words = append(words, Word{Text: value, X: x, Y: y, Size: size})
		}
		// Approximate advance for consecutive Tj without new Tm/Td. Exact
		// glyph positioning is not possible without a decoded font width map;
		// downstream recognition must keep those assignments in DRAFT state.
		x += float64(len([]rune(value))) * size * 0.48
	}
	return words
}

func parseRawObjects(b []byte) map[int]*indirect {
	objects := map[int]*indirect{}
	for _, m := range reObj.FindAllSubmatch(b, -1) {
		id, _ := strconv.Atoi(string(m[1]))
		o := &indirect{id: id, dictionary: m[3]}
		body := m[3]
		i := bytes.Index(body, []byte("stream"))
		if i >= 0 {
			head := body[:i]
			start := i + 6
			if start < len(body) && body[start] == '\r' {
				start++
			}
			if start < len(body) && body[start] == '\n' {
				start++
			}
			end := bytes.LastIndex(body, []byte("endstream"))
			if end > start {
				raw := body[start:end]
				if n := intGroup(head, reLen); n > 0 && n <= len(raw) {
					raw = raw[:n]
				} else {
					raw = bytes.TrimRight(raw, "\r\n")
				}
				o.dictionary = head
				o.encoded = raw
				o.hasStream = true
				o.decoded, _ = decodePDFStream(head, raw)
			}
		}
		objects[id] = o
	}
	unpackObjectStreams(objects)
	return objects
}
func extractLegacyPDF(b []byte) (PDFData, error) {
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		return PDFData{}, errors.New("not a PDF")
	}
	if len(b) > 100<<20 {
		return PDFData{}, errors.New("PDF too large")
	}
	o := parseRawObjects(b)
	pages := pdfPageOrder(o)
	if len(pages) == 0 {
		return PDFData{}, errors.New("unsupported PDF page tree")
	}
	out := PDFData{}
	for idx, p := range pages {
		pg := Page{Number: idx + 1, Width: 792, Height: 612, Extraction: "native text"}
		if mm := reBox.FindSubmatch(p.dictionary); len(mm) > 2 {
			pg.Width = f64(string(mm[1]))
			pg.Height = f64(string(mm[2]))
		}
		refs := []int{}
		if m := reContents.FindSubmatch(p.dictionary); len(m) > 1 {
			n, _ := strconv.Atoi(string(m[1]))
			refs = append(refs, n)
		}
		if m := reContentsArr.FindSubmatch(p.dictionary); len(m) > 1 {
			for _, r := range reRef.FindAllSubmatch(m[1], -1) {
				n, _ := strconv.Atoi(string(r[1]))
				refs = append(refs, n)
			}
		}
		for _, id := range refs {
			obj, ok := o[id]
			if !ok || !obj.hasStream {
				continue
			}
			for _, block := range reBTBlock.FindAllStringSubmatch(string(obj.decoded), -1) {
				pg.Words = append(pg.Words, parseBlock(block[1])...)
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
			pg.Extraction = "NO_NATIVE_TEXT; manual/approved OCR required"
			out.Issues = append(out.Issues, fmt.Sprintf("Page %d has no safely decoded text", idx+1))
		}
		out.Pages = append(out.Pages, pg)
	}
	return out, nil
}
func groupLines(words []Word) []string {
	type line struct {
		y     float64
		parts []Word
	}
	lines := []line{}
	for _, w := range words {
		ii := -1
		for i := range lines {
			if abs(lines[i].y-w.Y) < 2.6 {
				ii = i
				break
			}
		}
		if ii < 0 {
			lines = append(lines, line{y: w.Y})
			ii = len(lines) - 1
		}
		lines[ii].parts = append(lines[ii].parts, w)
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].y > lines[j].y })
	out := []string{}
	for _, ln := range lines {
		sort.Slice(ln.parts, func(i, j int) bool { return ln.parts[i].X < ln.parts[j].X })
		fields := []string{}
		for _, p := range ln.parts {
			fields = append(fields, p.Text)
		}
		out = append(out, strings.Join(fields, " | "))
	}
	return out
}
func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
