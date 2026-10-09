package manifest

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/whisk-run/contract/naughty"
)

// Fuzz targets for whisk.yaml and cron (docs/HARNESS.md §8.3). The seeds are every fixture and
// every naughty string, as the whole file and as the app's name; they run with go test.

func FuzzParse(f *testing.F) {
	for _, dir := range []string{"valid", "invalid"} {
		files, _ := filepath.Glob("../fixtures/manifests/" + dir + "/*.yaml")
		for _, file := range files {
			b, err := os.ReadFile(file)
			if err != nil {
				f.Fatal(err)
			}
			f.Add(b)
		}
	}
	for _, s := range naughty.Strings() {
		f.Add([]byte(s))
		f.Add([]byte("whisk: 1\nname: " + strconv.Quote(s) + "\n"))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		m, err := Parse(src)
		if err != nil {
			var ps Problems
			if !errors.As(err, &ps) || len(ps) == 0 {
				t.Fatalf("error is not Problems: %v", err)
			}
			for _, p := range ps {
				if !manifestCodes[p.Code] || p.Message == "" {
					t.Fatalf("problem without a manifest code or message: %+v", p)
				}
			}
			return
		}
		if !reNaughtySlug.MatchString(m.Name) {
			t.Fatalf("accepted name %q", m.Name)
		}
		if !reNaughtyPath.MatchString(m.Health.Path) {
			t.Fatalf("accepted health path %q", m.Health.Path)
		}
		for _, fn := range m.Functions {
			if fn.Cron != "" && CheckCron(fn.Cron) != nil {
				t.Fatalf("accepted cron %q", fn.Cron)
			}
		}
		// What Parse accepted, written back out with its defaults, parses to the same manifest.
		again, err := json.Marshal(m)
		if err != nil {
			t.Fatalf("accepted manifest does not marshal: %v", err)
		}
		m2, err := Parse(again)
		if err != nil {
			t.Fatalf("accepted manifest does not parse again: %v\n%s", err, again)
		}
		if !reflect.DeepEqual(m, m2) {
			t.Fatalf("round trip changed the manifest:\n%+v\n%+v", m, m2)
		}
	})
}

func FuzzCron(f *testing.F) {
	for _, s := range []string{"* * * * *", "*/5 * * * *", "0 9 * * 1-5", "0 0 29 2 *", "0 0 31 4 *", "59 23 31 12 7", "1-59/7 0-23/5 1-31/9 */4 0,3,7", "0 2 * * *", "30 1 * * 0"} {
		f.Add(s, int64(1788782400), 15, "UTC")
	}
	for _, s := range naughty.Strings() {
		f.Add(s, int64(0), 60, "Pacific/Auckland")
	}
	f.Add("30 2 * * *", int64(1790949600), 15, "Pacific/Auckland")
	f.Add("30 1 * * *", int64(1775311200), 15, "America/New_York")
	f.Fuzz(func(t *testing.T, expr string, after int64, minMinutes int, zone string) {
		loc, err := time.LoadLocation(zone)
		if err != nil || zone == "" || zone == "Local" {
			loc = time.UTC
		}
		// Keep the clock within years the platform can see.
		at := time.Unix(1_600_000_000+after%(40*365*86400), 0)
		valid := CheckCron(expr) == nil
		next, ok := NextCron(expr, loc, at)
		if ok && !valid {
			t.Fatalf("NextCron answered for invalid %q", expr)
		}
		if ok {
			if !next.After(at) {
				t.Fatalf("%q after %v gave %v, not later", expr, at, next)
			}
			sets, _ := parseCronSets(expr)
			n := next.In(loc)
			if n.Second() != 0 || !sets.minute[n.Minute()] || !sets.hour[n.Hour()] || !sets.month[int(n.Month())] || !sets.dayMatches(n) {
				t.Fatalf("%q fired at %v which it does not name", expr, n)
			}
			if again, ok := NextCron(expr, loc, next); ok && !again.After(next) {
				t.Fatalf("%q after %v gave %v", expr, next, again)
			}
		}
		spaced, changed := SpacedCron(expr, minMinutes)
		if !changed {
			if spaced != expr {
				t.Fatalf("SpacedCron changed %q to %q without saying so", expr, spaced)
			}
			return
		}
		if !valid && CheckCron(spaced) != nil {
			// A TZ= prefix is kept in front; what follows it must check.
			if _, rest, ok := cutTZ(spaced); !ok || CheckCron(rest) != nil {
				t.Fatalf("SpacedCron(%q) = %q, which does not check", expr, spaced)
			}
		}
		if again, changed := SpacedCron(spaced, minMinutes); changed {
			t.Fatalf("SpacedCron is not idempotent: %q -> %q -> %q", expr, spaced, again)
		}
	})
}

func cutTZ(expr string) (string, string, bool) {
	for i := 0; i < len(expr); i++ {
		if expr[i] == ' ' {
			return expr[:i], expr[i+1:], true
		}
	}
	return "", expr, false
}
