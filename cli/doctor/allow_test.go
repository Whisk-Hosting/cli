package doctor

import (
	"strings"
	"testing"
)

func TestAllowedAt(t *testing.T) {
	src := []byte("a\n// doctor: allow W091 on purpose\nset cookie\nother // doctor: allow W023\nlast\n")
	cases := []struct {
		line int
		rule string
		want bool
	}{
		{3, "W091", true},
		{2, "W091", true},
		{4, "W023", true},
		{5, "W023", true},
		{1, "W091", false},
		{3, "W092", false},
		{9, "W091", false},
	}
	for _, c := range cases {
		if got := allowedAt(src, c.line, c.rule); got != c.want {
			t.Errorf("line %d %s: %v", c.line, c.rule, got)
		}
	}
}

// A warning allowed by a comment moves to skipped; an error cannot be allowed.
func TestAllowComment(t *testing.T) {
	rep := runDoctor(t, with(passing, map[string]string{"src/poll.ts": "// doctor: allow W023 a progress bar in a CLI tool\nsetInterval(tick, 1000);\n"}), false)
	if has(rep, "W023") || !strings.Contains(strings.Join(rep.Skipped, "\n"), "W023: allowed at src/poll.ts:2") {
		t.Fatalf("findings %+v skipped %v", rep.Findings, rep.Skipped)
	}
	rep = runDoctor(t, with(passing, map[string]string{"src/config.ts": "// doctor: allow W010\nexport const key = \"sk_live_" + strings.Repeat("a1b2c3d4", 4) + "\";\n"}), false)
	if !has(rep, "W010") {
		t.Fatalf("an error was allowed: %+v", rep.Findings)
	}
}
