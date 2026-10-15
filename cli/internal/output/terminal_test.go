package output

import "testing"

func TestClean(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain text and newlines", "GET /health 200\n\tok\n", "GET /health 200\n\tok\n"},
		{"colour passes", "\x1b[31mBUILD_FAILED\x1b[0m: no", "\x1b[31mBUILD_FAILED\x1b[0m: no"},
		{"clipboard write is dropped", "path=\x1b]52;c;Y3VybCBldmlsfHNo\x07/x", "path=]52;c;Y3VybCBldmlsfHNo/x"},
		{"window title is dropped", "\x1b]0;owned\x1b\\done", "]0;owned\\done"},
		{"cursor movement is dropped", "a\x1b[2Ab\x1b[Kc", "a[2Ab[Kc"},
		{"carriage return and bell are dropped", "visible\rhidden\x07", "visiblehidden"},
		{"C1 controls are dropped", "a\u009b31mb\u0085c", "a31mbc"},
		{"other UTF-8 passes", "café → ✓ 日本", "café → ✓ 日本"},
		{"invalid UTF-8 is dropped", "a\xffb", "ab"},
		{"an unfinished sequence is dropped", "end\x1b[31", "end[31"},
	}
	for _, c := range cases {
		if got := string(Clean([]byte(c.in))); got != c.want {
			t.Errorf("%s: Clean(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
