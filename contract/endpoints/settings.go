package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The settings routes.
var (
	GetSettings      = New[None, apitypes.OrgSettings]("GET /v1/orgs/{org}/settings")
	PutSettings      = New[apitypes.OrgSettingsRequest, apitypes.OrgSettings]("PUT /v1/orgs/{org}/settings")
	MyPreferences    = New[None, apitypes.Preferences]("GET /v1/me/preferences")
	SetMyPreferences = New[apitypes.Preferences, apitypes.Preferences]("PUT /v1/me/preferences")
	CreateAgentToken = New[apitypes.CreateAgentTokenRequest, apitypes.MintedToken]("POST /v1/orgs/{org}/tokens")

	_ = public.Add(GetSettings, PutSettings, MyPreferences, SetMyPreferences, CreateAgentToken)
)
