package manifest

import (
	"reflect"
	"strings"
	"testing"

	"github.com/whisk-run/contract/redirects"
)

func TestRedirects(t *testing.T) {
	src := `whisk: 1
name: job-tracker
redirects:
  - from: /about-us
    to: /about
  - from: /blog/*
    to: /news/*
    status: 308
  - from: /old-promo
    status: 410
redirects_file: moved/redirects.txt
`
	m, err := Parse([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Redirects) != 3 || m.RedirectsFile != "moved/redirects.txt" || m.Redirects[1].Status != 308 {
		t.Fatalf("redirects = %+v %q", m.Redirects, m.RedirectsFile)
	}
	none, _ := Parse([]byte("whisk: 1\nname: job-tracker\n"))
	if none.Redirects == nil || len(none.Redirects) != 0 {
		t.Errorf("no redirects = %#v", none.Redirects)
	}

	with, problems := m.WithFile([]byte("# moved\n/contact.html /contact\n/team /about-us\n"))
	if len(problems) != 0 || len(with.Redirects) != 5 {
		t.Fatalf("WithFile = %+v %v", with.Redirects, problems)
	}
	if want := (redirects.Rule{From: "/contact.html", To: "/contact"}); !reflect.DeepEqual(with.Redirects[3], want) {
		t.Errorf("file rule = %+v", with.Redirects[3])
	}
	_, problems = m.WithFile([]byte("/about-us /x\n/about /about-us\nnot-a-rule\n"))
	if len(problems) != 1 || !strings.Contains(problems[0], "moved/redirects.txt line 3") {
		t.Errorf("a bad line = %v", problems)
	}
	_, problems = m.WithFile([]byte("/x /y\n/about-us /x\n"))
	if len(problems) != 1 || !strings.Contains(problems[0], "line 2: from /about-us is already redirected by redirect 0") {
		t.Errorf("a duplicate across manifest and file = %v", problems)
	}
	_, problems = m.WithFile([]byte("/about /about-us\n"))
	if len(problems) == 0 || !strings.Contains(strings.Join(problems, ";"), "loop") {
		t.Errorf("a loop across manifest and file = %v", problems)
	}
}

func TestRedirectsInvalid(t *testing.T) {
	cases := map[string]string{
		"redirects:\n  - from: about\n    to: /x\n":               "/redirects/0/from",
		"redirects:\n  - from: /a\n":                              "/redirects/0/to",
		"redirects:\n  - from: /a\n    to: /b\n    status: 303\n": "/redirects/0/status",
		"redirects:\n  - from: /a\n    too: /b\n":                 "/redirects/0/too",
		"redirects:\n  - from: /a\n    to: /a/\n":                 "/redirects/0/to",
		"redirects_file: ../redirects.txt\n":                      "/redirects_file",
		"redirects_file: /etc/redirects\n":                        "/redirects_file",
		"redirects_file: moved/\n":                                "/redirects_file",
	}
	for body, path := range cases {
		_, err := Parse([]byte("whisk: 1\nname: job-tracker\n" + body))
		ps, _ := err.(Problems)
		if len(ps) == 0 || ps[0].Path != path {
			t.Errorf("%q: problems %v, want one at %s", body, err, path)
		}
	}
}
