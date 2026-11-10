package apitypes

import "time"

// Rehearsals (FORKS.md §4): a production deploy that brings a fork a newer release of its
// product runs the release's migration on a private copy of the fork's live data first, and says
// what the migration did there to what people entered. A deploy whose migration would change any
// of it waits for an owner or admin to approve it (GRANT_NEEDED, kind data).

// RehearsalStatus is where a deploy's rehearsal stands.
type RehearsalStatus string

const (
	// RehearsalRunning is a rehearsal making its copy or running the migration on it.
	RehearsalRunning RehearsalStatus = "running"
	// RehearsalClean is a migration that ran on the copy and changed nothing anyone entered.
	RehearsalClean RehearsalStatus = "clean"
	// RehearsalChanged is a migration that ran on the copy and changed what people entered:
	// the deploy waits for an owner or admin to approve it, or went ahead once one did.
	RehearsalChanged RehearsalStatus = "changed"
	// RehearsalFailed is a migration that failed on the copy (REHEARSAL_FAILED), so it never ran
	// on the live data.
	RehearsalFailed RehearsalStatus = "failed"
	// RehearsalStopped is a rehearsal that did not finish: its deploy was cancelled or replaced,
	// or the node could not make the copy.
	RehearsalStopped RehearsalStatus = "stopped"
)

// Rehearsal is what a deploy's rehearsal did. Release is the number of the product's release the
// deploy brings and FromRelease the one the live app runs. Tables, Rows and Bytes describe the
// copy as it was made; Truncated is a database with more tables than a rehearsal records, whose
// others were not compared. Changes says in plain words what the migration did to what people
// entered, [] when nothing, and Report is the comparison it is read from: only the tables and
// columns where something changed, in counts, never a value. Copy is the copy kept for the
// business's people to look at with `whisk db query --database`, which its node drops at
// KeptUntil or within the hour after; "" once it is dropped or due to be. MigrateMS is how long
// the migration ran on the copy, for a rehearsal whose migration finished: a short one switches
// the update with the app's requests held (CONTROL-PLANE.md §6.5).
type Rehearsal struct {
	Status      RehearsalStatus  `json:"status"`
	Release     int              `json:"release"`
	FromRelease int              `json:"from_release"`
	Tables      int              `json:"tables"`
	Rows        int64            `json:"rows"`
	Bytes       int64            `json:"bytes"`
	Truncated   bool             `json:"truncated"`
	MigrateMS   int64            `json:"migrate_ms,omitempty"`
	Changes     []string         `json:"changes"`
	Report      []RehearsedTable `json:"report"`
	Copy        string           `json:"copy,omitempty"`
	KeptUntil   *time.Time       `json:"kept_until,omitempty"`
	StartedAt   time.Time        `json:"started_at"`
	EndedAt     *time.Time       `json:"ended_at,omitempty"`
}

// RehearsedTable is one table as the rehearsed migration left it, against the copy as it was
// made. Schema is "" for the app's own; NowSchema and NowName are where the table is after, the
// same unless the migration moved or renamed it. Rows are matched by the table's key when it has
// one (Keyed), else compared whole, and then a row whose values changed counts as one removed
// and one added. Truncated is a table with more columns than a rehearsal compares, and Error why
// the table could not be compared.
type RehearsedTable struct {
	Schema      string            `json:"schema"`
	Name        string            `json:"name"`
	NowSchema   string            `json:"now_schema"`
	NowName     string            `json:"now_name"`
	Removed     bool              `json:"removed"`
	Keyed       bool              `json:"keyed"`
	RowsBefore  int64             `json:"rows_before"`
	RowsAfter   int64             `json:"rows_after"`
	RowsRemoved int64             `json:"rows_removed"`
	RowsAdded   int64             `json:"rows_added"`
	Columns     []RehearsedColumn `json:"columns"`
	Truncated   bool              `json:"truncated"`
	Error       string            `json:"error,omitempty"`
}

// RehearsedColumn is one column of a table the migration kept, in the rows it kept: Values is
// how many held a value before, Changed how many hold a different one after, Emptied how many
// lost theirs and Filled how many gained one. A Removed column's values were lost with it.
type RehearsedColumn struct {
	Name    string `json:"name"`
	NowName string `json:"now_name"`
	Type    string `json:"type"`
	NowType string `json:"now_type"`
	Removed bool   `json:"removed"`
	Values  int64  `json:"values"`
	Changed int64  `json:"changed"`
	Emptied int64  `json:"emptied"`
	Filled  int64  `json:"filled"`
}
