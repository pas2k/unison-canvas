// Copyright (c) 2026 by Richard A. Wilkes. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, version 2.0. If a copy of the MPL was not distributed with
// this file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// This Source Code Form is "Incompatible With Secondary Licenses", as
// defined by the Mozilla Public License, version 2.0.

// Entry-point behavior checks driven through the public canvas surface.

package canvas

import (
	"math"
	"testing"

	"github.com/richardwilkes/canvas/geom"
	"github.com/richardwilkes/canvas/path"
	"github.com/richardwilkes/canvas/raster"
	"github.com/richardwilkes/canvas/stroke"
)

// TestCanvasSkewAndResetMatrix verifies Skew composes the same skew matrix SetSkew builds, rotating by
// RadiansToDegrees(pi/2) builds (approximately) the same matrix as SetRotate(90), and ResetMatrix returns the CTM to
// identity regardless of what preceded it.
func TestCanvasSkewAndResetMatrix(t *testing.T) {
	c, _ := newWhiteCanvas()
	c.Translate(3, 5)
	c.Skew(0.1, 0.2)

	want, _ := newWhiteCanvas()
	want.Translate(3, 5)
	var sk geom.Matrix
	sk.SetSkew(0.1, 0.2)
	want.Concat(&sk)
	got, ref := c.TotalMatrix(), want.TotalMatrix()
	if got.As9() != ref.As9() {
		t.Errorf("Skew matrix = %v, want %v", got.As9(), ref.As9())
	}

	r, _ := newWhiteCanvas()
	r.Rotate(geom.RadiansToDegrees(float32(math.Pi / 2)))
	var rot geom.Matrix
	rot.SetRotate(90)
	rm := r.TotalMatrix()
	g, w := rm.As9(), rot.As9()
	for i := range g {
		if d := g[i] - w[i]; d > 1e-3 || d < -1e-3 {
			t.Errorf("rotate(RadiansToDegrees(pi/2)) matrix = %v, want ~rotate(90) %v", g, w)
			break
		}
	}

	c.Rotate(37)
	c.Scale(2, 0.5)
	c.ResetMatrix()
	identity := geom.IdentityMatrix()
	if m := c.TotalMatrix(); !m.CheapEqual(&identity) {
		t.Errorf("ResetMatrix gave %v, want identity", m.As9())
	}
}

// TestCanvasQuickRejectPath covers QuickRejectPath's semantics: an empty path is always
// rejected, a path outside the clip is rejected, and a path inside it is not.
func TestCanvasQuickRejectPath(t *testing.T) {
	c, _ := newWhiteCanvas()
	c.ClipRect(geom.RectLTRB(10, 12, 30, 28), raster.ClipIntersect, false)

	var empty path.Path
	if !c.QuickRejectPath(&empty) {
		t.Error("an empty path should be quick-rejected")
	}
	var outside path.Path
	outside.MoveTo(40, 40)
	outside.LineTo(60, 44)
	outside.LineTo(50, 60)
	outside.Close()
	if !c.QuickRejectPath(&outside) {
		t.Error("a path outside the clip should be quick-rejected")
	}
	var inside path.Path
	inside.MoveTo(12, 14)
	inside.LineTo(24, 16)
	inside.LineTo(16, 24)
	inside.Close()
	if c.QuickRejectPath(&inside) {
		t.Error("a path inside the clip should not be quick-rejected")
	}
}

// TestCanvasDrawPointMatchesDrawPoints verifies DrawPoint lowers to a one-point DrawPoints draw, and — with an
// anti-aliased non-round cap — exercises the square-cap point proc.
func TestCanvasDrawPointMatchesDrawPoints(t *testing.T) {
	sp := solidPaint(0xFF2E64C8, true)
	sp.Style = StyleStroke
	sp.StrokeWidth = 3 // default CapButt: the AA square proc

	c1, p1 := newWhiteCanvas()
	c1.DrawPoint(12.5, 19.5, sp)

	c2, p2 := newWhiteCanvas()
	pt := [1]geom.Point{{X: 12.5, Y: 19.5}}
	c2.DrawPoints(PointModePoints, pt[:], sp)

	drew := false
	for i := range p1.Pix {
		if p1.Pix[i] != p2.Pix[i] {
			t.Fatal("DrawPoint output differs from a one-point DrawPoints")
		}
		if p1.Pix[i] != 0xFFFFFFFF {
			drew = true
		}
	}
	if !drew {
		t.Fatal("DrawPoint drew nothing")
	}
}

// TestPaintFillPathStroke exercises Paint.FillPath through to the stroker (the entry point behind unison's
// get-fill-path use): a stroked rectangle becomes a filled outline (true), while a zero-width hairline stroke reports
// false. StrokeSpec must agree with the paint's stroke fields.
func TestPaintFillPathStroke(t *testing.T) {
	var src path.Path
	src.AddRect(geom.Rect{Left: 0, Top: 0, Right: 20, Bottom: 10}, geom.DirectionCW)

	p := NewPaint()
	p.Style = StyleStroke
	p.StrokeWidth = 4

	spec := p.StrokeSpec()
	if spec.Width != 4 || spec.Style != stroke.PaintStyle(StyleStroke) {
		t.Fatalf("StrokeSpec = %+v, want width 4, stroke style", spec)
	}

	var dst path.Path
	if !p.FillPath(&src, &dst, nil, 1) {
		t.Fatal("stroking a width-4 rect should yield a fillable outline (true)")
	}
	if dst.IsEmpty() {
		t.Error("fill path is empty; expected the stroke outline")
	}

	// A zero-width stroke is a hairline: the contract returns false (draw dst as a hairline).
	p.StrokeWidth = 0
	var hair path.Path
	if p.FillPath(&src, &hair, nil, 1) {
		t.Error("a zero-width (hairline) stroke should report false")
	}
}
