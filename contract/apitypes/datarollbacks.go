package apitypes

import "time"

// Rollbacks with their data (CONTROL-PLANE.md §6.5): `whisk rollback --data` makes the version
// before the live one live again with the production database as it was just before the live
// version's deploy ran its migration. The database it replaces is set aside beside it, never
// dropped, and the app's requests wait at the edge while the two change places.

// DataRollbackState is where a rollback's data stands.
type DataRollbackState string

const (
	// DataRollbackWaiting is a rollback waiting for the deploys before it to stop, or copying the
	// database as it was beside the live one: nothing has changed yet.
	DataRollbackWaiting DataRollbackState = "waiting"
	// DataRollbackHeld is a rollback holding the app's requests at the edge while the live
	// version stops, the copy takes the live database's place and the version before it starts.
	DataRollbackHeld DataRollbackState = "held"
	// DataRollbackRestored is a rollback that went live on the data it put back.
	DataRollbackRestored DataRollbackState = "restored"
	// DataRollbackGivenBack is a rollback that did not go live once it had put the data back: the
	// database it replaced was put back in its place, and the version that was live runs on it.
	DataRollbackGivenBack DataRollbackState = "given_back"
	// DataRollbackUnchanged is a rollback that ended before it changed the data.
	DataRollbackUnchanged DataRollbackState = "unchanged"
)

// DataRollback is what a rollback with its data did. FromDeploy is the deploy whose migration the
// data goes back to just before, at PointAt; Instant says the copy was made in a moment from the
// instant copy taken then, rather than recovered from the backups. Aside is the name the live
// database was set aside under, which `whisk db query --database` reads, until its server drops
// it at KeptUntil at the earliest (absent when nothing drops it on a schedule); "" until the swap.
type DataRollback struct {
	State      DataRollbackState `json:"state"`
	FromDeploy string            `json:"from_deploy"`
	PointAt    time.Time         `json:"point_at"`
	Instant    bool              `json:"instant"`
	Aside      string            `json:"aside,omitempty"`
	KeptUntil  *time.Time        `json:"kept_until,omitempty"`
	StartedAt  time.Time         `json:"started_at"`
	EndedAt    *time.Time        `json:"ended_at,omitempty"`
}

// RollbackWithDataRequest is POST /orgs/:org/apps/:app/rollback-with-data: production rolled back
// to the version before the live one, with its data. Environment, when sent, is production.
type RollbackWithDataRequest struct {
	Environment string `json:"environment,omitempty"`
}
