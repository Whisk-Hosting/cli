package manifest

import (
	"testing"
	"time"
)

func TestNextCron(t *testing.T) {
	utc := time.UTC
	after := time.Date(2026, 9, 7, 12, 34, 56, 0, utc)
	syd, _ := time.LoadLocation("Australia/Sydney")
	cases := []struct {
		expr string
		loc  *time.Location
		want string
	}{
		{"0 6 * * *", utc, "2026-09-08T06:00:00Z"},
		{"* * * * *", utc, "2026-09-07T12:35:00Z"},
		{"*/15 * * * *", utc, "2026-09-07T12:45:00Z"},
		{"30 12 * * *", utc, "2026-09-08T12:30:00Z"},
		{"0 0 1 * *", utc, "2026-10-01T00:00:00Z"},
		{"0 9 * * 1", utc, "2026-09-14T09:00:00Z"},     // next Monday
		{"0 9 * * 7", utc, "2026-09-13T09:00:00Z"},     // 7 is Sunday
		{"0 9 15 * 1", utc, "2026-09-14T09:00:00Z"},    // dom OR dow when both restricted
		{"0 0 29 2 *", utc, "2028-02-29T00:00:00Z"},    // next leap day
		{"0 6 * * *", syd, "2026-09-07T20:00:00Z"},     // 06:00 Sydney the next day is 20:00 UTC today
		{"0 12-14 * * *", utc, "2026-09-07T13:00:00Z"}, // range
		{"5,10 1 * * *", utc, "2026-09-08T01:05:00Z"},  // list
	}
	for _, c := range cases {
		got, ok := NextCron(c.expr, c.loc, after)
		if !ok {
			t.Errorf("%q: no next run", c.expr)
			continue
		}
		if s := got.UTC().Format(time.RFC3339); s != c.want {
			t.Errorf("%q: got %s, want %s", c.expr, s, c.want)
		}
	}
	if _, ok := NextCron("0 0 31 2 *", utc, after); ok {
		t.Error("31 February should never fire")
	}
	if _, ok := NextCron("bad", utc, after); ok {
		t.Error("invalid expression should not fire")
	}
}

func TestSpacedCron(t *testing.T) {
	cases := []struct {
		in      string
		min     int
		want    string
		changed bool
	}{
		{"*/10 5-21 * * 1-5", 60, "0 5-21 * * 1-5", true},
		{"*/30 * * * *", 60, "0 * * * *", true},
		{"* * * * *", 60, "0 * * * *", true},
		{"15,45 * * * *", 60, "15 * * * *", true},
		{"30 8 * * *", 60, "30 8 * * *", false},
		{"7 */2 * * *", 60, "7 */2 * * *", false},
		{"*/5 * * * *", 15, "0,15,30,45 * * * *", true},
		{"*/10 5-21 * * 1-5", 15, "0,20,40 5-21 * * 1-5", true},
		{"*/15 * * * *", 15, "*/15 * * * *", false},
		{"*/30 * * * *", 15, "*/30 * * * *", false},
		{"0,50 * * * *", 15, "0 * * * *", true},
		{"5-10 * * * *", 15, "5 * * * *", true},
		{"* * * * *", 15, "0,15,30,45 * * * *", true},
		{"*/5 * * * *", 90, "0 * * * *", true},
		{"*/5 * * * *", 1, "*/5 * * * *", false},
		{"TZ=Pacific/Auckland */10 5-21 * * 1-5", 60, "TZ=Pacific/Auckland 0 5-21 * * 1-5", true},
		{"CRON_TZ=UTC 30 8 * * *", 15, "CRON_TZ=UTC 30 8 * * *", false},
		{"bad", 15, "bad", false},
	}
	for _, c := range cases {
		got, changed := SpacedCron(c.in, c.min)
		if got != c.want || changed != c.changed {
			t.Errorf("SpacedCron(%q, %d) = %q, %v; want %q, %v", c.in, c.min, got, changed, c.want, c.changed)
		}
	}
}
