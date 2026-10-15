package textenc

import "testing"

func TestDecode(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want string
	}{
		{"plain UTF-8", []byte("select 1"), "select 1"},
		{"UTF-8 with a byte-order mark", []byte("\xEF\xBB\xBFselect 1"), "select 1"},
		{"UTF-16 little-endian (PowerShell >)", []byte{0xFF, 0xFE, 's', 0, 'q', 0, 'l', 0, 0xE9, 0}, "sqlé"},
		{"UTF-16 big-endian", []byte{0xFE, 0xFF, 0, 'o', 0, 'k'}, "ok"},
		{"empty", nil, ""},
		{"a lone byte", []byte{0xFF}, "\xFF"},
	}
	for _, c := range cases {
		if got := string(Decode(c.in)); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
