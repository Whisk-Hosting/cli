package api

import (
	"context"

	"github.com/whisk-run/contract/apitypes"
	"github.com/whisk-run/contract/endpoints"
)

// Managed apps (MANAGED-APPS.md §10): the products a business may add, its copies of them, and
// pausing a copy. Forks (FORKS.md): the apps a business made from a product whose copies are its
// own code.
type (
	Managed           = apitypes.Managed
	ManagedProduct    = apitypes.ManagedProduct
	ManagedAddRequest = apitypes.ManagedAddRequest
	AppManaged        = apitypes.AppManaged
	ManagedRelease    = apitypes.ManagedRelease
	AppFork           = apitypes.AppFork
	ForkUpdate        = apitypes.ForkUpdate
	ForkSuggestion    = apitypes.ForkSuggestion
	ForkSuggestions   = apitypes.ForkSuggestions
)

// ProductForks is the kind of a product whose copies are the business's own code.
const ProductForks = apitypes.ProductForks

// SetManagedSettings changes a copy's settings; a value of "" removes that setting.
func (c *Client) SetManagedSettings(ctx context.Context, org, app string, settings map[string]string) (App, error) {
	return Send(ctx, c, endpoints.SetManagedSettings, apitypes.ManagedSettingsRequest{Settings: settings}, org, app)
}

// LinkManaged links a copy, under one of its product's roles, to an app of the business by id
// (PUT /orgs/:org/apps/:app/links/:role).
func (c *Client) LinkManaged(ctx context.Context, org, app, role, appID string) (App, error) {
	return Send(ctx, c, endpoints.PutLink, apitypes.LinkRequest{App: appID}, org, app, role)
}

// SetPaused pauses a copy (POST .../pause) or resumes it (POST .../resume).
func (c *Client) SetPaused(ctx context.Context, org, app string, paused bool) (App, error) {
	if paused {
		return Call(ctx, c, endpoints.PauseApp, org, app)
	}
	return Call(ctx, c, endpoints.ResumeApp, org, app)
}
