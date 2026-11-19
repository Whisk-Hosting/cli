package status

import (
	"encoding/json"
	"flag"
	"os"
	"testing"
)

var update = flag.Bool("update", false, "rewrite deploy.json")

// deploy.json is the deploy statuses for code that cannot import Go: the dashboard's tests read
// it, so its own list cannot drift from this one.
func TestDeployJSONIsCurrent(t *testing.T) {
	want, err := json.MarshalIndent(map[string]any{
		"statuses":     Deploys(),
		"finished":     Filter(Deploy.Finished),
		"supersedable": Filter(Deploy.Supersedable),
		"cancellable":  Filter(Deploy.Cancellable),
		"moves":        deployMoves,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if *update {
		if err := os.WriteFile("deploy.json", want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile("deploy.json")
	if err != nil || string(got) != string(want) {
		t.Fatalf("deploy.json is stale: run go test ./status -update")
	}
}

func TestEveryDeployEitherFinishesOrMovesOn(t *testing.T) {
	for _, d := range Deploys() {
		if !d.Valid() {
			t.Errorf("%s is listed but not valid", d)
		}
		if d.Finished() && d.Supersedable() {
			t.Errorf("%s is both finished and supersedable", d)
		}
	}
	if Deploy("nonsense").Valid() || Deploy("nonsense").Finished() {
		t.Error("an unknown status is neither valid nor finished")
	}
}

// The moves table (deployMoves): a deploy goes forward and never back, one that began switching
// is past cancelling, and a finished one moves nowhere but to superseded.
func TestDeployMoves(t *testing.T) {
	for _, c := range []struct {
		from, to Deploy
		want     bool
	}{
		{DeployQueued, DeployBuilding, true},
		{DeployBuilding, DeploySnapshotting, true},
		{DeployQueued, DeployStarting, true}, // no build, no migration
		{DeploySnapshotting, DeployMigrating, true},
		{DeployMigrating, DeploySnapshotting, false}, // never back
		{DeployHealthChecking, DeployStarting, false},
		{DeployLive, DeployHealthChecking, false},
		{DeployStarting, DeployQueued, true}, // its node was lost: placed again
		{DeploySwitching, DeployQueued, false},
		{DeployRestoring, DeployRestoring, true},
		{DeployMigrating, DeployMigrating, false},
		{DeployHealthChecking, DeployCancelled, true},
		{DeploySwitching, DeployCancelled, false},
		{DeployDraining, DeployCancelled, false},
		{DeployLive, DeployCancelled, false},
		{DeploySwitching, DeployFailed, true}, // the switch did not commit
		{DeployDraining, DeployFailed, false}, // it serves
		{DeploySwitching, DeployLive, true},
		{DeployDraining, DeployLive, true},
		{DeployHealthChecking, DeployLive, false}, // never live without switching
		{DeployLive, DeploySuperseded, true},
		{DeployBlocked, DeploySuperseded, true},
		{DeployBlocked, DeployStarting, false},
		{DeployCancelled, DeployFailed, false},
		{DeploySuperseded, DeployLive, false},
		{DeployFailed, DeployQueued, false},
	} {
		if got := c.from.CanMove(c.to); got != c.want {
			t.Errorf("%s to %s = %v, want %v", c.from, c.to, got, c.want)
		}
	}
	for _, d := range Deploys() {
		moves, listed := deployMoves[d]
		if !listed {
			t.Errorf("%s has no row in the moves table", d)
		}
		for _, to := range moves {
			if !to.Valid() {
				t.Errorf("%s moves to %q, which is no status", d, to)
			}
		}
		// What the predicates say and what the table allows are one rule.
		if d.Supersedable() != d.Cancellable() {
			t.Errorf("%s: supersedable %v, cancellable %v", d, d.Supersedable(), d.Cancellable())
		}
		if d.Supersedable() && !d.CanMove(DeploySuperseded) {
			t.Errorf("%s is supersedable and cannot move to superseded", d)
		}
		if !d.Finished() && !d.CanMove(DeployFailed) && d != DeployDraining {
			t.Errorf("%s is unfinished and cannot fail", d)
		}
		if d.Finished() && d != DeployLive && d != DeployBlocked && len(moves) != 0 {
			t.Errorf("%s is finished and moves on to %v", d, moves)
		}
	}
}
