package api

import (
	"context"
	"net/http"

	"github.com/whisk-run/contract/apitypes"
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
)

// ProductForks is the kind of a product whose copies are the business's own code.
const ProductForks = apitypes.ProductForks

// ListManaged reads GET /orgs/:org/managed: the products and the business's copies.
func (c *Client) ListManaged(ctx context.Context, org string) (Managed, error) {
	var out Managed
	return out, c.Do(ctx, http.MethodGet, "/orgs/"+pathSeg(org)+"/managed", nil, &out)
}

// AddManaged adds a copy of a product, or a fork of one whose copies are the business's own code;
// the app comes back, deploying the newest release.
func (c *Client) AddManaged(ctx context.Context, org string, req ManagedAddRequest) (App, error) {
	var out App
	return out, c.Do(ctx, http.MethodPost, "/orgs/"+pathSeg(org)+"/managed", req, &out)
}

// SetManagedSettings changes a copy's settings; a value of "" removes that setting.
func (c *Client) SetManagedSettings(ctx context.Context, org, app string, settings map[string]string) (App, error) {
	var out App
	return out, c.Do(ctx, http.MethodPatch, appPath(org, app)+"/managed", apitypes.ManagedSettingsRequest{Settings: settings}, &out)
}

// LinkManaged links a copy, under one of its product's roles, to an app of the business by id
// (PUT /orgs/:org/apps/:app/links/:role).
func (c *Client) LinkManaged(ctx context.Context, org, app, role, appID string) (App, error) {
	var out App
	return out, c.Do(ctx, http.MethodPut, appPath(org, app)+"/links/"+pathSeg(role), apitypes.LinkRequest{App: appID}, &out)
}

// SetPaused pauses a copy (POST .../pause) or resumes it (POST .../resume).
func (c *Client) SetPaused(ctx context.Context, org, app string, paused bool) (App, error) {
	verb := "/resume"
	if paused {
		verb = "/pause"
	}
	var out App
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+verb, nil, &out)
}

// UpdateFork brings a fork to its product's newest release (POST .../fork/update): the app as it
// stands, the deploy that carries its code when there is one, and whether its code moved. Code the
// business changed is never moved: that answers FORK_CHANGED.
func (c *Client) UpdateFork(ctx context.Context, org, app string) (ForkUpdate, error) {
	var out ForkUpdate
	return out, c.Do(ctx, http.MethodPost, appPath(org, app)+"/fork/update", nil, &out)
}
