package tracing

import (
	"github.com/cockroachdb/cockroach/pkg/util/syncutil"
	proto "github.com/gogo/protobuf/proto"
)

// Metadata is immutable, structured trace data.
type Metadata interface {
	proto.Message
}

func (s *span) AddMetadata(meta Metadata) {
	// TBD
}

// InflightMetadata is a metadata that is not yet immutable.
// Its purpose is to expose whatever partial information it
// already contains via an InflightRegistry.
type InflightMetadata interface {
	// Current returns an immutable Metadata that represents the data
	// currently in the InflightMetadata. This is typically a partially
	// populated Metadata; the particular implementation of the
	// Metadata should be clear about which partial states to expect.
	Current() Metadata
}

type InflightRegistry struct {
	mu struct {
		syncutil.Mutex
		m map[uint64]InflightSpan
	}
}

type SpanI interface {
	SpanID() uint64
	AddMetadata(Metadata)
}

type InflightSpan struct {
	sp SpanI
	r  *InflightRegistry
	mu struct {
		syncutil.Mutex
		inf []InflightMetadata // unsorted
	}
}

func (r *InflightRegistry) Register(sp SpanI) (_ InflightSpan, unregister func()) {
	// TODO this is super simplistic.
	id := sp.SpanID()
	isp := InflightSpan{
		sp: sp,
		r:  r,
	}
	r.mu.Lock()
	r.mu.m[id] = isp
	r.mu.Unlock()
	return isp, func() {
		r.mu.Lock()
		delete(r.mu.m, id)
		r.mu.Unlock()
	}
}

type InflightEntry struct {
	idx int
}

func (isp *InflightSpan) Add(imeta InflightMetadata) {
	isp.mu.Lock()
	defer isp.mu.Unlock()
	for i := range isp.mu.inf {
		if isp.mu.inf[i] == nil {
			isp.mu.inf[i] = imeta
			return
		}
	}
	isp.mu.inf = append(isp.mu.inf, imeta)
}

func (isp *InflightSpan) Del(imeta InflightMetadata) {
	isp.mu.Lock()
	defer isp.mu.Unlock()
	for i := range isp.mu.inf {
		if isp.mu.inf[i] == imeta {
			isp.mu.inf[i] = nil
			return
		}
	}
}
