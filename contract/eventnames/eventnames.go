// Package eventnames is how one workflow engine serves every app while each app writes plain
// Inngest event names (CONTRACT.md "Event names"). The engine holds every event of an app as
// "<app id>/<name>". The platform adds the prefix to what an app sends, registers and waits
// for, and takes it off the events the engine hands the app, so an app never sees it.
//
// Every function here is pure: it takes a JSON body and returns the rewritten body, or the same
// bytes and false when there was nothing to change.
package eventnames

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Namespace is the engine's name for an app's event. A name that already carries the app's
// prefix is returned as it is.
func Namespace(appID, name string) string {
	if appID == "" || strings.HasPrefix(name, appID+"/") {
		return name
	}
	return appID + "/" + name
}

// Bare is Namespace reversed: the name the app wrote.
func Bare(appID, name string) string { return strings.TrimPrefix(name, appID+"/") }

// CronRunPrefix begins the name of the event the platform sends to start a cron function by hand
// (whisk cron run); the function's id follows it. ScheduledTimer is the name the engine gives the
// event of a scheduled run, which is what an SDK expects a cron function to be started by.
const (
	CronRunPrefix  = "whisk/cron.run."
	ScheduledTimer = "inngest/scheduled.timer"
)

// Scheduled names every cron-run event in a body the engine sends as a scheduled run's event,
// prefixed or not, so a run started by hand reaches the function as its schedule would. An SDK
// that checks a run's event against the function's triggers (the TypeScript SDK) would refuse
// the cron-run name, which the function never declared; its data is the schedule's {cron}.
func Scheduled(appID string, body []byte) ([]byte, bool) {
	v, ok := decode(body)
	if !ok || appID == "" {
		return body, false
	}
	out, changed := walkEvents(v, func(name string) (string, bool) {
		if strings.HasPrefix(Bare(appID, name), CronRunPrefix) {
			return ScheduledTimer, true
		}
		return name, false
	})
	if !changed {
		return body, false
	}
	return encode(out, body)
}

// Strip takes the app's prefix off every event in a body the engine sends: the run's event and
// batch, an event a step returned (what step.waitForEvent resolved with), and an event carried
// in another's data (the one behind inngest/function.failed). A cron-run event becomes a
// scheduled run's event, as Scheduled. An event is an object with a string name and a data
// member; nothing else is touched.
func Strip(appID string, body []byte) ([]byte, bool) {
	v, ok := decode(body)
	if !ok || appID == "" {
		return body, false
	}
	prefix := appID + "/"
	out, changed := walkEvents(v, func(name string) (string, bool) {
		if !strings.HasPrefix(name, prefix) {
			return name, false
		}
		if bare := strings.TrimPrefix(name, prefix); !strings.HasPrefix(bare, CronRunPrefix) {
			return bare, true
		}
		return ScheduledTimer, true
	})
	if !changed {
		return body, false
	}
	return encode(out, body)
}

// walkEvents renames every event in v, at any depth, by rename.
func walkEvents(v any, rename func(string) (string, bool)) (any, bool) {
	switch x := v.(type) {
	case map[string]any:
		changed := false
		if name, ok := x["name"].(string); ok {
			if _, event := x["data"]; event {
				if next, c := rename(name); c {
					x["name"], changed = next, true
				}
			}
		}
		for k, child := range x {
			if next, c := walkEvents(child, rename); c {
				x[k], changed = next, true
			}
		}
		return x, changed
	case []any:
		changed := false
		for i, child := range x {
			if next, c := walkEvents(child, rename); c {
				x[i], changed = next, true
			}
		}
		return x, changed
	}
	return v, false
}

// waitOp is the opcode an SDK returns for step.waitForEvent. The event it waits for is
// opts.event when the SDK sets it, and its name otherwise, as the engine reads it.
const waitOp = "WaitForEvent"

// PrefixWaits gives every step.waitForEvent in an SDK's answer to the engine the app's prefix,
// so a run waits for its own app's event whatever name the code wrote. body is the SDK's list
// of opcodes; anything else is returned unchanged.
func PrefixWaits(appID string, body []byte) ([]byte, bool) {
	v, ok := decode(body)
	ops, list := v.([]any)
	if !ok || !list || appID == "" {
		return body, false
	}
	if !prefixOps(appID, ops) {
		return body, false
	}
	return encode(ops, body)
}

// PrefixCheckpointWaits is PrefixWaits for an SDK checkpoint, whose opcodes are its steps.
func PrefixCheckpointWaits(appID string, body []byte) ([]byte, bool) {
	v, ok := decode(body)
	call, obj := v.(map[string]any)
	if !ok || !obj || appID == "" {
		return body, false
	}
	renamed := canonicalKeys(call, "steps")
	ops, list := call["steps"].([]any)
	if !list || !prefixOps(appID, ops) && !renamed {
		return body, false
	}
	return encode(call, body)
}

// prefixOps names every wait in ops for the app. The engine is written in Go, which reads a
// field name without regard to case and an opcode by its lower-case spelling too, so "Op",
// "NAME" or "waitforevent" are read the way the engine reads them: each key is first given its
// own spelling, and a field spelled two ways is dropped, since which spelling the engine takes
// depends on their order.
func prefixOps(appID string, ops []any) bool {
	changed := false
	for _, raw := range ops {
		op, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if canonicalKeys(op, "op", "name", "opts") {
			changed = true
		}
		if opts, ok := op["opts"].(map[string]any); ok && canonicalKeys(opts, "event") {
			changed = true
		}
		if code, _ := op["op"].(string); !strings.EqualFold(code, waitOp) {
			continue
		}
		// The Go SDK names the step in name and the event in opts.event; the TypeScript and
		// Python SDKs put the event in name.
		target, key := op, "name"
		if opts, ok := op["opts"].(map[string]any); ok {
			if event, ok := opts["event"].(string); ok && event != "" {
				target, key = opts, "event"
			}
		}
		if name, ok := target[key].(string); ok && name != "" && name != Namespace(appID, name) {
			target[key], changed = Namespace(appID, name), true
		}
	}
	return changed
}

// canonicalKeys gives each field its own spelling in m, and drops a field m spells more than
// one way. It reports whether it changed m.
func canonicalKeys(m map[string]any, fields ...string) bool {
	changed := false
	for _, f := range fields {
		var found []string
		for k := range m {
			if strings.EqualFold(k, f) {
				found = append(found, k)
			}
		}
		switch {
		case len(found) > 1:
			for _, k := range found {
				delete(m, k)
			}
			changed = true
		case len(found) == 1 && found[0] != f:
			m[f] = m[found[0]]
			delete(m, found[0])
			changed = true
		}
	}
	return changed
}

// decode reads a body keeping numbers exactly as written, so a large id in an app's data
// survives the rewrite.
func decode(body []byte) (any, bool) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, false
	}
	return v, true
}

// encode writes a rewritten body with its keys sorted and no HTML escaping, the canonical form
// an SDK that verifies a parsed body signs against. A value that cannot be written leaves the
// original body in place.
func encode(v any, original []byte) ([]byte, bool) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return original, false
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), true
}
