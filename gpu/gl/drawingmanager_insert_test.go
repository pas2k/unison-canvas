// Copyright (c) 2026 by Richard A. Wilkes. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, version 2.0. If a copy of the MPL was not distributed with
// this file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// This Source Code Form is "Incompatible With Secondary Licenses", as
// defined by the Mozilla Public License, version 2.0.

// Regression tests for insertTaskBeforeLast's reorder-blocker bookkeeping. sortTasks partitions the DAG into
// topo-sorted spans at the indices in reorderBlockerTaskIndices. When insertTaskBeforeLast swaps the new task in ahead
// of a last task that is itself a reorder blocker, the blocker's recorded index must follow it from len(dag)-1 to
// len(dag), so the predicate that bumps it has to compare against len(dm.dag)-1. Comparing against len(dm.dag), as
// upstream Skia's GrDrawingManager::insertTaskBeforeLast does, can never fire (a recorded index never exceeds
// len(dag)-1), leaving the index stale and mis-partitioning the spans. The tests use stub tasks, so they need no GL
// context.

package gl

import (
	"slices"
	"testing"

	"github.com/richardwilkes/canvas/geom"
)

// stubRenderTask is a minimal RenderTask for exercising the drawing-manager DAG bookkeeping.
type stubRenderTask struct {
	RenderTaskBase
}

func newStubRenderTask(blocksReordering bool) *stubRenderTask {
	t := &stubRenderTask{}
	t.initRenderTask(t)
	if blocksReordering {
		t.setFlag(taskFlagBlocksReordering)
	}
	return t
}

func (t *stubRenderTask) Name() string { return "Stub" }
func (t *stubRenderTask) OnMakeClosed(*geom.IRect) ExpectedOutcome {
	return ExpectedOutcomeTargetUnchanged
}
func (t *stubRenderTask) OnIsUsed(*SurfaceProxy) bool             { return false }
func (t *stubRenderTask) GatherProxyIntervals(*ResourceAllocator) {}
func (t *stubRenderTask) OnExecute(*OpFlushState) bool            { return true }

// blockerInvariantHolds reports whether every recorded blocker index points at a task that actually blocks reordering,
// which is the invariant sortTasks depends on to place span boundaries correctly.
func blockerInvariantHolds(dm *DrawingManager) bool {
	for _, idx := range dm.reorderBlockerTaskIndices {
		if idx < 0 || idx >= len(dm.dag) || !dm.dag[idx].taskBase().BlocksReordering() {
			return false
		}
	}
	return true
}

// TestInsertTaskBeforeLastKeepsBlockerIndexOnBlocker verifies that the recorded blocker index keeps pointing at the
// blocker (not at the freshly inserted task): it advances when the blocker is the last task and, so the predicate does
// not over-fire, stays put when an earlier task is the blocker.
func TestInsertTaskBeforeLastKeepsBlockerIndexOnBlocker(t *testing.T) {
	for _, tc := range []struct {
		name         string
		blockerFirst bool // dag=[blocker, other] before the insert, rather than [other, blocker]
		wantIndex    int  // the blocker's recorded index after the insert
	}{
		{name: "trailing-blocker-advances", wantIndex: 2},
		{name: "earlier-blocker-stays", blockerFirst: true, wantIndex: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dm := &DrawingManager{}
			other := newStubRenderTask(false)
			blocker := newStubRenderTask(true)
			if tc.blockerFirst {
				dm.appendTask(blocker)
				dm.appendTask(other)
			} else {
				dm.appendTask(other)
				dm.appendTask(blocker)
			}

			inserted := newStubRenderTask(false)
			dm.insertTaskBeforeLast(inserted)

			if got := len(dm.dag); got != 3 {
				t.Fatalf("dag length = %d, want 3", got)
			}
			if dm.dag[tc.wantIndex] != RenderTask(blocker) {
				t.Errorf("blocker not at dag[%d] after insert; it holds %p, blocker is %p", tc.wantIndex,
					dm.dag[tc.wantIndex], blocker)
			}
			if dm.dag[1] != RenderTask(inserted) {
				t.Errorf("inserted task not placed before the last task; dag[1] is %p, inserted is %p", dm.dag[1],
					inserted)
			}
			if n := len(dm.reorderBlockerTaskIndices); n != 1 || dm.reorderBlockerTaskIndices[0] != tc.wantIndex {
				t.Errorf("reorderBlockerTaskIndices = %v, want [%d]", dm.reorderBlockerTaskIndices, tc.wantIndex)
			}
			if !blockerInvariantHolds(dm) {
				t.Errorf("blocker index no longer points at a reorder-blocking task: %v",
					dm.reorderBlockerTaskIndices)
			}
			// sortTasks partitions on the blocker index; a stale index would place the boundary on the wrong task. It
			// must at least not panic here, and the span before the blocker must keep its (dependency-free) order.
			span := slices.Clone(dm.dag[:tc.wantIndex])
			dm.sortTasks()
			for i, task := range span {
				if got := task.taskBase().index; got != uint32(i) {
					t.Errorf("span before the blocker sorted wrong: task %d has index %d", i, got)
				}
			}
		})
	}
}
