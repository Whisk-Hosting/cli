package doctor

import (
	"testing"
)

func TestRedirectRules(t *testing.T) {
	manifest := passing["whisk.yaml"] + "redirects:\n  - from: /about-us\n    to: /about\nredirects_file: moved.txt\n"
	cases := []struct {
		name  string
		file  *string
		rule  string
		file0 string
		line  int
		msg   string
	}{
		{name: "missing", rule: "W110", file0: "whisk.yaml", msg: "redirects_file names moved.txt, which does not exist."},
		{name: "bad line", file: ptr("# old site\n/a /b\n/c\n"), rule: "W110", file0: "moved.txt", line: 3},
		{name: "duplicate", file: ptr("/about-us/ /x\n"), rule: "W110", file0: "moved.txt", line: 1, msg: "from /about-us/ is already redirected by redirect 0"},
		{name: "loop", file: ptr("/about /about-us\n"), rule: "W110", msg: "to redirects in a loop: /about-us to /about to /about-us"},
		{name: "chain", file: ptr("/team /about-us\n"), rule: "W111", file0: "moved.txt", line: 1, msg: "/team redirects to /about-us, which redirects to /about."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			files := with(passing, map[string]string{"whisk.yaml": manifest})
			if c.file != nil {
				files["moved.txt"] = *c.file
			}
			rep := runDoctor(t, files, false)
			var found bool
			for _, f := range rep.Findings {
				found = found || (f.Rule == c.rule && (c.file0 == "" || f.File == c.file0) && (c.line == 0 || f.Line == c.line) && (c.msg == "" || f.Message == c.msg))
			}
			if !found {
				t.Errorf("no %s finding as expected: %+v", c.rule, rep.Findings)
			}
		})
	}
	clean := with(passing, map[string]string{"whisk.yaml": manifest, "moved.txt": "/blog/* /news/*\n/old-promo 410\n"})
	if rep := runDoctor(t, clean, false); len(rep.Findings) != 0 {
		t.Errorf("clean redirects: %+v", rep.Findings)
	}
}

func ptr(s string) *string { return &s }
