package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGraphicPageEvidence(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "SYNTHETIC_GRAPHIC_CONNECTOR.pdf"))
	if err != nil {
		t.Fatal(err)
	}
	gg := graphicInfoFromPDF(b)
	if len(gg) != 2 {
		t.Fatalf("want 2 pages, got %d", len(gg))
	}
	if !gg[0].GraphicsPresent || gg[0].VectorSegments == 0 {
		t.Fatalf("vector connector illustration should be counted: %#v", gg[0])
	}
	if gg[1].ImageObjects == 0 {
		t.Fatalf("raster illustration should be counted: %#v", gg[1])
	}
	v, err := viewAnalysis(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.GraphicPages) != 2 {
		t.Fatalf("graphic page data missing from API: %#v", v.GraphicPages)
	}
	if v.Pages != 2 {
		t.Fatal("wrong page count")
	}
}

func TestEmbeddedImageExtraction(t *testing.T) {
	b, e := os.ReadFile(filepath.Join("testdata", "SYNTHETIC_GRAPHIC_CONNECTOR.pdf"))
	if e != nil {
		t.Fatal(e)
	}
	images, e := embeddedImageGallery(b)
	if e != nil {
		t.Fatal(e)
	}
	if len(images) == 0 {
		t.Fatal("synthetic embedded image not retrieved from image XObject")
	}
	if images[0].Page != 2 || images[0].Width != 250 || images[0].Height != 180 {
		t.Fatalf("image metadata not associated with page 2: %#v", images[0])
	}
	if images[0].MIME != "image/png" && images[0].MIME != "image/jpeg" {
		t.Fatalf("unexpected image type %s", images[0].MIME)
	}
}
