package main

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

// GraphicCallout is source-grounded evidence, never a manufacturing instruction.
// Shapes establish a reference candidate, not its applicability or quantity.
type GraphicCallout struct {
	LeaderTargets  [][2]float64 `json:"leaderTargets"`
	LeaderStatus   string       `json:"leaderStatus"`
	Page           int          `json:"page"`
	Box            Box          `json:"box"`
	Shape          string       `json:"shape"`
	Reference      string       `json:"reference"`
	Kind           string       `json:"kind"`
	Status         string       `json:"status"`
	Matches        []string     `json:"matches"`
	DefinitionPage int          `json:"definitionPage,omitempty"`
	DefinitionBox  *Box         `json:"definitionBox,omitempty"`
	Quantity       string       `json:"quantity,omitempty"`
}

var calloutID = regexp.MustCompile(`^[0-9]{1,4}[A-Z]?$`)
var calloutQuantity = regexp.MustCompile(`(?i)^([1-9][0-9]{0,3})\s*[x×]$`)

func graphicalCallouts(x *DocExtraction, bom []BOMRow) []GraphicCallout {
	out := []GraphicCallout{}
	if x.Legacy {
		return out
	}
	for _, p := range x.Layouts {
		seen := map[string]bool{}
		add := func(b Box, shape string) {
			if b.W() < 3 || b.H() < 3 || b.W() > 60 || b.H() > 60 {
				return
			}
			ws := []LayoutWord{}
			for _, w := range p.Words {
				if b.Contains(w.Box.CX(), w.Box.CY()) {
					ws = append(ws, w)
				}
			}
			if len(ws) != 1 {
				return
			}
			w := ws[0]
			id := strings.TrimSpace(w.Text)
			if !calloutID.MatchString(id) || w.Box.W() > b.W()*.9 || w.Box.H() > b.H()*.9 {
				return
			}
			key := fmt.Sprintf("%s:%.1f:%.1f", shape, b.CX(), b.CY())
			if seen[key] {
				return
			}
			seen[key] = true
			c := GraphicCallout{Page: p.Page, Box: b, Shape: shape, Reference: canonicalFind(id), Kind: "unresolved", Status: "REVIEW — shape convention and applicability unverified", Matches: []string{}}
			for _, q := range p.Words {
				if q.Box.X0 >= b.X1 && q.Box.X0-b.X1 < 3*b.H() && math.Abs(q.Box.CY()-b.CY()) < b.H()*.8 {
					if m := calloutQuantity.FindStringSubmatch(strings.TrimSpace(q.Text)); m != nil {
						if c.Quantity != "" && c.Quantity != m[1] {
							c.Quantity = "AMBIGUOUS"
							break
						}
						c.Quantity = m[1]
					}
				}
			}
			out = append(out, c)
		}
		for _, c := range p.Circles {
			if c.Mark != "solid" {
				add(Box{c.CX - c.R, c.CY - c.R, c.CX + c.R, c.CY + c.R}, "circle")
			}
		}
		for _, g := range append(p.Polygons, fragmentedFlags(p)...) {
			if convexCallout(g.Points) {
				shape := "triangle"
				if len(g.Points) == 6 {
					shape = "hexagon"
				}
				add(g.Box, shape)
			}
		}
	}
	// Definition candidates require several neighboring text words on the same
	// baseline. Only matching flag shapes are reconciled; a circle is not a note.
	type definition struct {
		text string
		page int
		box  Box
	}
	defs := map[string][]definition{}
	for i := range out {
		c := &out[i]
		if c.Shape == "circle" {
			continue
		}
		p := x.Layouts[c.Page-1]
		ws := []LayoutWord{}
		for _, w := range p.Words {
			if w.Box.X0 > c.Box.X1 && w.Box.X0 < c.Box.X1+350 && math.Abs(w.Box.CY()-c.Box.CY()) < c.Box.H()*.35 {
				ws = append(ws, w)
			}
		}
		sort.Slice(ws, func(i, j int) bool { return ws[i].Box.X0 < ws[j].Box.X0 })
		if len(ws) == 0 || calloutID.MatchString(strings.TrimSpace(ws[0].Text)) || ws[0].Box.X0-c.Box.X1 > c.Box.H()*2 {
			continue
		}
		parts := []string{}
		box := c.Box
		for _, w := range ws {
			parts = append(parts, w.Text)
			box = box.Union(w.Box)
		}
		// Continue within the same note block, stopping before the next
		// numbered marker in its left column. Keep the source region visible.
		stop := c.Box.Y1 + c.Box.H()*2
		for _, w := range p.Words {
			if calloutID.MatchString(strings.TrimSpace(w.Text)) && math.Abs(w.Box.CX()-c.Box.CX()) < c.Box.W()*.5 && w.Box.CY() > c.Box.Y1 && w.Box.Y0 < stop {
				stop = w.Box.Y0
			}
		}
		continuation := []LayoutWord{}
		for _, w := range p.Words {
			if w.Box.CY() > c.Box.Y1 && w.Box.Y1 < stop && w.Box.X0 >= ws[0].Box.X0 && w.Box.X1 <= c.Box.X1+350 {
				continuation = append(continuation, w)
			}
		}
		sort.Slice(continuation, func(i, j int) bool {
			if math.Abs(continuation[i].Box.CY()-continuation[j].Box.CY()) > 3 {
				return continuation[i].Box.CY() < continuation[j].Box.CY()
			}
			return continuation[i].Box.X0 < continuation[j].Box.X0
		})
		for _, w := range continuation {
			parts = append(parts, w.Text)
			box = box.Union(w.Box)
		}
		text := strings.Join(parts, " ")
		if len(text) < 20 || len(strings.Fields(text)) < 3 {
			continue
		}
		c.Kind = "note definition candidate"
		c.Matches = []string{text}
		k := c.Shape + ":" + c.Reference
		defs[k] = append(defs[k], definition{text, c.Page, box})
	}
	for i := range out {
		c := &out[i]
		if c.Kind == "note definition candidate" {
			continue
		}
		if c.Shape == "circle" {
			c.Kind = "BOM reference candidate"
			seen := map[string]bool{}
			for _, b := range bom {
				if canonicalFind(b.Item) == c.Reference && b.Item != "" {
					text := strings.TrimSpace(b.Part) + " | " + b.Description
					if !seen[text] {
						c.Matches = append(c.Matches, text)
						seen[text] = true
					}
				}
			}
			if len(c.Matches) == 0 {
				c.Status = "UNRESOLVED — no recognized BOM item"
			} else if len(c.Matches) > 1 {
				c.Status = "AMBIGUOUS — conflicting BOM rows"
			}
		} else {
			c.Kind = "note reference candidate"
			ds := defs[c.Shape+":"+c.Reference]
			for _, d := range ds {
				c.Matches = append(c.Matches, d.text)
			}
			if len(ds) == 1 {
				c.DefinitionPage = ds[0].page
				b := ds[0].box
				c.DefinitionBox = &b
			} else if len(ds) == 0 {
				c.Status = "UNRESOLVED — no matching note definition"
			} else {
				c.Status = "AMBIGUOUS — multiple note definitions"
			}
		}
	}
	for i := range out {
		page := x.Layouts[out[i].Page-1]
		segments := []Segment{}
		for _, seg := range page.LeaderSegments {
			outline := false
			for _, c := range out {
				if c.Page == page.Page && c.Box.Contains(seg[0], seg[1]) && c.Box.Contains(seg[2], seg[3]) {
					outline = true
					break
				}
			}
			if !outline {
				segments = append(segments, seg)
			}
		}
		page.LeaderSegments = segments
		out[i].LeaderTargets, out[i].LeaderStatus = calloutLeaders(page, out[i].Box, out[i].Shape)
	}
	// Adjacent symbols can share a single leader. Preserve this as an explicit
	// group candidate; never multiply BOM quantities by the number of symbols.
	for i := range out {
		if out[i].Kind == "note definition candidate" {
			continue
		}
		group := map[int]bool{i: true}
		for changed := true; changed; {
			changed = false
			for j, c := range out {
				if group[j] || c.Page != out[i].Page || c.Kind == "note definition candidate" {
					continue
				}
				for k := range group {
					b := out[k].Box
					gap := math.Max(c.Box.X0-b.X1, b.X0-c.Box.X1)
					if gap >= -.5 && gap <= 1 && math.Abs(c.Box.CY()-b.CY()) < math.Min(c.Box.H(), b.H())*.25 {
						group[j] = true
						changed = true
						break
					}
				}
			}
		}
		targets := [][2]float64{}
		for j := range group {
			for _, t := range out[j].LeaderTargets {
				duplicate := false
				for _, v := range targets {
					if math.Hypot(t[0]-v[0], t[1]-v[1]) < .5 {
						duplicate = true
					}
				}
				if !duplicate {
					targets = append(targets, t)
				}
			}
		}
		if len(group) > 1 && len(targets) > 0 && len(out[i].LeaderTargets) == 0 {
			out[i].LeaderTargets = targets
			out[i].LeaderStatus = "Shared-symbol leader candidate; verify group scope"
		}
	}
	return out
}
func convexCallout(p [][2]float64) bool {
	if len(p) != 3 && len(p) != 6 {
		return false
	}
	sign := 0.0
	for i := range p {
		a, b, c := p[i], p[(i+1)%len(p)], p[(i+2)%len(p)]
		cross := (b[0]-a[0])*(c[1]-b[1]) - (b[1]-a[1])*(c[0]-b[0])
		if math.Abs(cross) < .01 {
			return false
		}
		if sign*cross < 0 {
			return false
		}
		sign = cross
	}
	return true
}

// Some exporters draw each side as a separate object. Reassemble small convex
// cycles around a numeric label; never close a gap or join a line crossing.
func fragmentedFlags(p PageLayout) []CalloutPolygon {
	out := []CalloutPolygon{}
	for _, w := range p.Words {
		if !calloutID.MatchString(strings.TrimSpace(w.Text)) {
			continue
		}
		radius := math.Max(w.Box.H()*3.5, 10)
		nodes := [][2]float64{}
		edges := map[int]map[int]bool{}
		node := func(q [2]float64) int {
			for i, n := range nodes {
				if math.Hypot(n[0]-q[0], n[1]-q[1]) < .15 {
					return i
				}
			}
			nodes = append(nodes, q)
			return len(nodes) - 1
		}
		for _, s := range p.Segments {
			a, b := [2]float64{s[0], s[1]}, [2]float64{s[2], s[3]}
			if math.Hypot(a[0]-w.Box.CX(), a[1]-w.Box.CY()) > radius || math.Hypot(b[0]-w.Box.CX(), b[1]-w.Box.CY()) > radius {
				continue
			}
			i, j := node(a), node(b)
			if i == j {
				continue
			}
			if edges[i] == nil {
				edges[i] = map[int]bool{}
			}
			if edges[j] == nil {
				edges[j] = map[int]bool{}
			}
			edges[i][j] = true
			edges[j][i] = true
		}
		if len(nodes) > 50 {
			continue
		}
		seen := map[string]bool{}
		budget := 2000
		var walk func([]int)
		walk = func(path []int) {
			budget--
			if budget < 0 {
				return
			}
			if len(path) > 6 {
				return
			}
			last := path[len(path)-1]
			if (len(path) == 3 || len(path) == 6) && edges[last][path[0]] {
				pts := [][2]float64{}
				for _, i := range path {
					pts = append(pts, nodes[i])
				}
				b := boxOfPoints(pts...)
				if convexCallout(pts) && b.Contains(w.Box.CX(), w.Box.CY()) && pointInFlag(pts, w.Box.CX(), w.Box.CY()) {
					key := fmt.Sprintf("%.1f:%.1f:%.1f:%.1f", b.X0, b.Y0, b.X1, b.Y1)
					if !seen[key] {
						seen[key] = true
						out = append(out, CalloutPolygon{Points: pts, Box: b})
					}
				}
			}
			for next := range edges[last] {
				if next < path[0] {
					continue
				}
				used := false
				for _, n := range path {
					if next == n {
						used = true
					}
				}
				if !used {
					walk(append(append([]int{}, path...), next))
				}
			}
		}
		for i := range nodes {
			walk([]int{i})
		}
	}
	return out
}
func pointInFlag(p [][2]float64, x, y float64) bool {
	sign := 0.0
	for i, a := range p {
		b := p[(i+1)%len(p)]
		c := (b[0]-a[0])*(y-a[1]) - (b[1]-a[1])*(x-a[0])
		if sign*c < 0 {
			return false
		}
		if math.Abs(c) > .01 {
			sign = c
		}
	}
	return true
}

// Only endpoint-connected straight segments are traversed; crossings do not
// imply connectivity. Branches, loops and truncated evidence remain held.
func calloutLeaders(p PageLayout, b Box, shape ...string) ([][2]float64, string) {
	if p.VectorDetailTruncated {
		return nil, "HELD: vector extraction incomplete"
	}
	tol := .5
	near := func(a, c [2]float64) bool { return math.Hypot(a[0]-c[0], a[1]-c[1]) <= tol }
	segs := []Segment{}
	seen := map[string]bool{}
	for _, s := range p.LeaderSegments {
		a, z := [2]float64{s[0], s[1]}, [2]float64{s[2], s[3]}
		if near(a, z) {
			continue
		}
		if a[0] > z[0] || a[0] == z[0] && a[1] > z[1] {
			a, z = z, a
		}
		k := fmt.Sprintf("%.1f,%.1f,%.1f,%.1f", a[0], a[1], z[0], z[1])
		if !seen[k] {
			seen[k] = true
			segs = append(segs, Segment{a[0], a[1], z[0], z[1]})
		}
	}
	border := func(a [2]float64) bool {
		if len(shape) > 0 && shape[0] == "circle" {
			return math.Abs(math.Hypot(a[0]-b.CX(), a[1]-b.CY())-b.W()/2) <= tol
		}

		dx := math.Max(b.X0-a[0], math.Max(0, a[0]-b.X1))
		dy := math.Max(b.Y0-a[1], math.Max(0, a[1]-b.Y1))
		inside := a[0] > b.X0+tol && a[0] < b.X1-tol && a[1] > b.Y0+tol && a[1] < b.Y1-tol
		return !inside && math.Hypot(dx, dy) <= tol
	}
	targets := [][2]float64{}
	ambiguous := false
	for i, s := range segs {
		a, z := [2]float64{s[0], s[1]}, [2]float64{s[2], s[3]}
		if !border(a) && !border(z) {
			continue
		}
		if border(z) {
			a, z = z, a
		}
		if b.Contains(z[0], z[1]) && !(len(shape) > 0 && shape[0] == "circle" && math.Hypot(z[0]-b.CX(), z[1]-b.CY()) > b.W()/2) {
			continue
		}
		used := map[int]bool{i: true}
		at := z
		for n := 0; n < 100; n++ {
			next := -1
			dest := at
			count := 0
			for j, t := range segs {
				if used[j] {
					continue
				}
				u, v := [2]float64{t[0], t[1]}, [2]float64{t[2], t[3]}
				if near(at, u) {
					next = j
					dest = v
					count++
				} else if near(at, v) {
					next = j
					dest = u
					count++
				}
			}
			if count == 0 {
				targets = append(targets, at)
				break
			}
			if count > 1 {
				ambiguous = true
				break
			}
			used[next] = true
			at = dest
			if n == 99 {
				ambiguous = true
			}
		}
	}
	if ambiguous {
		return targets, "HELD: branching leader needs review"
	}
	if len(targets) == 0 {
		return targets, "No independent leader resolved; shared groups need review"
	}
	return targets, "Traced endpoint candidate; verify target on page"
}
