package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The secrets routes.
var (
	ListOrgSecrets    = New[None, apitypes.Page[apitypes.Secret]]("GET /v1/orgs/{org}/secrets")
	ListAppSecrets    = New[None, apitypes.Page[apitypes.Secret]]("GET /v1/orgs/{org}/apps/{app}/secrets")
	DeclareSecret     = New[apitypes.DeclareSecretRequest, apitypes.Secret]("POST /v1/orgs/{org}/secrets")
	SetSecretValue    = New[apitypes.SecretValueRequest, apitypes.Secret]("PUT /v1/orgs/{org}/secrets/{name}/value")
	SecretVersions    = New[SecretVersionsQuery, apitypes.Page[apitypes.SecretVersion]]("GET /v1/orgs/{org}/secrets/{name}/versions")
	RollbackSecret    = New[apitypes.RollbackSecretRequest, apitypes.Secret]("POST /v1/orgs/{org}/secrets/{name}/rollback")
	SetSecretPreviews = New[apitypes.SecretPreviewsRequest, apitypes.Secret]("PUT /v1/orgs/{org}/secrets/{name}/previews")
	ShareSecret       = New[apitypes.ShareSecretRequest, apitypes.SharedSecret]("POST /v1/orgs/{org}/secrets/{name}/share")
	SecretReads       = New[SecretReadsQuery, apitypes.Page[apitypes.SecretRead]]("GET /v1/orgs/{org}/secrets/{name}/reads")

	_ = public.Add(ListOrgSecrets, ListAppSecrets, DeclareSecret, SetSecretValue, SecretVersions, RollbackSecret, SetSecretPreviews, ShareSecret, SecretReads)
)

// SecretVersionsQuery is what SecretVersions reads from the query.
type SecretVersionsQuery struct {
	App string `query:"app"`
}

// SecretReadsQuery is what SecretReads reads from the query.
type SecretReadsQuery struct {
	App   string `query:"app"`
	Limit int    `query:"limit"`
	Since string `query:"since"`
}
