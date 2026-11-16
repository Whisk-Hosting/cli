package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The account routes.
var (
	Whoami          = New[None, apitypes.Whoami]("GET /v1/whoami")
	RevokeSelf      = New[None, None]("DELETE /v1/tokens/self")
	GitPassword     = New[None, apitypes.GitPassword]("POST /v1/tokens/git")
	DeviceCode      = New[apitypes.DeviceCodeRequest, apitypes.DeviceCode]("POST /v1/device/code")
	DeviceInfo      = New[None, apitypes.DeviceInfo]("GET /v1/device/{code}")
	DeviceApprove   = New[apitypes.DeviceApproveRequest, apitypes.DeviceToken]("POST /v1/device/approve")
	MySessions      = New[None, apitypes.Page[apitypes.Session]]("GET /v1/me/sessions")
	RevokeMySession = New[None, None]("DELETE /v1/me/sessions/{id}")
	MyTokens        = New[None, apitypes.Page[apitypes.Token]]("GET /v1/me/tokens")
	RevokeMyToken   = New[None, None]("DELETE /v1/me/tokens/{id}")
	ListOrgs        = New[None, apitypes.Page[apitypes.Org]]("GET /v1/orgs")
	CreateOrg       = New[apitypes.CreateOrgRequest, apitypes.Org]("POST /v1/orgs")
	MarkOrgOpened   = New[None, None]("PUT /v1/orgs/{org}/opened")
	DeviceToken     = New[apitypes.DeviceTokenRequest, apitypes.DeviceToken]("POST /v1/device/token")

	_ = public.Add(Whoami, RevokeSelf, GitPassword, DeviceCode, DeviceInfo, DeviceApprove, MySessions, RevokeMySession, MyTokens, RevokeMyToken, ListOrgs, CreateOrg, MarkOrgOpened, DeviceToken)
)
