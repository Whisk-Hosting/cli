package endpoints

import "github.com/whisk-run/contract/apitypes"

// An app's own feature flags and Whisk's flags on for visitors (CONTROL-PLANE.md §6.37, §6.39).
var (
	AppFlags     = New[None, apitypes.AppFlagList]("GET /v1/orgs/{org}/apps/{app}/flags")
	SetAppFlag   = New[apitypes.AppFlagRequest, apitypes.AppFlag]("PUT /v1/orgs/{org}/apps/{app}/flags/{name}")
	AppFlagsOn   = New[FlagsOnQuery, apitypes.FlagsOn]("GET /v1/orgs/{org}/apps/{app}/flags/on")
	VisitorFlags = New[None, apitypes.VisitorFlags]("GET /v1/flags/visitors")

	_ = public.Add(AppFlags, SetAppFlag, AppFlagsOn, VisitorFlags)
)

// FlagsOnQuery is who AppFlagsOn asks about: a person's ID, or nobody for the flags on for
// everyone.
type FlagsOnQuery struct {
	User string `query:"user"`
}
