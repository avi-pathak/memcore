package value

import "bytes"

// List is the full representation of a Redis list: a doubly-linked list of
// binary-safe values with O(1) access at both ends. The compact encoding for
// small lists is a separate type that promotes to this one.
//
// A List owns the bytes pushed into it, so callers may reuse their buffers.
type List struct {
	head, tail *listNode
	length     int
}

type listNode struct {
	value      []byte
	prev, next *listNode
}

// NewList returns an empty list.
func NewList() *List { return &List{} }

// MakeList returns a Value wrapping l.
func MakeList(l *List) Value { return Value{kind: KindList, list: l} }

// List returns the list payload. It must only be called when Kind() == KindList.
func (v Value) List() *List {
	if v.kind != KindList {
		panic("value: List on kind " + v.kind.String())
	}
	return v.list
}

// Len reports the number of elements.
func (l *List) Len() int { return l.length }

// PushFront prepends a copy of b.
func (l *List) PushFront(b []byte) {
	n := &listNode{value: bytes.Clone(b), next: l.head}
	if l.head != nil {
		l.head.prev = n
	} else {
		l.tail = n
	}
	l.head = n
	l.length++
}

// PushBack appends a copy of b.
func (l *List) PushBack(b []byte) {
	n := &listNode{value: bytes.Clone(b), prev: l.tail}
	if l.tail != nil {
		l.tail.next = n
	} else {
		l.head = n
	}
	l.tail = n
	l.length++
}

// PopFront removes and returns the first element.
func (l *List) PopFront() ([]byte, bool) {
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

// PopBack removes and returns the last element.
func (l *List) PopBack() ([]byte, bool) {
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

// Range returns the elements between the start and stop ranks, inclusive, with
// Redis index semantics: negative indices count from the end, and the range is
// clamped to the list bounds. The returned slices alias the stored bytes, which
// are immutable.
func (l *List) Range(start, stop int) [][]byte {
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
