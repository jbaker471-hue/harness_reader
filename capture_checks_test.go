package main

import (
	"fmt"
	"testing"
)

// An invented point-to-point harness shaped like a typical two-connector, shielded-pair drawing:
// 48 two-conductor cables (96 wires) between two 128-position connectors, 18 ft long.
func inventedPairedAnalysis() (Analysis, []GraphicCallout, []HarnessLength) {
	a := Analysis{}
	a.Connectors = []Connector{{Tag: "TEST-J1", Part: "EX-1000/26AB11PA", CavityCount: 128}, {Tag: "BENCH-P2", Part: "EX-1000/26AB11SA", CavityCount: 128}}
	for i := 1; i <= 96; i++ {
		page := 3
		if i > 38 {
			page = 4
		}
		if i > 76 {
			page = 5
		}
		a.Wires = append(a.Wires, WireRow{Page: page, Connector: "TEST-J1", Cavity: fmt.Sprint(i), End: "BENCH-P2", EndCavity: fmt.Sprint(i)})
	}
	a.BOM = []BOMRow{
		{Item: "1", Qty: "1", Part: "EX-1000/26AB11PA", Description: "EXAMPLE CONNECTOR 128P PLUG MALE", Reference: "TEST-J1"},
		{Item: "2", Qty: "1", Part: "EX-1000/26AB11SA", Description: "EXAMPLE CONNECTOR 128P PLUG FEMALE", Reference: "BENCH-P2"},
		{Item: "4", Qty: "666", Part: "EX-CABLE-24", Description: "CABLE, 24 AWG, 2 COND, SHIELDED"},
		{Item: "5", Qty: "198", Part: "EX-CABLE-22", Description: "CABLE, 22 AWG, 2 COND, SHIELDED"},
		{Item: "12", Qty: "64", Part: "EX-SEAL-22", Description: "22AWG SEALING PLUG"},
	}
	co := []GraphicCallout{
		{Page: 3, Reference: "4", Quantity: "19"}, {Page: 4, Reference: "4", Quantity: "18"},
		{Page: 4, Reference: "5"}, {Page: 5, Reference: "5", Quantity: "10"},
		{Page: 2, Reference: "6"}, // a callout for something that is not a cable
	}
	return a, co, []HarnessLength{{Page: 2, Label: "DIMENSION", Text: "216", Tolerance: "-0.0/+1.0"}}
}

func checkByID(cs []CaptureCheck, id, conn string) *CaptureCheck {
	for i := range cs {
		if cs[i].ID == id && cs[i].Connector == conn {
			return &cs[i]
		}
	}
	return nil
}

func TestCaptureChecksAgreeWhenEveryReadingMatches(t *testing.T) {
	a, co, ls := inventedPairedAnalysis()
	cs := captureChecks(&a, co, ls)
	cc := checkByID(cs, "CONDUCTOR_COUNT", "")
	if cc == nil || cc.Status != CheckAgree || len(cc.Methods) != 3 {
		t.Fatalf("conductor count: %+v", cc)
	}
	for _, m := range cc.Methods {
		if m.Value != "96" {
			t.Fatalf("method %q read %s, want 96", m.Name, m.Value)
		}
	}
	for _, tag := range []string{"TEST-J1", "BENCH-P2"} {
		u := checkByID(cs, "UNUSED_CAVITIES", tag)
		if u == nil || u.Status != CheckAgree || len(u.Methods) != 4 || u.Methods[0].Value != "32" {
			t.Fatalf("empty cavities of %s: %+v", tag, u)
		}
	}
	if n := checkByID(cs, "CONNECTOR_COUNT", ""); n == nil || n.Status != CheckAgree || len(n.Methods) != 3 {
		t.Fatalf("connector count: %+v", n)
	}
}

func TestCaptureChecksFlagADisagreementAndNeverGuess(t *testing.T) {
	a, co, ls := inventedPairedAnalysis()
	co[0].Quantity = "18" // the callout says one cable fewer than the wiring and the parts list
	cc := checkByID(captureChecks(&a, co, ls), "CONDUCTOR_COUNT", "")
	if cc.Status != CheckDisagree {
		t.Fatalf("a callout one cable short must disagree: %+v", cc)
	}
	// a wire row lost: the wiring and the other readings now disagree
	a, co, ls = inventedPairedAnalysis()
	a.Wires = a.Wires[:95]
	if cc := checkByID(captureChecks(&a, co, ls), "CONDUCTOR_COUNT", ""); cc.Status != CheckDisagree {
		t.Fatalf("a missing wire row must disagree: %+v", cc)
	}
	// no cable length on the drawing: the length-based reading is simply absent, not a disagreement
	a, co, _ = inventedPairedAnalysis()
	cc = checkByID(captureChecks(&a, co, nil), "CONDUCTOR_COUNT", "")
	if cc.Status != CheckAgree || len(cc.Methods) != 2 {
		t.Fatalf("without a length two readings still agree: %+v", cc)
	}
	// a length that does not divide the cable quantity evenly gives no reading
	a, co, _ = inventedPairedAnalysis()
	cc = checkByID(captureChecks(&a, co, []HarnessLength{{Label: "DIMENSION", Text: "100"}}), "CONDUCTOR_COUNT", "")
	if len(cc.Methods) != 2 {
		t.Fatalf("an uneven split must not be used: %+v", cc.Methods)
	}
	// plugs that do not split evenly between the connectors are not a reading; the other three still agree
	a, co, ls = inventedPairedAnalysis()
	a.BOM[4].Qty = "63"
	if u := checkByID(captureChecks(&a, co, ls), "UNUSED_CAVITIES", "TEST-J1"); len(u.Methods) != 3 || u.Status != CheckAgree {
		t.Fatalf("63 plugs for 2 connectors: %+v", u)
	}
	// a wrong plug count disagrees with the wiring
	a, co, ls = inventedPairedAnalysis()
	a.BOM[4].Qty = "70"
	if u := checkByID(captureChecks(&a, co, ls), "UNUSED_CAVITIES", "TEST-J1"); u.Status != CheckDisagree {
		t.Fatalf("70 plugs against 32 empty each: %+v", u)
	}
}

func TestAsymmetricDimensionToleranceIsACandidate(t *testing.T) {
	p := Page{Number: 2, Width: 800, Height: 600, PageAccurate: true}
	p.Words = []Word{{Text: "216", X: 300, Y: 500, Size: 8}, {Text: "-0.0/+1.0", X: 322, Y: 500, Size: 8}}
	got := harnessDimensions([]Page{p})
	if len(got) != 1 || got[0].Text != "216" || got[0].Tolerance != "-0.0/+1.0" || got[0].Label != "DIMENSION" {
		t.Fatalf("dimension candidates: %+v", got)
	}
}

// End to end on an invented one-sheet drawing: wiring pairs, a parts list, a cable callout, a plug line and a
// dimension are read separately, and the checks line them up.
func inventedOneSheetCheckPDF(cables int, plugs int) []byte {
	text := func(x int, y float64, s string) string {
		return fmt.Sprintf("BT /F1 8 Tf 1 0 0 1 %d %.1f Tm (%s) Tj ET\n", x, y, s)
	}
	content := text(55, 770, "TEST-J1") + text(790, 770, "BENCH-P2")
	pin := 0
	for i := 0; i < cables; i++ {
		for _, col := range []string{"WHT", "BLU"} {
			pin++
			y := 740 - float64(pin)*20
			content += text(55, y, fmt.Sprint(pin)) + text(72, y, col) + text(420, y, "24") + text(440, y, "AWG") + text(765, y, col) + text(790, y, fmt.Sprint(pin)) + text(820, y, "SIG")
		}
	}
	// the cable callout: a circled item number and a quantity beside it
	content += "q 0.5 w 309 420 m 309 424.97 304.97 429 300 429 c 295.03 429 291 424.97 291 420 c 291 415.03 295.03 411 300 411 c 304.97 411 309 415.03 309 420 c h S Q\n"
	content += text(297, 417, "4") + text(318, 417, fmt.Sprintf("%dx", cables))
	content += text(300, 380, "216") + text(330, 380, "-0.0/+1.0")
	content += text(70, 330, "ITEM") + text(150, 330, "PART NUMBER") + text(350, 330, "DESCRIPTION") + text(560, 330, "REF DES") + text(650, 330, "QTY")
	rows := [][5]string{
		{"1", "D38999/26FJ35PA", "CONNECTOR PLUG MALE", "TEST-J1", "1"},
		{"2", "D38999/26FJ35SA", "CONNECTOR PLUG FEMALE", "BENCH-P2", "1"},
		{"4", "M27500-24RC2S06", "CABLE, 24 AWG, 2 COND, SHIELDED", "", fmt.Sprint(cables * 18)},
		{"12", "MS27488-22-2", "22AWG SEALING PLUG", "", fmt.Sprint(plugs)},
	}
	for i, r := range rows {
		y := 310 - float64(i)*20
		content += text(70, y, r[0]) + text(150, y, r[1]) + text(350, y, r[2]) + text(560, y, r[3]) + text(650, y, r[4])
	}
	return onePagePDF(content, "[0 0 1000 800]")
}

func TestCaptureChecksEndToEndOnAnInventedSheet(t *testing.T) {
	// 7 cables = 14 wires on two 128-position connectors, so 114 empty cavities each, and 228 plugs
	v, err := viewAnalysis(inventedOneSheetCheckPDF(7, 228))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range v.CaptureChecks {
		t.Logf("%s %s [%s] %s %+v", c.ID, c.Connector, c.Status, c.Message, c.Methods)
	}
	cc := checkByID(v.CaptureChecks, "CONDUCTOR_COUNT", "")
	if cc == nil || cc.Status != CheckAgree || len(cc.Methods) != 3 {
		t.Fatalf("conductor count: %+v", cc)
	}
	for _, tag := range []string{"TEST-J1", "BENCH-P2"} {
		if u := checkByID(v.CaptureChecks, "UNUSED_CAVITIES", tag); u == nil || u.Status != CheckAgree {
			t.Fatalf("empty cavities %s: %+v", tag, u)
		}
	}
	// a plug line that is wrong is reported, not absorbed
	v, err = viewAnalysis(inventedOneSheetCheckPDF(7, 200))
	if err != nil {
		t.Fatal(err)
	}
	if u := checkByID(v.CaptureChecks, "UNUSED_CAVITIES", "TEST-J1"); u == nil || u.Status != CheckDisagree {
		t.Fatalf("200 plugs must disagree: %+v", u)
	}
	found := false
	for _, i := range v.Issues {
		if i.Code == "CAPTURE_READINGS_DISAGREE" {
			found = true
		}
	}
	if !found {
		t.Fatal("a disagreement must raise a drawing alert")
	}
}

func TestCaptureChecksDoNotCountPinsOffTheInsertOrCableEnds(t *testing.T) {
	a, co, ls := inventedPairedAnalysis()
	// two wires land on pins that the insert does not have, and a purchased cable's own plug appears as a far end
	a.Wires = append(a.Wires, WireRow{Page: 3, Connector: "TEST-J1", Cavity: "200", End: "ETH1", EndCavity: "1"}, WireRow{Page: 3, Connector: "TEST-J1", Cavity: "201", End: "USB_DN1", EndCavity: "2"})
	cs := captureChecks(&a, co, ls)
	u := checkByID(cs, "UNUSED_CAVITIES", "TEST-J1")
	if u.Methods[0].Value != "32" {
		t.Fatalf("pins beyond the insert were subtracted: %+v", u.Methods[0])
	}
	if n := checkByID(cs, "CONNECTOR_COUNT", ""); n.Status != CheckAgree {
		t.Fatalf("a cable's own plug counted as a harness connector: %+v", n)
	}
}

func TestTwistedPairMathAddsReadingsAndChecksPairing(t *testing.T) {
	a, co, ls := inventedPairedAnalysis()
	cs := captureChecks(&a, co, ls)
	u := checkByID(cs, "UNUSED_CAVITIES", "TEST-J1")
	if u == nil || u.Status != CheckAgree || len(u.Methods) != 4 {
		t.Fatalf("empty cavities should have four readings (wiring, callouts, cable quantity, plugs): %+v", u)
	}
	for _, m := range u.Methods {
		if m.Value != "32" {
			t.Fatalf("reading %q = %s, want 32", m.Name, m.Value)
		}
	}
	if p := checkByID(cs, "PAIR_STRUCTURE", ""); p == nil || p.Status != CheckAgree {
		t.Fatalf("pairs: %+v", p)
	}
	// a lost wire row leaves one sheet with an odd count
	a, co, ls = inventedPairedAnalysis()
	a.Wires = a.Wires[:95]
	if p := checkByID(captureChecks(&a, co, ls), "PAIR_STRUCTURE", ""); p == nil || p.Status != CheckDisagree {
		t.Fatalf("an odd sheet must be reported: %+v", p)
	}
	// three connectors: the conductor total no longer says how many pins one of them uses
	a, co, ls = inventedPairedAnalysis()
	a.Connectors = append(a.Connectors, Connector{Tag: "TEST-J9", CavityCount: 128})
	for _, m := range checkByID(captureChecks(&a, co, ls), "UNUSED_CAVITIES", "TEST-J1").Methods {
		if m.Name == "Cavities in the insert − conductors from the cable callouts" {
			t.Fatal("pair math used on a harness that is not point to point")
		}
	}
}

func TestLengthIsConfirmedOnlyWhenBOMQuantityAndCalloutsAgree(t *testing.T) {
	a, co, ls := inventedPairedAnalysis()
	got := corroborateLengths(&a, co, ls)
	if len(got) != 1 || !got[0].Corroborated || got[0].Text != "216 IN" || got[0].Basis == "" {
		t.Fatalf("216 in should be confirmed by 666/18=37 and 198/18=11: %+v", got)
	}
	// 100 does not divide the cable quantity into the called-out number of cables
	if g := corroborateLengths(&a, co, []HarnessLength{{Label: "DIMENSION", Text: "100"}}); g[0].Corroborated {
		t.Fatalf("an uneven split was confirmed: %+v", g)
	}
	// 108 divides evenly (74 cables) but the callouts say 37, so it is not confirmed
	if g := corroborateLengths(&a, co, []HarnessLength{{Label: "DIMENSION", Text: "108"}}); g[0].Corroborated {
		t.Fatalf("a length that disagrees with the callouts was confirmed: %+v", g)
	}
	// no callouts, nothing to confirm against
	if g := corroborateLengths(&a, nil, ls); g[0].Corroborated {
		t.Fatalf("confirmed with nothing to compare: %+v", g)
	}
	// a labelled length is never rewritten
	if g := corroborateLengths(&a, co, []HarnessLength{{Label: "CUT", Text: "5 FT"}}); g[0].Corroborated || g[0].Text != "5 FT" {
		t.Fatalf("labelled length changed: %+v", g)
	}
}

func TestUpperLengthIsNominalPlusThePlusTolerance(t *testing.T) {
	// a tolerance beside a labelled length in feet has no known unit, so nothing is added
	if u := withUpperLengths([]HarnessLength{{Label: "CUT", Text: "5.0 FT", Tolerance: "±2"}})[0].Upper; u != "" {
		t.Fatalf("labelled CUT length got %q", u)
	}
	for _, c := range []struct{ text, tol, want string }{
		{"216", "-0.0/+1.0", "217 IN"},
		{"216 IN", "-0.0/+1.0", "217 IN"},
		{"216", "+1.0/-0.0", "217 IN"},
		{"216", "-0.5/+2.5", "218.5 IN"},
		{"48", "±0.5", "48.5 IN"},
		{"48", "+/-2", "50 IN"},
		{"5.0 FT", "±2", "7 FT"}, // a printed dimension: its tolerance is in its own unit
		{"216", "", ""},
		{"216", "garbled", ""},
	} {
		got := withUpperLengths([]HarnessLength{{Label: "DIMENSION", Text: c.text, Tolerance: c.tol}})[0].Upper
		if got != c.want {
			t.Fatalf("%q %q: upper %q, want %q", c.text, c.tol, got, c.want)
		}
	}
}
