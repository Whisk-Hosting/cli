package api

import (
	"context"
	"strings"

	"github.com/whisk-run/contract/apitypes"
	"github.com/whisk-run/contract/endpoints"
)

// Call sends a declared route that takes no input (contract/endpoints) and returns its answer,
// with the client's credential, retries and error object. values fill the route's path, in
// order.
func Call[Out any](ctx context.Context, c *Client, r endpoints.Route[endpoints.None, Out], values ...string) (Out, error) {
	return endpoints.Call(ctx, c.doer, r, values...)
}

// Send sends a declared route with its input and returns its answer.
func Send[In, Out any](ctx context.Context, c *Client, r endpoints.Route[In, Out], in In, values ...string) (Out, error) {
	return endpoints.Send(ctx, c.doer, r, in, values...)
}

// doer is Do for a declared route, whose path starts with the /v1 the client adds itself.
func (c *Client) doer(ctx context.Context, method, path string, in, out any) error {
	return c.Do(ctx, method, strings.TrimPrefix(path, "/v1"), in, out)
}

// Exec sends a declared route that takes no input and answers with no body.
func Exec(ctx context.Context, c *Client, r endpoints.Route[endpoints.None, endpoints.None], values ...string) error {
	return endpoints.Exec(ctx, c.doer, r, values...)
}

// All sends a declared list route page by page and returns every item.
func All[In, T any](ctx context.Context, c *Client, r endpoints.Route[In, apitypes.Page[T]], values ...string) ([]T, error) {
	var in In
	return endpoints.All(ctx, c.doer, r, in, values...)
}

// AllOf sends a declared list route with its query page by page and returns every item.
func AllOf[In, T any](ctx context.Context, c *Client, r endpoints.Route[In, apitypes.Page[T]], in In, values ...string) ([]T, error) {
	return endpoints.All(ctx, c.doer, r, in, values...)
}
