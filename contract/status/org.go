package status

import (
	"slices"
	"time"
)

// Org is a business's status (CONTROL-PLANE.md "A business's status"). Only the control plane's
// lifecycle moves it.
//
//whisk:enum
type Org string

// Business statuses.
const (
	OrgPending   Org = "pending"
	OrgActive    Org = "active"
	OrgFrozen    Org = "frozen"
	OrgShredding Org = "shredding"
	OrgShredded  Org = "shredded"
)

var orgs = []Org{OrgPending, OrgActive, OrgFrozen, OrgShredding, OrgShredded}

// Orgs is every business status.
func Orgs() []Org { return slices.Clone(orgs) }

// Valid says whether o is a business status at all.
func (o Org) Valid() bool { return slices.Contains(orgs, o) }

// App is an app's status: whether it is meant to run. How it is doing right now (running,
// sleeping, starting, broken) is its state, worked out from its deploys and containers.
//
//whisk:enum
type App string

// App statuses.
const (
	AppActive   App = "active"
	AppSleeping App = "sleeping"
	AppStopped  App = "stopped"
	AppDeleted  App = "deleted"
)

var apps = []App{AppActive, AppSleeping, AppStopped, AppDeleted}

// Follows says whether an app whose status a was decided by an event at decided now takes the
// status an event at at leaves it with. Nothing moves a deleted app, and an event that happened
// before the one the status follows no longer decides it: a wake reported after a later sleep
// does not mark a sleeping app awake. Times compare to the second, as the node reports them, and
// events of the same second apply in the order they arrive. decided is zero for a status no
// event has decided yet.
func (a App) Follows(to App, decided, at time.Time) bool {
	if a == AppDeleted || !to.Valid() {
		return false
	}
	return decided.IsZero() || !at.Truncate(time.Second).Before(decided.Truncate(time.Second))
}

// Apps is every app status.
func Apps() []App { return slices.Clone(apps) }

// Valid says whether a is an app status at all.
func (a App) Valid() bool { return slices.Contains(apps, a) }

// Node is a server's status in the fleet.
//
//whisk:enum
type Node string

// Node statuses.
const (
	NodeProvisioning Node = "provisioning"
	NodeReady        Node = "ready"
	NodeDraining     Node = "draining"
	NodeDown         Node = "down"
)

var nodes = []Node{NodeProvisioning, NodeReady, NodeDraining, NodeDown}

// NodeEvent is something that moves a node's status.
//
//whisk:enum
type NodeEvent string

// What happens to a node.
const (
	// NodeHello is the node enrolling or saying hello: it is up and says what it runs.
	NodeHello NodeEvent = "hello"
	// NodeSeen is a heartbeat.
	NodeSeen NodeEvent = "seen"
	// NodeSilent is the node not having reported for as long as a node may.
	NodeSilent NodeEvent = "silent"
	// NodeDrain and NodeUndrain are the operator's.
	NodeDrain   NodeEvent = "drain"
	NodeUndrain NodeEvent = "undrain"
)

// NodeFacts is what a node's next status depends on besides the one it has.
type NodeFacts struct {
	// Draining is whether an operator is draining it, which outlives the node going down.
	Draining bool
	// FailoverOpen is whether it has a failover not yet fenced: it stays down whatever it reports.
	FailoverOpen bool
	// Serves is whether live deploys are still placed on it.
	Serves bool
}

// NextNode is a node's status, and whether it is being drained, after an event. Whether a node
// answers and whether an operator is draining it are two things: a node that reports again is
// ready, or draining when it is being drained; a draining node that goes silent is down while
// apps are still placed on it, so they fail over like any other node's, and stays draining once
// it holds none, since a drained machine is expected to be switched off; draining a node that is
// down leaves it down, and it comes back draining. Pure, table-tested.
func NextNode(n Node, f NodeFacts, ev NodeEvent) (Node, bool) {
	up := NodeReady
	if f.Draining {
		up = NodeDraining
	}
	switch ev {
	case NodeHello, NodeSeen:
		switch {
		case n == NodeDown && f.FailoverOpen:
			return n, f.Draining
		case n == NodeDown, n == NodeProvisioning && ev == NodeHello:
			return up, f.Draining
		}
		return n, f.Draining
	case NodeSilent:
		if n == NodeReady || (n == NodeDraining && f.Serves) {
			return NodeDown, f.Draining
		}
		return n, f.Draining
	case NodeDrain:
		if n == NodeReady || n == NodeProvisioning {
			return NodeDraining, true
		}
		return n, true
	case NodeUndrain:
		if n == NodeDraining {
			return NodeReady, false
		}
		return n, false
	}
	return n, f.Draining
}

// Nodes is every node status.
func Nodes() []Node { return slices.Clone(nodes) }

// Valid says whether n is a node status at all.
func (n Node) Valid() bool { return slices.Contains(nodes, n) }
