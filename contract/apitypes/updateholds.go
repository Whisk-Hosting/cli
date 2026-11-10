package apitypes

import "time"

// Held updates (CONTROL-PLANE.md §6.5): an update of a fork whose rehearsal timed its migration as
// short switches with the app's requests waiting at the edge and the live version stopped, from
// just before its restore point until it serves, so nothing is written in between. An update that
// does not go live once its migration ran puts the data back as it was at the point, from the
// instant copy taken with it, and the version that was live answers again.

// UpdateHoldState is where a held update stands.
type UpdateHoldState string

const (
	// UpdateHoldHeld is an update holding the app's requests while the live version stops, the
	// restore point is taken, the migration runs and the new version starts.
	UpdateHoldHeld UpdateHoldState = "held"
	// UpdateHoldSwitched is an update that went live: the requests went on to it.
	UpdateHoldSwitched UpdateHoldState = "switched"
	// UpdateHoldPutBack is an update that did not go live after its migration ran: the data was
	// put back as it was at the restore point, and the version that was live answers again.
	UpdateHoldPutBack UpdateHoldState = "put_back"
	// UpdateHoldUnchanged is an update that ended before its migration ran: the version that was
	// live answers again on data nothing changed.
	UpdateHoldUnchanged UpdateHoldState = "unchanged"
	// UpdateHoldReleased is an update that went on without its hold: its restore point came
	// without an instant copy, so a failure could not have been put back at once, or it was placed
	// again after the machine it ran on stopped answering. The requests went on to the version that
	// was live, and the update went on as any deploy does.
	UpdateHoldReleased UpdateHoldState = "released"
)

// UpdateHold is how an update switched with the app's requests held. PointAt is when its restore
// point was taken, once the live version stopped; absent until then.
type UpdateHold struct {
	State     UpdateHoldState `json:"state"`
	PointAt   *time.Time      `json:"point_at,omitempty"`
	StartedAt time.Time       `json:"started_at"`
	EndedAt   *time.Time      `json:"ended_at,omitempty"`
}
