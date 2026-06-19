package value

import (
	"bytes"
	"encoding/binary"
)

// List is an ordered sequence of binary-safe values. A small list is held in a
// compact packed encoding; it promotes to a doubly-linked representation once it
// crosses its thresholds, and never demotes. A List owns the bytes pushed into
// it, so callers may reuse their buffers.
type List struct {
	pack  []byte
	count int
	full  *fullList
	max   Thresholds
}

// NewList returns an empty list bounded by max.
func NewList(max Thresholds) *List { return &List{max: max} }

// MakeList returns a Value wrapping l.
func MakeList(l *List) Value { return Value{kind: KindList, list: l} }

// List returns the list payload. It must only be called when Kind() == KindList.
func (v Value) List() *List {
	if v.kind != KindList {
		panic("value: List on kind " + v.kind.String())
	}
	return v.list
}

// Compact reports whether the list is still in its packed encoding. It is here
// for tests and introspection.
func (l *List) Compact() bool { return l.full == nil }

// Len reports the number of elements.
func (l *List) Len() int {
	if l.full != nil {
		return l.full.length
	}
	return l.count
}

// PushFront prepends a copy of b.
func (l *List) PushFront(b []byte) {
	if l.full != nil {
		l.full.pushFront(b)
		return
	}
	l.pack = append(packAppend(nil, b), l.pack...)
	l.count++
	l.promoteIfNeeded(len(b))
}

// PushBack appends a copy of b.
func (l *List) PushBack(b []byte) {
	if l.full != nil {
		l.full.pushBack(b)
		return
	}
	l.pack = packAppend(l.pack, b)
	l.count++
	l.promoteIfNeeded(len(b))
}

// PopFront removes and returns the first element.
func (l *List) PopFront() ([]byte, bool) {
	if l.full != nil {
		return l.full.popFront()
	}
	if l.count == 0 {
		return nil, false
	}
	n, w := binary.Uvarint(l.pack)
	out := bytes.Clone(l.pack[w : w+int(n)])
	l.pack = bytes.Clone(l.pack[w+int(n):])
	l.count--
	return out, true
}

// PopBack removes and returns the last element.
func (l *List) PopBack() ([]byte, bool) {
	if l.full != nil {
		return l.full.popBack()
	}
	if l.count == 0 {
		return nil, false
	}
	lastOff, dataOff, dataLen := 0, 0, 0
	for off := 0; off < len(l.pack); {
		n, w := binary.Uvarint(l.pack[off:])
		lastOff, dataOff, dataLen = off, off+w, int(n)
		off = off + w + int(n)
	}
	out := bytes.Clone(l.pack[dataOff : dataOff+dataLen])
	l.pack = l.pack[:lastOff]
	l.count--
	return out, true
}

// Range returns the elements between the start and stop ranks, inclusive, with
// Redis index semantics: negative indices count from the end, and the range is
// clamped to the list bounds.
func (l *List) Range(start, stop int) [][]byte {
	if l.full != nil {
		return l.full.rangeElems(start, stop)
	}
	lo, hi, ok := normalizeRange(start, stop, l.count)
	if !ok {
		return nil
	}
	out := make([][]byte, 0, hi-lo+1)
	i := 0
	packEach(l.pack, func(e []byte) bool {
		if i >= lo {
			out = append(out, bytes.Clone(e))
		}
		i++
		return i <= hi
	})
	return out
}

func (l *List) promoteIfNeeded(elemSize int) {
	if l.max.exceeded(l.count, elemSize) {
		l.promote()
	}
}

func (l *List) promote() {
	full := &fullList{}
	packEach(l.pack, func(e []byte) bool {
		full.pushBack(e)
		return true
	})
	l.full = full
	l.pack = nil
	l.count = 0
}

// fullList is the promoted representation: a doubly-linked list with O(1) access
// at both ends. Its element bytes are immutable once stored.
type fullList struct {
	head, tail *listNode
	length     int
}

type listNode struct {
	value      []byte
	prev, next *listNode
}

func (l *fullList) pushFront(b []byte) {
	n := &listNode{value: bytes.Clone(b), next: l.head}
	if l.head != nil {
		l.head.prev = n
	} else {
		l.tail = n
	}
	l.head = n
	l.length++
}

func (l *fullList) pushBack(b []byte) {
	n := &listNode{value: bytes.Clone(b), prev: l.tail}
	if l.tail != nil {
		l.tail.next = n
	} else {
		l.head = n
	}
	l.tail = n
	l.length++
}

func (l *fullList) popFront() ([]byte, bool) {
	if l.head == nil {
		return nil, false
	}
	n := l.head
	l.head = n.next
	if l.head != nil {
		l.head.prev = nil
	} else {
		l.tail = nil
	}
	l.length--
	return n.value, true
}

func (l *fullList) popBack() ([]byte, bool) {
	if l.tail == nil {
		return nil, false
	}
	n := l.tail
	l.tail = n.prev
	if l.tail != nil {
		l.tail.next = nil
	} else {
		l.head = nil
	}
	l.length--
	return n.value, true
}

func (l *fullList) rangeElems(start, stop int) [][]byte {
	lo, hi, ok := normalizeRange(start, stop, l.length)
	if !ok {
		return nil
	}
	out := make([][]byte, 0, hi-lo+1)
	n := l.head
	for i := 0; i < lo; i++ {
		n = n.next
	}
	for i := lo; i <= hi; i++ {
		out = append(out, n.value)
		n = n.next
	}
	return out
}
