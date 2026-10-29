package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Source-linked decoding of military/aerospace wire and cable part numbers.
// Every code meaning comes from assets/WIRE_SPEC_REFERENCE.csv, where each row
// carries its source URLs and a verification status. A code that is not in the
// table is reported as unresolved; it is never guessed.

type wireSpecRow struct {
	Kind, Code, Meaning, Spec, Status, Note string
	Sources                                 []string
	re                                      *regexp.Regexp
}

type WireField struct {
	Label   string   `json:"label"`
	Value   string   `json:"value"`
	Code    string   `json:"code,omitempty"`
	Spec    string   `json:"spec,omitempty"`
	Sources []string `json:"sources,omitempty"`
	Status  string   `json:"status"`
}

type WireDecode struct {
	Part          string      `json:"part"`
	Recognized    bool        `json:"recognized"`
	Decoded       bool        `json:"decoded"`
	Family        string      `json:"family,omitempty"`
	FamilySources []string    `json:"familySources,omitempty"`
	FamilyStatus  string      `json:"familyStatus,omitempty"`
	Gauge         string      `json:"gauge,omitempty"`
	Conductors    int         `json:"conductors,omitempty"`
	Fields        []WireField `json:"fields"`
	Unresolved    []string    `json:"unresolved"`
	Notes         []string    `json:"notes"`
}

var (
	wireSpecOnce sync.Once
	wireSpecRows []wireSpecRow
	wireSpecErr  error
)

func wireSpecTable() ([]wireSpecRow, error) {
	wireSpecOnce.Do(func() {
		rows, err := libraryCSV("WIRE_SPEC_REFERENCE.csv")
		if err != nil {
			wireSpecErr = err
			return
		}
		for _, r := range rows[1:] {
			if len(r) < 7 {
				wireSpecErr = fmt.Errorf("WIRE_SPEC_REFERENCE.csv: short row %v", r)
				return
			}
			row := wireSpecRow{Kind: r[0], Code: r[1], Meaning: r[2], Spec: r[3], Status: r[5], Note: r[6]}
			for _, s := range strings.Split(r[4], ";") {
				if s = strings.TrimSpace(s); s != "" {
					row.Sources = append(row.Sources, s)
				}
			}
			if row.Kind == "family" || row.Kind == "slash" {
				re, err := regexp.Compile(row.Code)
				if err != nil {
					wireSpecErr = fmt.Errorf("WIRE_SPEC_REFERENCE.csv: bad pattern %q: %w", row.Code, err)
					return
				}
				row.re = re
			}
			wireSpecRows = append(wireSpecRows, row)
		}
	})
	return wireSpecRows, wireSpecErr
}

var (
	reM27500    = regexp.MustCompile(`^(?:M|NEMA-?WC-?)?27500-?(\d{2})([A-Z]{2})(\d{1,2})([A-Z])(\d{2})$`)
	reAS22759   = regexp.MustCompile(`^(?:M|AS|SAE-AS)?22759/(\d{1,2})-(\d{1,2})(?:-(\d))?$`)
	reWireSpace = regexp.MustCompile(`\s+`)
)

func lookupWireCode(rows []wireSpecRow, kind, code string) *wireSpecRow {
	for i := range rows {
		if rows[i].Kind == kind && rows[i].Code == code {
			return &rows[i]
		}
	}
	return nil
}

// DecodeWirePart identifies the governing specification family and, where the
// part-number grammar is known, decodes its fields.
func DecodeWirePart(part string) (WireDecode, error) {
	rows, err := wireSpecTable()
	out := WireDecode{Fields: []WireField{}, Unresolved: []string{}, Notes: []string{}}
	if err != nil {
		return out, err
	}
	pn := strings.ToUpper(reWireSpace.ReplaceAllString(strings.TrimSpace(part), ""))
	out.Part = pn
	if pn == "" {
		out.Notes = append(out.Notes, "No part number supplied.")
		return out, nil
	}
	for _, r := range rows {
		if r.Kind == "family" && r.re.MatchString(pn) {
			out.Recognized, out.Family, out.FamilySources, out.FamilyStatus = true, r.Spec, r.Sources, r.Status
			break
		}
	}
	for _, r := range rows {
		if r.Kind == "slash" && strings.Contains(pn, r.Code) {
			out.Fields = append(out.Fields, WireField{Label: "Slash sheet", Value: r.Meaning, Spec: r.Spec, Sources: r.Sources, Status: r.Status})
		}
	}
	if !out.Recognized {
		out.Notes = append(out.Notes, "Not in the wire/cable reference library. Nothing was assumed about it.")
		return out, nil
	}
	switch {
	case reM27500.MatchString(pn):
		decodeM27500(&out, rows, reM27500.FindStringSubmatch(pn))
	case reAS22759.MatchString(pn):
		decodeAS22759(&out, rows, reAS22759.FindStringSubmatch(pn))
	default:
		out.Notes = append(out.Notes, "Specification family recognized, but this part number does not match a supported decoding format, so no fields were decoded.")
	}
	return out, nil
}

func decodeM27500(out *WireDecode, rows []wireSpecRow, m []string) {
	out.Decoded = true
	out.Gauge = m[1]
	n, _ := strconv.Atoi(m[3])
	out.Conductors = n
	out.Fields = append(out.Fields,
		WireField{Label: "Wire gauge", Value: m[1] + " AWG", Code: m[1], Status: "DECODED FROM PART NUMBER"},
		WireField{Label: "Insulated conductors", Value: strconv.Itoa(n), Code: m[3], Sources: out.FamilySources, Status: "DECODED FROM PART NUMBER — " + out.FamilyStatus},
	)
	add := func(kind, label, code, unresolved string) {
		if r := lookupWireCode(rows, kind, code); r != nil {
			out.Fields = append(out.Fields, WireField{Label: label, Value: r.Meaning, Code: code, Spec: r.Spec, Sources: r.Sources, Status: r.Status})
			return
		}
		out.Unresolved = append(out.Unresolved, unresolved)
		out.Fields = append(out.Fields, WireField{Label: label, Value: "UNRESOLVED", Code: code, Status: "CODE NOT IN REFERENCE LIBRARY"})
	}
	add("component", "Component wire", m[2], "Component-wire code "+m[2]+" is not in the reference library.")
	add("shield", "Shield", m[4], "Shield code "+m[4]+" is not in the reference library.")
	add("jacket", "Outer jacket", m[5], "Jacket code "+m[5]+" is not in the reference library.")
	out.Notes = append(out.Notes,
		"Twist is not stated by the part number. Do not assume the conductors are a twisted pair.",
		"The part-number shield letter does not say whether pairs are individually shielded. Take shield scope and termination from the drawing or specification.",
		"Conductor plating, insulation thickness, strands, temperature and voltage rating must be confirmed on the slash sheet.")
}

func decodeAS22759(out *WireDecode, rows []wireSpecRow, m []string) {
	out.Decoded = true
	out.Gauge = m[2]
	spec := "AS22759/" + m[1]
	var found *wireSpecRow
	for i := range rows {
		if rows[i].Kind == "component" && rows[i].Spec == spec {
			found = &rows[i]
			break
		}
	}
	out.Fields = append(out.Fields, WireField{Label: "Wire gauge", Value: m[2] + " AWG", Code: m[2], Status: "DECODED FROM PART NUMBER"})
	if found != nil {
		out.Fields = append(out.Fields, WireField{Label: "Slash sheet", Value: found.Meaning, Code: found.Code, Spec: spec, Sources: found.Sources, Status: found.Status})
	} else {
		out.Unresolved = append(out.Unresolved, "Slash sheet "+spec+" is not in the reference library.")
		out.Fields = append(out.Fields, WireField{Label: "Slash sheet", Value: "UNRESOLVED", Code: m[1], Spec: spec, Status: "SLASH SHEET NOT IN REFERENCE LIBRARY"})
	}
	if m[3] != "" {
		out.Unresolved = append(out.Unresolved, "Color code "+m[3]+" is not decoded: no color table has been supplied.")
	}
	out.Notes = append(out.Notes, "Confirm plating, insulation thickness, strands, temperature and voltage rating on the slash sheet.")
}
