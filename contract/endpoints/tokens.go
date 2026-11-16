package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The tokens routes.
var (
	ListTokens      = New[None, apitypes.Page[apitypes.Token]]("GET /v1/orgs/{org}/tokens")
	RevokeToken     = New[None, None]("DELETE /v1/orgs/{org}/tokens/{id}")
	ListAudit       = New[ListAuditQuery, apitypes.Page[apitypes.AuditEvent]]("GET /v1/orgs/{org}/audit")
	CreateDeployKey = New[apitypes.CreateDeployKeyRequest, apitypes.DeployKeyResponse]("POST /v1/orgs/{org}/apps/{app}/deploy-keys")

	_ = public.Add(ListTokens, RevokeToken, ListAudit, CreateDeployKey)
)

// ListAuditQuery is what ListAudit reads from the query.
type ListAuditQuery struct {
	Action string `query:"action"`
	Actor  string `query:"actor"`
	Cursor string `query:"cursor"`
	Limit  int    `query:"limit"`
	Since  string `query:"since"`
}
