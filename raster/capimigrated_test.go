// Copyright (c) 2026 by Richard A. Wilkes. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, version 2.0. If a copy of the MPL was not distributed with
// this file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// This Source Code Form is "Incompatible With Secondary Licenses", as
// defined by the Mozilla Public License, version 2.0.

// Contract checks for the public raster entry points.

package raster

import (
	"testing"

	"github.com/richardwilkes/canvas/geom"
	"github.com/richardwilkes/canvas/path"
)

// TestWrapBytes verifies WrapBytes' contract: a valid buffer wraps without copying, and each unsupported configuration
// (bad dimensions, narrow or non-whole-pixel rowBytes, short buffer) returns nil.
func TestWrapBytes(t *testing.T) {
	const w, h = 4, 3
	buf := make([]byte, w*h*4)
	pm := WrapBytes(buf, w, h, w*4)
	if pm == nil {
		t.Fatal("WrapBytes with a valid buffer returned nil")
	}
	if pm.Width != w || pm.Height != h {
		t.Fatalf("wrapped pixmap is %dx%d, want %dx%d", pm.Width, pm.Height, w, h)
	}
	// No copy: a write through the pixmap words lands in the caller's bytes.
	pm.Pix[0] = 0xAABBCCDD
	if buf[0] != 0xDD || buf[3] != 0xAA {
		t.Error("WrapBytes did not alias the caller's buffer")
	}

	if WrapBytes(buf, 0, h, w*4) != nil {
		t.Error("zero width: want nil")
	}
	if WrapBytes(buf, w, -1, w*4) != nil {
		t.Error("negative height: want nil")
	}
	if WrapBytes(buf, w, h, w*4-4) != nil {
		t.Error("rowBytes narrower than the row: want nil")
	}
	if WrapBytes(buf, w, h, w*4+2) != nil {
		t.Error("non-whole-pixel rowBytes: want nil")
	}
	if WrapBytes(buf[:w*h*4-4], w, h, w*4) != nil {
		t.Error("too-small buffer: want nil")
	}
	// A width whose width*4 overflows int32 to a negative value must still be rejected: a small rowBytes cannot hold
	// the row, and the guard must not be fooled by the wrapped-around product.
	if WrapBytes(buf, 1<<29, 1, 8) != nil {
		t.Error("width*4 overflow with narrow rowBytes: want nil")
	}
}

// TestAAFillMaskLane AA-fills shapes on the additive-mask lane. The interior must reach full coverage (the mask
// blitter's BlitRect rows), integer-aligned extremes and straight sides produce the zero-alpha edge columns its BlitV
// guard skips, so the pixels just outside stay untouched, and the AA rim carries partial coverage.
func TestAAFillMaskLane(t *testing.T) {
	for _, c := range []struct {
		name     string
		add      func(p *path.Path)
		outside  []geom.IRect // must stay untouched
		interior geom.IRect   // must be fully covered
		rim      geom.IRect   // must hold some partial coverage
	}{
		{
			// The interior is a horizontal run through the center; the rim is the top arc.
			name:     "circle",
			add:      func(p *path.Path) { p.AddCircle(17, 22, 12, geom.DirectionCW) },
			interior: geom.IRectLTRB(10, 22, 25, 23),
			outside:  []geom.IRect{geom.IRectLTRB(2, 2, 3, 3)},
			rim:      geom.IRectLTRB(12, 9, 23, 12),
		},
		{
			// The interior is the rows between the corner radii, edge to edge, and the outside is the column just past
			// each integer-aligned side; the rim is a rounded corner.
			name:     "round rect",
			add:      func(p *path.Path) { p.AddRoundRect(geom.RectLTRB(6, 9, 33, 27), 6, 3, geom.DirectionCW) },
			interior: geom.IRectLTRB(6, 13, 33, 24),
			outside:  []geom.IRect{geom.IRectLTRB(5, 13, 6, 24), geom.IRectLTRB(33, 13, 34, 24)},
			rim:      geom.IRectLTRB(6, 9, 13, 13),
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := path.New()
			c.add(p)
			dev := aaFill(t, p, 40, 40)
			for y := c.interior.Top; y < c.interior.Bottom; y++ {
				for x := c.interior.Left; x < c.interior.Right; x++ {
					if a := alphaAt(dev, x, y); a != 255 {
						t.Fatalf("interior (%d,%d) alpha %d, want 255", x, y, a)
					}
				}
			}
			for _, r := range c.outside {
				for y := r.Top; y < r.Bottom; y++ {
					for x := r.Left; x < r.Right; x++ {
						if a := alphaAt(dev, x, y); a != 0 {
							t.Fatalf("outside (%d,%d) alpha %d, want 0", x, y, a)
						}
					}
				}
			}
			for y := c.rim.Top; y < c.rim.Bottom; y++ {
				for x := c.rim.Left; x < c.rim.Right; x++ {
					if a := alphaAt(dev, x, y); a > 0 && a < 255 {
						return
					}
				}
			}
			t.Error("no partial-coverage AA rim found")
		})
	}
}
