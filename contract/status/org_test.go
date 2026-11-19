package status

import (
	"testing"
	"time"
)

// An app's status follows its latest event (App.Follows): one that happened before the event
// the status was decided by changes nothing, and nothing moves a deleted app.
func TestAppFollows(t *testing.T) {
	at := time.Unix(1_800_000_000, 0)
	for _, c := range []struct {
		name    string
		from    App
		to      App
		decided time.Time
		event   time.Time
		want    bool
	}{
		{"a status no event decided yet takes any", AppActive, AppSleeping, time.Time{}, at, true},
		{"a later sleep", AppActive, AppSleeping, at, at.Add(time.Minute), true},
		{"a wake reported after the sleep that followed it", AppSleeping, AppActive, at, at.Add(-time.Second), false},
		{"a sleep recorded after the wake that overtook it", AppActive, AppSleeping, at.Add(2 * time.Second), at, false},
		{"a wake after a container stopped", AppStopped, AppActive, at, at.Add(time.Second), true},
		{"a death older than the wake since", AppActive, AppStopped, at, at.Add(-time.Minute), false},
		{"the same second applies in the order it arrives", AppActive, AppSleeping, at.Add(700 * time.Millisecond), at.Add(200 * time.Millisecond), true},
		{"a deleted app takes nothing", AppDeleted, AppActive, at, at.Add(time.Hour), false},
		{"deleting follows any status", AppSleeping, AppDeleted, at, at.Add(time.Second), true},
		{"no status at all", AppActive, App("nonsense"), at, at.Add(time.Second), false},
	} {
		if got := c.from.Follows(c.to, c.decided, c.event); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
