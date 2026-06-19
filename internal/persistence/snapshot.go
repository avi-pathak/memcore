package persistence

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/avinashpathak/memcore/internal/value"
)

const snapshotMagic = "MEMCORE-SNAP1\n"

// Record is one key's persistent state: the database it belongs to, the key, an
// optional absolute expiry in Unix nanoseconds (0 for none), and the value.
type Record struct {
	DB       int
	Key      string
	ExpireAt int64
	Value    value.Value
}

// WriteSnapshot serializes a point-in-time dump to w. baseGen is the append-log
// generation the snapshot covers, recorded so recovery replays only later
// segments. emit is called once and must pass every record to write; it runs
// while the caller holds whatever locks make the iteration consistent.
func WriteSnapshot(w io.Writer, baseGen int, emit func(write func(Record) error) error) error {
	bw := bufio.NewWriter(w)
	if _, err := bw.WriteString(snapshotMagic); err != nil {
		return err
	}
	if err := writeUvarint(bw, uint64(baseGen)); err != nil {
		return err
	}
	if err := emit(func(rec Record) error { return writeRecord(bw, rec) }); err != nil {
		return err
	}
	return bw.Flush()
}

// ReadSnapshot parses a dump from r, calling apply for each record, and returns
// the base generation the snapshot covers. An empty or absent stream loads
// nothing and returns generation 0.
func ReadSnapshot(r io.Reader, limits value.Limits, apply func(Record) error) (int, error) {
	br := bufio.NewReader(r)
	magic := make([]byte, len(snapshotMagic))
	if _, err := io.ReadFull(br, magic); err != nil {
		if errors.Is(err, io.EOF) {
			return 0, nil
		}
		return 0, err
	}
	if string(magic) != snapshotMagic {
		return 0, errors.New("persistence: snapshot magic mismatch")
	}
	baseGen, err := binary.ReadUvarint(br)
	if err != nil {
		return 0, unexpectedEOF(err)
	}
	for {
		rec, err := readRecord(br, limits)
		if errors.Is(err, io.EOF) {
			return int(baseGen), nil
		}
		if err != nil {
			return int(baseGen), err
		}
		if err := apply(rec); err != nil {
			return int(baseGen), err
		}
	}
}

// snapshotBaseGen reads just the base generation from the snapshot in dir,
// reporting false when no readable snapshot exists.
func snapshotBaseGen(dir string) (int, bool) {
	f, err := os.Open(filepath.Join(dir, snapshotName))
	if err != nil {
		return 0, false
	}
	defer f.Close()
	br := bufio.NewReader(f)
	magic := make([]byte, len(snapshotMagic))
	if _, err := io.ReadFull(br, magic); err != nil || string(magic) != snapshotMagic {
		return 0, false
	}
	gen, err := binary.ReadUvarint(br)
	if err != nil {
		return 0, false
	}
	return int(gen), true
}

func writeRecord(bw *bufio.Writer, rec Record) error {
	if err := writeUvarint(bw, uint64(rec.DB)); err != nil {
		return err
	}
	if err := writeBytes(bw, []byte(rec.Key)); err != nil {
		return err
	}
	var ts [8]byte
	binary.LittleEndian.PutUint64(ts[:], uint64(rec.ExpireAt))
	if _, err := bw.Write(ts[:]); err != nil {
		return err
	}
	return writeValue(bw, rec.Value)
}

func readRecord(br *bufio.Reader, limits value.Limits) (Record, error) {
	db, err := binary.ReadUvarint(br)
	if err != nil {
		return Record{}, err // a clean EOF here marks the end of the records
	}
	key, err := readBytes(br)
	if err != nil {
		return Record{}, unexpectedEOF(err)
	}
	var ts [8]byte
	if _, err := io.ReadFull(br, ts[:]); err != nil {
		return Record{}, unexpectedEOF(err)
	}
	v, err := readValue(br, limits)
	if err != nil {
		return Record{}, unexpectedEOF(err)
	}
	return Record{
		DB:       int(db),
		Key:      string(key),
		ExpireAt: int64(binary.LittleEndian.Uint64(ts[:])),
		Value:    v,
	}, nil
}
