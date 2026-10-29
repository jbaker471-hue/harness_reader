package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
)

// Connector documentation library and insert-geometry store.
//
// CONNECTOR_SPEC_REFERENCE.csv records which specification, layout standard,
// contact, tooling and manufacturer documents apply to a connector family, with
// the source URLs that were supplied. Nothing in it is verified.
//
// CONNECTOR_INSERT_GEOMETRY.csv holds physical insert arrangements (cavity X/Y
// per arrangement), stored apart from connector part numbers so that several
// part numbers can share one layout. It is empty until rows are entered from a
// source document, and a part's cavities are drawn only from rows that pass the
// checks in buildInsertGeometry.

type ConnectorDocRef struct {
	Kind     string   `json:"kind"`
	Code     string   `json:"code"`
	Title    string   `json:"title"`
	Covers   string   `json:"covers"`
	Sources  []string `json:"sources"`
	Status   string   `json:"status"`
	Priority string   `json:"priority"`
	Layout   []string `json:"layoutStandards,omitempty"`
	Note     string   `json:"note,omitempty"`
}

var (
	connDocOnce sync.Once
	connDocs    []ConnectorDocRef
	connDocErr  error
)

func connectorDocTable() ([]ConnectorDocRef, error) {
	connDocOnce.Do(func() {
		rows, err := readLibrary("CONNECTOR_SPEC_REFERENCE.csv")
		if err != nil {
			connDocErr = err
			return
		}
		split := func(s string) []string {
			out := []string{}
			for _, p := range strings.Split(s, ";") {
				if p = strings.TrimSpace(p); p != "" {
					out = append(out, p)
				}
			}
			return out
		}
		for _, r := range rows[1:] {
			connDocs = append(connDocs, ConnectorDocRef{Kind: r[0], Code: r[1], Title: r[2], Covers: r[3], Sources: split(r[4]), Status: r[5], Priority: r[6], Layout: split(r[7]), Note: r[8]})
		}
	})
	return connDocs, connDocErr
}

func lookupConnectorDoc(kind, code string) *ConnectorDocRef {
	rows, err := connectorDocTable()
	if err != nil {
		return nil
	}
	for i := range rows {
		if rows[i].Kind == kind && rows[i].Code == code {
			return &rows[i]
		}
	}
	return nil
}

// geometry views, as seen from the named side of the named contact type
var insertViews = map[string]bool{"pin-front": true, "socket-front": true, "pin-rear": true, "socket-rear": true}

// buildInsertGeometry assembles one arrangement from CONNECTOR_INSERT_GEOMETRY.csv rows
// (header already removed). It returns nil with no problems when the arrangement has no
// rows, and nil with problems when the rows are unusable: a bad row never yields a partial layout.
func buildInsertGeometry(rows [][]string, standard, arrangement string) (*FaceGeometry, []string) {
	var g *FaceGeometry
	seen := map[string]bool{}
	var problems []string
	view, keying, status, src := "", "", "", ""
	for _, r := range rows {
		if len(r) < 13 || r[0] != arrangement || r[1] != standard {
			continue
		}
		bad := func(msg string) { problems = append(problems, fmt.Sprintf("%s cavity %q: %s", arrangement, r[3], msg)) }
		x, ex := strconv.ParseFloat(strings.TrimSpace(r[4]), 64)
		y, ey := strconv.ParseFloat(strings.TrimSpace(r[5]), 64)
		switch {
		case strings.TrimSpace(r[3]) == "":
			bad("blank cavity label")
		case seen[r[3]]:
			bad("repeated cavity label")
		case ex != nil || ey != nil || math.IsNaN(x+y) || math.IsInf(x+y, 0):
			bad("coordinates are not numbers")
		case !insertViews[r[7]]:
			bad("view must be pin-front, socket-front, pin-rear or socket-rear")
		case strings.TrimSpace(r[9]) == "" || strings.TrimSpace(r[10]) == "":
			bad("source and source page are required")
		case view != "" && view != r[7]:
			bad("rows give different views")
		case strings.TrimSpace(r[12]) == "":
			bad("status is required")
		default:
			if g == nil {
				g = &FaceGeometry{Shape: "circular", Positions: []GeoPos{}}
				view, keying, status, src = r[7], r[8], r[12], fmt.Sprintf("%s, page %s, %s", r[9], r[10], r[11])
			}
			seen[r[3]] = true
			g.Positions = append(g.Positions, GeoPos{Label: r[3], X: x, Y: y, R: 0})
		}
	}
	if len(problems) > 0 {
		return nil, problems
	}
	if g == nil {
		return nil, nil
	}
	g.View, g.Face, g.Status = view, view+" as given by the source", status
	g.Basis = "Insert coordinates read from " + src + "; keying: " + firstNonEmpty(keying, "not stated") + "."
	return g, nil
}

func insertGeometryFor(standard, arrangement string) (*FaceGeometry, []string) {
	rows, err := readLibrary("CONNECTOR_INSERT_GEOMETRY.csv")
	if err != nil {
		return nil, []string{"Insert geometry library unavailable: " + err.Error()}
	}
	return buildInsertGeometry(rows[1:], standard, arrangement)
}

// InsertLayoutInfo describes one insert arrangement the application knows about, whether or not its
// coordinates are stored. It is independent of any connector part number.
type InsertLayoutInfo struct {
	ID          string `json:"id"`
	Standard    string `json:"standard"`
	ShellSize   string `json:"shellSize,omitempty"`
	Contacts    int    `json:"contacts"`
	ContactSize string `json:"contactSize,omitempty"`
	View        string `json:"view,omitempty"`
	Source      string `json:"source"`
	SourcePage  string `json:"sourcePage,omitempty"`
	Status      string `json:"status"`
	Loaded      bool   `json:"loaded"`
	Reason      string `json:"reason,omitempty"`
}

// listInsertLayouts merges the stored geometry with the coverage list written by the extractor, so an
// arrangement that was seen in a source but could not be loaded is listed with the reason.
func listInsertLayouts() ([]InsertLayoutInfo, error) {
	geo, err := readLibrary("CONNECTOR_INSERT_GEOMETRY.csv")
	if err != nil {
		return nil, err
	}
	cov, err := readLibrary("CONNECTOR_INSERT_COVERAGE.csv")
	if err != nil {
		return nil, err
	}
	out := []InsertLayoutInfo{}
	idx := map[string]int{}
	for _, r := range geo[1:] {
		k := r[1] + "|" + r[0]
		i, ok := idx[k]
		if !ok {
			out = append(out, InsertLayoutInfo{ID: r[0], Standard: r[1], ShellSize: r[2], ContactSize: r[6], View: r[7], Source: r[9], SourcePage: r[10], Status: r[12], Loaded: true})
			i = len(out) - 1
			idx[k] = i
		}
		out[i].Contacts++
	}
	for _, r := range cov[1:] {
		k := r[1] + "|" + r[0]
		if _, ok := idx[k]; ok {
			continue
		}
		n, _ := strconv.Atoi(r[4])
		out = append(out, InsertLayoutInfo{ID: r[0], Standard: r[1], Contacts: n, ContactSize: r[5], Source: r[2], SourcePage: r[3], Status: "NOT LOADED", Reason: r[7]})
		idx[k] = len(out) - 1
	}
	return out, nil
}

// FamilyCoverage is the arrangement-level audit of one connector family. A family is never reported
// complete: the status is one of the fixed audit states in the file, and counts that are not known say so.
type FamilyCoverage struct {
	Family             string `json:"family"`
	Standard           string `json:"standard"`
	Retrieved          string `json:"documentsRetrieved"`
	ManualRequired     string `json:"documentsManualRequired"`
	ArrangementsKnown  string `json:"arrangementsKnown"`
	ArrangementsStored string `json:"arrangementsStored"`
	AuditStatus        string `json:"auditStatus"`
	Note               string `json:"note"`
}

func familyCoverage() ([]FamilyCoverage, error) {
	rows, err := readLibrary("CONNECTOR_FAMILY_COVERAGE.csv")
	if err != nil {
		return nil, err
	}
	out := []FamilyCoverage{}
	for _, r := range rows[1:] {
		out = append(out, FamilyCoverage{Family: r[0], Standard: r[1], Retrieved: r[2], ManualRequired: r[3], ArrangementsKnown: r[4], ArrangementsStored: r[5], AuditStatus: r[6], Note: r[7]})
	}
	return out, nil
}
