package dev

import (
	"strings"
	"testing"

	"github.com/whisk-run/contract/manifest"
)

// whisk dev's Valkey makes room the way the app's cache on Whisk does (kv_evict, SKILL.md §9),
// so a queue library that warns about eviction warns here first, and changing it makes a new
// compose file.
func TestDevCacheHasTheAppsEvictionPolicy(t *testing.T) {
	ports := Ports{Postgres: 1, PgBouncer: 2, Inngest: 3, Valkey: 4, Storage: 5, Console: 6}
	cases := []struct {
		yaml, want string
	}{
		{"kv: true\n", `"--maxmemory-policy", "allkeys-lru"`},
		{"kv: true\nkv_evict: false\n", `"--maxmemory-policy", "noeviction"`},
	}
	hashes := map[string]bool{}
	for _, tc := range cases {
		m, err := manifest.Parse([]byte("whisk: 1\nname: shop\n" + tc.yaml))
		if err != nil {
			t.Fatal(err)
		}
		if c := BuildPlan(m, ports, 3002, false).Compose; !strings.Contains(c, tc.want) {
			t.Errorf("%q: compose lacks %s:\n%s", tc.yaml, tc.want, c)
		}
		hashes[Hash(m, ports, 3002)] = true
	}
	if len(hashes) != len(cases) {
		t.Error("changing kv_evict keeps the old compose file")
	}
}
