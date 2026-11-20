package sysmem

import "testing"

func TestMemTotal(t *testing.T) {
	tests := []struct {
		name    string
		meminfo string
		want    int64
		ok      bool
	}{
		{
			name:    "host",
			meminfo: "MemTotal:       16303520 kB\nMemFree:         1253648 kB\n",
			want:    16303520 * 1024, ok: true,
		},
		{
			name:    "lxcfs container",
			meminfo: "MemTotal:        2097152 kB\nMemFree:         2000000 kB\nMemAvailable:    2050000 kB\n",
			want:    2147483648, ok: true,
		},
		{name: "no MemTotal line", meminfo: "MemFree:         1253648 kB\n"},
		{name: "unknown unit", meminfo: "MemTotal:       16303520 MB\n"},
		{name: "not a number", meminfo: "MemTotal:       lots kB\n"},
		{name: "too large", meminfo: "MemTotal:       9223372036854775807 kB\n"},
		{name: "empty"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := memTotal(tc.meminfo)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("got %d, %v; want %d, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}
