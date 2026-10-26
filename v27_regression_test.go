package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"time"
)

// signature summarises the engineering records independent of encoding.
func signature(a Analysis) string {
	parts := []string{}
	for _, c := range a.Connectors {
		parts = append(parts, "C "+c.Tag+" "+c.Part)
	}
	for _, w := range a.Wires {
		parts = append(parts, fmt.Sprintf("W %s:%s>%s:%s sig=%s wire=%s color=%s", w.Connector, w.Cavity, w.End, w.EndCavity, w.Signal, w.Wire, w.Color))
	}
	for _, b := range a.BOM {
		parts = append(parts, fmt.Sprintf("B %s|%s|%s|%s", b.Item, b.Qty, b.Part, b.Reference))
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}

// Every encoding of the same drawing must produce identical records.
func TestAllEncodingsProduceIdenticalRecords(t *testing.T) {
	var ref string
	for _, v := range synthVariants {
		b, err := BuildSyntheticVariant(v.Name)
		if err != nil {
			t.Fatal(err)
		}
		a, err := AnalyzeDocument(b)
		if err != nil {
			t.Fatalf("%s: %v", v.Name, err)
		}
		if len(a.Connectors) != 2 || len(a.Wires) != 7 || len(a.BOM) != 3 {
			t.Fatalf("%s: records %d/%d/%d, want 2/7/3", v.Name, len(a.Connectors), len(a.Wires), len(a.BOM))
		}
		sig := signature(a)
		if ref == "" {
			ref = sig
			for _, want := range []string{"W J12:1>J21:3 sig=VBUS", "W P2:A>TB1:4 sig=+28V wire=W101 color=RED", "B 3|A/R|M23053/5-104-0", "C J12 D38999/26FG35SA"} {
				if !strings.Contains(sig, want) {
					t.Fatalf("reference record missing %q in\n%s", want, sig)
				}
			}
			continue
		}
		if sig != ref {
			t.Fatalf("%s produced different records:\n%s\n--- reference ---\n%s", v.Name, sig, ref)
		}
	}
}

// Characterisation of the v2.6 defect: the legacy interpreter loses all
// wiring/BOM rows on rotated pages and everything on glyph-split text.
func TestLegacyInterpreterFailsWhereV27Succeeds(t *testing.T) {
	for _, name := range []string{"rotate90", "rotate180", "rotate270", "sideways", "nestedmixed", "glyphs"} {
		b, _ := BuildSyntheticVariant(name)
		d, err := extractPDFEnhanced26(b)
		if err != nil {
			t.Fatal(err)
		}
		old, _ := AnalyzeData(d)
		if len(old.Wires)+len(old.BOM) != 0 {
			t.Fatalf("%s: legacy unexpectedly read %d wires / %d BOM", name, len(old.Wires), len(old.BOM))
		}
	}
}

func TestOutlinedTextIsFailedInterpretationNotEmptyDrawing(t *testing.T) {
	x, err := ExtractDocument(BuildSyntheticOutlinedText())
	if err != nil {
		t.Fatal(err)
	}
	v, err := viewFromExtraction(x)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(v.Interpretation, "FAILED") {
		t.Fatalf("interpretation %q", v.Interpretation)
	}
	codes := map[string]string{}
	for _, i := range v.Issues {
		codes[i.Code] = i.Category
	}
	for _, c := range []string{"PDF_PARSE_INCOMPLETE", "TEXT_AS_GRAPHICS", "NO_CONNECTORS", "NO_CAVITY_ROWS", "BOM_NOT_FOUND"} {
		if codes[c] != CatExtraction {
			t.Fatalf("finding %s category %q (all: %v)", c, codes[c], codes)
		}
	}
	for _, i := range v.Issues {
		if i.Category == CatConflict {
			t.Fatalf("extraction failure reported as drawing conflict: %+v", i)
		}
	}
	if v.Diagnostics[0].Status != "NO READABLE TEXT — manual page review" {
		t.Fatalf("diagnostic status %q", v.Diagnostics[0].Status)
	}
	// Region review on a text-less area must say so instead of returning nothing.
	r, err := analyzeRegion(x, RegionRequest{Page: 1, Box: Box{50, 40, 400, 300}, Kind: "bom"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Fragments != 0 || !strings.Contains(r.Note, "No machine-readable text") {
		t.Fatalf("region note %q", r.Note)
	}
}

func TestRegionReadsRotatedPinoutTable(t *testing.T) {
	b, _ := BuildSyntheticVariant("rotate270")
	x, err := ExtractDocument(b)
	if err != nil {
		t.Fatal(err)
	}
	r, err := analyzeRegion(x, RegionRequest{Page: 1, Box: Box{490, 300, 790, 370}, Kind: "pinout"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.AutoWires) != 3 || r.AutoWires[0].Connector != "P2" || r.HeaderRow != 1 {
		t.Fatalf("auto rows %+v header %d", r.AutoWires, r.HeaderRow)
	}
	if strings.Join(r.Roles, ",") != "pin,signal,wire,color,to" {
		t.Fatalf("roles %v", r.Roles)
	}
	// Sideways page: the selection is read in the text's own direction.
	b, _ = BuildSyntheticVariant("sideways")
	x, _ = ExtractDocument(b)
	r, _ = analyzeRegion(x, RegionRequest{Page: 1, Box: Box{300, 0, 380, 300}, Kind: "pinout"})
	if r.Frame != 90 || len(r.AutoWires) != 3 {
		t.Fatalf("sideways region frame %d rows %d lines %v", r.Frame, len(r.AutoWires), r.Lines)
	}
}

func TestBOMRegionWithoutTitle(t *testing.T) {
	b, _ := BuildSyntheticVariant("upright")
	x, _ := ExtractDocument(b)
	// Select only the table rows and heading, not the "BILL OF MATERIALS" title.
	r, err := analyzeRegion(x, RegionRequest{Page: 2, Box: Box{50, 100, 560, 165}, Kind: "bom"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.AutoBOM) != 3 || r.AutoBOM[1].Part != "M39029/56-348" {
		t.Fatalf("BOM rows %+v lines %v", r.AutoBOM, r.Lines)
	}
}

func TestRepeatedIdenticalAssignmentIsCorroborationNotConflict(t *testing.T) {
	rows := []WireRow{
		{Page: 1, Connector: "P1", Cavity: "5", End: "J2", EndCavity: "3", Signal: "A"},
		{Page: 4, Connector: "P1", Cavity: "5", End: "J2", EndCavity: "3", Wire: "W5"},
		{Page: 1, Connector: "P1", Cavity: "6", End: "J2", EndCavity: "4"},
		{Page: 5, Connector: "P1", Cavity: "6", End: "J3", EndCavity: "1"},
	}
	m := mergeWireRows(rows)
	if len(m) != 3 {
		t.Fatalf("merged %d rows: %+v", len(m), m)
	}
	if m[0].Corroboration != 2 || m[0].Wire != "W5" || m[0].Signal != "A" {
		t.Fatalf("corroborated row %+v", m[0])
	}
	a, _ := AnalyzeData(PDFData{})
	_ = a
}

func TestReferenceLibrariesMatchSchema(t *testing.T) {
	if err := validateReferenceLibraries(); err != nil {
		t.Fatal(err)
	}
	rows, err := crimpReference()
	if err != nil || len(rows) < 250 {
		t.Fatalf("crimp reference %d rows, err %v", len(rows), err)
	}
}

func TestSelfTestPasses(t *testing.T) {
	r := RunSelfTest()
	if !r.Pass {
		t.Fatalf("self-test failed: %+v", r.Cases)
	}
}

func TestLifecycleOnlyExplicitCloseWhileEdgeAlive(t *testing.T) {
	l := &lifecycle{}
	start := time.Now().Add(-10 * time.Minute)
	if r := l.expired(start, true); r != "" {
		t.Fatalf("expired while Edge alive without beats: %q", r)
	}
	l.beat()
	l.lastBeat = time.Now().Add(-heartbeatTimeout - time.Second)
	if r := l.expired(start, true); r != "" {
		t.Fatalf("minimized window (no beats) must not stop the backend while Edge is alive: %q", r)
	}
	if r := l.expired(start, false); r == "" {
		t.Fatal("heartbeat timeout ignored without tracked Edge process")
	}
	l.beat()
	l.closing()
	l.closingAt = time.Now().Add(-closingGrace - time.Second)
	if r := l.expired(start, true); r == "" {
		t.Fatal("explicit window close ignored")
	}
	l.beat() // a reload sends a new heartbeat and cancels the close
	if r := l.expired(start, true); r != "" {
		t.Fatalf("reload treated as close: %q", r)
	}
}

// Writes the synthetic variants for manual inspection: OUTDIR=... go test -run TestWriteSyntheticPDFs
func TestWriteSyntheticPDFs(t *testing.T) {
	d := os.Getenv("OUTDIR")
	if d == "" {
		t.Skip("set OUTDIR to write fixtures")
	}
	for _, v := range synthVariants {
		b, _ := BuildSyntheticVariant(v.Name)
		os.WriteFile(d+"/SYNTH_"+v.Name+".pdf", b, 0644)
	}
	os.WriteFile(d+"/SYNTH_outlined_text_negative.pdf", BuildSyntheticOutlinedText(), 0644)
	os.WriteFile(d+"/SYNTH_asme_tables.pdf", BuildSyntheticASME(), 0644)
	os.WriteFile(d+"/SYNTH_house_standard_headings.pdf", BuildSyntheticUnknownHeadings(), 0644)
}
