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

// A node's status after each thing that happens to it (NextNode): whether it answers and whether
// an operator is draining it are kept apart.
func TestNextNode(t *testing.T) {
	draining := NodeFacts{Draining: true}
	for _, c := range []struct {
		name      string
		from      Node
		facts     NodeFacts
		event     NodeEvent
		want      Node
		wantDrain bool
	}{
		{"a new node says hello", NodeProvisioning, NodeFacts{}, NodeHello, NodeReady, false},
		{"a heartbeat alone does not end provisioning", NodeProvisioning, NodeFacts{}, NodeSeen, NodeProvisioning, false},
		{"a down node reports again", NodeDown, NodeFacts{}, NodeSeen, NodeReady, false},
		{"a down node with a failover not yet fenced stays down", NodeDown, NodeFacts{FailoverOpen: true}, NodeSeen, NodeDown, false},
		{"and at its hello too", NodeDown, NodeFacts{FailoverOpen: true}, NodeHello, NodeDown, false},
		{"a node drained while down comes back draining", NodeDown, draining, NodeSeen, NodeDraining, true},
		{"a draining node stays draining whatever it reports", NodeDraining, draining, NodeHello, NodeDraining, true},
		{"a ready node goes silent", NodeReady, NodeFacts{}, NodeSilent, NodeDown, false},
		{"a draining node that still serves apps goes silent", NodeDraining, NodeFacts{Draining: true, Serves: true}, NodeSilent, NodeDown, true},
		{"a drained node that serves none is expected to go quiet", NodeDraining, draining, NodeSilent, NodeDraining, true},
		{"a node not yet up is not down", NodeProvisioning, NodeFacts{}, NodeSilent, NodeProvisioning, false},
		{"draining a ready node", NodeReady, NodeFacts{}, NodeDrain, NodeDraining, true},
		{"draining a down node leaves it down", NodeDown, NodeFacts{}, NodeDrain, NodeDown, true},
		{"undraining", NodeDraining, draining, NodeUndrain, NodeReady, false},
		{"undraining a node that went down while drained leaves it down", NodeDown, draining, NodeUndrain, NodeDown, false},
		{"undraining a ready node changes nothing", NodeReady, NodeFacts{}, NodeUndrain, NodeReady, false},
	} {
		got, drain := NextNode(c.from, c.facts, c.event)
		if got != c.want || drain != c.wantDrain {
			t.Errorf("%s: %s draining %v, want %s draining %v", c.name, got, drain, c.want, c.wantDrain)
		}
	}
}
