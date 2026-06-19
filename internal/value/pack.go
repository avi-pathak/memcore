package value

import "encoding/binary"

// A pack is a flat byte slice holding a sequence of length-prefixed elements:
// each element is a uvarint length followed by that many bytes. It is the
// compact encoding behind small collections, trading the per-element pointer and
// map overhead of the full structures for linear scans over one allocation.

func packAppend(buf, elem []byte) []byte {
	var hdr [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(hdr[:], uint64(len(elem)))
	buf = append(buf, hdr[:n]...)
	return append(buf, elem...)
}

// packEach calls fn for each element in buf, stopping early if fn returns false.
// The element slices alias buf and must be copied before they outlive it.
func packEach(buf []byte, fn func(elem []byte) bool) {
	for len(buf) > 0 {
		n, w := binary.Uvarint(buf)
		buf = buf[w:]
		elem := buf[:n]
		buf = buf[n:]
		if !fn(elem) {
			return
		}
	}
}
