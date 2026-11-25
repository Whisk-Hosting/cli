package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The routes of control-plane/internal/api/handlers_data.go.
var (
	StartRestore    = New[apitypes.StartRestoreRequest, apitypes.RestoreJSON]("POST /v1/orgs/{org}/apps/{app}/restore")
	ListRestores    = New[None, apitypes.Page[apitypes.RestoreJSON]]("GET /v1/orgs/{org}/apps/{app}/restores")
	GetRestore      = New[None, apitypes.RestoreJSON]("GET /v1/orgs/{org}/apps/{app}/restores/{id}")
	DiscardRestore  = New[None, apitypes.RestoreJSON]("DELETE /v1/orgs/{org}/apps/{app}/restores/{id}")
	AuthoriseImport = New[apitypes.AuthoriseImportRequest, apitypes.DatabaseImportAuthorisation]("POST /v1/orgs/{org}/apps/{app}/db/imports")
	DbURL           = New[apitypes.DbEnvironmentRequest, apitypes.DatabaseURL]("POST /v1/orgs/{org}/apps/{app}/db/url")
	DbReader        = New[apitypes.DbEnvironmentRequest, apitypes.DatabaseURL]("POST /v1/orgs/{org}/apps/{app}/db/reader")
	DbSnapshot      = New[apitypes.DbEnvironmentRequest, apitypes.DatabaseSnapshot]("POST /v1/orgs/{org}/apps/{app}/db/snapshot")
	DbQuery         = New[apitypes.DbQueryRequest, apitypes.DatabaseQueryResult]("POST /v1/orgs/{org}/apps/{app}/db/query")
	DbSchema        = New[apitypes.DbSchemaRequest, apitypes.DatabaseSchema]("POST /v1/orgs/{org}/apps/{app}/db/schema")

	_ = public.Add(StartRestore, ListRestores, GetRestore, DiscardRestore, AuthoriseImport, DbURL, DbReader, DbSnapshot, DbQuery, DbSchema)
)
