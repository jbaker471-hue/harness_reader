package main

import (
	"bytes"
	"compress/zlib"
	"encoding/ascii85"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// This intentionally extracts structural statistics ONLY. No document text,
// metadata, filenames, raw coordinates, or part numbers are retained in a report.
// Analyzed bytes remain in process memory for the duration of each request.
type PageStats struct {
	Page             int     `json:"page"`
	StreamCount      int     `json:"content_streams"`
	DecodedBytes     int     `json:"content_decoded_bytes"`
	TextOperators    int     `json:"text_show_operators"`
	TextObjects      int     `json:"text_objects"`
	TinyTextRatio    float64 `json:"short_text_chunk_ratio"`
	DrawingOperators int     `json:"vector_path_operators"`
	Evidence         string  `json:"evidence"`
}
type Issue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
	Action   string `json:"action"`
}
type Diagnostics struct {
	Schema             string      `json:"schema"`
	PageCount          int         `json:"page_count"`
	HasEncryption      bool        `json:"encrypted"`
	PDFVersion         string      `json:"pdf_version"`
	TotalObjects       int         `json:"indirect_objects"`
	ObjectStreams      int         `json:"object_streams"`
	CompressedStreams  int         `json:"compressed_streams"`
	UnsupportedFilters int         `json:"unsupported_filter_streams"`
	FontObjects        int         `json:"font_objects"`
	ToUnicodeMaps      int         `json:"to_unicode_references"`
	ContentStreams     int         `json:"content_streams"`
	TextOperators      int         `json:"text_show_operators"`
	Pages              []PageStats `json:"pages"`
	Issues             []Issue     `json:"issues"`
}
type indirect struct {
	id                int
	gen               int
	dictionary        []byte
	decoded           []byte
	encoded           []byte
	hasStream         bool
	decodeError       bool
	filterUnsupported bool
}

var (
	reObj          = regexp.MustCompile(`(?s)(\d+)\s+(\d+)\s+obj\b(.*?)\bendobj\b`)
	rePage         = regexp.MustCompile(`/Type\s*/Page\b`)
	reFont         = regexp.MustCompile(`/Type\s*/Font\b`)
	reCount        = regexp.MustCompile(`/Count\s+(\d+)`)
	reContents     = regexp.MustCompile(`/Contents\s+(\d+)\s+\d+\s+R`)
	reContentsArr  = regexp.MustCompile(`(?s)/Contents\s*\[([^]]+)\]`)
	reRef          = regexp.MustCompile(`(\d+)\s+\d+\s+R`)
	reKids         = regexp.MustCompile(`(?s)/Kids\s*\[([^]]*)\]`)
	rePagesRef     = regexp.MustCompile(`/Pages\s+(\d+)\s+\d+\s+R`)
	reFilterArr    = regexp.MustCompile(`(?s)/Filter\s*\[([^]]*)\]`)
	reFilterSingle = regexp.MustCompile(`/Filter\s*/([A-Za-z0-9]+)`)
	reNames        = regexp.MustCompile(`/([A-Za-z0-9]+)`)
	reLen          = regexp.MustCompile(`/Length\s+(\d+)\b`)
	reObjs         = regexp.MustCompile(`/N\s+(\d+)`)
	reFirst        = regexp.MustCompile(`/First\s+(\d+)`)
	reDigits       = regexp.MustCompile(`\d+`)
	reTextOps      = regexp.MustCompile(`\b(Tj|TJ|'|\")\b`)
	reBT           = regexp.MustCompile(`\bBT\b`)
	reVector       = regexp.MustCompile(`\b(m|l|c|re|h|S|s|f|F|B|b)\b`)
	reShowLit      = regexp.MustCompile(`(?s)\((?:\\.|[^\\)])*\)\s*(?:Tj|')`)
	reShowHex      = regexp.MustCompile(`<[0-9A-Fa-f\s]+>\s*Tj\b`)
	reShowArray    = regexp.MustCompile(`(?s)\[(?:[^]]*)\]\s*TJ\b`)
	reBytesLit     = regexp.MustCompile(`(?s)\((?:\\.|[^\\)])*\)`)
)

func issue(code, severity, detail, action string) Issue { return Issue{code, severity, detail, action} }
func intGroup(b []byte, re *regexp.Regexp) int {
	m := re.FindSubmatch(b)
	if len(m) < 2 {
		return 0
	}
	x, _ := strconv.Atoi(string(m[1]))
	return x
}
func safeInflate(b []byte) ([]byte, error) {
	r, e := zlib.NewReader(bytes.NewReader(b))
	if e != nil {
		return nil, e
	}
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, 32<<20))
}
func decodePDFStream(head, raw []byte) ([]byte, bool) {
	filters := []string{}
	if m := reFilterArr.FindSubmatch(head); len(m) > 0 {
		for _, name := range reNames.FindAllSubmatch(m[1], -1) {
			filters = append(filters, string(name[1]))
		}
	} else if m := reFilterSingle.FindSubmatch(head); len(m) > 0 {
		filters = append(filters, string(m[1]))
	}
	data := raw
	for _, filter := range filters {
		switch filter {
		case "ASCII85Decode", "A85":
			encoded := bytes.TrimSpace(bytes.TrimSuffix(bytes.TrimSpace(data), []byte("~>")))
			buf := make([]byte, len(encoded)+8)
			n, _, err := ascii85.Decode(buf, encoded, true)
			if err != nil {
				return nil, false
			}
			data = buf[:n]
		case "ASCIIHexDecode", "AHx":
			encoded := bytes.TrimSpace(bytes.TrimSuffix(bytes.TrimSpace(data), []byte(">")))
			encoded = bytes.Join(bytes.Fields(encoded), nil)
			if len(encoded)%2 != 0 {
				encoded = append(encoded, '0')
			}
			dec := make([]byte, hex.DecodedLen(len(encoded)))
			n, err := hex.Decode(dec, encoded)
			if err != nil {
				return nil, false
			}
			data = dec[:n]
		case "FlateDecode", "Fl":
			out, err := safeInflate(data)
			if err != nil {
				return nil, false
			}
			data = out
		default:
			return nil, false
		}
	}
	return data, true
}
func pdfPageOrder(objects map[int]*indirect) []*indirect {
	roots := []int{}
	for _, o := range objects {
		if bytes.Contains(o.dictionary, []byte("/Type /Catalog")) || bytes.Contains(o.dictionary, []byte("/Type/Catalog")) {
			if m := rePagesRef.FindSubmatch(o.dictionary); len(m) > 0 {
				n, _ := strconv.Atoi(string(m[1]))
				roots = append(roots, n)
			}
		}
	}
	sort.Ints(roots)
	pages := []*indirect{}
	visited := map[int]bool{}
	var visit func(id int)
	visit = func(id int) {
		if visited[id] {
			return
		}
		visited[id] = true
		o, ok := objects[id]
		if !ok {
			return
		}
		if rePage.Match(o.dictionary) {
			pages = append(pages, o)
			return
		}
		if m := reKids.FindSubmatch(o.dictionary); len(m) > 0 {
			for _, r := range reRef.FindAllSubmatch(m[1], -1) {
				n, _ := strconv.Atoi(string(r[1]))
				visit(n)
			}
		}
	}
	for _, id := range roots {
		visit(id)
	}
	if len(pages) > 0 {
		return pages
	}
	for _, o := range objects {
		if rePage.Match(o.dictionary) {
			pages = append(pages, o)
		}
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].id < pages[j].id })
	return pages
}
func unpackObjectStreams(objects map[int]*indirect) {
	// PDF 1.5 objects can be nested inside /ObjStm streams. The header consists
	// of object-id/offset pairs, followed by concatenated object dictionaries.
	expanded := map[int]*indirect{}
	for _, o := range objects {
		if !bytes.Contains(o.dictionary, []byte(`/Type /ObjStm`)) && !bytes.Contains(o.dictionary, []byte(`/Type/ObjStm`)) {
			continue
		}
		n, first := intGroup(o.dictionary, reObjs), intGroup(o.dictionary, reFirst)
		if n <= 0 || n > 100000 || first <= 0 || first >= len(o.decoded) {
			continue
		}
		hdr := reDigits.FindAll(o.decoded[:first], -1)
		if len(hdr) < 2*n {
			continue
		}
		for i := 0; i < n; i++ {
			id, _ := strconv.Atoi(string(hdr[2*i]))
			off, _ := strconv.Atoi(string(hdr[2*i+1]))
			end := len(o.decoded) - first
			if i+1 < n {
				nx, _ := strconv.Atoi(string(hdr[2*(i+1)+1]))
				end = nx
			}
			if id <= 0 || off < 0 || end <= off || first+end > len(o.decoded) {
				continue
			}
			expanded[id] = &indirect{id: id, dictionary: o.decoded[first+off : first+end]}
		}
	}
	for id, o := range expanded {
		if _, ok := objects[id]; !ok {
			objects[id] = o
		}
	}
}
func contentMetrics(data []byte) (textOps int, textObjects int, tiny float64, vector int) {
	textObjects = len(reBT.FindAll(data, -1))
	vector = len(reVector.FindAll(data, -1))
	// A single show may contain a full word or only one encoded glyph. This is a
	// structural fragmentation indicator, not an OCR quality or character count.
	lit := reShowLit.FindAll(data, -1)
	arr := reShowArray.FindAll(data, -1)
	hex := reShowHex.FindAll(data, -1)
	textOps = len(lit) + len(arr) + len(hex)
	short := 0
	for _, m := range lit {
		b := reBytesLit.Find(m)
		if len(b) <= 4 {
			short++
		}
	}
	for _, m := range arr {
		inside := reBytesLit.FindAll(m, -1)
		total := 0
		for _, str := range inside {
			total += len(str) - 2
		}
		if total <= 2 {
			short++
		}
	}
	for _, m := range hex {
		if len(m) <= 9 {
			short++
		}
	}
	if textOps > 0 {
		tiny = float64(short) / float64(textOps)
	}
	return
}
func Diagnose(b []byte) (Diagnostics, error) {
	var d Diagnostics
	d.Schema = "trivex.local_pdf_diagnostic.v1"
	if len(b) < 8 || !bytes.HasPrefix(b, []byte("%PDF-")) {
		return d, errors.New("not a PDF file")
	}
	if len(b) > 120<<20 {
		return d, errors.New("file exceeds local 120 MB safety limit")
	}
	if len(b) >= 8 {
		d.PDFVersion = string(b[5:8])
		if len(d.PDFVersion) != 3 || d.PDFVersion[1] != '.' {
			d.PDFVersion = "unknown"
		}
	}
	d.HasEncryption = bytes.Contains(b, []byte("/Encrypt"))
	objects := make(map[int]*indirect)
	rawObjMatches := reObj.FindAllSubmatch(b, -1)
	for _, m := range rawObjMatches {
		num, _ := strconv.Atoi(string(m[1]))
		gen, _ := strconv.Atoi(string(m[2]))
		body := m[3]
		o := &indirect{id: num, gen: gen, dictionary: body}
		i := bytes.Index(body, []byte("stream"))
		if i >= 0 {
			head := body[:i]
			start := i + len("stream")
			if start < len(body) && body[start] == '\r' {
				start++
			}
			if start < len(body) && body[start] == '\n' {
				start++
			}
			end := bytes.LastIndex(body, []byte("endstream"))
			if end > start {
				raw := body[start:end]
				if length := intGroup(head, reLen); length > 0 && length <= len(raw) {
					raw = raw[:length]
				} else if bytes.HasSuffix(raw, []byte("\r\n")) {
					raw = raw[:len(raw)-2]
				} else if bytes.HasSuffix(raw, []byte("\n")) {
					raw = raw[:len(raw)-1]
				}
				o.dictionary = head
				o.hasStream = true
				if bytes.Contains(head, []byte("/Filter")) {
					d.CompressedStreams++
				}
				out, ok := decodePDFStream(head, raw)
				if !ok {
					o.decodeError = true
					o.filterUnsupported = true
				} else {
					o.decoded = out
				}
			}
		}
		objects[num] = o
	}
	unpackObjectStreams(objects)
	d.TotalObjects = len(objects)
	pages := []*indirect{}
	for _, o := range objects {
		if bytes.Contains(o.dictionary, []byte("/Type /ObjStm")) || bytes.Contains(o.dictionary, []byte("/Type/ObjStm")) {
			d.ObjectStreams++
		}
		if reFont.Match(o.dictionary) {
			d.FontObjects++
		}
		d.ToUnicodeMaps += bytes.Count(o.dictionary, []byte("/ToUnicode"))
	}
	pages = pdfPageOrder(objects)
	d.PageCount = len(pages)
	if d.PageCount == 0 {
		max := 0
		for _, o := range objects {
			v := intGroup(o.dictionary, reCount)
			if v > max && v < 10000 {
				max = v
			}
		}
		d.PageCount = max
	}
	for ix, p := range pages {
		s := PageStats{Page: ix + 1}
		refs := []int{}
		if m := reContents.FindSubmatch(p.dictionary); len(m) > 0 {
			n, _ := strconv.Atoi(string(m[1]))
			refs = append(refs, n)
		}
		if m := reContentsArr.FindSubmatch(p.dictionary); len(m) > 0 {
			for _, r := range reRef.FindAllSubmatch(m[1], -1) {
				n, _ := strconv.Atoi(string(r[1]))
				refs = append(refs, n)
			}
		}
		for _, ref := range refs {
			o, ok := objects[ref]
			if !ok || !o.hasStream {
				continue
			}
			s.StreamCount++
			s.DecodedBytes += len(o.decoded)
			shows, bt, short, vec := contentMetrics(o.decoded)
			prev := s.TextOperators
			s.TextOperators += shows
			s.TextObjects += bt
			s.DrawingOperators += vec
			if s.TextOperators > 0 {
				s.TinyTextRatio = (s.TinyTextRatio*float64(prev) + short*float64(shows)) / float64(s.TextOperators)
			}
			if o.decodeError || o.filterUnsupported {
				d.UnsupportedFilters++
				s.Evidence = "UNSUPPORTED_CONTENT_STREAM"
			}
		}
		if s.Evidence == "" {
			switch {
			case s.StreamCount == 0:
				s.Evidence = "CONTENT_MAPPING_UNAVAILABLE"
			case s.TextOperators == 0:
				s.Evidence = "NO_TEXT_SHOW_OPERATORS"
			case s.TinyTextRatio > .70 && s.TextOperators >= 30:
				s.Evidence = "HIGH_GLYPH_FRAGMENTATION"
			default:
				s.Evidence = "STRUCTURAL_TEXT_PRESENT"
			}
		}
		d.TextOperators += s.TextOperators
		d.ContentStreams += s.StreamCount
		d.Pages = append(d.Pages, s)
	}
	if d.HasEncryption {
		d.Issues = append(d.Issues, issue("PDF-001", "BLOCK", "PDF declares encryption.", "Use an authorized decrypted copy within your approved environment."))
	}
	if len(rawObjMatches) == 0 {
		d.Issues = append(d.Issues, issue("PDF-002", "BLOCK", "No readable indirect objects could be enumerated.", "This PDF may require a more complete standards-compliant parser."))
	}
	if len(pages) == 0 {
		d.Issues = append(d.Issues, issue("PDF-003", "WARN", "Page objects were not individually resolvable.", "Per-page findings are incomplete; do not assume missing pages lack wiring."))
	}
	if d.UnsupportedFilters > 0 {
		d.Issues = append(d.Issues, issue("PDF-004", "WARN", "One or more compressed streams use filters not decoded by this diagnostic tool.", "Check the source in a full PDF reader and escalate its structure to the parser team."))
	}
	noText := 0
	frag := 0
	unknown := 0
	for _, p := range d.Pages {
		if p.Evidence == "NO_TEXT_SHOW_OPERATORS" {
			noText++
		}
		if p.Evidence == "HIGH_GLYPH_FRAGMENTATION" {
			frag++
		}
		if p.Evidence == "CONTENT_MAPPING_UNAVAILABLE" || p.Evidence == "UNSUPPORTED_CONTENT_STREAM" {
			unknown++
		}
	}
	if noText > 0 {
		d.Issues = append(d.Issues, issue("PDF-005", "WARN", fmt.Sprintf("%d page(s) have no detectable text-show operators.", noText), "The page may be raster-only, vector-only, or encoded unusually; use local visual verification."))
	}
	if frag > 0 {
		d.Issues = append(d.Issues, issue("PDF-006", "WARN", fmt.Sprintf("%d page(s) show highly fragmented text operators.", frag), "Use positional glyph grouping or a table-aware extraction path, not simple word matching."))
	}
	if unknown > 0 {
		d.Issues = append(d.Issues, issue("PDF-007", "WARN", fmt.Sprintf("%d page(s) could not be fully linked to decoded content streams.", unknown), "Do not interpret missing detections as absent connectors/BOM items."))
	}
	if d.PageCount == 0 {
		d.Issues = append(d.Issues, issue("PDF-008", "BLOCK", "No reliable document page count found.", "Do not release any cavity checklist."))
	}
	if len(d.Issues) == 0 {
		d.Issues = append(d.Issues, issue("PDF-000", "INFO", "Structural inspection found no supported format blockers.", "This is NOT evidence that connector, netlist, BOM, or tooling recognition is complete."))
	}
	// Ensure every string written to the report comes from fixed source code or
	// computed aggregate numbers. Never put decoded page bytes into Diagnostics.
	return d, nil
}
func diagnoseSummary(d Diagnostics) string {
	ss := []string{fmt.Sprintf("Trivex local diagnostic (%s)", d.Schema), fmt.Sprintf("Pages: %d; indirect objects: %d; text operators: %d", d.PageCount, d.TotalObjects, d.TextOperators)}
	for _, is := range d.Issues {
		ss = append(ss, fmt.Sprintf("%s [%s]: %s", is.Code, is.Severity, is.Detail))
	}
	return strings.Join(ss, "\n")
}
