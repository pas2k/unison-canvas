// Copyright (c) 2026 by Richard A. Wilkes. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, version 2.0. If a copy of the MPL was not distributed with
// this file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// This Source Code Form is "Incompatible With Secondary Licenses", as
// defined by the Mozilla Public License, version 2.0.

// Live-context tests for GPU images & surfaces: TextureFromImage upload + readback round trips, MakeNonTextureImage,
// the RenderTargetSurface snapshot/compatible-surface/readback lanes, and the snapshot-is-frozen (copy-on-snapshot)
// guarantee. Skips when no GL context is available.

package gl_test

import (
	"testing"

	"github.com/richardwilkes/canvas/colorcore"
	"github.com/richardwilkes/canvas/geom"
	"github.com/richardwilkes/canvas/gpu"
	"github.com/richardwilkes/canvas/gpu/gl"
	"github.com/richardwilkes/canvas/imagecore"
)

func rgba8888Premul(w, h int32) imagecore.ImageInfo {
	info, _ := imagecore.MakeInfo(w, h, imagecore.ColorTypeRGBA8888, imagecore.AlphaTypePremul)
	return info
}

// sourceRGBA reads an imagecore image out as RGBA8888-premul bytes (the round-trip reference).
func sourceRGBA(t *testing.T, img *imagecore.Image) []byte {
	t.Helper()
	buf := make([]byte, int(img.Width())*int(img.Height())*4)
	if !img.ReadPixels(rgba8888Premul(img.Width(), img.Height()), buf, int(img.Width())*4, 0, 0,
		imagecore.CachingAllow) {
		t.Fatal("source ReadPixels failed")
	}
	return buf
}

func assertBytesEqual(t *testing.T, got, want []byte, tol int, tag string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: length %d != %d", tag, len(got), len(want))
	}
	for i := range got {
		d := int(got[i]) - int(want[i])
		if d < -tol || d > tol {
			t.Fatalf("%s: byte %d = %d, want %d (tol %d)", tag, i, got[i], want[i], tol)
		}
	}
}

func assertUniformRGBA(t *testing.T, data []byte, want [4]byte, tol int, tag string) {
	t.Helper()
	for off := 0; off+4 <= len(data); off += 4 {
		for c := 0; c < 4; c++ {
			d := int(data[off+c]) - int(want[c])
			if d < -tol || d > tol {
				t.Fatalf("%s: pixel byte %d = %d, want %d", tag, off+c, data[off+c], want[c])
			}
		}
	}
}

func readTexImageRGBA(t *testing.T, tex *gl.TextureImage) []byte {
	t.Helper()
	buf := make([]byte, int(tex.Width())*int(tex.Height())*4)
	if !tex.ReadPixels(rgba8888Premul(tex.Width(), tex.Height()), buf, int(tex.Width())*4, 0, 0,
		imagecore.CachingAllow) {
		t.Fatal("TextureImage.ReadPixels failed")
	}
	return buf
}

func readSurfaceRGBA(t *testing.T, s *gl.RenderTargetSurface) []byte {
	t.Helper()
	buf := make([]byte, int(s.Width())*int(s.Height())*4)
	if !s.ReadPixels(rgba8888Premul(s.Width(), s.Height()), buf, int(s.Width())*4, 0, 0) {
		t.Fatal("RenderTargetSurface.ReadPixels failed")
	}
	return buf
}

func TestLiveTextureFromImageReadback(t *testing.T) {
	_, dc := newLiveDirectContext(t)

	if gl.TextureFromImage(dc, nil, gpu.MipmappedNo, gpu.BudgetedYes) != nil {
		t.Error("TextureFromImage(nil image) should be nil")
	}

	checker := newCheckerImage(t) // 4x4 red/white non-uniform pattern (orientation-sensitive)
	alphaInfo, _ := imagecore.MakeInfo(4, 4, imagecore.ColorTypeAlpha8, imagecore.AlphaTypePremul)
	alphaData := make([]byte, 16)
	for i := range alphaData {
		alphaData[i] = byte(i * 15)
	}
	alpha := imagecore.NewRasterData(alphaInfo, alphaData, 4)
	if alpha == nil {
		t.Fatal("alpha image creation failed")
	}

	for _, tc := range []struct {
		img       *imagecore.Image
		name      string
		want      []byte              // the exact readback in info's format
		info      imagecore.ImageInfo // the readback format
		colorType gpu.ColorType
		alphaOnly bool
	}{
		{
			name:      "rgba",
			img:       checker,
			info:      rgba8888Premul(4, 4),
			want:      sourceRGBA(t, checker),
			colorType: gpu.ColorTypeRGBA8888,
		},
		{
			name:      "alpha8",
			img:       alpha,
			info:      alphaInfo,
			want:      alphaData,
			colorType: gpu.ColorTypeAlpha8,
			alphaOnly: true,
		},
	} {
		tex := gl.TextureFromImage(dc, tc.img, gpu.MipmappedNo, gpu.BudgetedYes)
		if tex == nil {
			t.Fatalf("%s: TextureFromImage returned nil", tc.name)
		}
		if tex.Width() != 4 || tex.Height() != 4 {
			t.Fatalf("%s: dims = %dx%d, want 4x4", tc.name, tex.Width(), tex.Height())
		}
		if !tex.IsTextureBacked() || tex.IsAlphaOnly() != tc.alphaOnly {
			t.Fatalf("%s: texture-backed = %v, alpha-only = %v, want true and %v", tc.name, tex.IsTextureBacked(),
				tex.IsAlphaOnly(), tc.alphaOnly)
		}
		if tex.ColorType() != tc.colorType {
			t.Fatalf("%s: color type = %v, want %v", tc.name, tex.ColorType(), tc.colorType)
		}

		// Round trip: source -> texture -> readback must be byte-exact (top-left, no flip).
		got := make([]byte, len(tc.want))
		if !tex.ReadPixels(tc.info, got, tc.info.MinRowBytes(), 0, 0, imagecore.CachingAllow) {
			t.Fatalf("%s: ReadPixels failed", tc.name)
		}
		assertBytesEqual(t, got, tc.want, 0, tc.name+" texture readback")

		// MakeNonTextureImage reads back into a CPU image; its pixels must match the source too.
		nonTex := tex.MakeNonTextureImage()
		if nonTex == nil {
			t.Fatalf("%s: MakeNonTextureImage returned nil", tc.name)
		}
		got = make([]byte, len(tc.want))
		if !nonTex.ReadPixels(tc.info, got, tc.info.MinRowBytes(), 0, 0, imagecore.CachingAllow) {
			t.Fatalf("%s: non-texture image ReadPixels failed", tc.name)
		}
		assertBytesEqual(t, got, tc.want, 0, tc.name+" non-texture image")
		tex.Release()
	}
}

func TestLiveRenderTargetSurfaceSnapshot(t *testing.T) {
	_, dc := newLiveDirectContext(t)

	const w, h = 16, 16
	s := gl.MakeRenderTargetSurface(dc, gpu.ColorTypeRGBA8888, geom.ISize{Width: w, Height: h}, 1,
		gpu.MipmappedNo, gpu.OriginTopLeft, gpu.BudgetedYes, "snap-test", nil)
	if s == nil {
		t.Fatal("MakeRenderTargetSurface returned nil")
	}
	if s.Width() != w || s.Height() != h {
		t.Fatalf("surface dims = %dx%d", s.Width(), s.Height())
	}

	red := [4]byte{255, 0, 0, 255}
	blue := [4]byte{0, 0, 255, 255}

	// Snapshot the red content, then draw blue over the surface with no intervening read of the snapshot — the snapshot
	// must still be frozen at red (the copy is baked at snapshot time, independent of later draw/read ordering).
	s.Canvas().Clear(colorcore.Red)
	snap1 := s.MakeImageSnapshot()
	if snap1 == nil {
		t.Fatal("MakeImageSnapshot returned nil")
	}
	defer snap1.Release()

	s.Canvas().Clear(colorcore.Blue)
	assertUniformRGBA(t, readSurfaceRGBA(t, s), blue, 1, "surface after blue clear")
	assertUniformRGBA(t, readTexImageRGBA(t, snap1), red, 1, "snapshot1 stays frozen at red")

	snap2 := s.MakeImageSnapshot()
	if snap2 == nil {
		t.Fatal("second MakeImageSnapshot returned nil")
	}
	defer snap2.Release()
	assertUniformRGBA(t, readTexImageRGBA(t, snap2), blue, 1, "snapshot2")
}

func TestLiveRenderTargetSurfaceMakeSurface(t *testing.T) {
	_, dc := newLiveDirectContext(t)

	s := gl.MakeRenderTargetSurface(dc, gpu.ColorTypeRGBA8888, geom.ISize{Width: 16, Height: 16}, 1,
		gpu.MipmappedNo, gpu.OriginTopLeft, gpu.BudgetedYes, "compat-parent", nil)
	if s == nil {
		t.Fatal("parent surface nil")
	}
	s.Canvas().Clear(colorcore.Red)

	s2 := s.MakeSurface(8, 8)
	if s2 == nil {
		t.Fatal("MakeSurface returned nil")
	}
	if s2.Width() != 8 || s2.Height() != 8 {
		t.Fatalf("compatible surface dims = %dx%d, want 8x8", s2.Width(), s2.Height())
	}
	s2.Canvas().Clear(colorcore.Green)

	green := [4]byte{0, 255, 0, 255}
	red := [4]byte{255, 0, 0, 255}
	assertUniformRGBA(t, readSurfaceRGBA(t, s2), green, 1, "compatible surface")
	// The compatible surface is independent; the parent is untouched.
	assertUniformRGBA(t, readSurfaceRGBA(t, s), red, 1, "parent after compatible surface drawn")
}

func TestLiveRenderTargetSurfaceReadPixelsConvert(t *testing.T) {
	_, dc := newLiveDirectContext(t)

	s := gl.MakeRenderTargetSurface(dc, gpu.ColorTypeRGBA8888, geom.ISize{Width: 8, Height: 8}, 1,
		gpu.MipmappedNo, gpu.OriginTopLeft, gpu.BudgetedYes, "convert-test", nil)
	if s == nil {
		t.Fatal("surface nil")
	}
	s.Canvas().Clear(colorcore.Red)

	// Read back into BGRA8888: RGBA red [255,0,0,255] becomes BGRA [0,0,255,255].
	bgra, _ := imagecore.MakeInfo(8, 8, imagecore.ColorTypeBGRA8888, imagecore.AlphaTypePremul)
	buf := make([]byte, 8*8*4)
	if !s.ReadPixels(bgra, buf, 8*4, 0, 0) {
		t.Fatal("BGRA ReadPixels failed")
	}
	assertUniformRGBA(t, buf, [4]byte{0, 0, 255, 255}, 1, "BGRA readback")
}
