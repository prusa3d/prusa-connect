package winservice_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prusa3d/prusa-connect/internal/kernel/winservice"
)

func TestLogFileMovesAsideBeforeTheLimit(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "logs", "server.log")
	log, err := winservice.OpenLog(path, 10, 2)
	require.NoError(t, err)

	for _, line := range []string{"aaaa\n", "bbbb\n", "cccc\n", "dddd\n", "eeee\n"} {
		written, writeErr := log.Write([]byte(line))
		require.NoError(t, writeErr)
		require.Equal(t, len(line), written)
	}
	require.NoError(t, log.Close())

	require.Equal(t, "eeee\n", read(t, path))
	require.Equal(t, "cccc\ndddd\n", read(t, path+".1"))
	require.Equal(t, "aaaa\nbbbb\n", read(t, path+".2"))
	require.NoFileExists(t, path+".3", "a log older than keep files is deleted")
}

func TestLogFileKeepsALineLargerThanTheLimitWhole(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "server.log")
	log, err := winservice.OpenLog(path, 4, 1)
	require.NoError(t, err)

	_, err = log.Write([]byte("a line longer than four bytes\n"))
	require.NoError(t, err)
	_, err = log.Write([]byte("next\n"))
	require.NoError(t, err)
	require.NoError(t, log.Close())

	require.Equal(t, "a line longer than four bytes\n", read(t, path+".1"))
	require.Equal(t, "next\n", read(t, path))
}

func TestLogFileAppendsAndStartsBelowTheLimit(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		existing string
		current  string
		previous string
	}{
		{name: "below the limit appends", existing: "old\n", current: "old\nnew\n"},
		{name: "at the limit moves aside at once", existing: "0123456789", current: "new\n", previous: "0123456789"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			path := filepath.Join(t.TempDir(), "server.log")
			require.NoError(t, os.WriteFile(path, []byte(tc.existing), 0o600))

			log, err := winservice.OpenLog(path, 10, 3)
			require.NoError(t, err)
			_, err = log.Write([]byte("new\n"))
			require.NoError(t, err)
			require.NoError(t, log.Close())

			require.Equal(t, tc.current, read(t, path))
			if tc.previous == "" {
				require.NoFileExists(t, path+".1")
			} else {
				require.Equal(t, tc.previous, read(t, path+".1"))
			}
		})
	}
}

func TestLogFileRefusesWritesAfterClose(t *testing.T) {
	t.Parallel()

	log, err := winservice.OpenLog(filepath.Join(t.TempDir(), "server.log"), 10, 1)
	require.NoError(t, err)
	require.NoError(t, log.Close())
	require.NoError(t, log.Close(), "a second close is harmless")

	_, err = log.Write([]byte("late\n"))
	require.ErrorIs(t, err, os.ErrClosed)
}

func TestOpenLogRefusesALimitOrKeepBelowOne(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		limit int64
		keep  int
	}{
		{name: "zero limit", limit: 0, keep: 1},
		{name: "negative limit", limit: -1, keep: 1},
		{name: "zero keep", limit: 10, keep: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := winservice.OpenLog(filepath.Join(t.TempDir(), "server.log"), tc.limit, tc.keep)
			require.Error(t, err)
		})
	}
}

// Every line written from several goroutines arrives whole, in one of the
// files, and none is lost to a rotation.
func TestLogFileLosesNoLineUnderConcurrentWrites(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "server.log")
	log, err := winservice.OpenLog(path, 64, 100)
	require.NoError(t, err)

	const writers, lines = 8, 50
	var group sync.WaitGroup
	for range writers {
		group.Go(func() {
			for range lines {
				_, writeErr := log.Write([]byte("0123456789\n"))
				assert.NoError(t, writeErr)
			}
		})
	}
	group.Wait()
	require.NoError(t, log.Close())

	matches, err := filepath.Glob(path + "*")
	require.NoError(t, err)

	total := 0
	for _, match := range matches {
		for _, line := range strings.SplitAfter(read(t, match), "\n") {
			if line == "" {
				continue
			}
			require.Equal(t, "0123456789\n", line)
			total++
		}
	}
	require.Equal(t, writers*lines, total)
}

func read(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(data)
}

// A log that cannot be moved aside, as when a virus scanner holds the older
// file, is written on past the limit rather than lost.
func TestLogFileWritesOnWhenItCannotMoveAside(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "server.log")
	blocker := path + ".1"
	require.NoError(t, os.MkdirAll(filepath.Join(blocker, "held"), 0o700))

	log, err := winservice.OpenLog(path, 4, 1)
	require.NoError(t, err)

	for _, line := range []string{"one\n", "two\n"} {
		_, err = log.Write([]byte(line))
		require.NoError(t, err)
	}
	require.NoError(t, log.Close())

	require.Equal(t, "one\ntwo\n", read(t, path))
	require.DirExists(t, blocker)
}
