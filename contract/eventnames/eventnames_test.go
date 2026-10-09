package eventnames

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNamespaceAndBare(t *testing.T) {
	cases := []struct{ app, name, full string }{
		{"app_a", "po.created", "app_a/po.created"},
		{"app_a", "app_a/po.created", "app_a/po.created"},
		{"app_a", "whisk/approval.decided", "app_a/whisk/approval.decided"},
		{"app_a", "app_b/po.created", "app_a/app_b/po.created"},
		{"", "po.created", "po.created"},
	}
	for _, c := range cases {
		if got := Namespace(c.app, c.name); got != c.full {
			t.Errorf("Namespace(%q, %q) = %q, want %q", c.app, c.name, got, c.full)
		}
		if c.app != "" && c.name != c.full && Bare(c.app, c.full) != c.name {
			t.Errorf("Bare(%q, %q) = %q, want %q", c.app, c.full, Bare(c.app, c.full), c.name)
		}
	}
}

func TestStrip(t *testing.T) {
	cases := []struct {
		name, in, want string
		changed        bool
	}{
		{
			name:    "the run's event and batch",
			in:      `{"event":{"name":"app_a/po.created","data":{"id":1}},"events":[{"name":"app_a/po.created","data":{"id":1}}],"ctx":{"run_id":"r"}}`,
			want:    `{"ctx":{"run_id":"r"},"event":{"data":{"id":1},"name":"po.created"},"events":[{"data":{"id":1},"name":"po.created"}]}`,
			changed: true,
		},
		{
			name:    "an event a waitForEvent step resolved with",
			in:      `{"event":{"name":"app_a/feat.wait","data":{}},"steps":{"abc":{"type":"data","data":{"name":"app_a/feat.resume","data":{"answer":42},"id":"01J"}},"def":null}}`,
			want:    `{"event":{"data":{},"name":"feat.wait"},"steps":{"abc":{"data":{"data":{"answer":42},"id":"01J","name":"feat.resume"},"type":"data"},"def":null}}`,
			changed: true,
		},
		{
			name:    "the event behind inngest/function.failed",
			in:      `{"event":{"name":"inngest/function.failed","data":{"function_id":"f","event":{"name":"app_a/po.created","data":{}}}}}`,
			want:    `{"event":{"data":{"event":{"data":{},"name":"po.created"},"function_id":"f"},"name":"inngest/function.failed"}}`,
			changed: true,
		},
		{
			name: "a step output with a name but no data is not an event",
			in:   `{"steps":{"abc":{"name":"app_a/report.pdf"}}}`,
			want: `{"steps":{"abc":{"name":"app_a/report.pdf"}}}`,
		},
		{
			name: "another app's prefix is left alone",
			in:   `{"event":{"name":"app_b/po.created","data":{}}}`,
			want: `{"event":{"name":"app_b/po.created","data":{}}}`,
		},
		{
			name:    "large numbers survive exactly",
			in:      `{"event":{"name":"app_a/x","data":{"id":12345678901234567890,"f":1.50}}}`,
			want:    `{"event":{"data":{"f":1.50,"id":12345678901234567890},"name":"x"}}`,
			changed: true,
		},
		{
			name:    "no HTML escaping",
			in:      `{"event":{"name":"app_a/x","data":{"s":"<a>&"}}}`,
			want:    `{"event":{"data":{"s":"<a>&"},"name":"x"}}`,
			changed: true,
		},
		{
			name:    "a run started by hand reaches the function as a scheduled run",
			in:      `{"event":{"name":"app_a/whisk/cron.run.training-reminders","data":{"cron":"0 9 * * *"}},"events":[{"name":"app_a/whisk/cron.run.training-reminders","data":{"cron":"0 9 * * *"}}]}`,
			want:    `{"event":{"data":{"cron":"0 9 * * *"},"name":"inngest/scheduled.timer"},"events":[{"data":{"cron":"0 9 * * *"},"name":"inngest/scheduled.timer"}]}`,
			changed: true,
		},
		{name: "not JSON", in: `nope`, want: `nope`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, changed := Strip("app_a", []byte(c.in))
			if string(got) != c.want || changed != c.changed {
				t.Errorf("Strip = %s (%v), want %s (%v)", got, changed, c.want, c.changed)
			}
		})
	}
}

// TestScheduled pins the cron-run rename for an app that writes prefixed names, whose calls are
// not stripped: the TypeScript SDK refuses a run whose event its function never declared.
func TestScheduled(t *testing.T) {
	cases := []struct {
		name, in, want string
		changed        bool
	}{
		{
			name:    "prefixed cron-run event",
			in:      `{"event":{"name":"app_a/whisk/cron.run.app-nightly","data":{"cron":"0 6 * * *"}}}`,
			want:    `{"event":{"data":{"cron":"0 6 * * *"},"name":"inngest/scheduled.timer"}}`,
			changed: true,
		},
		{
			name: "any other event is left alone",
			in:   `{"event":{"name":"app_a/po.created","data":{}}}`,
			want: `{"event":{"name":"app_a/po.created","data":{}}}`,
		},
		{
			name: "another app's cron-run event is left alone",
			in:   `{"event":{"name":"app_b/whisk/cron.run.x","data":{}}}`,
			want: `{"event":{"name":"app_b/whisk/cron.run.x","data":{}}}`,
		},
	}
	for _, c := range cases {
		got, changed := Scheduled("app_a", []byte(c.in))
		if string(got) != c.want || changed != c.changed {
			t.Errorf("%s: Scheduled = %s (%v), want %s (%v)", c.name, got, changed, c.want, c.changed)
		}
	}
}

func TestPrefixWaits(t *testing.T) {
	cases := []struct {
		name, in, want string
		changed        bool
	}{
		{
			name:    "the TypeScript SDK's opcode",
			in:      `[{"displayName":"resume","op":"WaitForEvent","id":"a2b4","name":"feat.resume","opts":{"timeout":"5m","if":"event.data.marker == async.data.marker"}}]`,
			want:    `[{"displayName":"resume","id":"a2b4","name":"app_a/feat.resume","op":"WaitForEvent","opts":{"if":"event.data.marker == async.data.marker","timeout":"5m"}}]`,
			changed: true,
		},
		{
			name:    "the Go SDK's opcode names the event in opts.event",
			in:      `[{"op":"WaitForEvent","id":"a","name":"resume","opts":{"event":"feat.resume","timeout":"1h"}}]`,
			want:    `[{"id":"a","name":"resume","op":"WaitForEvent","opts":{"event":"app_a/feat.resume","timeout":"1h"}}]`,
			changed: true,
		},
		{
			name: "an already named wait is unchanged",
			in:   `[{"op":"WaitForEvent","id":"a","name":"app_a/feat.resume","opts":{}}]`,
			want: `[{"op":"WaitForEvent","id":"a","name":"app_a/feat.resume","opts":{}}]`,
		},
		{
			name:    "another app's event becomes this app's",
			in:      `[{"op":"WaitForEvent","id":"a","name":"app_b/po.created","opts":{}}]`,
			want:    `[{"id":"a","name":"app_a/app_b/po.created","op":"WaitForEvent","opts":{}}]`,
			changed: true,
		},
		{
			name:    "the engine's own events are this app's too",
			in:      `[{"op":"WaitForEvent","id":"a","name":"inngest/function.finished","opts":{}}]`,
			want:    `[{"id":"a","name":"app_a/inngest/function.finished","op":"WaitForEvent","opts":{}}]`,
			changed: true,
		},
		{
			name: "other opcodes are untouched",
			in:   `[{"op":"StepRun","id":"a","name":"feat.resume","data":{}},{"op":"Sleep","id":"b","name":"1h"}]`,
			want: `[{"op":"StepRun","id":"a","name":"feat.resume","data":{}},{"op":"Sleep","id":"b","name":"1h"}]`,
		},
		{name: "a function's return value", in: `{"doubled":42}`, want: `{"doubled":42}`},
		{name: "not JSON", in: `ok`, want: `ok`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, changed := PrefixWaits("app_a", []byte(c.in))
			if string(got) != c.want || changed != c.changed {
				t.Errorf("PrefixWaits = %s (%v), want %s (%v)", got, changed, c.want, c.changed)
			}
		})
	}
}

func TestPrefixCheckpointWaits(t *testing.T) {
	in := `{"run_id":"r","fn_id":"f","steps":[{"op":"StepRun","name":"record","data":1},{"op":"WaitForEvent","name":"feat.resume","opts":{}}]}`
	got, changed := PrefixCheckpointWaits("app_a", []byte(in))
	var v struct {
		Steps []map[string]any `json:"steps"`
	}
	if !changed || json.Unmarshal(got, &v) != nil {
		t.Fatalf("PrefixCheckpointWaits = %s (%v)", got, changed)
	}
	names := []any{v.Steps[0]["name"], v.Steps[1]["name"]}
	if !reflect.DeepEqual(names, []any{"record", "app_a/feat.resume"}) {
		t.Errorf("step names %v", names)
	}
	if _, changed := PrefixCheckpointWaits("app_a", []byte(`{"run_id":"r","steps":[{"op":"StepRun","name":"x"}]}`)); changed {
		t.Error("a checkpoint with no wait changed")
	}
}
