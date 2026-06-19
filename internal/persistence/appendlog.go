package persistence

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/avinashpathak/memcore/internal/value"
)

const (
	snapshotName = "memcore.snapshot"
	appendName   = "memcore.aof"
)

// FSync policies.
const (
	FSyncAlways   = "always"
	FSyncEverySec = "everysec"
	FSyncNo       = "no"
)

// Store owns the on-disk snapshot and the append log for a data directory. The
// append log is split into generation-numbered segments; a snapshot records the
// base generation it covers, so recovery replays only the segments at or above
// it and a crash mid-compaction can never lose or double-apply writes.
//
// Append is safe for concurrent callers. Rotate, InstallSnapshot, and Close are
// serialized against Append and against each other by the same lock; the server
// runs at most one compaction at a time.
type Store struct {
	dir    string
	fsync  string
	limits value.Limits

	mu     sync.Mutex
	gen    int
	seg    *os.File
	writer *bufio.Writer
}

// Open prepares dir and opens the active append-log segment for appending. The
// active generation is at least the base generation of any existing snapshot, so
// later writes are never shadowed by it.
func Open(dir, fsync string, limits value.Limits) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, fsync: fsync, limits: limits}
	gen := highestSegment(dir)
	if base, ok := snapshotBaseGen(dir); ok && base > gen {
		gen = base
	}
	if err := s.openSegment(gen); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) segmentPath(gen int) string {
	return filepath.Join(s.dir, fmt.Sprintf("%s.%d", appendName, gen))
}

func (s *Store) openSegment(gen int) error {
	f, err := os.OpenFile(s.segmentPath(gen), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	s.gen = gen
	s.seg = f
	s.writer = bufio.NewWriter(f)
	return nil
}

// NextGen reports the generation a Rotate would switch to, which is the base
// generation of the snapshot taken alongside it.
func (s *Store) NextGen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gen + 1
}

// Append records one write command against database db. With the "always"
// policy it fsyncs before returning; otherwise it buffers.
func (s *Store) Append(db int, args [][]byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := writeCommand(s.writer, db, args); err != nil {
		return err
	}
	if s.fsync == FSyncAlways {
		return s.flushLocked()
	}
	return nil
}

// Flush flushes buffered records and fsyncs the active segment. The everysec
// policy calls it on a timer; shutdown calls it once.
func (s *Store) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.flushLocked()
}

func (s *Store) flushLocked() error {
	if err := s.writer.Flush(); err != nil {
		return err
	}
	return s.seg.Sync()
}

// Close flushes and closes the active segment.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.flushLocked(); err != nil {
		return err
	}
	return s.seg.Close()
}

// Rotate flushes and closes the active segment and opens the next one, returning
// the new generation. Subsequent appends go to it. The caller takes this step
// while holding the locks that froze the snapshotted state, so no write straddles
// the boundary.
func (s *Store) Rotate() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writer.Flush(); err != nil {
		return 0, err
	}
	if err := s.seg.Close(); err != nil {
		return 0, err
	}
	if err := s.openSegment(s.gen + 1); err != nil {
		return 0, err
	}
	return s.gen, nil
}

// InstallSnapshot writes the serialized snapshot durably, then removes the
// segments it covers. The write happens off any data lock; a crash before the
// rename leaves the previous snapshot and every segment in place.
func (s *Store) InstallSnapshot(snapshot []byte, baseGen int) error {
	tmp := filepath.Join(s.dir, snapshotName+".tmp")
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := f.Write(snapshot); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(s.dir, snapshotName)); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for g := 0; g < baseGen; g++ {
		_ = os.Remove(s.segmentPath(g))
	}
	return nil
}

func highestSegment(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	highest := 0
	for _, e := range entries {
		var gen int
		if _, err := fmt.Sscanf(e.Name(), appendName+".%d", &gen); err == nil && gen > highest {
			highest = gen
		}
	}
	return highest
}

func writeCommand(bw *bufio.Writer, db int, args [][]byte) error {
	if err := writeUvarint(bw, uint64(db)); err != nil {
		return err
	}
	if err := writeUvarint(bw, uint64(len(args))); err != nil {
		return err
	}
	for _, a := range args {
		if err := writeBytes(bw, a); err != nil {
			return err
		}
	}
	return nil
}

func readCommand(br *bufio.Reader) (db int, args [][]byte, err error) {
	d, err := binary.ReadUvarint(br)
	if err != nil {
		return 0, nil, err // a clean EOF here marks the end of the log
	}
	n, err := binary.ReadUvarint(br)
	if err != nil {
		return 0, nil, unexpectedEOF(err)
	}
	args = make([][]byte, n)
	for i := range args {
		a, err := readBytes(br)
		if err != nil {
			return 0, nil, unexpectedEOF(err)
		}
		args[i] = a
	}
	return int(d), args, nil
}
