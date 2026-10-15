package naughty

import "testing"

func TestPick(t *testing.T) {
	for _, c := range []struct {
		full bool
		min  int
		max  int
	}{
		{true, 500 + len(Extra), 1 << 20},
		{false, len(Extra) + 10, len(Extra) + len(list)/sampleEvery + 1},
	} {
		got := len(pick(c.full))
		if got < c.min || got > c.max {
			t.Errorf("pick(%v) gave %d strings, want %d to %d", c.full, got, c.min, c.max)
		}
	}
}

func TestStringsIsACopy(t *testing.T) {
	s := Strings()
	s[0] = "changed"
	if Strings()[0] == "changed" {
		t.Fatal("Strings must return a copy")
	}
}
