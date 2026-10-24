// Copyright (c) 2026 by Richard A. Wilkes. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, version 2.0. If a copy of the MPL was not distributed with
// this file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// This Source Code Form is "Incompatible With Secondary Licenses", as
// defined by the Mozilla Public License, version 2.0.

package pathops

import (
	"testing"

	"github.com/richardwilkes/canvas/geom"
	"github.com/richardwilkes/canvas/path"
)

func buildContours(t *testing.T, p *path.Path) *opContourHead {
	t.Helper()
	head := &opContourHead{}
	state := newOpGlobalState(head)
	b := newOpEdgeBuilder(p, head, state)
	if !b.finish() {
		t.Fatal("edge builder finish() returned false")
	}
	return head
}

// realContours returns the built contours that carry at least one segment (the head is never unlinked, so an empty path
// leaves it in the list with a zero count).
func realContours(head *opContourHead) []*opContour {
	var out []*opContour
	for c := &head.opContour; c != nil; c = c.next {
		if c.count > 0 {
			out = append(out, c)
		}
	}
	return out
}

func segVerbs(c *opContour) []path.Verb {
	var out []path.Verb
	for s := c.first(); s != nil; s = s.next {
		out = append(out, s.verb)
	}
	return out
}

func verbsEqual(a []path.Verb, b ...path.Verb) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestEdgeBuilderContours checks the contours and segment verbs the edge builder makes from each path, plus any
// per-row segment points or weight.
func TestEdgeBuilderContours(t *testing.T) {
	line, quad, conic, cubic := path.VerbLine, path.VerbQuad, path.VerbConic, path.VerbCubic
	cases := []struct {
		name   string
		p      *path.Path
		verbs  [][]path.Verb      // per contour
		segs   [][2]geom.Point    // head and tail of each first-contour segment
		starts map[int]geom.Point // contour index -> head of its first segment
		ctrl   []geom.Point       // leading points of the first segment
		weight float32            // when nonzero, the first segment's weight
	}{
		{
			name:  "triangle",
			p:     path.New().MoveTo(0, 0).LineTo(60, 0).LineTo(30, 50).Close(),
			verbs: [][]path.Verb{{line, line, line}},
			segs:  [][2]geom.Point{{pt(0, 0), pt(60, 0)}, {pt(60, 0), pt(30, 50)}, {pt(30, 50), pt(0, 0)}},
		},
		{
			name:  "rect",
			p:     path.New().AddRect(geom.RectLTRB(0, 0, 20, 10), geom.DirectionCW),
			verbs: [][]path.Verb{{line, line, line, line}},
		},
		{
			// The final lineTo returns to the start point, so the close is degenerate and adds no extra closing line.
			name:  "explicit close to start",
			p:     path.New().MoveTo(0, 0).LineTo(10, 0).LineTo(10, 10).LineTo(0, 0).Close(),
			verbs: [][]path.Verb{{line, line, line}},
		},
		{
			name: "two contours",
			p: path.New().MoveTo(0, 0).LineTo(60, 0).LineTo(30, 50).Close().
				MoveTo(100, 100).LineTo(160, 100).LineTo(130, 150).Close(),
			verbs:  [][]path.Verb{{line, line, line}, {line, line, line}},
			starts: map[int]geom.Point{1: pt(100, 100)},
		},
		{
			// A monotonic quad whose control tangent does not reverse is added whole (no max-curvature split).
			name:  "monotonic quad",
			p:     path.New().MoveTo(0, 0).QuadTo(10, 0, 10, 10).Close(),
			verbs: [][]path.Verb{{quad, line}},
			ctrl:  []geom.Point{pt(0, 0), pt(10, 0), pt(10, 10)},
		},
		{
			name:   "monotonic conic",
			p:      path.New().MoveTo(0, 0).ConicTo(10, 0, 10, 10, 0.7).Close(),
			verbs:  [][]path.Verb{{conic, line}},
			weight: 0.7,
		},
		{
			// A self-intersecting (loop) cubic must be split by complexBreak before it enters the model.
			name: "loop cubic splits",
			p: path.New().MoveTo(635.625, 614.687).
				CubicTo(171.625, 236.188, 1064.62, 135.688, 516.625, 570.187).Close(),
			verbs: [][]path.Verb{{cubic, cubic, line}},
		},
	}
	for _, c := range cases {
		cs := realContours(buildContours(t, c.p))
		if len(cs) != len(c.verbs) {
			t.Fatalf("%s: contour count = %d, want %d", c.name, len(cs), len(c.verbs))
		}
		for i, want := range c.verbs {
			if v := segVerbs(cs[i]); !verbsEqual(v, want...) {
				t.Fatalf("%s: contour %d verbs = %v, want %v", c.name, i, v, want)
			}
		}
		s := cs[0].first()
		for i, want := range c.segs {
			if s.head.pt() != want[0] || s.tail.pt() != want[1] {
				t.Fatalf("%s: segment %d = %v->%v, want %v->%v", c.name, i, s.head.pt(), s.tail.pt(), want[0], want[1])
			}
			s = s.next
		}
		for i, want := range c.starts {
			if got := cs[i].first().head.pt(); got != want {
				t.Fatalf("%s: contour %d starts at %v, want %v", c.name, i, got, want)
			}
		}
		for i, want := range c.ctrl {
			if got := cs[0].first().pts[i]; got != want {
				t.Fatalf("%s: first segment point %d = %v, want %v", c.name, i, got, want)
			}
		}
		if c.weight != 0 {
			if w := cs[0].first().weight; w != c.weight {
				t.Fatalf("%s: first segment weight = %v, want %v", c.name, w, c.weight)
			}
		}
	}
}

func TestEdgeBuilderOpenContourAutoCloses(t *testing.T) {
	// An open contour (no explicit close) is closed by preFetch, materializing the closing line.
	p := path.New()
	p.MoveTo(0, 0).LineTo(60, 0).LineTo(30, 50)
	head := buildContours(t, p)
	cs := realContours(head)
	if len(cs) != 1 {
		t.Fatalf("contour count = %d, want 1", len(cs))
	}
	if v := segVerbs(cs[0]); !verbsEqual(v, path.VerbLine, path.VerbLine, path.VerbLine) {
		t.Fatalf("verbs = %v, want three lines (incl. the closing edge)", v)
	}
	var last *opSegment
	for s := cs[0].first(); s != nil; s = s.next {
		last = s
	}
	if last.tail.pt() != pt(0, 0) {
		t.Fatalf("closing segment ends at %v, want (0,0)", last.tail.pt())
	}
}

func TestEdgeBuilderMonotonicCubic(t *testing.T) {
	// A cubic monotonic in x and y is not complex-broken; it is added whole.
	p := path.New()
	p.MoveTo(0, 0).CubicTo(10, 5, 20, 25, 30, 30).Close()
	head := buildContours(t, p)
	cs := realContours(head)
	if len(cs) != 1 {
		t.Fatalf("contour count = %d, want 1", len(cs))
	}
	if v := segVerbs(cs[0]); !verbsEqual(v, path.VerbCubic, path.VerbLine) {
		t.Fatalf("verbs = %v, want cubic then closing line", v)
	}
}

func TestEdgeBuilderDegenerateLineSkipped(t *testing.T) {
	// The zero-length lineTo is dropped by preFetch; the triangle survives intact.
	p := path.New()
	p.MoveTo(0, 0).LineTo(0, 0).LineTo(60, 0).LineTo(30, 50).Close()
	head := buildContours(t, p)
	cs := realContours(head)
	if len(cs) != 1 {
		t.Fatalf("contour count = %d, want 1", len(cs))
	}
	if v := segVerbs(cs[0]); !verbsEqual(v, path.VerbLine, path.VerbLine, path.VerbLine) {
		t.Fatalf("verbs = %v, want three lines (degenerate dropped)", v)
	}
}

func TestEdgeBuilderFillMask(t *testing.T) {
	makeTri := func(ft path.FillType) *opContour {
		p := path.New()
		p.SetFillType(ft)
		p.MoveTo(0, 0).LineTo(60, 0).LineTo(30, 50).Close()
		return &buildContours(t, p).opContour
	}
	if makeTri(path.FillEvenOdd).xor != true {
		t.Fatal("even-odd fill should set the contour xor flag")
	}
	if makeTri(path.FillInverseEvenOdd).xor != true {
		t.Fatal("inverse-even-odd fill should set the contour xor flag")
	}
	if makeTri(path.FillWinding).xor != false {
		t.Fatal("winding fill should clear the contour xor flag")
	}
	if makeTri(path.FillInverseWinding).xor != false {
		t.Fatal("inverse-winding fill should clear the contour xor flag")
	}
}

func TestEdgeBuilderEmptyPath(t *testing.T) {
	head := buildContours(t, path.New())
	if head.count != 0 {
		t.Fatalf("empty path head count = %d, want 0", head.count)
	}
	if len(realContours(head)) != 0 {
		t.Fatalf("empty path produced %d real contours, want 0", len(realContours(head)))
	}
}

func TestEdgeBuilderConicSplit(t *testing.T) {
	// A conic whose control tangent reverses is chopped at max curvature into two conics.
	p := path.New()
	p.MoveTo(0, 0).ConicTo(10, 20, 20, 0, 0.5).Close()
	head := buildContours(t, p)
	cs := realContours(head)
	if len(cs) != 1 {
		t.Fatalf("contour count = %d, want 1", len(cs))
	}
	if v := segVerbs(cs[0]); !verbsEqual(v, path.VerbConic, path.VerbConic, path.VerbLine) {
		t.Fatalf("verbs = %v, want two conic halves + closing line", v)
	}
}

func TestEdgeBuilderTwoOperands(t *testing.T) {
	// addOperand appends a second path; walk sets operand at the first-operand boundary, so the second path's contour
	// is marked as the operand.
	pa := path.New()
	pa.MoveTo(0, 0).LineTo(10, 0).LineTo(5, 8).Close()
	pb := path.New()
	pb.MoveTo(100, 0).LineTo(110, 0).LineTo(105, 8).Close()
	head := &opContourHead{}
	state := newOpGlobalState(head)
	b := newOpEdgeBuilder(pa, head, state)
	b.addOperand(pb)
	if !b.finish() {
		t.Fatal("edge builder finish() returned false")
	}
	cs := realContours(head)
	if len(cs) != 2 {
		t.Fatalf("contour count = %d, want 2", len(cs))
	}
	if cs[0].operand != false {
		t.Fatal("first contour should not be the operand")
	}
	if cs[1].operand != true {
		t.Fatal("second contour should be the operand")
	}
}

func TestComplexBreak(t *testing.T) {
	// A cubic monotonic in x and y needs no split.
	mono := [4]geom.Point{pt(0, 0), pt(10, 5), pt(20, 25), pt(30, 30)}
	var mt [3]float32
	if n := complexBreak(mono, mt[:]); n != 0 {
		t.Fatalf("monotonic cubic breaks = %d, want 0", n)
	}
	// A loop cubic whose self-intersection parameter lies inside [0,1] is split once, near the crossing.
	loop := [4]geom.Point{pt(635.625, 614.687), pt(171.625, 236.188), pt(1064.62, 135.688), pt(516.625, 570.187)}
	var lt [3]float32
	n := complexBreak(loop, lt[:])
	if n != 1 {
		t.Fatalf("loop cubic breaks = %d, want 1", n)
	}
	if lt[0] <= 0 || lt[0] >= 1 {
		t.Fatalf("loop split t = %v, want in (0,1)", lt[0])
	}
	// A serpentine (two inflections) and a cusp both drive the non-loop branch through findInflections /
	// findMaxCurvature / calcPrecision, and each yields a split point in (0,1).
	serp := [4]geom.Point{pt(285.625, 499.687), pt(411.625, 808.188), pt(1064.62, 135.688), pt(1042.63, 585.187)}
	var stv [3]float32
	if ns := complexBreak(serp, stv[:]); ns != 1 || stv[0] <= 0 || stv[0] >= 1 {
		t.Fatalf("serpentine breaks = %d, t = %v, want 1 split in (0,1)", ns, stv[0])
	}
	cusp := [4]geom.Point{pt(0, 0), pt(1, 1), pt(1, 0), pt(0, 1)}
	var ctv [3]float32
	if nc := complexBreak(cusp, ctv[:]); nc != 1 || ctv[0] <= 0 || ctv[0] >= 1 {
		t.Fatalf("cusp breaks = %d, t = %v, want 1 split in (0,1)", nc, ctv[0])
	}
}
