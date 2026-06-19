package persistence

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/avinashpathak/memcore/internal/value"
)

// Recover restores state from the latest snapshot and replays the append-log
// segments at or above the generation the snapshot covers. restore receives each
// snapshot record; replay receives each logged command with its database index.
// It returns how many snapshot records were loaded and how many log commands
// were replayed. A truncated trailing record, the mark of a crash mid-append, is
// tolerated and simply ends replay of that segment.
func Recover(dir string, limits value.Limits, restore func(Record) error, replay func(db int, args [][]byte) error) (loaded, replayed int, err error) {
	baseGen, loaded, err := loadSnapshot(dir, limits, restore)
	if err != nil {
		return loaded, 0, err
	}
	replayed, err = replaySegments(dir, baseGen, replay)
	return loaded, replayed, err
}

func loadSnapshot(dir string, limits value.Limits, restore func(Record) error) (baseGen, loaded int, err error) {
	f, err := os.Open(filepath.Join(dir, snapshotName))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	defer f.Close()
	baseGen, err = ReadSnapshot(f, limits, func(rec Record) error {
		loaded++
		return restore(rec)
	})
	return baseGen, loaded, err
}

func replaySegments(dir string, baseGen int, replay func(db int, args [][]byte) error) (int, error) {
	highest := highestSegment(dir)
	total := 0
	for gen := baseGen; gen <= highest; gen++ {
		n, err := replaySegment(dir, gen, replay)
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func replaySegment(dir string, gen int, replay func(db int, args [][]byte) error) (int, error) {
	f, err := os.Open(filepath.Join(dir, fmt.Sprintf("%s.%d", appendName, gen)))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	defer f.Close()

	br := bufio.NewReader(f)
	replayed := 0
	for {
		db, args, err := readCommand(br)
		switch {
		case errors.Is(err, io.EOF):
			return replayed, nil
		case errors.Is(err, io.ErrUnexpectedEOF):
			// A partial trailing record left by a crash mid-append; stop here.
			return replayed, nil
		case err != nil:
			return replayed, err
		}
		if err := replay(db, args); err != nil {
			return replayed, err
		}
		replayed++
	}
}
