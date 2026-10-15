package output

import "testing"

func TestSeconds(t *testing.T) {
	cases := map[int64]string{
		7312:  "7.3 s",
		1000:  "1.0 s",
		950:   "1.0 s",
		949:   "0.9 s",
		100:   "0.1 s",
		40:    "0.1 s",
		0:     "0.1 s",
		-5:    "0.1 s",
		59999: "60.0 s",
		12345: "12.3 s",
		12350: "12.4 s",
	}
	for ms, want := range cases {
		if got := Seconds(ms); got != want {
			t.Errorf("Seconds(%d) = %q, want %q", ms, got, want)
		}
	}
}
