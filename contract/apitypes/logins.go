package apitypes

import "time"

// DeviceScope is the login the device approval page starts on (CONTROL-PLANE.md §4.5): the app
// whisk login is working on, one business, none until the person picks a business, or a new
// business. The whole account is a choice the person makes, never a default.
type DeviceScope string

const (
	DeviceScopeApp  DeviceScope = "app"
	DeviceScopeOrg  DeviceScope = "org"
	DeviceScopePick DeviceScope = "pick"
	DeviceScopeNew  DeviceScope = "new"
)

// DeviceSuggestion is the narrowest login that fits what whisk login said it is working on:
// the scope, the business (org_id, for app and org) and the app (for app).
type DeviceSuggestion struct {
	Scope DeviceScope `json:"scope"`
	OrgID string      `json:"org_id,omitempty"`
	App   *App        `json:"app,omitempty"`
}

// LoginScope is what a whisk login acts on: the person's whole account (the businesses recorded
// on it), one business (and any added to it), or one app.
type LoginScope string

const (
	LoginScopeAccount  LoginScope = "account"
	LoginScopeBusiness LoginScope = "business"
	LoginScopeApp      LoginScope = "app"
)

// Login is one of the person's whisk logins as they manage it (GET /me/logins/:id,
// CONTROL-PLANE.md §4.4): the token (never its value), what it acts on, the businesses it
// covers, and, when businesses can be added to it, the person's businesses it does not cover.
type Login struct {
	Token      Token           `json:"token"`
	Scope      LoginScope      `json:"scope"`
	Businesses []LoginBusiness `json:"businesses"`
	Extendable bool            `json:"extendable"`
	Addable    []Org           `json:"addable"`
}

// LoginBusiness is one business a login covers. Own is the business a login for one business
// or app was approved for, which stays on it while it lives; AddedAt is when a person added
// the business, absent for the businesses recorded when it was approved.
type LoginBusiness struct {
	Org     Org        `json:"org"`
	Own     bool       `json:"own"`
	AddedAt *time.Time `json:"added_at,omitempty"`
}

// LoginBusinessRequest is POST /me/logins/:id/businesses: the business (slug or id) to add.
type LoginBusinessRequest struct {
	Org string `json:"org"`
}
