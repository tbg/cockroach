// Copyright 2014 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

package engine

import (
	"encoding/binary"
	"fmt"

	"bytes"

	"github.com/cockroachdb/cockroach/pkg/util/hlc"
	"github.com/cockroachdb/cockroach/pkg/util/protoutil"
	"github.com/pkg/errors"
)

// BatchType represents the type of an entry in an encoded RocksDB batch.
type BatchType byte

// These constants come from rocksdb/db/dbformat.h.
const (
	BatchTypeDeletion BatchType = 0x0
	BatchTypeValue              = 0x1
	BatchTypeMerge              = 0x2
	// BatchTypeLogData                    = 0x3
	// BatchTypeColumnFamilyDeletion       = 0x4
	// BatchTypeColumnFamilyValue          = 0x5
	// BatchTypeColumnFamilyMerge          = 0x6
	// BatchTypeSingleDeletion             = 0x7
	// BatchTypeColumnFamilySingleDeletion = 0x8
)

const (
	// The batch header is composed of an 8-byte sequence number (all zeroes) and
	// 4-byte count of the number of entries in the batch.
	headerSize       int = 12
	countPos             = 8
	initialBatchSize     = 1 << 10
	maxVarintLen32       = 5
)

// RocksDBBatchBuilder is used to construct the RocksDB batch representation.
// From the RocksDB code, the representation of a batch is:
//
//   WriteBatch::rep_ :=
//      sequence: fixed64
//      count: fixed32
//      data: record[count]
//   record :=
//      kTypeValue varstring varstring
//      kTypeDeletion varstring
//      kTypeSingleDeletion varstring
//      kTypeMerge varstring varstring
//      kTypeColumnFamilyValue varint32 varstring varstring
//      kTypeColumnFamilyDeletion varint32 varstring varstring
//      kTypeColumnFamilySingleDeletion varint32 varstring varstring
//      kTypeColumnFamilyMerge varint32 varstring varstring
//   varstring :=
//      len: varint32
//      data: uint8[len]
//
// The RocksDBBatchBuilder code currently only supports kTypeValue
// (BatchTypeValue), kTypeDeletion (BatchTypeDeletion)and kTypeMerge
// (BatchTypeMerge) operations. Before a batch is written to the RocksDB
// write-ahead-log, the sequence number is 0. The "fixed32" format is little
// endian.
//
// The keys encoded into the batch are MVCC keys: a string key with a timestamp
// suffix. MVCC keys are encoded as:
//
//   <key>[<wall_time>[<logical>]]<#timestamp-bytes>
//
// The <wall_time> and <logical> portions of the key are encoded as 64 and
// 32-bit big-endian integers. A custom RocksDB comparator is used to maintain
// the desired ordering as these keys do not sort lexicographically correctly.
// Note that the encoding of these keys needs to match up with the encoding in
// rocksdb/db.cc:EncodeKey().
type RocksDBBatchBuilder struct {
	repr  []byte
	count int
}

func (b *RocksDBBatchBuilder) maybeInit() {
	if b.repr == nil {
		b.repr = make([]byte, headerSize, initialBatchSize)
	}
}

// Finish returns the constructed batch representation. After calling Finish,
// the builder may be used to construct another batch, but the returned []byte
// is only valid until the next builder method is called.
func (b *RocksDBBatchBuilder) Finish() []byte {
	repr := b.getRepr()
	b.repr = b.repr[:headerSize]
	b.count = 0
	return repr
}

// Len returns the number of bytes currently in the under construction repr.
func (b *RocksDBBatchBuilder) Len() int {
	return len(b.repr)
}

// getRepr constructs the batch representation and returns it.
func (b *RocksDBBatchBuilder) getRepr() []byte {
	b.maybeInit()
	buf := b.repr[countPos:headerSize]
	v := uint32(b.count)
	buf[0] = byte(v)
	buf[1] = byte(v >> 8)
	buf[2] = byte(v >> 16)
	buf[3] = byte(v >> 24)
	return b.repr
}

func (b *RocksDBBatchBuilder) grow(n int) {
	newSize := len(b.repr) + n
	if newSize > cap(b.repr) {
		newCap := 2 * cap(b.repr)
		for newCap < newSize {
			newCap *= 2
		}
		newRepr := make([]byte, len(b.repr), newCap)
		copy(newRepr, b.repr)
		b.repr = newRepr
	}
	b.repr = b.repr[:newSize]
}

func putUvarint32(buf []byte, x uint32) int {
	i := 0
	for x >= 0x80 {
		buf[i] = byte(x) | 0x80
		x >>= 7
		i++
	}
	buf[i] = byte(x)
	return i + 1
}

func putUint32(b []byte, v uint32) {
	b[0] = byte(v >> 24)
	b[1] = byte(v >> 16)
	b[2] = byte(v >> 8)
	b[3] = byte(v)
}

func putUint64(b []byte, v uint64) {
	b[0] = byte(v >> 56)
	b[1] = byte(v >> 48)
	b[2] = byte(v >> 40)
	b[3] = byte(v >> 32)
	b[4] = byte(v >> 24)
	b[5] = byte(v >> 16)
	b[6] = byte(v >> 8)
	b[7] = byte(v)
}

// encodeKey encodes an MVCC key into the batch, reserving extra bytes in
// b.repr for use in encoding a value as well. This encoding must match with
// the encoding in engine/db.cc:EncodeKey().
func (b *RocksDBBatchBuilder) encodeKey(key MVCCKey, extra int) {
	length := 1 + len(key.Key)
	timestampLength := 0
	if key.Timestamp != (hlc.Timestamp{}) {
		timestampLength = 1 + 8
		if key.Timestamp.Logical != 0 {
			timestampLength += 4
		}
	}
	length += timestampLength

	pos := 1 + len(b.repr)
	b.grow(1 + maxVarintLen32 + length + extra)
	n := putUvarint32(b.repr[pos:], uint32(length))
	b.repr = b.repr[:len(b.repr)-(maxVarintLen32-n)]
	pos += n
	copy(b.repr[pos:], key.Key)
	if timestampLength > 0 {
		pos += len(key.Key)
		b.repr[pos] = 0
		pos++
		putUint64(b.repr[pos:], uint64(key.Timestamp.WallTime))
		if key.Timestamp.Logical != 0 {
			pos += 8
			putUint32(b.repr[pos:], uint32(key.Timestamp.Logical))
		}
	}
	b.repr[len(b.repr)-1-extra] = byte(timestampLength)
}

func (b *RocksDBBatchBuilder) encodeKeyValue(key MVCCKey, value []byte, tag byte) {
	b.maybeInit()
	b.count++

	l := uint32(len(value))
	extra := int(l) + maxVarintLen32

	pos := len(b.repr)
	b.encodeKey(key, extra)
	b.repr[pos] = tag

	pos = len(b.repr) - extra
	n := putUvarint32(b.repr[pos:], l)
	b.repr = b.repr[:len(b.repr)-(maxVarintLen32-n)]
	copy(b.repr[pos+n:], value)
}

// Put sets the given key to the value provided.
func (b *RocksDBBatchBuilder) Put(key MVCCKey, value []byte) {
	b.encodeKeyValue(key, value, BatchTypeValue)
}

// Merge is a high-performance write operation used for values which are
// accumulated over several writes. Multiple values can be merged sequentially
// into a single key; a subsequent read will return a "merged" value which is
// computed from the original merged values.
func (b *RocksDBBatchBuilder) Merge(key MVCCKey, value []byte) {
	b.encodeKeyValue(key, value, BatchTypeMerge)
}

// Clear removes the item from the db with the given key.
func (b *RocksDBBatchBuilder) Clear(key MVCCKey) {
	b.maybeInit()
	b.count++
	pos := len(b.repr)
	b.encodeKey(key, 0)
	b.repr[pos] = byte(BatchTypeDeletion)
}

// ApplyRepr applies the mutations in repr to the current batch.
func (b *RocksDBBatchBuilder) ApplyRepr(repr []byte) error {
	if len(repr) < headerSize {
		return errors.Errorf("batch repr too small: %d < %d", len(repr), headerSize)
	}
	b.maybeInit()
	pos := len(b.repr)
	data := repr[headerSize:]
	b.grow(len(data))
	copy(b.repr[pos:], data)
	b.count += int(binary.LittleEndian.Uint32(repr[countPos:headerSize]))
	return nil
}

// EncodeKey encodes an engine.MVCC key into the RocksDB representation. This
// encoding must match with the encoding in engine/db.cc:EncodeKey().
func EncodeKey(key MVCCKey) []byte {
	// TODO(dan): Unify this with (*RocksDBBatchBuilder).encodeKey.

	const (
		timestampSentinelLen      = 1
		walltimeEncodedLen        = 8
		logicalEncodedLen         = 4
		timestampEncodedLengthLen = 1
	)

	timestampLength := 0
	if key.Timestamp != (hlc.Timestamp{}) {
		timestampLength = timestampSentinelLen + walltimeEncodedLen
		if key.Timestamp.Logical != 0 {
			timestampLength += logicalEncodedLen
		}
	}

	dbKey := make([]byte, len(key.Key)+timestampLength+timestampEncodedLengthLen)
	copy(dbKey, key.Key)

	pos := len(key.Key)
	if timestampLength > 0 {
		dbKey[pos] = 0
		pos += timestampSentinelLen
		putUint64(dbKey[pos:], uint64(key.Timestamp.WallTime))
		pos += walltimeEncodedLen
		if key.Timestamp.Logical != 0 {
			putUint32(dbKey[pos:], uint32(key.Timestamp.Logical))
			pos += logicalEncodedLen
		}
	}
	dbKey[len(dbKey)-1] = byte(timestampLength)

	return dbKey
}

// SplitMVCCKey returns the key and timestamp components of an encoded MVCC key. This
// decoding must match engine/db.cc:SplitKey().
func SplitMVCCKey(mvccKey []byte) (key []byte, ts []byte, ok bool) {
	if len(mvccKey) == 0 {
		return nil, nil, false
	}
	tsLen := int(mvccKey[len(mvccKey)-1])
	keyPartEnd := len(mvccKey) - 1 - tsLen
	if keyPartEnd < 0 {
		return nil, nil, false
	}

	key = mvccKey[:keyPartEnd]
	if tsLen > 0 {
		ts = mvccKey[keyPartEnd+1 : len(mvccKey)-1]
	}
	return key, ts, true
}

// DecodeKey decodes an engine.MVCCKey from its serialized representation. This
// decoding must match engine/db.cc:DecodeKey().
func DecodeKey(encodedKey []byte) (MVCCKey, error) {
	key, ts, ok := SplitMVCCKey(encodedKey)
	if !ok {
		return MVCCKey{}, errors.Errorf("invalid encoded mvcc key: '%x'", encodedKey)
	}

	mvccKey := MVCCKey{Key: key}
	switch len(ts) {
	case 0:
		// No-op.
	case 8:
		mvccKey.Timestamp.WallTime = int64(binary.BigEndian.Uint64(ts[0:8]))
	case 12:
		mvccKey.Timestamp.WallTime = int64(binary.BigEndian.Uint64(ts[0:8]))
		mvccKey.Timestamp.Logical = int32(binary.BigEndian.Uint32(ts[8:12]))
	default:
		return MVCCKey{}, errors.Errorf(
			"invalid encoded mvcc key: %x bad timestamp %x", encodedKey, ts)
	}

	return mvccKey, nil
}

// RocksDBBatchReader is used to iterate the entries in a RocksDB batch
// representation.
//
// Example:
// r, err := NewRocksDBBatchReader(...)
// if err != nil {
//   return err
// }
// for r.Next() {
// 	 switch r.BatchType() {
// 	 case BatchTypeDeletion:
// 	   fmt.Printf("delete(%x)", r.Key())
// 	 case BatchTypeValue:
// 	   fmt.Printf("put(%x,%x)", r.Key(), r.Value())
// 	 case BatchTypeMerge:
// 	   fmt.Printf("merge(%x,%x)", r.Key(), r.Value())
// 	 }
// }
// if err != nil {
//   return nil
// }
type RocksDBBatchReader struct {
	repr []byte

	// The error encountered during iterator, if any
	err error

	// The total number of entries, decoded from the batch header
	count uint32

	// The following all represent the current entry and are updated by Next.
	// `value` is not applicable for BatchTypeDeletion.
	offset int
	typ    BatchType
	key    []byte
	value  []byte
}

// NewRocksDBBatchReader creates a RocksDBBatchReader from the given repr and
// verifies the header.
func NewRocksDBBatchReader(repr []byte) (*RocksDBBatchReader, error) {
	if len(repr) < headerSize {
		return nil, errors.Errorf("batch repr too small: %d < %d", len(repr), headerSize)
	}
	seq := binary.LittleEndian.Uint64(repr[:countPos])
	if seq != 0 {
		return nil, errors.Errorf("bad sequence: expected 0, but found %d", seq)
	}

	// Set offset to -1 so the first call to Next will increment it to 0.
	r := &RocksDBBatchReader{repr: repr, offset: -1}
	r.count = binary.LittleEndian.Uint32(repr[countPos:headerSize])
	r.repr = r.repr[headerSize:]
	return r, nil
}

func (orig *RocksDBBatchReader) Clone() RocksDBBatchReader {
	return RocksDBBatchReader{
		repr:   orig.repr,
		err:    orig.err,
		count:  orig.count,
		offset: orig.offset,
		typ:    orig.typ,
		key:    orig.key,
		value:  orig.value,
	}
}

func (r *RocksDBBatchReader) DebugState() string {
	return fmt.Sprintf("offset:%d/%d, (%+v, %+v)",
		r.offset, r.count, r.key, r.value)
}

func (orig *RocksDBBatchReader) DebugBatch() string {
	r := orig.Clone()
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("%d keys: [\n", r.count))

	for {
		r.Next()
		if t, err := r.Valid(); !t || err != nil {
			buf.WriteString(fmt.Sprintf("(%v, %v),\n", t, err))
			break
		}
		buf.WriteString(fmt.Sprintf("(%v, %v),\n", r.key, r.value))
	}

	buf.WriteString("]")
	return buf.String()
}

// Close is part of the SimpleIterator interface.
func (r *RocksDBBatchReader) Close() {
	r.repr = nil
	r.key = nil
	r.value = nil
	r.err = nil
	r.count = 0
	r.offset = 0
}

// TODO(arjun): we really don't want to support Seek(), but it's required for the
// SimpleIterator interface. For now we support Seek() if we're already at the
// requested key, and panic otherwise
func (r *RocksDBBatchReader) Seek(seekTo MVCCKey) {
	//fmt.Printf("Seeking to %+v\n", seekTo)
	for {
		var decodedKey MVCCKey
		decodedKey, r.err = DecodeKey(r.key)
		if r.err != nil {
			return
		}
		if ok, _ := r.Valid(); !ok {
			return
		}
		if cmp := bytes.Compare(decodedKey.Key, seekTo.Key); cmp == 0 {
			// We found the key, now we need to find the requested timestamp
			for {
				var decodedKey MVCCKey
				decodedKey, r.err = DecodeKey(r.key)
				if r.err != nil {
					return
				}
				if ok, _ := r.Valid(); !ok {
					return
				}
				if decodedKey.Timestamp.Equal(seekTo.Timestamp) {
					return
				}
				r.Next()
			}
		} else if cmp < 0 {
			r.NextKey()
		} else {
			// TODO(arjun)
			panic(fmt.Sprintf("seek to %+v from %+v not supported", seekTo, r.key))
		}
	}
}

// Count returns the declared number of entries in the batch.
func (r *RocksDBBatchReader) Count() int {
	return int(r.count)
}

// Error returns the error, if any, which the iterator encountered.
func (r *RocksDBBatchReader) Error() error {
	return r.err
}

// BatchType returns the type of the current batch entry.
func (r *RocksDBBatchReader) BatchType() BatchType {
	return r.typ
}

// Key returns the key of the current batch entry.
func (r *RocksDBBatchReader) Key() []byte {
	return r.key
}

// MVCCKey decodes and returns the MVCC key of the current batch entry.
func (r *RocksDBBatchReader) MVCCKey() (MVCCKey, error) {
	return DecodeKey(r.key)
}

// UnsafeKey decodes and returns the MVCC key of the current batch entry.
func (r *RocksDBBatchReader) UnsafeKey() MVCCKey {
	key, err := r.MVCCKey()
	if err != nil {
		panic("todo UnsafeKey")
	}
	return key
}

// Value returns the value of the current batch entry. Value panics if the
// BatchType is BatchTypeDeleted.
func (r *RocksDBBatchReader) UnsafeValue() []byte {
	if r.typ == BatchTypeDeletion {
		panic("cannot call Value on a deletion entry")
	}
	return r.value
}

func (r *RocksDBBatchReader) ValueProto(msg protoutil.Message) error {
	return protoutil.Unmarshal(r.UnsafeValue(), msg)
}

func (r *RocksDBBatchReader) Valid() (bool, error) {
	return r.key != nil, r.err
}

// Next advances to the next entry in the batch, returning false when the batch
// is empty.
func (r *RocksDBBatchReader) Next() {
	r.offset++
	if len(r.repr) == 0 {
		if r.offset < int(r.count) {
			r.err = errors.Errorf("invalid batch: expected %d entries but found %d", r.count, r.offset)
		}
		r.key = nil
		return
	}

	r.typ = BatchType(r.repr[0])
	r.repr = r.repr[1:]
	switch r.typ {
	case BatchTypeDeletion:
		r.key, r.err = r.varstring()
	case BatchTypeValue:
		fallthrough
	case BatchTypeMerge:
		if r.key, r.err = r.varstring(); r.err != nil {
			return
		}
		r.value, r.err = r.varstring()
	default:
		r.err = errors.Errorf("unexpected type %d", r.typ)
		r.key = nil
	}
}

func (r *RocksDBBatchReader) NextKey() {
	curKey, err := DecodeKey(r.key)
	if err != nil {
		r.err = err
		return
	}
	startKey := curKey

	for bytes.Compare(startKey.Key, curKey.Key) == 0 {
		r.Next()
		if ok, _ := r.Valid(); !ok {
			return
		}

		curKey, err = DecodeKey(r.key)
		if err != nil {
			r.err = err
			return
		}
	}
}

func (r *RocksDBBatchReader) varstring() ([]byte, error) {
	v, n := binary.Uvarint(r.repr)
	if n <= 0 {
		return nil, fmt.Errorf("unable to decode uvarint")
	}
	r.repr = r.repr[n:]
	if v == 0 {
		return nil, nil
	}
	if v > uint64(len(r.repr)) {
		return nil, fmt.Errorf("malformed varstring, expected %d bytes, but only %d remaining",
			v, len(r.repr))
	}
	s := r.repr[:v]
	r.repr = r.repr[v:]
	return s, nil
}

// RocksDBBatchCount provides an efficient way to get the count of mutations
// in a RocksDB Batch representation.
func RocksDBBatchCount(repr []byte) (int, error) {
	if len(repr) < headerSize {
		return 0, errors.Errorf("batch repr too small: %d < %d", len(repr), headerSize)
	}
	return int(binary.LittleEndian.Uint32(repr[countPos:headerSize])), nil
}

var _ SimpleIterator = &RocksDBBatchReader{}
