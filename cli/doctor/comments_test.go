package doctor

import (
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/stack"
)

func TestBlankComments(t *testing.T) {
	cases := []struct {
		name string
		lang stack.Lang
		in   string
		want string
	}{
		{"cron string", stack.JS, `cron("*/3 * * * *"); step.run("count"); /* gone */ x`, `cron("*/3 * * * *"); step.run("count");  x`},
		{"glob then cron", stack.JS, "get(\"/files/*\");\nstep.run(\"count\");\nconst s = '*/5 * * * *';\n", "get(\"/files/*\");\nstep.run(\"count\");\nconst s = '*/5 * * * *';\n"},
		{"template literal", stack.JS, "const g = `src/**`; // note\nrun(\"a\")", "const g = `src/**`; \nrun(\"a\")"},
		{"url in string", stack.JS, `fetch("https://x.test/a") // call`, `fetch("https://x.test/a") `},
		{"escaped quote", stack.JS, `s = "a\"/*"; t = "b" // c`, `s = "a\"/*"; t = "b" `},
		{"block keeps lines", stack.JS, "a /* one\ntwo */ b", "a \n b"},
		{"line comment", stack.JS, "// \"x\"\nrun(\"y\")", "\nrun(\"y\")"},
		{"regex literal", stack.JS, "const r = /\\/*/; run(\"z\") /* c */", "const r = /\\/*/; run(\"z\") "},
		{"division", stack.JS, "a = b / c /* d */", "a = b / c "},
		{"unclosed quote stops at line end", stack.JS, "it's /* x */\nrun(\"q\")", "it's /* x */\nrun(\"q\")"},
		{"go raw string", stack.GO, "c := `*/3 * * * *` // every 3\nrun(\"r\")", "c := `*/3 * * * *` \nrun(\"r\")"},
		{"go rune", stack.GO, "c := '\\'' /* q */", "c := '\\'' "},
		{"python hash in string", stack.PY, "s = \"#1\"  # note\nrun('a')", "s = \"#1\"  \nrun('a')"},
		{"python triple quote", stack.PY, "\"\"\"doc # not a comment\n\"\"\"\n# gone\nx", "\"\"\"doc # not a comment\n\"\"\"\n\nx"},
		{"python cron", stack.PY, "cron = \"*/3 * * * *\"\nrun(\"count\")", "cron = \"*/3 * * * *\"\nrun(\"count\")"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := string(blankComments(c.lang, []byte(c.in)))
			if got != c.want {
				t.Errorf("blankComments(%q)\n got %q\nwant %q", c.in, got, c.want)
			}
			if strings.Count(got, "\n") != strings.Count(c.in, "\n") {
				t.Errorf("line count changed: %q", got)
			}
		})
	}
}
