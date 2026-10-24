// Copyright (c) 2026 by Richard A. Wilkes. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, version 2.0. If a copy of the MPL was not distributed with
// this file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// This Source Code Form is "Incompatible With Secondary Licenses", as
// defined by the Mozilla Public License, version 2.0.

// Tests exercising the double-precision point/vector primitives: equality, arithmetic, distance, approximate/rough
// equality, and normalize including its infinity/NaN behavior.

package pathops

import (
	"math"
	"testing"
)

var dPointVectorTests = []dPoint{
	{x: 0, y: 0},
	{x: 1, y: 0},
	{x: 0, y: 1},
	{x: 2, y: 1},
	{x: 1, y: 2},
	{x: 1, y: 1},
	{x: 2, y: 2},
}

func TestPathOpsDPoint(t *testing.T) {
	for _, pt := range dPointVectorTests {
		p := pt
		if !p.equals(pt) {
			t.Fatalf("p == pt failed for %v", pt)
		}
		if pt.x != pt.x || pt.y != pt.y {
			t.Fatalf("pt != pt for %v", pt)
		}
		v := p.sub(pt)
		p = dPoint{x: p.x + v.x, y: p.y + v.y} // p += v
		if !p.equals(pt) {
			t.Fatalf("p += v changed p for %v", pt)
		}
		p = dPoint{x: p.x - v.x, y: p.y - v.y} // p -= v
		if !p.equals(pt) {
			t.Fatalf("p -= v changed p for %v", pt)
		}
		if !p.approximatelyEqual(pt) {
			t.Fatalf("approximatelyEqual failed for %v", pt)
		}
		sPt := pt.asPoint()
		p.set(sPt)
		if !p.equals(pt) {
			t.Fatalf("set(asSkPoint) mismatch for %v", pt)
		}
		if !p.approximatelyEqualPt(sPt) {
			t.Fatalf("approximatelyEqualPt failed for %v", pt)
		}
		if !p.roughlyEqual(pt) {
			t.Fatalf("roughlyEqual failed for %v", pt)
		}
		p.x, p.y = 0, 0
		if !(p.x == 0 && p.y == 0) {
			t.Fatalf("zeroing failed for %v", pt)
		}
		if !p.approximatelyZero() {
			t.Fatalf("approximatelyZero failed after zeroing for %v", pt)
		}
		if pt.distanceSquared(p) != pt.x*pt.x+pt.y*pt.y {
			t.Fatalf("distanceSquared mismatch for %v", pt)
		}
		if !approximatelyEqual(pt.distance(p), math.Sqrt(pt.x*pt.x+pt.y*pt.y)) {
			t.Fatalf("distance mismatch for %v", pt)
		}
	}
}

func TestPathOpsDVector(t *testing.T) {
	for index := 0; index < len(dPointVectorTests)-1; index++ {
		v1 := dPointVectorTests[index+1].sub(dPointVectorTests[index])
		v2 := dPointVectorTests[index].sub(dPointVectorTests[index+1])
		v1 = dVector{x: v1.x + v2.x, y: v1.y + v2.y} // v1 += v2
		if v1.x != 0 || v1.y != 0 {
			t.Fatalf("v1 += v2 not zero at %d", index)
		}
		v2 = dVector{x: v2.x - v2.x, y: v2.y - v2.y} // v2 -= v2
		if v2.x != 0 || v2.y != 0 {
			t.Fatalf("v2 -= v2 not zero at %d", index)
		}
		v1 = dPointVectorTests[index+1].sub(dPointVectorTests[index])
		v1 = dVector{x: v1.x / 2, y: v1.y / 2} // v1 /= 2
		v1 = dVector{x: v1.x * 2, y: v1.y * 2} // v1 *= 2
		orig := dPointVectorTests[index+1].sub(dPointVectorTests[index])
		v1 = dVector{x: v1.x - orig.x, y: v1.y - orig.y} // v1 -= (tests[i+1] - tests[i])
		if v1.x != 0 || v1.y != 0 {
			t.Fatalf("v1 round-trip not zero at %d", index)
		}
		sv := v1.asVector()
		if sv.X != 0 || sv.Y != 0 {
			t.Fatalf("asSkVector not zero at %d", index)
		}
		v1 = dPointVectorTests[index+1].sub(dPointVectorTests[index])
		lenSq := v1.lengthSquared()
		v1Dot := v1.dot(v1)
		if lenSq != v1Dot {
			t.Fatalf("lengthSquared != dot(self) at %d", index)
		}
		if !approximatelyEqual(math.Sqrt(lenSq), v1.length()) {
			t.Fatalf("length mismatch at %d", index)
		}
		if v1.cross(v1) != 0 {
			t.Fatalf("cross(self) != 0 at %d", index)
		}
	}
}

func TestDVectorNormalize(t *testing.T) {
	// 1/4096 matches the default scalar-nearly-zero threshold.
	const tol = 1.0 / (1 << 12)
	nan := math.NaN()
	for _, tc := range []struct {
		name   string
		in     dVector
		finite bool
		want   dVector // a NaN component requires NaN
		tol    float64
	}{
		{
			name:   "positive",
			in:     dVector{x: 1.2, y: 3.4},
			finite: true,
			want:   dVector{x: 0.332820, y: 0.942990},
			tol:    tol,
		},
		{
			name:   "negative y",
			in:     dVector{x: 2.3, y: -4.5},
			finite: true,
			want:   dVector{x: 0.455111, y: -0.890435},
			tol:    tol,
		},
		{name: "zero vector gives NaN", in: dVector{x: 0, y: 0}, want: dVector{x: nan, y: nan}},
		{name: "overflowing length gives zero", in: dVector{x: math.MaxFloat64, y: math.MaxFloat64}, finite: true},
	} {
		v := tc.in
		v.normalize()
		if v.isFinite() != tc.finite {
			t.Fatalf("%s: isFinite = %v, want %v", tc.name, v.isFinite(), tc.finite)
		}
		for _, c := range [][2]float64{{v.x, tc.want.x}, {v.y, tc.want.y}} {
			got, want := c[0], c[1]
			if math.IsNaN(want) {
				if !math.IsNaN(got) {
					t.Fatalf("%s: %f is not nan", tc.name, got)
				}
			} else if math.Abs(got-want) > tc.tol {
				t.Fatalf("%s: %f != %f", tc.name, got, want)
			}
		}
	}
}
