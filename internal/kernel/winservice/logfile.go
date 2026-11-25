package winservice

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

// LogFile is a log file that moves itself aside when it grows past a size, so
// a server that runs for months keeps a bounded amount of log on disk. A
// service has no terminal, so this file is where its log is read.
//
// The file at path is the current one. path.1 is the one before it, up to
// path.<keep>, and an older file is deleted. Safe for use from several
// goroutines.
type LogFile struct {
	mu    sync.Mutex
	path  string
	limit int64
	keep  int
	file  *os.File
	size  int64
}

// OpenLog opens the log at path for appending, and creates its folder. A file
// is moved aside before a write that would take it past limit bytes. A file
// that already holds limit bytes or more is moved aside at once, so each start
// begins below the limit.
func OpenLog(path string, limit int64, keep int) (*LogFile, error) {
	if limit <= 0 || keep < 1 {
		return nil, fmt.Errorf("open log %s: limit %d and keep %d must be positive", path, limit, keep)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create the log folder: %w", err)
	}

	log := &LogFile{path: path, limit: limit, keep: keep}
	if err := log.open(); err != nil {
		return nil, err
	}

	// A file that could not be moved aside stays the current one, past the
	// limit, and the first write tries again.
	if log.size >= limit {
		if err := log.rotate(); err != nil && log.file == nil {
			return nil, err
		}
	}

	return log, nil
}

// Write appends p, moving the file aside first when p would take it past the
// limit. One write larger than the limit goes into a file of its own whole, so
// a line is never split across two files.
func (l *LogFile) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return 0, os.ErrClosed
	}

	// A file that could not be moved aside, such as one a virus scanner holds
	// open, is written on past the limit. Losing the log is worse than a
	// large file, and the next write tries again.
	if l.size > 0 && l.size+int64(len(p)) > l.limit {
		if err := l.rotate(); err != nil && l.file == nil {
			return 0, err
		}
	}

	written, err := l.file.Write(p)
	l.size += int64(written)

	return written, err
}

// Close closes the current file. A write after Close returns os.ErrClosed.
func (l *LogFile) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return nil
	}

	err := l.file.Close()
	l.file = nil

	return err
}

func (l *LogFile) open() error {
	file, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open the log: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		return errors.Join(fmt.Errorf("read the size of the log: %w", err), file.Close())
	}

	l.file, l.size = file, info.Size()

	return nil
}

// rotate closes the current file before it renames it, because Windows refuses
// to rename a file that is open. Whatever the renames do, it opens the file at
// path again, so a failed move leaves the log open. It returns the first error
// it met, and l.file is nil only when the file could not be opened again.
func (l *LogFile) rotate() error {
	closeErr := l.file.Close()
	l.file = nil

	var moveErr error
	if closeErr != nil {
		moveErr = fmt.Errorf("close the log before moving it aside: %w", closeErr)
	} else {
		moveErr = l.moveAside()
	}

	return errors.Join(moveErr, l.open())
}

func (l *LogFile) moveAside() error {
	if err := os.Remove(l.numbered(l.keep)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete the oldest log: %w", err)
	}

	for index := l.keep - 1; index >= 1; index-- {
		err := os.Rename(l.numbered(index), l.numbered(index+1))
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("move an older log aside: %w", err)
		}
	}

	if err := os.Rename(l.path, l.numbered(1)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("move the log aside: %w", err)
	}

	return nil
}

func (l *LogFile) numbered(index int) string {
	return l.path + "." + strconv.Itoa(index)
}
