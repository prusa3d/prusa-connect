package sysmem

import (
	"math"
	"os"

	"golang.org/x/sys/unix"
)

// physical uses sysinfo(2): /proc/meminfo is hidden when procfs is mounted
// with subset=pid. When /proc/meminfo can be read, a smaller MemTotal there
// caps the result. In an LXC container, such as one on Proxmox, lxcfs serves
// a /proc/meminfo with the container's memory limit, but sysinfo(2) still
// reports the host's memory. The limit is set on a cgroup above the
// container's cgroup namespace, so cgroupLimit cannot read it.
func physical() (int64, bool) {
	var si unix.Sysinfo_t
	if err := unix.Sysinfo(&si); err != nil {
		return 0, false
	}
	n := uint64(si.Totalram) * uint64(si.Unit) //nolint:unconvert // uint32 on 32-bit targets
	if n > math.MaxInt64 {
		return 0, false
	}
	total := int64(n)
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		if m, ok := memTotal(string(data)); ok && m < total {
			total = m
		}
	}
	return total, true
}
