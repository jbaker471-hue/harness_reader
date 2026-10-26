package main

import (
	"math"
	"testing"
)

// The computed placement box of every picture must coincide with the pixels
// PDFium actually renders, and the gallery crop must look like the page.
func TestImagePlacementMatchesRenderedPage(t *testing.T) {
	upright := [4]string{"red", "green", "blue", "yellow"}
	for _, v := range synthVariants {
		v := v
		t.Run(v.Name, func(t *testing.T) {
			b, err := BuildSyntheticVariant(v.Name)
			if err != nil {
				t.Fatal(err)
			}
			engineMu.Lock()
			defer engineMu.Unlock()
			d, err := openPDF(b)
			if err != nil {
				t.Fatal(err)
			}
			defer d.close()
			pl, err := d.layoutPage(0)
			if err != nil {
				t.Fatal(err)
			}
			if math.Abs(pl.Width-v.DisplayW) > 0.5 || math.Abs(pl.Height-v.DisplayH) > 0.5 {
				t.Fatalf("display size %vx%v, want %vx%v", pl.Width, pl.Height, v.DisplayW, v.DisplayH)
			}
			if len(pl.Images) != 1 {
				t.Fatalf("images = %d, want 1", len(pl.Images))
			}
			im := pl.Images[0]
			const scale = 2.0
			page, err := d.renderPage(0, scale)
			if err != nil {
				t.Fatal(err)
			}
			crop := cropDisplay(page, scale, Box{im.Box.X0 + 2, im.Box.Y0 + 2, im.Box.X1 - 2, im.Box.Y1 - 2}, 0)
			got := quadrants(crop)
			want := upright
			switch {
			case v.ImageMirrored:
				want = [4]string{"green", "red", "yellow", "blue"}
			case v.Name == "sideways":
				// whole drawing turned 90° counter-clockwise on the page
				want = [4]string{"green", "yellow", "red", "blue"}
			}
			if got != want {
				t.Fatalf("rendered crop quadrants %v, want %v (box %+v)", got, want, im.Box)
			}
			// The raw stored pixels differ from the page exactly when expected.
			raw, err := d.rawImagePixels(0, im.ID)
			if err != nil {
				t.Fatal(err)
			}
			rawQ := quadrants(raw)
			if v.RawFlipped != (rawQ != got) {
				t.Fatalf("raw stored quadrants %v vs displayed %v; RawFlipped=%v", rawQ, got, v.RawFlipped)
			}
			if v.Name == "topdown" && (!im.Mirrored || im.Rotation != 0) {
				t.Fatalf("bottom-up storage not reported: %+v", im)
			}
		})
	}
}
