package value

import (
	"bytes"
	"encoding/binary"
	"math"
	"math/rand/v2"
	"sort"
)

// ZMember pairs a member with its score, as returned by a range query.
type ZMember struct {
	Member []byte
	Score  float64
}

// ZSet is a set of members ordered by score. A small sorted set is held in a
// compact packed encoding kept in score order; it promotes to a member-to-score
// map paired with a skip list once it crosses its thresholds, and never demotes.
type ZSet struct {
	pack  []byte
	count int
	full  *fullZSet
	max   Thresholds
}

// NewZSet returns an empty sorted set bounded by max.
func NewZSet(max Thresholds) *ZSet { return &ZSet{max: max} }

// MakeZSet returns a Value wrapping z.
func MakeZSet(z *ZSet) Value { return Value{kind: KindZSet, zset: z} }

// ZSet returns the sorted-set payload. It must only be called when
// Kind() == KindZSet.
func (v Value) ZSet() *ZSet {
	if v.kind != KindZSet {
		panic("value: ZSet on kind " + v.kind.String())
	}
	return v.zset
}

// Compact reports whether the sorted set is still in its packed encoding.
func (z *ZSet) Compact() bool { return z.full == nil }

// Len reports the number of members.
func (z *ZSet) Len() int {
	if z.full != nil {
		return len(z.full.scores)
	}
	return z.count
}

// Add sets member's score and reports whether the member is new. An existing
// member's score is updated.
func (z *ZSet) Add(member []byte, score float64) bool {
	if z.full != nil {
		return z.full.add(member, score)
	}
	added := z.compactAdd(member, score)
	if z.max.exceeded(z.count, len(member)) {
		z.promote()
	}
	return added
}

// Score returns member's score.
func (z *ZSet) Score(member []byte) (float64, bool) {
	if z.full != nil {
		return z.full.score(member)
	}
	var sc float64
	found := false
	scanZPairs(z.pack, func(m []byte, s float64) bool {
		if bytes.Equal(m, member) {
			sc, found = s, true
			return false
		}
		return true
	})
	return sc, found
}

// Range returns the members between the start and stop ranks, inclusive, in
// ascending score order, with Redis index semantics.
func (z *ZSet) Range(start, stop int) []ZMember {
	lo, hi, ok := normalizeRange(start, stop, z.Len())
	if !ok {
		return nil
	}
	if z.full != nil {
		return z.full.rangeByRank(lo, hi)
	}
	out := make([]ZMember, 0, hi-lo+1)
	i := 0
	scanZPairs(z.pack, func(m []byte, s float64) bool {
		if i >= lo {
			out = append(out, ZMember{Member: bytes.Clone(m), Score: s})
		}
		i++
		return i <= hi
	})
	return out
}

// compactAdd rebuilds the packed encoding with member set to score, keeping it
// ordered. It is O(n) in the number of members, which is bounded by the
// promotion threshold.
func (z *ZSet) compactAdd(member []byte, score float64) bool {
	type entry struct {
		member []byte
		score  float64
	}
	entries := make([]entry, 0, z.count+1)
	found := false
	scanZPairs(z.pack, func(m []byte, s float64) bool {
		if bytes.Equal(m, member) {
			found = true
			entries = append(entries, entry{bytes.Clone(member), score})
		} else {
			entries = append(entries, entry{bytes.Clone(m), s})
		}
		return true
	})
	if !found {
		entries = append(entries, entry{bytes.Clone(member), score})
		z.count++
	}
	sort.Slice(entries, func(i, j int) bool {
		return zskipLess(entries[i].score, string(entries[i].member), entries[j].score, string(entries[j].member))
	})
	rebuilt := make([]byte, 0, len(z.pack)+len(member)+binary.MaxVarintLen64+8)
	for _, e := range entries {
		rebuilt = packAppendZ(rebuilt, e.member, e.score)
	}
	z.pack = rebuilt
	return !found
}

func (z *ZSet) promote() {
	full := newFullZSet()
	scanZPairs(z.pack, func(m []byte, s float64) bool {
		full.add(m, s)
		return true
	})
	z.full = full
	z.pack = nil
	z.count = 0
}

// packAppendZ appends a member element followed by its score as eight raw bytes.
func packAppendZ(buf, member []byte, score float64) []byte {
	buf = packAppend(buf, member)
	var sb [8]byte
	binary.LittleEndian.PutUint64(sb[:], math.Float64bits(score))
	return append(buf, sb[:]...)
}

// scanZPairs walks a pack of member-and-score pairs, calling fn for each. The
// member slices alias the pack.
func scanZPairs(buf []byte, fn func(member []byte, score float64) bool) {
	for len(buf) > 0 {
		n, w := binary.Uvarint(buf)
		buf = buf[w:]
		member := buf[:n]
		buf = buf[n:]
		score := math.Float64frombits(binary.LittleEndian.Uint64(buf[:8]))
		buf = buf[8:]
		if !fn(member, score) {
			return
		}
	}
}

// fullZSet is the promoted representation: a member-to-score map for O(1) score
// lookup, with a skip list that keeps members ordered for range queries.
type fullZSet struct {
	scores map[string]float64
	order  *zskiplist
}

func newFullZSet() *fullZSet {
	return &fullZSet{scores: make(map[string]float64), order: newZskiplist()}
}

func (z *fullZSet) add(member []byte, score float64) bool {
	m := string(member)
	if old, ok := z.scores[m]; ok {
		if old != score {
			z.order.delete(m, old)
			z.order.insert(m, score)
			z.scores[m] = score
		}
		return false
	}
	z.scores[m] = score
	z.order.insert(m, score)
	return true
}

func (z *fullZSet) score(member []byte) (float64, bool) {
	s, ok := z.scores[string(member)]
	return s, ok
}

func (z *fullZSet) rangeByRank(lo, hi int) []ZMember {
	out := make([]ZMember, 0, hi-lo+1)
	z.order.eachInRank(lo, hi, func(member string, score float64) {
		out = append(out, ZMember{Member: []byte(member), Score: score})
	})
	return out
}

const (
	zskipMaxLevel = 32
	zskipP        = 0.25
)

type zskiplistNode struct {
	member string
	score  float64
	next   []*zskiplistNode
}

// zskiplist orders members by (score, member). It is the ordering index behind
// a ZSet; the map alongside it answers score lookups.
type zskiplist struct {
	head   *zskiplistNode
	level  int
	length int
}

func newZskiplist() *zskiplist {
	return &zskiplist{
		head:  &zskiplistNode{next: make([]*zskiplistNode, zskipMaxLevel)},
		level: 1,
	}
}

func zskipRandomLevel() int {
	level := 1
	for level < zskipMaxLevel && rand.Float64() < zskipP {
		level++
	}
	return level
}

// zskipLess reports whether (s1, m1) sorts before (s2, m2): by score, then by
// member lexicographically, matching Redis.
func zskipLess(s1 float64, m1 string, s2 float64, m2 string) bool {
	if s1 != s2 {
		return s1 < s2
	}
	return m1 < m2
}

func (sl *zskiplist) insert(member string, score float64) {
	var update [zskipMaxLevel]*zskiplistNode
	x := sl.head
	for i := sl.level - 1; i >= 0; i-- {
		for x.next[i] != nil && zskipLess(x.next[i].score, x.next[i].member, score, member) {
			x = x.next[i]
		}
		update[i] = x
	}
	level := zskipRandomLevel()
	if level > sl.level {
		for i := sl.level; i < level; i++ {
			update[i] = sl.head
		}
		sl.level = level
	}
	node := &zskiplistNode{member: member, score: score, next: make([]*zskiplistNode, level)}
	for i := 0; i < level; i++ {
		node.next[i] = update[i].next[i]
		update[i].next[i] = node
	}
	sl.length++
}

func (sl *zskiplist) delete(member string, score float64) {
	var update [zskipMaxLevel]*zskiplistNode
	x := sl.head
	for i := sl.level - 1; i >= 0; i-- {
		for x.next[i] != nil && zskipLess(x.next[i].score, x.next[i].member, score, member) {
			x = x.next[i]
		}
		update[i] = x
	}
	x = x.next[0]
	if x == nil || x.member != member || x.score != score {
		return
	}
	for i := 0; i < sl.level; i++ {
		if update[i].next[i] == x {
			update[i].next[i] = x.next[i]
		}
	}
	for sl.level > 1 && sl.head.next[sl.level-1] == nil {
		sl.level--
	}
	sl.length--
}

// eachInRank calls fn for the nodes at ranks lo..hi inclusive (0-based), in
// ascending order.
func (sl *zskiplist) eachInRank(lo, hi int, fn func(member string, score float64)) {
	x := sl.head.next[0]
	for i := 0; i < lo && x != nil; i++ {
		x = x.next[0]
	}
	for i := lo; i <= hi && x != nil; i++ {
		fn(x.member, x.score)
		x = x.next[0]
	}
}
