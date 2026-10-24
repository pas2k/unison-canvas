// Copyright (c) 2026 by Richard A. Wilkes. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, version 2.0. If a copy of the MPL was not distributed with
// this file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// This Source Code Form is "Incompatible With Secondary Licenses", as
// defined by the Mozilla Public License, version 2.0.

package path

import (
	"slices"
	"testing"

	"github.com/richardwilkes/canvas/geom"
)

func collectEdges(p *Path) []EdgeIterResult {
	var out []EdgeIterResult
	it := NewEdgeIter(p)
	for {
		e, ok := it.Next()
		if !ok {
			return out
		}
		cp := e
		cp.Pts = append([]geom.Point(nil), e.Pts...)
		out = append(out, cp)
	}
}

// TestEdgeIterReleaseDropsPath checks that Release clears the retained *Path and leaves the iterator in its fresh
// state, so a pooled owner (the rasterizer's edge builders keep an EdgeIter and an EdgeClipScratch across fills) pins
// no path while idle.
func TestEdgeIterReleaseDropsPath(t *testing.T) {
	p := New()
	p.MoveTo(0, 0)
	p.LineTo(10, 0)
	p.LineTo(10, 10)
	p.Close()

	it := NewEdgeIter(p)
	if _, ok := it.Next(); !ok {
		t.Fatal("expected an edge")
	}
	if it.path == nil {
		t.Fatal("bad test setup: the iterator should hold the path")
	}
	it.Release()
	if it.path != nil {
		t.Fatal("Release left the iterator holding the path")
	}
	if *it != (EdgeIter{conicIdx: -1}) {
		t.Fatalf("Release left residual state: %+v", *it)
	}
	// A released iterator is still reusable.
	it.Reset(p)
	if _, ok := it.Next(); !ok {
		t.Fatal("a released iterator must still be usable after Reset")
	}

	var s EdgeClipScratch
	s.ClipPath(p, geom.RectLTRB(-100, -100, 100, 100), true, func(*geom.EdgeClipper, bool) {})
	if s.iter.path == nil {
		t.Fatal("bad test setup: the clip scratch should hold the path")
	}
	s.Release()
	if s.iter.path != nil {
		t.Fatal("Release left the clip scratch holding the path")
	}
	// A released scratch is still reusable: ClipPath resets everything it needs.
	edges := 0
	s.ClipPath(p, geom.RectLTRB(-100, -100, 100, 100), true, func(*geom.EdgeClipper, bool) { edges++ })
	if edges == 0 {
		t.Fatal("a released clip scratch must still be usable")
	}
}

func TestEdgeIterEmitsEdgesAndCloseLines(t *testing.T) {
	for _, c := range []struct {
		name  string
		build func(p *Path)
		want  []EdgeIterResult
	}{
		{name: "empty", build: func(*Path) {}},
		{name: "move only", build: func(p *Path) {
			p.MoveTo(1, 1)
			p.MoveTo(2, 2)
		}},
		{
			name: "open contour auto-closes",
			build: func(p *Path) {
				p.MoveTo(0, 0)
				p.LineTo(10, 0)
				p.LineTo(10, 10)
			},
			want: []EdgeIterResult{
				{Verb: VerbLine, Pts: []geom.Point{{X: 0, Y: 0}, {X: 10, Y: 0}}, IsNewContour: true},
				{Verb: VerbLine, Pts: []geom.Point{{X: 10, Y: 0}, {X: 10, Y: 10}}},
				{Verb: VerbLine, Pts: []geom.Point{{X: 10, Y: 10}, {X: 0, Y: 0}}},
			},
		},
		{
			name: "multiple contours and conics",
			build: func(p *Path) {
				p.MoveTo(0, 0)
				p.ConicTo(5, 0, 5, 5, 0.7)
				p.Close()
				p.MoveTo(20, 20)
				p.QuadTo(25, 20, 25, 25)
			},
			want: []EdgeIterResult{
				{Verb: VerbConic, Pts: []geom.Point{{X: 0, Y: 0}, {X: 5, Y: 0}, {X: 5, Y: 5}}, IsNewContour: true},
				{Verb: VerbLine, Pts: []geom.Point{{X: 5, Y: 5}, {X: 0, Y: 0}}},
				{Verb: VerbQuad, Pts: []geom.Point{{X: 20, Y: 20}, {X: 25, Y: 20}, {X: 25, Y: 25}}, IsNewContour: true},
				{Verb: VerbLine, Pts: []geom.Point{{X: 25, Y: 25}, {X: 20, Y: 20}}},
			},
		},
	} {
		p := New()
		c.build(p)
		edges := collectEdges(p)
		if len(edges) != len(c.want) {
			t.Errorf("%s: got %d edges, want %d: %+v", c.name, len(edges), len(c.want), edges)
			continue
		}
		for i, e := range edges {
			w := c.want[i]
			if e.Verb != w.Verb || e.IsNewContour != w.IsNewContour || !slices.Equal(e.Pts, w.Pts) {
				t.Errorf("%s: edge %d = %+v, want %+v", c.name, i, e, w)
			}
		}
	}
}

func TestEdgeIterConicWeight(t *testing.T) {
	p := New()
	p.MoveTo(0, 0)
	p.ConicTo(5, 0, 5, 5, 0.75)
	it := NewEdgeIter(p)
	e, ok := it.Next()
	if !ok || e.Verb != VerbConic {
		t.Fatalf("expected conic, got %+v ok=%v", e, ok)
	}
	if it.ConicWeight() != 0.75 {
		t.Errorf("weight = %v", it.ConicWeight())
	}
}

func TestMapRectPerspectiveStraddling(t *testing.T) {
	// A perspective matrix whose w plane cuts through the rect: the faithful mapRect clips rather than projecting the
	// negative-w corners.
	var m geom.Matrix
	m.SetAll(
		1, 0, 0,
		0, 1, 0,
		0, -0.01, 0.05,
	) // w = 0.05 - 0.01*y crosses zero at y = 5
	src := geom.RectLTRB(0, 0, 10, 100)
	got, stays := MapRect(&m, src)
	if stays {
		t.Error("perspective mapRect must report stays == false")
	}
	if got.IsEmpty() {
		t.Error("clipped mapRect should not be empty")
	}
	if !got.IsFinite() {
		t.Errorf("clipped mapRect must be finite, got %v", got)
	}
	// Surviving points were clipped to w >= ~1/16384, so their projected coordinates must be bounded.
	const big = 1e7
	if got.Left < -big || got.Right > big || got.Top < -big || got.Bottom > big {
		t.Errorf("clipped mapRect unexpectedly large: %v", got)
	}
	// Non-perspective delegates to geom.
	var id geom.Matrix
	id.SetAll(2, 0, 1, 0, 2, 1, 0, 0, 1)
	gotAffine, _ := MapRect(&id, geom.RectLTRB(0, 0, 1, 1))
	if gotAffine != geom.RectLTRB(1, 1, 3, 3) {
		t.Errorf("affine MapRect = %v", gotAffine)
	}
}
