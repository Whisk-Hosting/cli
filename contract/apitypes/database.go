package apitypes

import "time"

// DatabaseURL is a connection string for an environment's database.
type DatabaseURL struct {
	URL         string    `json:"url"`
	Environment string    `json:"environment"`
	ExpiresAt   time.Time `json:"expires_at"`
	// Reachable says where the address answers: "anywhere" through the database proxy
	// (§6.22), or "platform" when it is the environment's own address inside its app's
	// network, which only the platform's hosts reach; db query is the way in then.
	Reachable string `json:"reachable"`
}

// DatabaseSnapshot is a restore point marker. Restorable is whether the org's plan can return to it
// with whisk restore, and RestoreDays for how long.
type DatabaseSnapshot struct {
	ID          string    `json:"id"`
	Environment string    `json:"environment"`
	LSN         string    `json:"lsn"`
	CreatedAt   time.Time `json:"created_at"`
	Restorable  bool      `json:"restorable"`
	RestoreDays int64     `json:"restore_days"`
}

// DatabaseImportAuthorisation is what the CLI posts the dump with.
type DatabaseImportAuthorisation struct {
	ID        string            `json:"id"`
	URL       string            `json:"url"`
	Method    string            `json:"method"`
	Fields    map[string]string `json:"fields"`
	MaxBytes  int64             `json:"max_bytes"`
	ExpiresAt time.Time         `json:"expires_at"`
}

// DatabaseStatementResult is what one statement did: its command tag and the rows it touched.
type DatabaseStatementResult struct {
	Command string `json:"command"`
	Rows    int64  `json:"rows"`
}

// DatabaseTableEffect is what the SQL did to one table, as PostgreSQL counted it inside the
// transaction: triggers and cascades included.
type DatabaseTableEffect struct {
	Table    string `json:"table"`
	Inserted int64  `json:"inserted"`
	Updated  int64  `json:"updated"`
	Deleted  int64  `json:"deleted"`
}

// DatabaseEffect is everything the SQL did, which decides whether it needs a code and whether the
// confirmed run matches the preview.
type DatabaseEffect struct {
	Statements []DatabaseStatementResult `json:"statements"`
	Tables     []DatabaseTableEffect     `json:"tables"`
	// Replaces names statements that redefine something that exists (create or replace).
	Replaces []string `json:"replaces,omitempty"`
	// Definitions are the existing tables, views, functions, types and schemas the SQL removed
	// or changed, read from the catalog before and after inside the transaction. However the
	// SQL got there (a DO block, a function that runs DDL, TRUNCATE inside either), a
	// dropped, altered, emptied or redefined object shows here.
	Definitions []DatabaseDefinitionChange `json:"definitions,omitempty"`
}

// DatabaseDefinitionChange is one existing object the SQL removed or changed.
type DatabaseDefinitionChange struct {
	Object string `json:"object"` // "table public.customers", "function public.total(integer)"
	Change string `json:"change"` // removed | changed
}

// DatabaseQueryResult is what db query answers: the last statement's rows, and the effect.
type DatabaseQueryResult struct {
	Environment  string            `json:"environment"`
	Database     string            `json:"database"`
	Columns      []string          `json:"columns"`
	Rows         [][]*string       `json:"rows"`
	RowCount     int64             `json:"row_count"`
	Truncated    bool              `json:"truncated"`
	Command      string            `json:"command"`
	Effect       DatabaseEffect    `json:"effect"`
	RestorePoint *DatabaseSnapshot `json:"restore_point,omitempty"`
	DurationMS   int64             `json:"duration_ms"`
}

// DatabaseColumn is one column as db schema shows it.
type DatabaseColumn struct {
	Name     string  `json:"name"`
	Type     string  `json:"type"`
	Nullable bool    `json:"nullable"`
	Default  *string `json:"default"`
}

// DatabaseConstraint is a primary key, foreign key, unique or check constraint, as PostgreSQL
// prints its definition.
type DatabaseConstraint struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"` // primary_key | foreign_key | unique | check | exclusion
	Definition string `json:"definition"`
}

// DatabaseTable is one table, view or materialized view.
type DatabaseTable struct {
	Schema      string               `json:"schema"`
	Name        string               `json:"name"`
	Kind        string               `json:"kind"` // table | view | materialized_view
	RowEstimate *int64               `json:"rows_estimate"`
	Columns     []DatabaseColumn     `json:"columns"`
	Constraints []DatabaseConstraint `json:"constraints"`
}

// DatabaseSchema is what db schema answers.
type DatabaseSchema struct {
	Environment string          `json:"environment"`
	Database    string          `json:"database"`
	Postgres    string          `json:"postgres"` // the server's version, major.minor
	Tables      []DatabaseTable `json:"tables"`
	Truncated   bool            `json:"truncated"`
}

var _ = more.addWire(DatabaseColumn{}, DatabaseConstraint{}, DatabaseDefinitionChange{}, DatabaseEffect{}, DatabaseImportAuthorisation{}, DatabaseQueryResult{}, DatabaseSchema{}, DatabaseSnapshot{}, DatabaseStatementResult{}, DatabaseTable{}, DatabaseTableEffect{}, DatabaseURL{})
