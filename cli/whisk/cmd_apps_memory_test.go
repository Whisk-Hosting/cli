package whisk

import (
	"strings"
	"testing"
	"time"

	"github.com/whisk-run/cli/internal/api"
)

func TestMemoryLines(t *testing.T) {
	now := time.Date(2026, 9, 29, 6, 0, 0, 0, time.UTC)
	killed := now.Add(-18 * time.Minute)
	cases := []struct {
		name string
		m    *api.AppMemory
		want []string
	}{
		{"nothing reported", nil, nil},
		{"comfortable", &api.AppMemory{LimitBytes: 256 << 20, UsedBytes: 55 << 20, PeakBytes: 90 << 20}, []string{"memory 55 MiB now, peak 90 MiB of 256 MiB (35%); /tmp counts"}},
		{"near the limit", &api.AppMemory{LimitBytes: 256 << 20, UsedBytes: 55 << 20, PeakBytes: 220 << 20}, []string{"peak 220 MiB of 256 MiB (85%)", "near the memory limit"}},
		{"killed", &api.AppMemory{LimitBytes: 256 << 20, UsedBytes: 52 << 20, PeakBytes: 256 << 20, LastOutOfMemoryAt: &killed}, []string{"(100%)", "killed for memory 18m0s ago (APP_OUT_OF_MEMORY)"}},
	}
	for _, c := range cases {
		got := strings.Join(memoryLines(c.m, now), "\n")
		if len(c.want) == 0 && got != "" {
			t.Errorf("%s: got %q", c.name, got)
		}
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: %q lacks %q", c.name, got, w)
			}
		}
	}
}
