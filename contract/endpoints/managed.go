package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The managed routes.
var (
	ListManaged        = New[None, apitypes.Managed]("GET /v1/orgs/{org}/managed")
	AddManaged         = New[apitypes.ManagedAddRequest, apitypes.App]("POST /v1/orgs/{org}/managed")
	SetManagedSettings = New[apitypes.ManagedSettingsRequest, apitypes.App]("PATCH /v1/orgs/{org}/apps/{app}/managed")
	PutLink            = New[apitypes.LinkRequest, apitypes.App]("PUT /v1/orgs/{org}/apps/{app}/links/{role}")
	PauseApp           = New[None, apitypes.App]("POST /v1/orgs/{org}/apps/{app}/pause")
	ResumeApp          = New[None, apitypes.App]("POST /v1/orgs/{org}/apps/{app}/resume")
	UpdateFork         = New[None, apitypes.ForkUpdate]("POST /v1/orgs/{org}/apps/{app}/fork/update")
	SetForkChannel     = New[apitypes.ForkChannelRequest, apitypes.App]("PUT /v1/orgs/{org}/apps/{app}/fork/channel")
	ForkChanges        = New[None, apitypes.ForkChanges]("GET /v1/orgs/{org}/apps/{app}/fork/changes")
	SuggestChange      = New[apitypes.ForkSuggestRequest, apitypes.ForkSuggestion]("POST /v1/orgs/{org}/apps/{app}/fork/suggestions")
	ForkSuggestions    = New[None, apitypes.ForkSuggestions]("GET /v1/orgs/{org}/apps/{app}/fork/suggestions")

	_ = public.Add(ListManaged, AddManaged, SetManagedSettings, PutLink, PauseApp, ResumeApp, UpdateFork, SetForkChannel, ForkChanges, SuggestChange, ForkSuggestions)
)
