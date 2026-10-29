package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"image"
	"math"
	"strings"
	"time"
)

func sha256Hex(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// ---------------------------------------------------------------------------
// Reference library schema validation (loud failure, never silent skips).

var librarySchemas = map[string][]string{
	"CONNECTOR_LAYOUT_REFERENCE.csv":     {"family", "arrangement", "cavity_ids", "source", "status"},
	"DMC_EXPANDED_REFERENCE.csv":         {"contact", "superseded_aliases", "type", "mating_size", "barrel_size", "wire_range", "alternate_crimp_paths", "insertion", "removal", "source_locator", "verification_status"},
	"MANUFACTURER_CONTACT_REFERENCE.csv": {"Manufacturer", "Contact P/N", "Alternate Identities", "Family", "Sex", "Wire AWG range", "Required Operation / Tool Combinations", "Verified Tooling Status", "Reference URL", "Review Notes"},
	"SPECIALTY_CONTACTS_REFERENCE.csv":   {"contact", "family", "kind", "sex", "wire", "source", "status", "operations"},
	"ACCESSORY_REFERENCE.csv":            {"kind", "match", "class", "requirement", "source", "status", "specification", "alternates", "alternate_source", "alternate_status"},
	"WIRE_CABLE_REFERENCE.csv":           {"pattern", "family", "construction", "preparation", "termination", "test", "source", "status"},
	"TOOL_EQUIVALENCE_REFERENCE.csv":     {"specification", "operation", "primary", "alternates", "conditions", "source_primary", "source_alternate", "status"},
}

func readLibrary(name string) ([][]string, error) {
	b, err := assets.ReadFile("assets/" + name)
	if err != nil {
		return nil, err
	}
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf})))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	want, ok := librarySchemas[name]
	if !ok {
		return nil, fmt.Errorf("%s: no schema registered", name)
	}
	if len(rows) == 0 || strings.Join(rows[0], "|") != strings.Join(want, "|") {
		return nil, fmt.Errorf("%s: header does not match the expected %d-column schema", name, len(want))
	}
	for i, row := range rows[1:] {
		if len(row) != len(want) {
			return nil, fmt.Errorf("%s: data row %d has %d columns, expected %d", name, i+2, len(row), len(want))
		}
	}
	return rows, nil
}

func validateReferenceLibraries() error {
	for name := range librarySchemas {
		if _, err := readLibrary(name); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// In-app self-test: proves on the user's own PC that the bundled PDF engine
// interprets rotated, flipped, form-nested and glyph-split drawings the same
// way, and shows pictures in their displayed orientation. Uses only built-in
// synthetic NON-CUI drawings; nothing is read from disk or sent anywhere.

type SelfTestCase struct {
	Variant     string `json:"variant"`
	Description string `json:"description"`
	Pass        bool   `json:"pass"`
	Detail      string `json:"detail"`
}

type SelfTestReport struct {
	Version  string         `json:"version"`
	Engine   string         `json:"engine"`
	Pass     bool           `json:"pass"`
	Cases    []SelfTestCase `json:"cases"`
	Duration string         `json:"duration"`
}

func RunSelfTest() SelfTestReport {
	t0 := time.Now()
	rep := SelfTestReport{Version: AppVersion, Engine: EngineName(), Pass: true, Cases: []SelfTestCase{}}
	if err := validateReferenceLibraries(); err != nil {
		rep.Pass = false
		rep.Cases = append(rep.Cases, SelfTestCase{Variant: "reference-libraries", Description: "Tooling CSV schema", Pass: false, Detail: err.Error()})
	} else {
		rep.Cases = append(rep.Cases, SelfTestCase{Variant: "reference-libraries", Description: "Tooling CSV schema", Pass: true, Detail: "All reference libraries match their column schema"})
	}
	for _, v := range synthVariants {
		c := SelfTestCase{Variant: v.Name, Description: v.Description}
		err := selfTestVariant(v)
		c.Pass = err == nil
		if err != nil {
			c.Detail = err.Error()
			rep.Pass = false
		} else {
			c.Detail = "2 connectors, 7 conductor candidates, 3 BOM rows; picture orientation matches the rendered page"
		}
		rep.Cases = append(rep.Cases, c)
	}
	// Defense-drawing table conventions (parts list built upward, two-line
	// headings, grouped FROM/TO wire list, connector table via FIND NO.).
	c := SelfTestCase{Variant: "asme-tables", Description: "Parts list with heading at the bottom, two-line headings, FROM/TO wire list, connector table, sealing plug/cap excluded"}
	if err := selfTestASME(); err != nil {
		c.Detail, rep.Pass = err.Error(), false
	} else {
		c.Pass, c.Detail = true, "P1/J2 resolved via connector table; 4 conductors (3 corroborated); 6 BOM rows; accessories not counted as connectors"
	}
	rep.Cases = append(rep.Cases, c)
	rep.Duration = time.Since(t0).Round(time.Millisecond).String()
	return rep
}

func selfTestVariant(v synthVariant) error {
	b, err := BuildSyntheticVariant(v.Name)
	if err != nil {
		return err
	}
	x, err := ExtractDocument(b)
	if err != nil {
		return err
	}
	if x.Legacy {
		return fmt.Errorf("PDFium unavailable: %s", x.EngineError)
	}
	a, _, _, err := AnalyzeExtraction(x)
	if err != nil {
		return err
	}
	if len(a.Connectors) != 2 || len(a.Wires) != 7 || len(a.BOM) != 3 {
		return fmt.Errorf("records %d/%d/%d, expected 2/7/3", len(a.Connectors), len(a.Wires), len(a.BOM))
	}
	want := map[string]string{"J12:1": "J21:3", "J12:4": "J21:6", "P2:A": "TB1:4", "P2:C": "TB1:6"}
	for _, w := range a.Wires {
		k := w.Connector + ":" + w.Cavity
		if exp, ok := want[k]; ok {
			if got := w.End + ":" + w.EndCavity; got != exp {
				return fmt.Errorf("%s maps to %s, expected %s", k, got, exp)
			}
			delete(want, k)
		}
	}
	if len(want) > 0 {
		return fmt.Errorf("missing assignments %v", want)
	}
	pl := x.Layouts[0]
	if math.Abs(pl.Width-v.DisplayW) > 0.5 || math.Abs(pl.Height-v.DisplayH) > 0.5 || len(pl.Images) != 1 {
		return fmt.Errorf("page geometry %vx%v / %d pictures", pl.Width, pl.Height, len(pl.Images))
	}
	engineMu.Lock()
	d, err := openPDF(b)
	if err != nil {
		engineMu.Unlock()
		return err
	}
	img, err := d.renderPage(0, 2)
	d.close()
	engineMu.Unlock()
	if err != nil {
		return err
	}
	im := pl.Images[0]
	got := quadrants(cropDisplay(img, 2, Box{im.Box.X0 + 2, im.Box.Y0 + 2, im.Box.X1 - 2, im.Box.Y1 - 2}, 0))
	exp := [4]string{"red", "green", "blue", "yellow"}
	if v.ImageMirrored {
		exp = [4]string{"green", "red", "yellow", "blue"}
	} else if v.Name == "sideways" {
		exp = [4]string{"green", "yellow", "red", "blue"}
	}
	if got != exp {
		return fmt.Errorf("picture crop shows %v, expected %v", got, exp)
	}
	return nil
}

func quadrants(img image.Image) [4]string {
	b := img.Bounds()
	name := func(x, y int) string {
		r, g, bl, _ := img.At(x, y).RGBA()
		r, g, bl = r>>8, g>>8, bl>>8
		switch {
		case r > 180 && g < 90 && bl < 90:
			return "red"
		case g > 140 && r < 90 && bl < 110:
			return "green"
		case bl > 160 && r < 100:
			return "blue"
		case r > 180 && g > 160 && bl < 90:
			return "yellow"
		}
		return "?"
	}
	w, h := b.Dx(), b.Dy()
	at := func(fx, fy float64) string { return name(b.Min.X+int(fx*float64(w)), b.Min.Y+int(fy*float64(h))) }
	return [4]string{at(.25, .25), at(.75, .25), at(.25, .75), at(.75, .75)}
}

func selfTestASME() error {
	a, err := AnalyzeDocument(BuildSyntheticASME())
	if err != nil {
		return err
	}
	conns := map[string]string{}
	for _, c := range a.Connectors {
		conns[c.Tag] = c.Part
	}
	if len(a.Connectors) != 2 || conns["P1"] != "D38999/26FJ35SN" || conns["J2"] != "D38999/26FA35PN" {
		return fmt.Errorf("connectors %v", conns)
	}
	if len(a.BOM) != 6 || len(a.Wires) != 4 {
		return fmt.Errorf("records BOM %d / conductors %d, expected 6 / 4", len(a.BOM), len(a.Wires))
	}
	for _, w := range a.Wires {
		exp := map[string]string{"1": "J2:A", "2": "J2:B", "3": "J2:C", "4": "J2:D"}[w.Cavity]
		if w.Connector != "P1" || w.End+":"+w.EndCavity != exp {
			return fmt.Errorf("P1:%s maps to %s:%s", w.Cavity, w.End, w.EndCavity)
		}
	}
	return nil
}
