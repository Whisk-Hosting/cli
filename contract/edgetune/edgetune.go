// Package edgetune holds the edge's tunables (docs/CONTROL-PLANE.md §11 "Tunables"): the
// timings and limits the Caddy modules work by that an operator may change while the edge runs.
// The control plane registers each as a tunable under its key, lists and bounds it, and writes
// every stored value into the edge document as each whisk_gate, whisk_trap and whisk_wake
// handler's "tune" object; the modules Apply it when they are provisioned, so a stored value
// reaches every edge with the next document. The edge's own copy of each default is the one
// here, so the two cannot drift apart.
package edgetune

import (
	"fmt"
	"maps"
	"sync/atomic"
	"time"
)

// Kind is how a knob's value is written: a duration in nanoseconds, a count, or a size in bytes.
type Kind string

const (
	Duration Kind = "duration"
	Count    Kind = "count"
	Bytes    Kind = "bytes"
)

// Knob is one edge tunable: its key (edge.<name>), what it is, and its default, floor and
// ceiling, durations in nanoseconds.
type Knob struct {
	Key, Label        string
	Kind              Kind
	Default, Min, Max int64
}

func d(t time.Duration) int64 { return int64(t) }

// Knobs are every edge tunable.
var Knobs = []Knob{
	{Key: "edge.challenge_lifetime", Label: "How long a proof-of-work challenge the gate hands out stays good", Kind: Duration, Default: d(10 * time.Minute), Min: d(time.Minute), Max: d(time.Hour)},
	{Key: "edge.mcp_body_limit", Label: "How much of an MCP request's body the gate reads to judge it", Kind: Bytes, Default: 1 << 20, Min: 64 << 10, Max: 16 << 20},
	{Key: "edge.service_cache_ttl", Label: "How long the gate keeps a service token's decision", Kind: Duration, Default: d(time.Minute), Min: d(5 * time.Second), Max: d(10 * time.Minute)},
	{Key: "edge.trap_strikes", Label: "Probes of the trap's paths within the window that bar an address", Kind: Count, Default: 3, Min: 1, Max: 20},
	{Key: "edge.trap_window", Label: "The window the trap counts an address's probes in", Kind: Duration, Default: d(10 * time.Minute), Min: d(time.Minute), Max: d(time.Hour)},
	{Key: "edge.pass_lifetime", Label: "How long a pass lets a person's browser through a bar", Kind: Duration, Default: d(24 * time.Hour), Min: d(time.Hour), Max: d(30 * 24 * time.Hour)},
	{Key: "edge.person_bar", Label: "The longest an address a person used is barred for", Kind: Duration, Default: d(time.Hour), Min: d(time.Minute), Max: d(24 * time.Hour)},
	{Key: "edge.wake_poll_margin", Label: "How long past the wake timeout the waking page keeps asking", Kind: Duration, Default: d(15 * time.Second), Min: d(5 * time.Second), Max: d(2 * time.Minute)},
	{Key: "edge.cert_call_timeout", Label: "The longest the certificate store waits for the control plane", Kind: Duration, Default: d(10 * time.Second), Min: d(2 * time.Second), Max: d(time.Minute)},
	{Key: "edge.cert_lock_ttl", Label: "How long a certificate lock lasts without being renewed", Kind: Duration, Default: d(time.Minute), Min: d(15 * time.Second), Max: d(10 * time.Minute)},
}

var stored atomic.Pointer[map[string]int64]

// Apply makes values the edge's tunables, every knob values leaves out at its default, and gives
// the keys it refused: unknown, or outside their bounds.
func Apply(values map[string]int64) (refused []string) {
	m := map[string]int64{}
	for k, v := range values {
		knob, ok := ByKey(k)
		if !ok || v < knob.Min || v > knob.Max {
			refused = append(refused, k)
			continue
		}
		m[k] = v
	}
	stored.Store(&m)
	return refused
}

// Values is what Apply was last given and kept.
func Values() map[string]int64 {
	if p := stored.Load(); p != nil {
		return maps.Clone(*p)
	}
	return map[string]int64{}
}

// ByKey is the knob named key.
func ByKey(key string) (Knob, bool) {
	for _, k := range Knobs {
		if k.Key == key {
			return k, true
		}
	}
	return Knob{}, false
}

// Get is the knob's value now: the one applied, else its default.
func (k Knob) Get() int64 {
	if p := stored.Load(); p != nil {
		if v, ok := (*p)[k.Key]; ok {
			return v
		}
	}
	return k.Default
}

// Of is the value now of the knob named key; it panics on a key no knob has, a mistake in code.
func Of(key string) int64 {
	k, ok := ByKey(key)
	if !ok {
		panic(fmt.Sprintf("edgetune: no knob %q", key))
	}
	return k.Get()
}

// DurationOf is Of as a duration.
func DurationOf(key string) time.Duration { return time.Duration(Of(key)) }
