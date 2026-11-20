package sysmem

import (
	"math"
	"strconv"
	"strings"
)

const kibibyte = 1024

// memTotal returns the MemTotal line of a /proc/meminfo text in bytes.
func memTotal(meminfo string) (int64, bool) {
	for line := range strings.SplitSeq(meminfo, "\n") {
		rest, found := strings.CutPrefix(line, "MemTotal:")
		if !found {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) != 2 || fields[1] != "kB" {
			return 0, false
		}
		n, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil || n <= 0 || n > math.MaxInt64/kibibyte {
			return 0, false
		}
		return n * kibibyte, true
	}
	return 0, false
}
