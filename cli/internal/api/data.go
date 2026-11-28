package api

import (
	"context"
	"encoding/json"
	"time"

	"github.com/whisk-run/contract/apitypes"
	"github.com/whisk-run/contract/endpoints"
)

// ---- data (CONTROL-PLANE.md §6.18, CLI.md §5.9) --------------------------------------------

// DBURL is an environment's connection string. Reachable is "anywhere" when it points at the
// platform's public database proxy and "platform" when it is the environment's own address,
// which only the platform's hosts reach.
type DBURL = apitypes.DatabaseURL

// PrivateURL reports whether the string works only from inside the platform.
func PrivateURL(u DBURL) bool { return u.Reachable == "platform" }

// DatabaseURL mints a connection string for an environment's database, valid for a few
// minutes and audited. POST /orgs/:org/apps/:app/db/url {environment}.
func (c *Client) DatabaseURL(ctx context.Context, org, app, environment string) (DBURL, error) {
	return Send(ctx, c, endpoints.DbURL, apitypes.DbEnvironmentRequest{Environment: environment}, org, app)
}

// DatabaseReader mints a read-only connection string of a login of its own, for an hour, audited.
// POST /orgs/:org/apps/:app/db/reader {environment}.
func (c *Client) DatabaseReader(ctx context.Context, org, app, environment string) (DBURL, error) {
	return Send(ctx, c, endpoints.DbReader, apitypes.DbEnvironmentRequest{Environment: environment}, org, app)
}

// Snapshot is a restore point marker; Restorable is whether the org's plan can return to it with
// whisk restore.
type Snapshot = apitypes.DatabaseSnapshot

// SnapshotDatabase records a restore point now. POST /orgs/:org/apps/:app/db/snapshot.
func (c *Client) SnapshotDatabase(ctx context.Context, org, app, environment string) (Snapshot, error) {
	return Send(ctx, c, endpoints.DbSnapshot, apitypes.DbEnvironmentRequest{Environment: environment}, org, app)
}

// Restore is a self-service point-in-time restore as the API records it.
type Restore = apitypes.RestoreJSON

// RestoreError is the error a restore recorded, nil when it has none.
func RestoreError(r Restore) *ErrorBody {
	if len(r.Error) == 0 || string(r.Error) == "null" {
		return nil
	}
	var e ErrorBody
	if json.Unmarshal(r.Error, &e) != nil {
		return &ErrorBody{Code: "RESTORE_FAILED", Message: "The restore recorded an error the CLI cannot read."}
	}
	return &e
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
	return Send(ctx, c, endpoints.StartRestore, apitypes.StartRestoreRequest{Environment: environment, At: at.UTC(), Swap: swap}, org, app)
}

// RestoreBefore starts a restore of an environment's database to the restore point a deploy took
// just before its migration, swapped in when swap is set. POST /orgs/:org/apps/:app/restore
// {before, swap, environment}.
func (c *Client) RestoreBefore(ctx context.Context, org, app, environment, deploy string, swap bool) (Restore, error) {
	return Send(ctx, c, endpoints.StartRestore, apitypes.StartRestoreRequest{Environment: environment, Before: deploy, Swap: swap}, org, app)
}

// GetRestore reads one restore. GET /orgs/:org/apps/:app/restores/:id.
func (c *Client) GetRestore(ctx context.Context, org, app, id string) (Restore, error) {
	return Call(ctx, c, endpoints.GetRestore, org, app, id)
}

// DiscardRestore drops the copy a restore made beside the live database, by the restore's id.
// DELETE /orgs/:org/apps/:app/restores/:id.
func (c *Client) DiscardRestore(ctx context.Context, org, app, id string) (Restore, error) {
	return Call(ctx, c, endpoints.DiscardRestore, org, app, id)
}

// ListRestores returns the app's restores, newest first. GET /orgs/:org/apps/:app/restores.
func (c *Client) ListRestores(ctx context.Context, org, app string) ([]Restore, error) {
	out, err := Call(ctx, c, endpoints.ListRestores, org, app)
	return out.Items, err
}

// QueryResult is what POST /db/query answers: the last statement's rows and what the SQL did.
type QueryResult = apitypes.DatabaseQueryResult

// QueryRequest is one db query call; Confirm alone runs SQL a preview held back.
type QueryRequest = apitypes.DbQueryRequest

// QueryDatabase runs SQL on the platform. POST /orgs/:org/apps/:app/db/query. SQL that would
// change or remove existing data answers CONFIRM_REQUIRED with a code in details.
func (c *Client) QueryDatabase(ctx context.Context, org, app string, req QueryRequest) (QueryResult, error) {
	return Send(ctx, c, endpoints.DbQuery, req, org, app)
}

// DBSchema is the environment's tables as POST /db/schema answers them.
type DBSchema = apitypes.DatabaseSchema

// DBTable is one table, view or materialized view.
type DBTable = apitypes.DatabaseTable

// DatabaseSchema reads the tables. POST /orgs/:org/apps/:app/db/schema {environment, database}.
func (c *Client) DatabaseSchema(ctx context.Context, org, app, environment, database string) (DBSchema, error) {
	return Send(ctx, c, endpoints.DbSchema, apitypes.DbSchemaRequest{Environment: environment, Database: database}, org, app)
}

// FilesRestored is what a restore of deleted files did (CONTROL-PLANE.md §6.13).
type FilesRestored = apitypes.FilesRestored

// RestoreFiles puts back the app's files deleted at or after since, under prefix.
// POST /orgs/:org/apps/:app/files/restore.
func (c *Client) RestoreFiles(ctx context.Context, org, app string, since time.Time, prefix string) (FilesRestored, error) {
	return Send(ctx, c, endpoints.RestoreFiles, apitypes.RestoreFilesRequest{Since: since.UTC(), Prefix: prefix}, org, app)
}
