package status

import "slices"

// Build is an image build's status. A build runs once and ends once.
//
//whisk:enum
type Build string

// Build statuses.
const (
	BuildQueued    Build = "queued"
	BuildRunning   Build = "running"
	BuildSucceeded Build = "succeeded"
	BuildFailed    Build = "failed"
	BuildCancelled Build = "cancelled"
)

var builds = []Build{BuildQueued, BuildRunning, BuildSucceeded, BuildFailed, BuildCancelled}

// Builds is every build status.
func Builds() []Build { return slices.Clone(builds) }

// buildMoves is every move a build's status may make. A finished build keeps its status.
var buildMoves = map[Build][]Build{
	BuildQueued:    {BuildRunning, BuildSucceeded, BuildFailed, BuildCancelled},
	BuildRunning:   {BuildRunning, BuildSucceeded, BuildFailed, BuildCancelled},
	BuildSucceeded: {},
	BuildFailed:    {},
	BuildCancelled: {},
}

// CanMove says whether a build in status b may move to status to (buildMoves).
func (b Build) CanMove(to Build) bool { return slices.Contains(buildMoves[b], to) }

// Finished says whether a build has ended: nothing moves it on.
func (b Build) Finished() bool {
	moves, known := buildMoves[b]
	return known && len(moves) == 0
}

// BuildsFinished is the statuses Finished holds for.
func BuildsFinished() []Build {
	return slices.DeleteFunc(Builds(), func(b Build) bool { return !b.Finished() })
}
