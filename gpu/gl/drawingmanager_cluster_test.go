// Copyright (c) 2026 by Richard A. Wilkes. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, version 2.0. If a copy of the MPL was not distributed with
// this file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// This Source Code Form is "Incompatible With Secondary Licenses", as
// defined by the Mozilla Public License, version 2.0.

// Regression tests for taskClusterVisit's multi/zero-target barrier handling. A task without exactly one target is a
// full barrier for each of its targets: the clustering pass must forget the cluster tail of every one of them (deleting
// lastTaskMap[base.Target(j)] for each j, not lastTaskMap[base.Target(0)] repeatedly) so no later same-target task is
// reordered across the barrier. The tests use stub tasks, so they need no GL context.

package gl

import (
	"slices"
	"testing"
)

// newStubWithTargets builds a non-blocking stub render task with the given targets, setting the field directly (rather
// than via addTarget) so the test needs no DrawingManager or ref-counting.
func newStubWithTargets(targets ...*SurfaceProxy) *stubRenderTask {
	t := newStubRenderTask(false)
	t.targets = append(t.targets, targets...)
	return t
}

func llistOrder(l *taskLList) []RenderTask {
	var out []RenderTask
	for t := l.head; t != nil; t = t.taskBase().llNext {
		out = append(out, t)
	}
	return out
}

// TestTaskClusterVisitBarrierClearsExactlyItsTargets verifies that visiting a task without exactly one target clears
// the cluster tail recorded for every one of its targets, not just the first, and leaves other cluster tails alone.
func TestTaskClusterVisitBarrierClearsExactlyItsTargets(t *testing.T) {
	a := &SurfaceProxy{}
	b := &SurfaceProxy{}
	for _, tc := range []struct {
		name    string
		primed  []*SurfaceProxy // proxies given a recorded cluster tail before the visit
		targets []*SurfaceProxy // the barrier's targets
	}{
		{name: "multi-target", primed: []*SurfaceProxy{a, b}, targets: []*SurfaceProxy{a, b}},
		{name: "zero-target", primed: []*SurfaceProxy{a}},
	} {
		lastTaskMap := make(map[*SurfaceProxy]RenderTask)
		for _, p := range tc.primed {
			lastTaskMap[p] = newStubWithTargets(p)
		}
		var llist taskLList
		if reordered := taskClusterVisit(newStubWithTargets(tc.targets...), &llist, lastTaskMap); reordered {
			t.Errorf("%s: barrier must not report reordering, got true", tc.name)
		}
		for i, p := range tc.primed {
			_, ok := lastTaskMap[p]
			if isTarget := slices.Contains(tc.targets, p); ok == isTarget {
				t.Errorf("%s: primed proxy %d's cluster tail present = %v, want %v", tc.name, i, ok, !isTarget)
			}
		}
	}
}

// TestClusterRenderTasksStillClustersWhenSafe guards against over-correction: a middle task whose target differs from a
// surrounding same-target pair must still be pulled aside so the pair clusters.
func TestClusterRenderTasksStillClustersWhenSafe(t *testing.T) {
	a := &SurfaceProxy{}
	c := &SurfaceProxy{}
	t1 := newStubWithTargets(a)
	t2 := newStubWithTargets(c)
	t3 := newStubWithTargets(a)

	var llist taskLList
	if !clusterRenderTasks([]RenderTask{t1, t2, t3}, &llist) {
		t.Fatalf("expected clustering to reorder the differing middle task")
	}
	got := llistOrder(&llist)
	want := []RenderTask{t2, t1, t3}
	if len(got) != len(want) {
		t.Fatalf("cluster produced %d tasks, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cluster order[%d] = %p, want %p", i, got[i], want[i])
		}
	}
}
