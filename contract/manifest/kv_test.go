package manifest

import (
	"errors"
	"testing"
)

// A cache evicts unless the app sets kv_evict: false, which keeps every key for a job queue
// such as BullMQ (SKILL.md §9), and kv_evict means nothing without kv: true.
func TestKVEvict(t *testing.T) {
	cases := []struct {
		name, yaml string
		keeps      bool
		problem    string
	}{
		{"a cache evicts by default", "kv: true\n", false, ""},
		{"evicting said out loud", "kv: true\nkv_evict: true\n", false, ""},
		{"a queue's cache keeps every key", "kv: true\nkv_evict: false\n", true, ""},
		{"no cache", "", false, ""},
		{"kv_evict without a cache", "kv_evict: false\n", false, "/kv_evict"},
		{"kv_evict is a yes or no", "kv: true\nkv_evict: queue\n", false, "/kv_evict"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := Parse([]byte("whisk: 1\nname: shop\n" + tc.yaml))
			if tc.problem != "" {
				var ps Problems
				if !errors.As(err, &ps) || len(ps) == 0 || ps[0].Path != tc.problem {
					t.Fatalf("got %v, want a problem at %s", err, tc.problem)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if m.KeepsKVKeys() != tc.keeps {
				t.Errorf("keeps every key %v, want %v", m.KeepsKVKeys(), tc.keeps)
			}
		})
	}
}
