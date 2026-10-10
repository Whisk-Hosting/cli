package main

import "testing"

func TestNoticeBody(t *testing.T) {
	cases := []struct{ code, occurrence, detail, want string }{
		{"H107_NEEDS_SOMEONE", "one", "", `{"code":"H107_NEEDS_SOMEONE","occurrence":"one"}`},
		{"H107_NEEDS_SOMEONE", "two", "line\none", `{"code":"H107_NEEDS_SOMEONE","detail":"line\none","occurrence":"two"}`},
		{"", "", "", `{"code":"","occurrence":""}`},
		{`A"B`, "x", "", `{"code":"A\"B","occurrence":"x"}`},
	}
	for _, c := range cases {
		if got := string(noticeBody(c.code, c.occurrence, c.detail)); got != c.want {
			t.Errorf("noticeBody(%q, %q, %q) = %s, want %s", c.code, c.occurrence, c.detail, got, c.want)
		}
	}
}
