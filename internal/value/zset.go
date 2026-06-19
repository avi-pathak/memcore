package value

import "math/rand/v2"

// ZSet is the full representation of a Redis sorted set: a map from member to
// score for O(1) score lookup, and a skip list that keeps members ordered by
// score (ties broken by member) for range queries. The compact encoding for
// small sorted sets is a separate type that promotes to this one.
type ZSet struct {
	scores map[string]float64
	order  *zskiplist
}

// ZMember pairs a member with its score, as returned by a range query.
type ZMember struct {
	Member []byte
	Score  float64
}

// NewZSet returns an empty sorted set.
func NewZSet() *ZSet {
	return &ZSet{scores: make(map[string]float64), order: newZskiplist()}
}

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

// Len reports the number of members.
func (z *ZSet) Len() int { return len(z.scores) }

// Add sets member's score and reports whether the member is new. An existing
// member's score is updated in place.
func (z *ZSet) Add(member []byte, score float64) bool {
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

// Score returns member's score.
func (z *ZSet) Score(member []byte) (float64, bool) {
	s, ok := z.scores[string(member)]
	return s, ok
}

// Range returns the members between the start and stop ranks, inclusive, in
// ascending score order, with Redis index semantics.
func (z *ZSet) Range(start, stop int) []ZMember {
	lo, hi, ok := normalizeRange(start, stop, len(z.scores))
	if !ok {
		return nil
	}
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
