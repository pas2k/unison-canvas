// Copyright (c) 2026 by Richard A. Wilkes. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, version 2.0. If a copy of the MPL was not distributed with
// this file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// This Source Code Form is "Incompatible With Secondary Licenses", as
// defined by the Mozilla Public License, version 2.0.

// Live-context tests for the general filter-mask thread-safe cache. A mask-filtered shape's filtered A8 mask is
// memoized under a unique key so a repeated draw reuses the upload. A Solid-style blur is used so the draw skips the
// analytic direct-filter-mask rect lane and takes the general filter-mask path. Skips when no GL context is available.

package gl_test

import (
	"testing"

	"github.com/richardwilkes/canvas/maskfilter"
)

// TestLiveMaskFilterCacheDistinctSigmas verifies distinct blur parameters key distinct entries: two draws that differ
// only in sigma each add their own entry rather than colliding.
func TestLiveMaskFilterCacheDistinctSigmas(t *testing.T) {
	_, dc := newLiveDirectContext(t)
	cache := dc.ThreadSafeCache()

	base := cache.NumEntries()
	drawBlurredRect(t, dc, 64, 22, 22, 42, 42, maskfilter.BlurSolid, 3)
	drawBlurredRect(t, dc, 64, 22, 22, 42, 42, maskfilter.BlurSolid, 4)
	if got := cache.NumEntries() - base; got != 2 {
		t.Fatalf("expected 2 distinct cache entries for two sigmas, got %d", got)
	}
}
