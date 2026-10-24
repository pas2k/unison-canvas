// Copyright (c) 2026 by Richard A. Wilkes. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, version 2.0. If a copy of the MPL was not distributed with
// this file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// This Source Code Form is "Incompatible With Secondary Licenses", as
// defined by the Mozilla Public License, version 2.0.

package pdf

import (
	"slices"
	"strings"
	"testing"

	"github.com/richardwilkes/canvas/canvas"
	"github.com/richardwilkes/canvas/geom"
	"github.com/richardwilkes/canvas/path"
	"github.com/richardwilkes/canvas/shaders"
)

// patternMatrices returns the /Matrix entry of every pattern in the document.
func patternMatrices(data []byte) []string {
	var out []string
	for _, d := range allObjectDicts(data) {
		if !strings.Contains(d, "/PatternType") {
			continue
		}
		i := strings.Index(d, "/Matrix [")
		if i < 0 {
			continue
		}
		rest := d[i+len("/Matrix ["):]
		if j := strings.Index(rest, "]"); j >= 0 {
			out = append(out, rest[:j])
		}
	}
	return out
}

func newTestGradient() shaders.Shader {
	return shaders.NewLinearGradient(geom.Point{}, geom.Point{X: 100}, twoStops, nil, shaders.TileClamp, nil)
}

// The lanes below all run their content entry at an identity CTM, so the CTM has to be folded into the shader instead;
// without that the pattern lands at the initial transform alone.
const unfoldedPatternMatrix = "1 0 0 -1 0 100"

// TestDrawPaintFoldsCTMIntoShader checks that DrawPaint under a non-identity CTM positions its shader the same way the
// equivalent full-coverage DrawRect does, and that the shader-nil path of the fold emits no pattern.
func TestDrawPaintFoldsCTMIntoShader(t *testing.T) {
	for _, tc := range []struct {
		shader shaders.Shader
		name   string
		want   []string
	}{
		// initialTransform [1 0 0 -1 0 100] preConcat translate(10,20): the y-flip turns +20 into 100-20 = 80.
		{name: "gradient", shader: newTestGradient(), want: []string{"1 0 0 -1 10 80"}},
		{name: "no shader", shader: nil, want: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := canvas.NewPaint()
			p.Shader = tc.shader
			data := renderPDF(t, 100, 100, func(c *canvas.Canvas) {
				c.Translate(10, 20)
				c.DrawPaint(p)
			})
			validatePDF(t, data)
			got := patternMatrices(data)
			if !slices.Equal(got, tc.want) {
				t.Errorf("DrawPaint pattern /Matrix entries = %v, want %v", got, tc.want)
			}

			// The same coverage drawn as a rect (whose content entry carries the CTM) must position identically.
			ref := renderPDF(t, 100, 100, func(c *canvas.Canvas) {
				c.Translate(10, 20)
				c.DrawRect(geom.RectLTRB(-10, -20, 90, 80), p)
			})
			validatePDF(t, ref)
			if refMatrices := patternMatrices(ref); !slices.Equal(refMatrices, got) {
				t.Errorf("DrawPaint pattern /Matrix = %v, DrawRect equivalent = %v", got, refMatrices)
			}
		})
	}
}

// TestPerspectivePathFoldsCTMIntoShader checks internalDrawPath's perspective lane, which pre-transforms the path and
// resets the CTM to identity; the shader has to make the same trip.
func TestPerspectivePathFoldsCTMIntoShader(t *testing.T) {
	p := canvas.NewPaint()
	p.Shader = newTestGradient()
	var persp geom.Matrix
	persp.SetAll(1, 0, 10, 0, 1, 20, 0, 0.002, 1)
	data := renderPDF(t, 100, 100, func(c *canvas.Canvas) {
		c.Concat(&persp)
		c.DrawRect(geom.RectLTRB(0, 0, 60, 60), p)
	})
	validatePDF(t, data)
	got := patternMatrices(data)
	if len(got) != 1 {
		t.Fatalf("expected exactly one shading pattern, got %d: %v", len(got), got)
	}
	if got[0] == unfoldedPatternMatrix {
		t.Errorf("perspective path pattern /Matrix = [%s]; the CTM was not folded into the shader", got[0])
	}
	// The perspective CTM's translation survives into the pattern's (split-off) affine part.
	if !strings.HasSuffix(got[0], " 10 80") {
		t.Errorf("perspective path pattern /Matrix = [%s], want the CTM's translation (10, 80)", got[0])
	}
}

// TestInversePathFoldsCTMIntoShader checks handleInversePath, which computes the positive path in device space and
// draws it at an identity CTM. Its hairline lane instead re-enters internalDrawPath with the real CTM and so must not
// fold the CTM into the shader a second time.
func TestInversePathFoldsCTMIntoShader(t *testing.T) {
	for _, tc := range []struct {
		name     string
		hairline bool
	}{
		{name: "fill", hairline: false},
		{name: "hairline stroke", hairline: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := canvas.NewPaint()
			p.Shader = newTestGradient()
			if tc.hairline {
				p.Style = canvas.StyleStroke
				p.StrokeWidth = 0
			}
			data := renderPDF(t, 100, 100, func(c *canvas.Canvas) {
				c.Translate(10, 20)
				inv := (&path.Path{}).AddRect(geom.RectLTRB(0, 0, 30, 30), geom.DirectionCW)
				inv.SetFillType(path.FillInverseWinding)
				c.DrawPath(inv, p)
			})
			validatePDF(t, data)
			got := patternMatrices(data)
			if len(got) != 1 {
				t.Fatalf("expected exactly one shading pattern, got %d: %v", len(got), got)
			}
			if got[0] != "1 0 0 -1 10 80" {
				t.Errorf("%s inverse path pattern /Matrix = [%s], want [1 0 0 -1 10 80]", tc.name, got[0])
			}
		})
	}
}
