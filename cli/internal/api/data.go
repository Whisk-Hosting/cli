package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// ---- data (CONTROL-PLANE.md §6.18, CLI.md §5.9) --------------------------------------------

// DBURL is an environment's connection string. Reachable is "anywhere" when it points at the
// platform's public database proxy and "platform" when it is the environment's own address,
// which only the platform's hosts reach; an older platform leaves it empty.
type DBURL struct {
	URL         string    `json:"url"`
	Environment string    `json:"environment"`
	ExpiresAt   time.Time `json:"expires_at"`
	Reachable   string    `json:"reachable,omitempty"`
}

// Private reports whether the string works only from inside the platform.
func (u DBURL) Private() bool { return u.Reachable == "platform" }

// DatabaseURL mints a connection string for an environment's database, valid for a few
// minutes and audited. POST /orgs/:org/apps/:app/db/url {environment}.
func (c *Client) DatabaseURL(ctx context.Context, org, app, environment string) (DBURL, error) {
	var out DBURL
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/db/url", envBody(environment), &out)
}

// Snapshot is a restore point marker.
type Snapshot struct {
	ID          string    `json:"id"`
	Environment string    `json:"environment"`
	LSN         string    `json:"lsn,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	// Restorable is whether the org's plan can return to this point with whisk restore; absent
	// from a server that does not say.
	Restorable  *bool `json:"restorable,omitempty"`
	RestoreDays int64 `json:"restore_days,omitempty"`
}

// SnapshotDatabase records a restore point now. POST /orgs/:org/apps/:app/db/snapshot.
func (c *Client) SnapshotDatabase(ctx context.Context, org, app, environment string) (Snapshot, error) {
	var out Snapshot
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/db/snapshot", envBody(environment), &out)
}

// Restore is a self-service point-in-time restore as the API records it.
type Restore struct {
	ID             string    `json:"id"`
	Environment    string    `json:"environment"`
	At             time.Time `json:"at"`
	Import         string    `json:"import,omitempty"` // the import whose dump it loads, instead of a time
	Swap           bool      `json:"swap"`
	Status         string    `json:"status"` // queued | running | done | failed
	TargetDatabase string    `json:"target_database,omitempty"`
	// PreviousDatabase is the database a swap replaced, kept for 7 days.
	PreviousDatabase string     `json:"previous_database,omitempty"`
	Error            *ErrorBody `json:"error,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
}

// ErrorBody is the error object as it appears inside a record.
type ErrorBody struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Fix     string         `json:"fix"`
	Details map[string]any `json:"details,omitempty"`
}

// UnmarshalJSON reads the error object bare or as the platform stores it on a record, wrapped
// as {"error": {…}}.
func (e *ErrorBody) UnmarshalJSON(b []byte) error {
	type bare ErrorBody
	var wrapped struct {
		Error *bare `json:"error"`
	}
	if err := json.Unmarshal(b, &wrapped); err == nil && wrapped.Error != nil {
		*e = ErrorBody(*wrapped.Error)
		return nil
	}
	return json.Unmarshal(b, (*bare)(e))
}

// RestoreDatabase starts a PITR of an environment's database into a fresh database beside it,
// swapped in when swap is set. POST /orgs/:org/apps/:app/restore {at, swap, environment}.
func (c *Client) RestoreDatabase(ctx context.Context, org, app, environment string, at time.Time, swap bool) (Restore, error) {
	body := envBody(environment)
	body["at"] = at.UTC().Format(time.RFC3339)
	body["swap"] = swap
	var out Restore
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/restore", body, &out)
}

// GetRestore reads one restore. GET /orgs/:org/apps/:app/restores/:id.
func (c *Client) GetRestore(ctx context.Context, org, app, id string) (Restore, error) {
	var out Restore
	return out, c.Do(ctx, http.MethodGet, appPath(org, app)+"/restores/"+pathSeg(id), nil, &out)
}

// ListRestores returns the app's restores, newest first. GET /orgs/:org/apps/:app/restores.
func (c *Client) ListRestores(ctx context.Context, org, app string) ([]Restore, error) {
	p, err := getPage[Restore](ctx, c, appPath(org, app)+"/restores", "", 100)
	return p.Items, err
}

func envBody(environment string) map[string]any {
	body := map[string]any{}
	if environment != "" {
		body["environment"] = environment
	}
	return body
}

// QueryResult is what POST /db/query answers: the last statement's rows and what the SQL did.
type QueryResult struct {
	Environment  string         `json:"environment"`
	Database     string         `json:"database"`
	Columns      []string       `json:"columns"`
	Rows         [][]*string    `json:"rows"`
	RowCount     int64          `json:"row_count"`
	Truncated    bool           `json:"truncated"`
	Command      string         `json:"command"`
	Effect       map[string]any `json:"effect"`
	RestorePoint *Snapshot      `json:"restore_point,omitempty"`
	DurationMS   int64          `json:"duration_ms"`
}

// QueryRequest is one db query call; Confirm alone runs SQL a preview held back.
type QueryRequest struct {
	SQL         string `json:"sql,omitempty"`
	Environment string `json:"environment,omitempty"`
	Database    string `json:"database,omitempty"`
	Confirm     string `json:"confirm,omitempty"`
	Limit       int    `json:"limit,omitempty"`
}

// QueryDatabase runs SQL on the platform. POST /orgs/:org/apps/:app/db/query. SQL that would
// change or remove existing data answers CONFIRM_REQUIRED with a code in details.
func (c *Client) QueryDatabase(ctx context.Context, org, app string, req QueryRequest) (QueryResult, error) {
	var out QueryResult
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/db/query", req, &out)
}

// DBSchema is the environment's tables as POST /db/schema answers them.
type DBSchema struct {
	Environment string    `json:"environment"`
	Database    string    `json:"database"`
	Postgres    string    `json:"postgres"`
	Tables      []DBTable `json:"tables"`
	Truncated   bool      `json:"truncated"`
}

// DBTable is one table, view or materialized view.
type DBTable struct {
	Schema      string `json:"schema"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	RowEstimate *int64 `json:"rows_estimate"`
	Columns     []struct {
		Name     string  `json:"name"`
		Type     string  `json:"type"`
		Nullable bool    `json:"nullable"`
		Default  *string `json:"default"`
	} `json:"columns"`
	Constraints []struct {
		Name       string `json:"name"`
		Kind       string `json:"kind"`
		Definition string `json:"definition"`
	} `json:"constraints"`
}

// DatabaseSchema reads the tables. POST /orgs/:org/apps/:app/db/schema {environment, database}.
func (c *Client) DatabaseSchema(ctx context.Context, org, app, environment, database string) (DBSchema, error) {
	body := envBody(environment)
	if database != "" {
		body["database"] = database
	}
	var out DBSchema
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/db/schema", body, &out)
}
