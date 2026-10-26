package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"regexp"
	"strconv"
)

type EmbeddedImage struct {
	Page    int    `json:"page"`
	Name    string `json:"name"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	MIME    string `json:"mime"`
	Data    string `json:"data"`
	Caption string `json:"caption"`
}

var (
	reImgWidth  = regexp.MustCompile(`/Width\s+(\d+)`)
	reImgHeight = regexp.MustCompile(`/Height\s+(\d+)`)
	reBits      = regexp.MustCompile(`/BitsPerComponent\s+(\d+)`)
)

func (o *indirect) embeddedImageBytes() (string, []byte, bool) {
	if o == nil {
		return "", nil, false
	}
	if bytes.Contains(o.dictionary, []byte("/DCTDecode")) {
		if len(o.encoded) > 6 && bytes.HasPrefix(o.encoded, []byte{0xff, 0xd8}) {
			if _, err := jpeg.DecodeConfig(bytes.NewReader(o.encoded)); err == nil {
				return "image/jpeg", o.encoded, true
			}
		}
		return "", nil, false
	}
	// A subset of non-predictor 8-bit Flate image XObjects can be reconstructed.
	if !bytes.Contains(o.dictionary, []byte("/FlateDecode")) || bytes.Contains(o.dictionary, []byte("/DecodeParms")) {
		return "", nil, false
	}
	width, height := intGroup(o.dictionary, reImgWidth), intGroup(o.dictionary, reImgHeight)
	bits := intGroup(o.dictionary, reBits)
	if width < 1 || height < 1 || width > 6000 || height > 6000 || bits != 8 || width*height > 9_000_000 {
		return "", nil, false
	}
	channels := 0
	switch {
	case bytes.Contains(o.dictionary, []byte("/DeviceRGB")):
		channels = 3
	case bytes.Contains(o.dictionary, []byte("/DeviceGray")):
		channels = 1
	default:
		return "", nil, false
	}
	if len(o.decoded) != width*height*channels {
		return "", nil, false
	}
	rgba := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			i := (y*width + x) * channels
			if channels == 1 {
				v := o.decoded[i]
				rgba.Set(x, y, color.RGBA{v, v, v, 255})
			} else {
				rgba.Set(x, y, color.RGBA{o.decoded[i], o.decoded[i+1], o.decoded[i+2], 255})
			}
		}
	}
	var buf bytes.Buffer
	if e := png.Encode(&buf, rgba); e != nil {
		return "", nil, false
	}
	return "image/png", buf.Bytes(), true
}
func embeddedImageGallery(pdf []byte) ([]EmbeddedImage, error) {
	objs := parseRawObjects(pdf)
	pages := pdfPageOrder(objs)
	if len(pages) == 0 {
		return nil, fmt.Errorf("no readable PDF page tree")
	}
	items := []EmbeddedImage{}
	totalPreviewBytes := 0
	used := map[string]bool{}
	for pi, p := range pages {
		refs := map[string]int{}
		var walk func(*indirect, int)
		walk = func(o *indirect, dep int) {
			if o == nil || dep > 8 {
				return
			}
			for _, m := range reXObjectResource.FindAllSubmatch(o.dictionary, -1) {
				for _, match := range reXObjectReference.FindAllSubmatch(m[1], -1) {
					n, _ := strconv.Atoi(string(match[2]))
					refs[string(match[1])] = n
				}
			}
			if m := rePageParent.FindSubmatch(o.dictionary); len(m) > 1 {
				n, _ := strconv.Atoi(string(m[1]))
				walk(objs[n], dep+1)
			}
		}
		walk(p, 0)
		for _, streamID := range contentRefs(p) {
			stream := objs[streamID]
			if stream == nil {
				continue
			}
			for _, m := range rePDFDo.FindAllSubmatch(stream.decoded, -1) {
				name := string(m[1])
				key := fmt.Sprintf("%d/%s", pi, name)
				if used[key] {
					continue
				}
				used[key] = true
				o := objs[refs[name]]
				if o == nil || (!bytes.Contains(o.dictionary, []byte("/Subtype /Image")) && !bytes.Contains(o.dictionary, []byte("/Subtype/Image"))) {
					continue
				}
				mime, payload, ok := o.embeddedImageBytes()
				if !ok || len(payload) > 3_000_000 || totalPreviewBytes+len(payload) > 12_000_000 {
					continue
				}
				im := EmbeddedImage{Page: pi + 1, Name: name, Width: intGroup(o.dictionary, reImgWidth), Height: intGroup(o.dictionary, reImgHeight), MIME: mime, Data: base64.StdEncoding.EncodeToString(payload), Caption: "Embedded image, identity not determined (could be connector, logo or other artwork)"}
				items = append(items, im)
				totalPreviewBytes += len(payload)
				if len(items) >= 32 {
					return items, nil
				}
			}
		}
	}
	return items, nil
}
