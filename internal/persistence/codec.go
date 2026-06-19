// Package persistence gives Memcore durability: an append log of write commands
// and periodic snapshots, plus recovery that replays the log over the latest
// snapshot at boot. Snapshots are taken without forking; the cost is a brief
// pause while the live state is captured, documented in ARCHITECTURE.md.
package persistence

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/avinashpathak/memcore/internal/value"
)

// Value encoding tags, one per Kind.
const (
	tagString = 's'
	tagList   = 'l'
	tagHash   = 'h'
	tagSet    = 'p' // "plain" set, distinct from the string tag
	tagZSet   = 'z'
)

func writeUvarint(w *bufio.Writer, n uint64) error {
	var buf [binary.MaxVarintLen64]byte
	m := binary.PutUvarint(buf[:], n)
	_, err := w.Write(buf[:m])
	return err
}

func writeBytes(w *bufio.Writer, b []byte) error {
	if err := writeUvarint(w, uint64(len(b))); err != nil {
		return err
	}
	_, err := w.Write(b)
	return err
}

func readBytes(r *bufio.Reader) ([]byte, error) {
	n, err := binary.ReadUvarint(r)
	if err != nil {
		return nil, err
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}
	return b, nil
}

func writeValue(w *bufio.Writer, v value.Value) error {
	switch v.Kind() {
	case value.KindString:
		if err := w.WriteByte(tagString); err != nil {
			return err
		}
		return writeBytes(w, v.Str())
	case value.KindList:
		return writeElements(w, tagList, v.List().Range(0, -1))
	case value.KindHash:
		// Pairs returns a flat field,value sequence; the element count is the
		// number of pairs.
		pairs := v.Hash().Pairs()
		if err := w.WriteByte(tagHash); err != nil {
			return err
		}
		if err := writeUvarint(w, uint64(len(pairs)/2)); err != nil {
			return err
		}
		for _, p := range pairs {
			if err := writeBytes(w, p); err != nil {
				return err
			}
		}
		return nil
	case value.KindSet:
		return writeElements(w, tagSet, v.Set().Members())
	case value.KindZSet:
		members := v.ZSet().Range(0, -1)
		if err := w.WriteByte(tagZSet); err != nil {
			return err
		}
		if err := writeUvarint(w, uint64(len(members))); err != nil {
			return err
		}
		for _, m := range members {
			if err := writeBytes(w, m.Member); err != nil {
				return err
			}
			if err := writeScore(w, m.Score); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("persistence: cannot encode value kind %v", v.Kind())
	}
}

func writeElements(w *bufio.Writer, tag byte, elems [][]byte) error {
	if err := w.WriteByte(tag); err != nil {
		return err
	}
	if err := writeUvarint(w, uint64(len(elems))); err != nil {
		return err
	}
	for _, e := range elems {
		if err := writeBytes(w, e); err != nil {
			return err
		}
	}
	return nil
}

func writeScore(w *bufio.Writer, score float64) error {
	var sb [8]byte
	binary.LittleEndian.PutUint64(sb[:], math.Float64bits(score))
	_, err := w.Write(sb[:])
	return err
}

func readValue(r *bufio.Reader, limits value.Limits) (value.Value, error) {
	tag, err := r.ReadByte()
	if err != nil {
		return value.Value{}, err
	}
	switch tag {
	case tagString:
		b, err := readBytes(r)
		if err != nil {
			return value.Value{}, err
		}
		return value.MakeString(b), nil
	case tagList:
		l := value.NewList(limits.List)
		if err := readElements(r, func(b []byte) { l.PushBack(b) }); err != nil {
			return value.Value{}, err
		}
		return value.MakeList(l), nil
	case tagHash:
		n, err := binary.ReadUvarint(r)
		if err != nil {
			return value.Value{}, err
		}
		h := value.NewHash(limits.Hash)
		for i := uint64(0); i < n; i++ {
			field, err := readBytes(r)
			if err != nil {
				return value.Value{}, err
			}
			val, err := readBytes(r)
			if err != nil {
				return value.Value{}, err
			}
			h.Set(string(field), val)
		}
		return value.MakeHash(h), nil
	case tagSet:
		s := value.NewSet(limits.Set)
		if err := readElements(r, func(b []byte) { s.Add(b) }); err != nil {
			return value.Value{}, err
		}
		return value.MakeSet(s), nil
	case tagZSet:
		n, err := binary.ReadUvarint(r)
		if err != nil {
			return value.Value{}, err
		}
		z := value.NewZSet(limits.ZSet)
		for i := uint64(0); i < n; i++ {
			member, err := readBytes(r)
			if err != nil {
				return value.Value{}, err
			}
			score, err := readScore(r)
			if err != nil {
				return value.Value{}, err
			}
			z.Add(member, score)
		}
		return value.MakeZSet(z), nil
	default:
		return value.Value{}, fmt.Errorf("persistence: unknown value tag %q", tag)
	}
}

func readElements(r *bufio.Reader, add func([]byte)) error {
	n, err := binary.ReadUvarint(r)
	if err != nil {
		return err
	}
	for i := uint64(0); i < n; i++ {
		b, err := readBytes(r)
		if err != nil {
			return err
		}
		add(b)
	}
	return nil
}

func readScore(r *bufio.Reader) (float64, error) {
	var sb [8]byte
	if _, err := io.ReadFull(r, sb[:]); err != nil {
		return 0, err
	}
	return math.Float64frombits(binary.LittleEndian.Uint64(sb[:])), nil
}

// unexpectedEOF maps a stream that ends partway through a record to
// io.ErrUnexpectedEOF, so a reader can tell a clean record boundary from a
// truncated trailing record left by a crash.
func unexpectedEOF(err error) error {
	if errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}
