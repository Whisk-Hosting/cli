package status

import "slices"

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

// Nodes is every node status.
func Nodes() []Node { return slices.Clone(nodes) }

// Valid says whether n is a node status at all.
func (n Node) Valid() bool { return slices.Contains(nodes, n) }
