package eventnames

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/whisk-run/contract/naughty"
)

// A naughty event name round-trips through the prefix, and bodies built from naughty text come
// back as valid JSON with only event names changed.
func TestNaughtyNames(t *testing.T) {
	const app = "01J8Z3WQ9XN4M6P8R0T2V4X6Y8"
	for _, s := range naughty.Strings() {
		ns := Namespace(app, s)
		if !strings.HasPrefix(ns, app+"/") {
			t.Errorf("Namespace(%q) = %q has no prefix", s, ns)
		}
		if Namespace(app, ns) != ns {
			t.Errorf("Namespace is not idempotent on %q", s)
		}
		if !strings.HasPrefix(s, app+"/") && Bare(app, ns) != s {
			t.Errorf("Bare(Namespace(%q)) = %q", s, Bare(app, ns))
		}
		if Namespace("", s) != s {
			t.Errorf("Namespace without an app changed %q", s)
		}
		if Namespace(s, "po.created") == "" {
			t.Errorf("Namespace(%q, po.created) is empty", s)
		}
	}
}

func TestNaughtyBodies(t *testing.T) {
	const app = "01J8Z3WQ9XN4M6P8R0T2V4X6Y8"
	for _, s := range naughty.Strings() {
		name, _ := json.Marshal(s)
		ns, _ := json.Marshal(app + "/" + s)
		bodies := map[string][]byte{
			"raw":   []byte(s),
			"run":   []byte(`{"event":{"name":` + string(ns) + `,"data":{"note":` + string(name) + `}},"events":[{"name":` + string(ns) + `,"data":{}}],"ctx":{"run_id":` + string(name) + `}}`),
			"waits": []byte(`[{"op":"WaitForEvent","name":` + string(name) + `,"opts":{"event":` + string(name) + `}},{"op":"Step","name":` + string(name) + `}]`),
			"check": []byte(`{"steps":[{"op":"WaitForEvent","name":` + string(name) + `}],"note":` + string(name) + `}`),
		}
		for kind, body := range bodies {
			for fn, f := range map[string]func(string, []byte) ([]byte, bool){"Strip": Strip, "PrefixWaits": PrefixWaits, "PrefixCheckpointWaits": PrefixCheckpointWaits} {
				out, changed := f(app, body)
				if !changed {
					if string(out) != string(body) {
						t.Errorf("%s(%s %q) changed the body without saying so", fn, kind, s)
					}
					continue
				}
				if !json.Valid(out) {
					t.Errorf("%s(%s %q) wrote invalid JSON: %s", fn, kind, s, out)
				}
			}
		}
		if !utf8.ValidString(s) {
			continue
		}
		out, changed := Strip(app, bodies["run"])
		var run struct {
			Event struct {
				Name string            `json:"name"`
				Data map[string]string `json:"data"`
			} `json:"event"`
			Ctx map[string]string `json:"ctx"`
		}
		if !changed || json.Unmarshal(out, &run) != nil || run.Event.Name != s || run.Event.Data["note"] != s || run.Ctx["run_id"] != s {
			t.Errorf("Strip on a run named %q gave %s", s, out)
		}
		if s == "" {
			continue
		}
		out, changed = PrefixWaits(app, bodies["waits"])
		var ops []map[string]any
		if !changed || json.Unmarshal(out, &ops) != nil || ops[0]["opts"].(map[string]any)["event"] != Namespace(app, s) || ops[1]["name"] != s {
			t.Errorf("PrefixWaits on %q gave %s", s, out)
		}
	}
}
