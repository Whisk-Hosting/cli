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
	DeployQueued   Deploy = "queued"
	DeployBuilding Deploy = "building"
	// DeployRehearsing is a production deploy that brings a fork a newer release running its
	// migration on a private copy of the live data before the data itself (FORKS.md §4).
	DeployRehearsing Deploy = "rehearsing"
	// DeployRestoring is a rollback with its data putting the database back to just before the
	// live version's migration, while the app's requests wait at the edge (CONTROL-PLANE.md §6.5).
	DeployRestoring      Deploy = "restoring"
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
	DeployQueued, DeployBuilding, DeployRehearsing, DeployRestoring, DeploySnapshotting, DeployMigrating, DeployStarting,
	DeployHealthChecking, DeploySwitching, DeployDraining, DeployLive, DeployFailed,
	DeployRolledBack, DeployCancelled, DeployBlocked, DeploySuperseded,
}

// Deploys is every deploy status.
func Deploys() []Deploy { return slices.Clone(deploys) }

// Valid says whether d is a deploy status at all.
func (d Deploy) Valid() bool { return slices.Contains(deploys, d) }

// Finished says whether a deploy has stopped for good, one way or another: its job is over. The
// only move left is a newer deploy replacing one that is live or blocked (deployMoves).
func (d Deploy) Finished() bool {
	switch d {
	case DeployLive, DeployFailed, DeployRolledBack, DeployCancelled, DeployBlocked, DeploySuperseded:
		return true
	case DeployQueued, DeployBuilding, DeployRehearsing, DeployRestoring, DeploySnapshotting, DeployMigrating, DeployStarting,
		DeployHealthChecking, DeploySwitching, DeployDraining:
		return false
	}
	return false
}

// Supersedable says whether a newer request for the environment stops a deploy in this status:
// one that has not begun switching traffic. A deploy that is switching or draining finishes, and
// a rollback with its data that is stopped puts the data it replaced back first.
func (d Deploy) Supersedable() bool {
	switch d {
	case DeployQueued, DeployBuilding, DeployRehearsing, DeployRestoring, DeploySnapshotting, DeployMigrating, DeployStarting, DeployHealthChecking:
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
	case DeployQueued, DeployBuilding, DeployRehearsing, DeployRestoring, DeploySnapshotting, DeployMigrating, DeployFailed,
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

// deployMoves is every move a deploy's status may make: the statuses each one may be followed by.
// A deploy goes forward through the pipeline, skipping the steps it has no use for, and never
// back, with these exceptions: one whose node was lost before it began switching is queued again,
// a restore and a restore point report more than once, and a deploy that began switching can no
// longer be cancelled, only fail to take over, finish or be replaced by a newer one.
var deployMoves = map[Deploy][]Deploy{
	DeployQueued:         {DeployQueued, DeployBuilding, DeployRehearsing, DeployRestoring, DeploySnapshotting, DeployMigrating, DeployStarting, DeployHealthChecking, DeploySwitching, DeployFailed, DeployCancelled, DeployBlocked, DeploySuperseded},
	DeployBuilding:       {DeployQueued, DeployRehearsing, DeployRestoring, DeploySnapshotting, DeployMigrating, DeployStarting, DeployHealthChecking, DeploySwitching, DeployFailed, DeployCancelled, DeployBlocked, DeploySuperseded},
	DeployRehearsing:     {DeployQueued, DeployRestoring, DeploySnapshotting, DeployMigrating, DeployStarting, DeployHealthChecking, DeploySwitching, DeployFailed, DeployCancelled, DeployBlocked, DeploySuperseded},
	DeployRestoring:      {DeployQueued, DeployRestoring, DeploySnapshotting, DeployMigrating, DeployStarting, DeployHealthChecking, DeploySwitching, DeployFailed, DeployCancelled, DeployBlocked, DeploySuperseded},
	DeploySnapshotting:   {DeployQueued, DeploySnapshotting, DeployMigrating, DeployStarting, DeployHealthChecking, DeploySwitching, DeployFailed, DeployCancelled, DeployBlocked, DeploySuperseded},
	DeployMigrating:      {DeployQueued, DeployStarting, DeployHealthChecking, DeploySwitching, DeployFailed, DeployCancelled, DeployBlocked, DeploySuperseded},
	DeployStarting:       {DeployQueued, DeployHealthChecking, DeploySwitching, DeployFailed, DeployCancelled, DeployBlocked, DeploySuperseded},
	DeployHealthChecking: {DeployQueued, DeploySwitching, DeployFailed, DeployCancelled, DeployBlocked, DeploySuperseded},
	DeploySwitching:      {DeployDraining, DeployLive, DeployFailed, DeploySuperseded},
	DeployDraining:       {DeployLive, DeploySuperseded},
	DeployLive:           {DeploySuperseded},
	DeployBlocked:        {DeploySuperseded},
	DeployFailed:         {},
	DeployRolledBack:     {},
	DeployCancelled:      {},
	DeploySuperseded:     {},
}

// CanMove says whether a deploy in status d may move to status to (deployMoves).
func (d Deploy) CanMove(to Deploy) bool { return slices.Contains(deployMoves[d], to) }

// Cancellable says whether a person may still cancel a deploy in this status.
func (d Deploy) Cancellable() bool { return d.CanMove(DeployCancelled) }
