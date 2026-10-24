// Copyright (c) 2026 by Richard A. Wilkes. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, version 2.0. If a copy of the MPL was not distributed with
// this file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// This Source Code Form is "Incompatible With Secondary Licenses", as
// defined by the Mozilla Public License, version 2.0.

// Unit tests for the SDF strike lane: MakeSDFTMaskSpec produces MaskSDF glyphs whose bounds are the A8 bounds outset by
// DistanceFieldPad, the ActionSDFT digest action accepts exactly the SDF-format in-atlas glyphs (strike_test.go's
// TestAtlasActionSizeGates covers the atlas-size half), and the generated image is a distance field consistent with the
// A8 coverage (inside > 128 where coverage is high, far pad below threshold).

package font

import (
	"os"
	"testing"

	"github.com/richardwilkes/canvas/stroke"
)

func loadSDFTestFont(t *testing.T, size float32) *Font {
	t.Helper()
	data, err := os.ReadFile("testdata/Roboto-Regular.ttf")
	if err != nil {
		t.Fatal(err)
	}
	tf, err := NewTypefaceFromData(data, 0)
	if err != nil {
		t.Fatal(err)
	}
	return NewFont(tf, size, 1, 0)
}

// sdfGlyphID returns a glyph ID with a real outline ('A').
func sdfGlyphID(t *testing.T, f *Font) uint16 {
	t.Helper()
	var glyphs [1]uint16
	if f.TextToGlyphs([]byte("A"), TextEncodingUTF8, glyphs[:]) != 1 || glyphs[0] == 0 {
		t.Fatal("no glyph for 'A'")
	}
	return glyphs[0]
}

// TestSDFTMaskSpecGlyph pins MaskSDF as a stored, handled rec format, for a filled glyph and for a stroked one. A
// stroked paint sets FrameWidth, which routes makeGlyph through internalGetPath + generateMetricsFromPath, the lane
// where upstream normalizes anything that isn't BW/A8/LCD16 back to A8 (see scalercontext.go's header). This port
// deliberately omits that normalization, so either way the glyph must come out MaskSDF, carry the DistanceFieldPad
// outset over the same A8 glyph, and render through the distance-field lane rather than as a plain coverage mask.
func TestSDFTMaskSpecGlyph(t *testing.T) {
	// The SDF strike font is what gpu/text's GetSDFFont produces: EdgingAntiAlias, no subpixel; use the DF mask size
	// directly (162, the large bucket).
	f := loadSDFTestFont(t, 162)
	f.SetEdging(EdgingAntiAlias)
	f.SetSubpixel(false)
	gid := sdfGlyphID(t, f)
	// Failing to resolve the space is fatal rather than a skip: a cmap lane regressing to glyph 0 is itself a bug, and
	// guarding the drop check below on the lookup would lose the check exactly then.
	var space [1]uint16
	if f.TextToGlyphs([]byte(" "), TextEncodingUTF8, space[:]) != 1 || space[0] == 0 {
		t.Fatal("no glyph for the space character")
	}

	for _, c := range []struct {
		paint *ScalerPaint
		name  string
	}{
		{name: "fill"},
		{name: "stroke", paint: &ScalerPaint{Style: stroke.PaintStyleStroke, Width: 3, MiterLimit: 4}},
	} {
		t.Run(c.name, func(t *testing.T) {
			sdfSpec := MakeSDFTMaskSpec(f, c.paint)
			if sdfSpec.Rec.Format != MaskSDF {
				t.Fatalf("SDF spec format = %v, want MaskSDF", sdfSpec.Rec.Format)
			}
			a8Spec := MakeWithNoDeviceSpec(f, c.paint)
			if a8Spec.Rec.Format != MaskA8 {
				t.Fatalf("A8 spec format = %v, want MaskA8", a8Spec.Rec.Format)
			}
			if c.paint != nil && (sdfSpec.Rec.FrameWidth != c.paint.Width || a8Spec.Rec.FrameWidth != c.paint.Width) {
				t.Fatalf("SDF/A8 rec FrameWidth = %v/%v, want %v (the path-generated-metrics lane)",
					sdfSpec.Rec.FrameWidth, a8Spec.Rec.FrameWidth, c.paint.Width)
			}
			// The SDF rec is a distinct strike key: same typeface/size but a different format.
			if sdfSpec.Rec == a8Spec.Rec {
				t.Fatal("SDF and A8 recs must differ (distinct strikes)")
			}

			sdfStrike := sdfSpec.FindOrCreateStrike()
			a8Strike := a8Spec.FindOrCreateStrike()

			sdfGlyph, action := sdfStrike.DigestFor(ActionSDFT, PackGlyphID(gid))
			if action != GlyphActionAccept {
				t.Fatalf("kSDFT action = %v, want accept", action)
			}
			if sdfGlyph.Format != MaskSDF {
				t.Fatalf("glyph format = %v, want MaskSDF (the path lane must not normalize it to A8)", sdfGlyph.Format)
			}
			a8Glyph, action := a8Strike.DigestFor(ActionDirectMask, PackGlyphID(gid))
			if action != GlyphActionAccept {
				t.Fatalf("A8 direct action = %v, want accept", action)
			}

			// Bounds: the A8 bounds outset by DistanceFieldPad on each side.
			if sdfGlyph.Left != a8Glyph.Left-DistanceFieldPad ||
				sdfGlyph.Top != a8Glyph.Top-DistanceFieldPad ||
				sdfGlyph.Width != a8Glyph.Width+2*DistanceFieldPad ||
				sdfGlyph.Height != a8Glyph.Height+2*DistanceFieldPad {
				t.Errorf("SDF bounds L%d T%d W%d H%d vs A8 L%d T%d W%d H%d: want pad %d",
					sdfGlyph.Left, sdfGlyph.Top, sdfGlyph.Width, sdfGlyph.Height,
					a8Glyph.Left, a8Glyph.Top, a8Glyph.Width, a8Glyph.Height, DistanceFieldPad)
			}

			// ActionSDFT accepts only SDF-format glyphs, and drops one that rasterizes to nothing rather than letting
			// it reach the atlas.
			if _, action = a8Strike.DigestFor(ActionSDFT, PackGlyphID(gid)); action != GlyphActionReject {
				t.Errorf("A8 glyph kSDFT action = %v, want reject", action)
			}
			if _, action = sdfStrike.DigestFor(ActionSDFT, PackGlyphID(space[0])); action != GlyphActionDrop {
				t.Errorf("space kSDFT action = %v, want drop", action)
			}

			// The image is a distance field: the pad corner is far outside (0) and the strongest-coverage texel of the
			// A8 mask reads inside (> 128) at the corresponding padded position.
			sdfGlyph = sdfStrike.PrepareImage(PackGlyphID(gid))
			if !sdfGlyph.HasImage() {
				t.Fatal("SDF glyph has no image")
			}
			if got := sdfGlyph.Image[0]; got != 0 {
				t.Errorf("pad corner = %d, want 0", got)
			}
			a8Glyph = a8Strike.PrepareImage(PackGlyphID(gid))
			bestX, bestY, bestV := 0, 0, uint8(0)
			for y := 0; y < int(a8Glyph.Height); y++ {
				for x := 0; x < int(a8Glyph.Width); x++ {
					if v := a8Glyph.Image[y*int(a8Glyph.Width)+x]; v > bestV {
						bestV = v
						bestX, bestY = x, y
					}
				}
			}
			if bestV < 200 {
				t.Fatalf("A8 mask has no strong texel (max %d)", bestV)
			}
			sdfW := int(sdfGlyph.Width)
			inside := sdfGlyph.Image[(bestY+DistanceFieldPad)*sdfW+bestX+DistanceFieldPad]
			if inside <= 128 {
				t.Errorf("SDF at strongest A8 texel = %d, want > 128 (inside)", inside)
			}
		})
	}
}
