// Copyright (c) 2026 by Richard A. Wilkes. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, version 2.0. If a copy of the MPL was not distributed with
// this file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// This Source Code Form is "Incompatible With Secondary Licenses", as
// defined by the Mozilla Public License, version 2.0.

package canvas

import (
	"testing"

	"github.com/richardwilkes/canvas/colorfilter"
	"github.com/richardwilkes/canvas/geom"
	"github.com/richardwilkes/canvas/imagecore"
	"github.com/richardwilkes/canvas/maskfilter"
	"github.com/richardwilkes/canvas/raster"
	"github.com/richardwilkes/canvas/shaders"
)

// gradientImage builds a w x h opaque image with distinct per-pixel values.
func gradientImage(w, h int32) *imagecore.Image {
	info, _ := imagecore.MakeInfo(w, h, imagecore.ColorTypeRGBA8888, imagecore.AlphaTypeOpaque)
	p := imagecore.NewPixels(info)
	for y := int32(0); y < h; y++ {
		for x := int32(0); x < w; x++ {
			p.Words[y*p.RowElems+x] = 0xFF000000 | uint32(x*40) | uint32(y*40)<<8 | 0x30<<16
		}
	}
	return imagecore.FromPixels(p)
}

func TestDrawImageIntegerOffsetIsSprite(t *testing.T) {
	for _, alpha := range []uint32{0xFF, 0x80} {
		img := gradientImage(3, 3)
		pix := raster.NewPixmap(8, 8)
		c := NewForPixmap(pix)
		var paint *Paint
		if alpha != 0xFF {
			paint = NewPaint()
			paint.Color = paint.Color.WithAlpha(uint8(alpha))
		}
		c.DrawImage(img, 2, 3, shaders.SamplingOptions{}, paint)

		// The sprite lane blends with a premul lerp of scale alpha+1 (src over dst=0): channel*(alpha+1)>>8.
		src := img.PeekPixels(imagecore.CachingAllow)
		for y := int32(0); y < 3; y++ {
			for x := int32(0); x < 3; x++ {
				s := src.Words[y*src.RowElems+x]
				var want uint32
				for shift := 0; shift < 32; shift += 8 {
					want |= (s >> shift & 0xFF * (alpha + 1) >> 8) << shift
				}
				got := pix.Pix[(y+3)*8+(x+2)]
				if got != want {
					t.Fatalf("alpha %#x: (%d,%d) = %08x want %08x", alpha, x, y, got, want)
				}
			}
		}
		if pix.Pix[0] != 0 || pix.Pix[8*7+7] != 0 {
			t.Fatalf("alpha %#x: pixels outside the image were written", alpha)
		}
	}
}

func TestDrawImageRectScales(t *testing.T) {
	type probe struct{ dx, dy, sx, sy int32 }
	for _, tc := range []struct {
		name       string
		probes     []probe
		src        geom.Rect
		dst        geom.Rect
		imgSize    int32
		pixSize    int32
		constraint SrcRectConstraint
	}{
		{
			// Nearest upscale 4x: destination pixel (1,1) shows source pixel (0,0), etc.
			name:       "nearest 4x upscale",
			imgSize:    2,
			pixSize:    8,
			src:        geom.RectWH(2, 2),
			dst:        geom.RectWH(8, 8),
			constraint: ConstraintFast,
			probes:     []probe{{1, 1, 0, 0}, {6, 1, 1, 0}, {6, 6, 1, 1}},
		},
		{
			// Only the bottom-right 2x2 of the source fills the destination.
			name:       "strict source subset",
			imgSize:    4,
			pixSize:    4,
			src:        geom.RectLTRB(2, 2, 4, 4),
			dst:        geom.RectWH(4, 4),
			constraint: ConstraintStrict,
			probes:     []probe{{0, 0, 2, 2}, {3, 3, 3, 3}},
		},
	} {
		img := gradientImage(tc.imgSize, tc.imgSize)
		pix := raster.NewPixmap(tc.pixSize, tc.pixSize)
		c := NewForPixmap(pix)
		c.DrawImageRect(img, tc.src, tc.dst, shaders.SamplingOptions{}, nil, tc.constraint)

		src := img.PeekPixels(imagecore.CachingAllow)
		for _, p := range tc.probes {
			got := pix.Pix[p.dy*tc.pixSize+p.dx]
			if want := src.Words[p.sy*src.RowElems+p.sx]; got != want {
				t.Errorf("%s: dst (%d,%d) = %08x want src (%d,%d) %08x", tc.name, p.dx, p.dy, got, p.sx, p.sy, want)
			}
		}
	}
}

func TestDrawImageNineGeometry(t *testing.T) {
	// 6x6 image: 2px border of red, 2x2 green center.
	info, _ := imagecore.MakeInfo(6, 6, imagecore.ColorTypeRGBA8888, imagecore.AlphaTypeOpaque)
	p := imagecore.NewPixels(info)
	for y := int32(0); y < 6; y++ {
		for x := int32(0); x < 6; x++ {
			col := uint32(0xFF0000FF) // red
			if x >= 2 && x < 4 && y >= 2 && y < 4 {
				col = 0xFF00FF00 // green
			}
			p.Words[y*6+x] = col
		}
	}
	img := imagecore.FromPixels(p)

	pix := raster.NewPixmap(16, 16)
	c := NewForPixmap(pix)
	c.DrawImageNine(img, geom.IRect{Left: 2, Top: 2, Right: 4, Bottom: 4}, geom.RectWH(16, 16),
		shaders.FilterNearest, nil)

	// Corners keep the 2px red border; the stretched center is green.
	if pix.Pix[0] != 0xFF0000FF || pix.Pix[1*16+1] != 0xFF0000FF {
		t.Fatalf("corner = %08x %08x", pix.Pix[0], pix.Pix[17])
	}
	if pix.Pix[8*16+8] != 0xFF00FF00 {
		t.Fatalf("center = %08x", pix.Pix[8*16+8])
	}
	if pix.Pix[15*16+15] != 0xFF0000FF {
		t.Fatalf("far corner = %08x", pix.Pix[15*16+15])
	}
	// Edge midpoints: top edge (stretched horizontally) stays red.
	if pix.Pix[0*16+8] != 0xFF0000FF {
		t.Fatalf("top edge = %08x", pix.Pix[8])
	}
}

func TestDrawImageNineInvalidCenterFallsBack(t *testing.T) {
	img := gradientImage(4, 4)
	pix := raster.NewPixmap(8, 8)
	c := NewForPixmap(pix)
	// Empty center: invalid lattice → drawImageRect of the whole image.
	c.DrawImageNine(img, geom.IRect{Left: 2, Top: 2, Right: 2, Bottom: 2}, geom.RectWH(8, 8),
		shaders.FilterNearest, nil)
	src := img.PeekPixels(imagecore.CachingAllow)
	if pix.Pix[1*8+1] != src.Words[0] {
		t.Fatalf("fallback draw wrong: %08x want %08x", pix.Pix[9], src.Words[0])
	}
}

func TestSaveLayerRestoreWithColorFilter(t *testing.T) {
	// A saveLayer restore paint carrying a color filter must filter the layer contents (the drawSprite shader lane).
	pix := raster.NewPixmap(4, 4)
	c := NewForPixmap(pix)

	restorePaint := NewPaint()
	// SrcIn against green: output = green scaled by source alpha.
	restorePaint.ColorFilter = colorfilter.NewBlend(0xFF00FF00, raster.BlendSrcIn)

	c.SaveLayer(nil, restorePaint)
	red := NewPaint()
	red.Color = 0xFFFF0000
	c.DrawRect(geom.RectWH(4, 4), red)
	c.Restore()

	if got := pix.Pix[5]; got != 0xFF00FF00 {
		t.Fatalf("filtered restore = %08x want FF00FF00 (device word B<<16|G<<8|R)", got)
	}
}

func TestDrawImageRotatedUsesShaderLane(t *testing.T) {
	img := gradientImage(4, 4)
	pix := raster.NewPixmap(12, 12)
	c := NewForPixmap(pix)
	c.Translate(6, 2)
	c.Rotate(45)
	paint := NewPaint()
	paint.AntiAlias = true
	c.DrawImage(img, 0, 0, shaders.SamplingOptions{Filter: shaders.FilterLinear}, paint)
	// Just verify pixels landed and the unrotated corner stayed clear.
	found := false
	for _, v := range pix.Pix {
		if v != 0 {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("rotated draw produced no pixels")
	}
	if pix.Pix[11*12] != 0 {
		t.Fatal("bottom-left corner should be untouched")
	}
}

func TestDrawImageColorFilteredSpriteFallsToShader(t *testing.T) {
	// A color filter forces the sprite lane to fall through to the shader path, which folds the filter into the shader.
	img := gradientImage(2, 2)
	pix := raster.NewPixmap(4, 4)
	c := NewForPixmap(pix)
	paint := NewPaint()
	paint.ColorFilter = colorfilter.NewBlend(0xFFFFFFFF, raster.BlendSrcIn) // white by src alpha
	c.DrawImage(img, 0, 0, shaders.SamplingOptions{}, paint)
	if got := pix.Pix[0]; got != 0xFFFFFFFF {
		t.Fatalf("filtered image draw = %08x want FFFFFFFF", got)
	}
}

// TestDrawImageNineInvalidCenterKeepsCallerPaint: the invalid-lattice fallback is a plain whole-image draw, not a set
// of abutting lattice patches, so it must not run the paint through cleanPaintForLattice. That cleaning drops the mask
// filter and forces AntiAlias off, which on a rotated fallback draw is the difference between a smoothed and a jagged
// edge — a silent quality loss for a paint the caller never asked to have stripped.
func TestDrawImageNineInvalidCenterKeepsCallerPaint(t *testing.T) {
	img := gradientImage(4, 4)
	// An empty center rect makes the lattice invalid, forcing the fallback.
	invalidCenter := geom.IRect{Left: 2, Top: 2, Right: 2, Bottom: 2}
	validCenter := geom.IRect{Left: 1, Top: 1, Right: 3, Bottom: 3}

	drawNine := func(center geom.IRect, aa bool, mf maskfilter.MaskFilter) *raster.Pixmap {
		pix := raster.NewPixmap(24, 24)
		c := NewForPixmap(pix)
		c.Translate(12, 12)
		c.Rotate(20) // a rotated draw: the image edges no longer land on pixel boundaries
		c.Translate(-8, -8)
		p := NewPaint()
		p.AntiAlias = aa
		p.MaskFilter = mf
		c.DrawImageNine(img, center, geom.RectWH(16, 16), shaders.FilterNearest, p)
		return pix
	}

	aaPix := drawNine(invalidCenter, true, nil)
	bwPix := drawNine(invalidCenter, false, nil)
	if equalPixmaps(aaPix, bwPix) {
		t.Error("the invalid-lattice fallback ignored the paint's AntiAlias flag")
	}
	// Antialiasing shows up as edge pixels that are neither fully covered nor untouched.
	partial := 0
	for i := range aaPix.Pix {
		if aaPix.Pix[i] != bwPix.Pix[i] && aaPix.Pix[i] != 0 {
			partial++
		}
	}
	if partial == 0 {
		t.Error("the antialiased fallback produced no partially covered edge pixels")
	}

	// The lattice path proper still cleans the paint of its mask filter and AntiAlias: those patches abut, and AA on
	// their shared edges seams.
	blur := maskfilter.NewBlur(maskfilter.BlurNormal, 2, true)
	if !equalPixmaps(drawNine(validCenter, true, blur), drawNine(validCenter, false, nil)) {
		t.Error("the lattice patch draws must still be cleaned of the mask filter and AntiAlias")
	}
}

// unpremulRowImage builds a 1-row straight-alpha image: one color repeated at each of the given alphas.
func unpremulRowImage(t *testing.T, alphas []uint8) *imagecore.Image {
	t.Helper()
	info, ok := imagecore.MakeInfo(int32(len(alphas)), 1, imagecore.ColorTypeRGBA8888, imagecore.AlphaTypeUnpremul)
	if !ok || info.AlphaType != imagecore.AlphaTypeUnpremul {
		t.Fatal("straight-alpha RGBA8888 no longer forms a valid image info")
	}
	p := imagecore.NewPixels(info)
	for i, a := range alphas {
		p.Words[i] = 16 | 8<<8 | 96<<16 | uint32(a)<<24 // color (16,8,96), straight alpha
	}
	return imagecore.FromPixels(p)
}

// TestDrawImageUnpremulSpriteMatchesShaderLane pins the two image-draw lanes to the same answer for a straight-alpha
// source. The sprite lane used to blit such a source as if it were already premultiplied, which composited
// dst = src + (1-a)*dst over the unmultiplied bytes: a fully transparent pixel added its whole color to the backdrop.
// Which lane runs depends only on whether the transform lands on integer pixel bounds, so the two must agree.
func TestDrawImageUnpremulSpriteMatchesShaderLane(t *testing.T) {
	alphas := []uint8{0, 64, 128, 255}
	img := unpremulRowImage(t, alphas)
	const bg = 229 | 229<<8 | 127<<16 | 255<<24 // opaque background (229,229,127)

	drawRow := func(scaleX float32) []uint32 {
		pix := raster.NewPixmap(6, 1)
		for i := range pix.Pix {
			pix.Pix[i] = bg
		}
		c := NewForPixmap(pix)
		count := c.Save()
		var m geom.Matrix
		m.SetAll(scaleX, 0, 0, 0, 1, 0, 0, 0, 1)
		c.Concat(&m)
		c.DrawImageRect(img, geom.RectWH(4, 1), geom.RectWH(4, 1),
			shaders.SamplingOptions{Filter: shaders.FilterNearest}, NewPaint(), ConstraintFast)
		c.RestoreToCount(count)
		return append([]uint32(nil), pix.Pix[:len(alphas)]...)
	}

	// Scale 1 lands on integer pixel bounds, so the draw takes the sprite lane; scale 1.125 does not, so it takes the
	// shader lane, while nearest filtering still maps device columns 0..3 to source samples 0..3.
	sampling := shaders.SamplingOptions{Filter: shaders.FilterNearest}
	var spriteMatrix, shaderMatrix geom.Matrix
	spriteMatrix.SetAll(1, 0, 0, 0, 1, 0, 0, 0, 1)
	shaderMatrix.SetAll(1.125, 0, 0, 0, 1, 0, 0, 0, 1)
	if !treatAsSprite(&spriteMatrix, int32(len(alphas)), 1, sampling, false) {
		t.Fatal("the unscaled draw no longer takes the sprite lane; the test compares nothing")
	}
	if treatAsSprite(&shaderMatrix, int32(len(alphas)), 1, sampling, false) {
		t.Fatal("the 1.125-scaled draw now takes the sprite lane; the test compares nothing")
	}

	sprite := drawRow(1)
	shader := drawRow(1.125)
	for i, a := range alphas {
		if sprite[i] != shader[i] {
			t.Errorf("alpha %d: sprite lane %08x, shader lane %08x", a, sprite[i], shader[i])
		}
	}
	// The fully transparent pixel must leave the backdrop exactly as it was, and the opaque one must land on the
	// source color itself.
	if sprite[0] != bg {
		t.Errorf("transparent pixel = %08x, want the untouched background %08x", sprite[0], bg)
	}
	if want := uint32(16 | 8<<8 | 96<<16 | 255<<24); sprite[3] != want {
		t.Errorf("opaque pixel = %08x, want %08x", sprite[3], want)
	}
}
