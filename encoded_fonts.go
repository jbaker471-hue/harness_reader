package main

import (
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"
)

var (
	reEvent       = regexp.MustCompile(`(?s)([-+]?\d+(?:\.\d+)?\s+[-+]?\d+(?:\.\d+)?\s+[-+]?\d+(?:\.\d+)?\s+[-+]?\d+(?:\.\d+)?\s+[-+]?\d+(?:\.\d+)?\s+[-+]?\d+(?:\.\d+)?\s+Tm\b|[-+]?\d+(?:\.\d+)?\s+[-+]?\d+(?:\.\d+)?\s+T[dD]\b|[-+]?\d+(?:\.\d+)?\s+TL\b|/\S+\s+[-+]?\d+(?:\.\d+)?\s+Tf\b|T\*|\[(?:\([^)]*(?:\\.[^)]*)*\)|[^]])*\]\s*TJ|\((?:\\.|[^\\)])*\)\s*Tj|<[0-9A-Fa-f\s]+>\s*Tj)`)
	reFontGroup26 = regexp.MustCompile(`(?s)/Font\s*<<(.*?)>>`)
	reFontMap26   = regexp.MustCompile(`/([A-Za-z0-9_.-]+)\s+(\d+)\s+\d+\s+R`)
	reToUnicode26 = regexp.MustCompile(`/ToUnicode\s+(\d+)\s+\d+\s+R`)
	reBfChar26    = regexp.MustCompile(`(?s)(\d+)\s+beginbfchar(.*?)endbfchar`)
	reBfRange26   = regexp.MustCompile(`(?s)(\d+)\s+beginbfrange(.*?)endbfrange`)
	reCMapLine26  = regexp.MustCompile(`(?m)^\s*<([A-Fa-f0-9]+)>\s+<([A-Fa-f0-9]+)>(?:\s+<([A-Fa-f0-9]+)>)?`)
	reCodespace26 = regexp.MustCompile(`(?s)begincodespacerange\s*<([0-9A-Fa-f]+)>\s*<([0-9A-Fa-f]+)>`)
	reUseFont26   = regexp.MustCompile(`/([A-Za-z0-9_.-]+)\s+[-+\d.]+\s+Tf`)
)

type fontMap26 struct {
	length int
	glyphs map[string]string
}

func fromUTF16Hex26(input string) string {
	b, e := hex.DecodeString(input)
	if e != nil {
		return ""
	}
	if len(b)%2 > 0 {
		return string(b)
	}
	u := []uint16{}
	for i := 0; i < len(b); i += 2 {
		u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
	}
	return string(utf16.Decode(u))
}
func cmap26(data []byte) fontMap26 {
	c := fontMap26{length: 2, glyphs: map[string]string{}}
	if m := reCodespace26.FindSubmatch(data); len(m) > 1 {
		c.length = len(m[1]) / 2
	}
	// bfrange may encode a contiguous Unicode range, including supplementary glyphs.
	for _, group := range reBfRange26.FindAllSubmatch(data, -1) {
		for _, row := range reCMapLine26.FindAllSubmatch(group[2], -1) {
			if len(row[3]) == 0 {
				continue
			}
			start, e1 := strconv.ParseUint(string(row[1]), 16, 32)
			end, e2 := strconv.ParseUint(string(row[2]), 16, 32)
			base, e3 := strconv.ParseUint(string(row[3]), 16, 32)
			if e1 != nil || e2 != nil || e3 != nil || end < start || end-start > 20000 {
				continue
			}
			width := len(row[1])
			for v := start; v <= end; v++ {
				code := strings.ToUpper(strconv.FormatUint(v, 16))
				code = strings.Repeat("0", max26(0, width-len(code))) + code
				u := base + (v - start)
				if u <= 0xFFFF {
					c.glyphs[code] = string(rune(u))
				} else if u <= 0x10FFFF {
					c.glyphs[code] = string(rune(u))
				}
			}
		}
	}
	for _, group := range reBfChar26.FindAllSubmatch(data, -1) {
		for _, row := range reCMapLine26.FindAllSubmatch(group[2], -1) {
			if len(row[3]) != 0 {
				continue
			}
			c.glyphs[strings.ToUpper(string(row[1]))] = fromUTF16Hex26(string(row[2]))
		}
	}
	return c
}
func max26(x, y int) int {
	if x > y {
		return x
	}
	return y
}
func fontResources26(obj *indirect, objects map[int]*indirect, depth int) map[string]fontMap26 {
	out := map[string]fontMap26{}
	if obj == nil || depth > 12 {
		return out
	}
	var collect func([]byte)
	collect = func(dict []byte) {
		for _, part := range reFontGroup26.FindAllSubmatch(dict, -1) {
			for _, m := range reFontMap26.FindAllSubmatch(part[1], -1) {
				id, _ := strconv.Atoi(string(m[2]))
				font := objects[id]
				if font == nil {
					continue
				}
				if match := reToUnicode26.FindSubmatch(font.dictionary); len(match) > 1 {
					n, _ := strconv.Atoi(string(match[1]))
					cmap := objects[n]
					if cmap != nil && cmap.hasStream {
						out[string(m[1])] = cmap26(cmap.decoded)
					}
				}
			}
		}
		if m := resourceRef26.FindSubmatch(dict); len(m) > 1 {
			id, _ := strconv.Atoi(string(m[1]))
			if r := objects[id]; r != nil {
				for k, v := range fontResources26(r, objects, depth+1) {
					out[k] = v
				}
			}
		}
	}
	if m := rePageParent.FindSubmatch(obj.dictionary); len(m) > 1 {
		id, _ := strconv.Atoi(string(m[1]))
		for k, v := range fontResources26(objects[id], objects, depth+1) {
			out[k] = v
		}
	}
	collect(obj.dictionary)
	return out
}

// decode hex-coded glyph streams using the font's PDF ToUnicode CMap.
func decodeFontParts26(event string, c fontMap26) string {
	parts := reStr.FindAllString(event, -1)
	out := strings.Builder{}
	for _, p := range parts {
		if !strings.HasPrefix(p, "<") {
			out.WriteString(pdfDecodeString(p))
			continue
		}
		raw := strings.ToUpper(strings.Join(strings.Fields(strings.Trim(p, "<>")), ""))
		stride := c.length * 2
		if stride <= 0 {
			stride = 4
		}
		for i := 0; i+stride <= len(raw); i += stride {
			key := raw[i : i+stride]
			if text, ok := c.glyphs[key]; ok {
				out.WriteString(text)
			}
		}
	}
	return out.String()
}

// A decoder-aware variant of the existing operator parser. For pages using
// arbitrary embedded font glyph IDs the old decoder outputs noise.
func parseBlockFont26(b string, fonts map[string]fontMap26) []Word {
	if len(fonts) == 0 {
		return parseBlock(b)
	}
	out := []Word{}
	x, y, size, leading := 0.0, 0.0, 8.0, 0.0
	active := ""
	for _, loc := range reEvent.FindAllStringIndex(b, -1) {
		event := b[loc[0]:loc[1]]
		if m := reTm.FindStringSubmatch(event); len(m) > 6 {
			x, y = f64(m[5]), f64(m[6])
			continue
		}
		if m := reTd.FindStringSubmatch(event); len(m) > 2 {
			x += f64(m[1])
			y += f64(m[2])
			if strings.HasSuffix(event, "TD") {
				leading = -f64(m[2])
			}
			continue
		}
		if strings.TrimSpace(event) == "T*" {
			y -= leading
			continue
		}
		if m := reTf.FindStringSubmatch(event); len(m) > 1 {
			size = f64(m[1])
			if m2 := reUseFont26.FindStringSubmatch(event); len(m2) > 1 {
				active = m2[1]
			}
			continue
		}
		if strings.HasSuffix(strings.TrimSpace(event), "TL") {
			v := strings.Fields(event)
			if len(v) > 0 {
				leading = f64(v[0])
			}
			continue
		}
		text := ""
		if font, ok := fonts[active]; ok {
			text = decodeFontParts26(event, font)
		} else {
			for _, str := range reStr.FindAllString(event, -1) {
				text += pdfDecodeString(str)
			}
		}
		text = strings.TrimSpace(text)
		if len(text) == 0 || len(text) > 1500 {
			continue
		}
		out = append(out, Word{Text: text, X: x, Y: y, Size: size})
		x += float64(len([]rune(text))) * size * 0.48
	}
	return out
}
