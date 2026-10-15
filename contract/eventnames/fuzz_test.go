package eventnames

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/whisk-run/contract/naughty"
)

// FuzzRewrite (docs/HARNESS.md §8.3) feeds every rewrite a fuzzed body and app id: an
// unchanged answer is the same bytes, a changed one is valid JSON, each rewrite is idempotent,
// (for an app id that JSON can carry), and a name that went through Namespace comes back with Bare.
func FuzzRewrite(f *testing.F) {
	const app = "01J8Z3WQ9XN4M6P8R0T2V4X6Y8"
	f.Add(app, []byte(`{"event":{"name":"`+app+`/po.created","data":{}}}`))
	f.Add(app, []byte(`[{"op":"WaitForEvent","name":"x","opts":{"event":"po.approved"}}]`))
	f.Add(app, []byte(`{"steps":[{"op":"WaitForEvent","name":"po.approved"}]}`))
	f.Add(app, []byte(`[{"OP":"waitforevent","Name":"other/po.created","oPts":{"EVENT":"other/x"}}]`))
	f.Add(app, []byte(`{"Steps":[{"op":"WaitForEvent","name":"a","NAME":"b"}]}`))
	for _, s := range naughty.Strings() {
		name, _ := json.Marshal(s)
		f.Add(app, []byte(s))
		f.Add(s, []byte(`[{"op":"WaitForEvent","name":`+string(name)+`}]`))
		f.Add(app, []byte(`{"event":{"name":`+string(name)+`,"data":{"n":1e400}}}`))
	}
	type rewrite func(string, []byte) ([]byte, bool)
	f.Fuzz(func(t *testing.T, appID string, body []byte) {
		for fn, rw := range map[string]rewrite{"Strip": Strip, "PrefixWaits": PrefixWaits, "PrefixCheckpointWaits": PrefixCheckpointWaits} {
			out, changed := rw(appID, body)
			if !changed {
				if !bytes.Equal(out, body) {
					t.Fatalf("%s changed the body without saying so", fn)
				}
				continue
			}
			if appID == "" {
				t.Fatalf("%s changed a body without an app", fn)
			}
			if !json.Valid(out) {
				t.Fatalf("%s wrote invalid JSON: %s", fn, out)
			}
			if again, changed := rw(appID, out); changed && fn != "Strip" && utf8.ValidString(appID) {
				t.Fatalf("%s is not idempotent:\n%s\n%s", fn, out, again)
			}
		}
		if appID == app {
			out, _ := PrefixWaits(app, body)
			engineWaits(t, out)
			if cp, _ := PrefixCheckpointWaits(app, body); json.Valid(cp) {
				var call map[string]json.RawMessage
				if json.Unmarshal(cp, &call) == nil {
					for k, v := range call {
						if strings.EqualFold(k, "steps") {
							engineWaits(t, v)
						}
					}
				}
			}
		}
		s := string(body)
		ns := Namespace(appID, s)
		if Namespace(appID, ns) != ns {
			t.Fatalf("Namespace is not idempotent on %q", s)
		}
		if appID != "" && !strings.HasPrefix(ns, appID+"/") {
			t.Fatalf("Namespace(%q) has no prefix", s)
		}
		if appID != "" && !strings.HasPrefix(s, appID+"/") && Bare(appID, ns) != s {
			t.Fatalf("Bare(Namespace(%q)) = %q", s, Bare(appID, ns))
		}
	})
}

// engineWaits reads opcodes the way the engine does, field names without case and the opcode
// by any case, and fails on a wait for an event outside the app.
func engineWaits(t *testing.T, body []byte) {
	t.Helper()
	const app = "01J8Z3WQ9XN4M6P8R0T2V4X6Y8"
	var ops []json.RawMessage
	if json.Unmarshal(body, &ops) != nil {
		return
	}
	for _, raw := range ops {
		var op struct {
			Op   string          `json:"op"`
			Name string          `json:"name"`
			Opts json.RawMessage `json:"opts"`
		}
		_ = json.Unmarshal(raw, &op)
		var opts struct {
			Event string `json:"event"`
		}
		_ = json.Unmarshal(op.Opts, &opts)
		target := opts.Event
		if target == "" {
			target = op.Name
		}
		if strings.EqualFold(op.Op, waitOp) && target != "" && !strings.HasPrefix(target, app+"/") {
			t.Fatalf("the engine reads a wait for %q in %s", target, body)
		}
	}
}
