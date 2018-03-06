package tracker

import (
	"github.com/petermattis/goid"
	"sync"
	"runtime"
)

type entry struct {
	depth int
	name string
}

type Tracker struct{
	reg sync.Map
	scratch [8192]byte
}

func (t *Tracker) Track(name string) {
	id := goid.Get()
	if _, loaded := t.reg.LoadOrStore(id, struct{
		name: name,
		depth: 
	}); loaded {
		panic("Track() called twice for the same goroutine")
	}
}

func (t *Tracker) Status() [][2]string {
	var r [][2]string
	scratch := t.scratch[:0]
	for ; runtime.Stack(scratch, true) == len(scratch); scratch = make([]byte, 0, 2*len(scratch)) {}

	t.reg.Range(func(k, v interface{}) bool {
		s := findInStack(scratch, k.(int), )
		return true // more
	})
}
