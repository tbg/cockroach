// Copyright 2020 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package tracing

import "github.com/cockroachdb/cockroach/pkg/util/syncutil"

type id struct {
	traceID uint64
	spanID  uint64
}

// node is implemented by *Span and by *Recording.
// nb: might save allocations to make this a tuple struct instead.
// That way we don't need to force Recording on the heap.
type node interface {
	ID() id
	GetRecording() Recording
	AddChild(node)
	Children() []node
}

type storage struct {
	mu    syncutil.Mutex
	roots map[id]node
}

func (s *storage) add(parentOrNil, child node) {
	id := child.ID()
	s.mu.Lock()
	if parentOrNil == nil {
		s.roots[id] = child
	} else {
		parentOrNil.AddChild(child)
	}
	s.mu.Unlock()
}

func (s *storage) release(id id) {
	s.mu.Lock()
	// Drop the whole subtree we have for this particular root.
	// Note that this is a noop if `id` is not a local root.
	// We could do a little more here, like emit a warning
	// when there are non-finished nodes in the tree.
	delete(s.roots, id)

	s.mu.Unlock()
}

func (s *storage) getRecording() []Recording {

}

/*

(18,12)-->(10,100)
(17,9) -->(185,4)--->(1204,102)

release(185,4) --> do nothing
release(1204,102) --> do nothing
release (17,9) --> release that subtree

*/
