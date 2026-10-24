// Copyright (c) 2026 by Richard A. Wilkes. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, version 2.0. If a copy of the MPL was not distributed with
// this file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// This Source Code Form is "Incompatible With Secondary Licenses", as
// defined by the Mozilla Public License, version 2.0.

// Focused unit tests for dCubic's curve-vs-curve members used by the cubic solver (convexHull, chopAt, subDivide,
// dxdyAtT, otherPts, collapsed/controlsInside). The end-to-end cubic intersection drivers live in tsectcurve_test.go.
// cu4 is defined in dcubiclineintersect_test.go.

package pathops

import "testing"

// mkCubic builds a dCubic from double-precision points (debugSet semantics).
func mkCubic(pts [4]dPoint) dCubic {
	var c dCubic
	c.debugSet(pts)
	return c
}

// TestCubicConvexHull verifies the hull point count, and that a four-point hull's order is a permutation.
func TestCubicConvexHull(t *testing.T) {
	for _, tc := range []struct {
		name string
		c    dCubic
		want int
	}{
		// A convex-quadrilateral cubic has a four-point hull.
		{name: "square", c: mkCubic(cu4(0, 0, 0, 1, 1, 1, 1, 0)), want: 4},
		// (0,0),(4,0),(0,4) form a triangle; (1,1) is strictly inside it.
		{name: "triangle", c: mkCubic(cu4(0, 0, 4, 0, 0, 4, 1, 1)), want: 3},
	} {
		var order [4]int
		got := tc.c.convexHull(&order)
		if got != tc.want {
			t.Fatalf("%s: convexHull count=%d, want %d", tc.name, got, tc.want)
		}
		if got != 4 {
			continue
		}
		seen := [4]bool{}
		for _, o := range order {
			if o < 0 || o > 3 || seen[o] {
				t.Fatalf("%s: convexHull order not a permutation: %v", tc.name, order)
			}
			seen[o] = true
		}
	}
}

// TestCubicSubDivideInvariant verifies the subdivision invariant: sub over [t1,t2] at s equals the parent at t1 +
// s*(t2-t1). This exercises both the chopAt lanes (t1==0 / t2==1, at t=0.5 and elsewhere) and the general lane.
func TestCubicSubDivideInvariant(t *testing.T) {
	cubics := []dCubic{
		mkCubic(cu4(0, 0, 0, 1, 1, 1, 1, 0)),
		mkCubic(cu4(0, 1, 4, 5, 1, 0, 5, 3)),
		mkCubic(cu4(2, 3, 0, 4, 3, 2, 5, 3)),
		mkCubic(cu4(945.08099365234375, 747.1619873046875, 982.5679931640625, 747.1619873046875,
			1013.6290283203125, 719.656005859375, 1019.1910400390625, 683.72601318359375)),
	}
	ranges := [][2]float64{{0, 1}, {0, 0.5}, {0.5, 1}, {0, 0.3}, {0.3, 1}, {0, 0.9}, {0.9, 1}, {0.25, 0.75}, {0.3, 0.6}}
	samples := []float64{0, 0.25, 0.5, 0.75, 1}
	for ci, c := range cubics {
		for _, r := range ranges {
			t1, t2 := r[0], r[1]
			sub := c.subDivide(t1, t2)
			for _, s := range samples {
				got := sub.ptAtT(s)
				want := c.ptAtT(t1 + s*(t2-t1))
				if !got.approximatelyEqual(want) {
					t.Fatalf("cubic[%d] subDivide(%g,%g) at s=%g: got (%g,%g) want (%g,%g)",
						ci, t1, t2, s, got.x, got.y, want.x, want.y)
				}
			}
		}
	}
}

// TestCubicDxdyAtTEnds verifies the endpoint tangents: dxdyAtT(0)=3*(p1-p0), dxdyAtT(1)=3*(p3-p2).
func TestCubicDxdyAtTEnds(t *testing.T) {
	c := mkCubic(cu4(1, 2, 3, 7, 5, 1, 6, 4))
	d0 := c.dxdyAtT(0)
	if d0.x != 3*(c.pts[1].x-c.pts[0].x) || d0.y != 3*(c.pts[1].y-c.pts[0].y) {
		t.Fatalf("dxdyAtT(0)=(%g,%g)", d0.x, d0.y)
	}
	d1 := c.dxdyAtT(1)
	if d1.x != 3*(c.pts[3].x-c.pts[2].x) || d1.y != 3*(c.pts[3].y-c.pts[2].y) {
		t.Fatalf("dxdyAtT(1)=(%g,%g)", d1.x, d1.y)
	}
}

// TestCubicOtherPts verifies the endpoint-odd-man point selection.
func TestCubicOtherPts(t *testing.T) {
	c := mkCubic(cu4(1, 2, 3, 4, 5, 6, 7, 8))
	if o := c.otherPts(0); o != [3]dPoint{c.pts[1], c.pts[2], c.pts[3]} {
		t.Fatalf("otherPts(0)=%v", o)
	}
	if o := c.otherPts(3); o != [3]dPoint{c.pts[0], c.pts[1], c.pts[2]} {
		t.Fatalf("otherPts(3)=%v", o)
	}
}

// TestCubicCollapsedControls verifies collapsed and controlsInside.
func TestCubicCollapsedControls(t *testing.T) {
	if !mkCubic(cu4(3, 3, 3, 3, 3, 3, 3, 3)).collapsed() {
		t.Fatal("coincident cubic should be collapsed")
	}
	if mkCubic(cu4(0, 0, 0, 1, 1, 1, 1, 0)).collapsed() {
		t.Fatal("square cubic should not be collapsed")
	}
	// A cubic whose control points project between the endpoints along the chord.
	if !mkCubic(cu4(0, 0, 1, 1, 2, 1, 3, 0)).controlsInside() {
		t.Fatal("expected controlsInside true")
	}
}
