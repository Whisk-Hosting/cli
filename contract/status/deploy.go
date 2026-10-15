// Package status names the closed sets of states Whisk reports, so the platform, the CLI, the
// connector and the dashboard read them from one place. Each set is a type marked
// `//whisk:enum`: whisklint requires a switch over it to name every value and refuses a bare
// string literal standing in for one outside this package.
package status

import "slices"

// Deploy is a deploy's status. Every deploy ends in exactly one finished status.
//
//whisk:enum
type Deploy string

// Deploy statuses, in pipeline order, then the finished ones.
const (
	DeployQueued         Deploy = "queued"
	DeployBuilding       Deploy = "building"
	DeploySnapshotting   Deploy = "snapshotting"
	DeployMigrating      Deploy = "migrating"
	DeployStarting       Deploy = "starting"
	DeployHealthChecking Deploy = "health_checking"
	DeploySwitching      Deploy = "switching"
	DeployDraining       Deploy = "draining"
	DeployLive           Deploy = "live"
	DeployFailed         Deploy = "failed"
	DeployRolledBack     Deploy = "rolled_back"
	DeployCancelled      Deploy = "cancelled"
	DeployBlocked        Deploy = "blocked"
	DeploySuperseded     Deploy = "superseded"
)

var deploys = []Deploy{
	DeployQueued, DeployBuilding, DeploySnapshotting, DeployMigrating, DeployStarting,
	DeployHealthChecking, DeploySwitching, DeployDraining, DeployLive, DeployFailed,
	DeployRolledBack, DeployCancelled, DeployBlocked, DeploySuperseded,
}

// Deploys is every deploy status.
func Deploys() []Deploy { return slices.Clone(deploys) }

// Valid says whether d is a deploy status at all.
func (d Deploy) Valid() bool { return slices.Contains(deploys, d) }

// Finished says whether a deploy has stopped for good, one way or another: nothing moves it on.
func (d Deploy) Finished() bool {
	switch d {
	case DeployLive, DeployFailed, DeployRolledBack, DeployCancelled, DeployBlocked, DeploySuperseded:
		return true
	case DeployQueued, DeployBuilding, DeploySnapshotting, DeployMigrating, DeployStarting,
		DeployHealthChecking, DeploySwitching, DeployDraining:
		return false
	}
	return false
}

// Supersedable says whether a newer request for the environment stops a deploy in this status:
// one that has not begun switching traffic. A deploy that is switching or draining finishes.
func (d Deploy) Supersedable() bool {
	switch d {
	case DeployQueued, DeployBuilding, DeploySnapshotting, DeployMigrating, DeployStarting, DeployHealthChecking:
		return true
	case DeploySwitching, DeployDraining, DeployLive, DeployFailed, DeployRolledBack,
		DeployCancelled, DeployBlocked, DeploySuperseded:
		return false
	}
	return false
}

// Running says whether a deploy's container is meant to be up: it has started and has not
// finished or been replaced (a live deploy keeps running until another takes over).
func (d Deploy) Running() bool {
	switch d {
	case DeployStarting, DeployHealthChecking, DeploySwitching, DeployDraining, DeployLive:
		return true
	case DeployQueued, DeployBuilding, DeploySnapshotting, DeployMigrating, DeployFailed,
		DeployRolledBack, DeployCancelled, DeployBlocked, DeploySuperseded:
		return false
	}
	return false
}

// Abandoned says whether a deploy was stopped from outside its job: cancelled or superseded.
func (d Deploy) Abandoned() bool { return d == DeployCancelled || d == DeploySuperseded }

// Filter is the statuses among all for which keep holds, in order.
func Filter(keep func(Deploy) bool) []Deploy {
	return slices.DeleteFunc(Deploys(), func(d Deploy) bool { return !keep(d) })
}
