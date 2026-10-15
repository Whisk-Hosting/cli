package stack

import "testing"

func TestIsTest(t *testing.T) {
	cases := map[string]bool{
		"src/whisk.ts":            false,
		"src/whisk.test.ts":       true,
		"app/test_naughty.py":     true,
		"tests/conftest.py":       true,
		"internal/x/x_test.go":    true,
		"fuzz/run.mjs":            true,
		"fuzz/targets/whisk.ts":   true,
		"src/fuzzy/match.ts":      false,
		"src/lib/fuzz-helpers.ts": false,
	}
	for p, want := range cases {
		if got := IsTest(p); got != want {
			t.Errorf("IsTest(%q) = %v, want %v", p, got, want)
		}
	}
}
