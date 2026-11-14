package api

import (
	"context"
	"net/http"

	"github.com/whisk-run/contract/apitypes"
)

// CLIMiss is one command line the CLI could not run, by its shape (CLI.md §7).
type CLIMiss = apitypes.CLIMiss

// ReportCLIMiss posts POST /v1/cli/misses. It works without a token.
func (c *Client) ReportCLIMiss(ctx context.Context, m CLIMiss) error {
	var out apitypes.CLIMissReceipt
	return c.Do(ctx, http.MethodPost, "/cli/misses", m, &out)
}
