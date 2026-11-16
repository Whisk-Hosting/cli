package api

import (
	"context"

	"github.com/whisk-run/contract/apitypes"
	"github.com/whisk-run/contract/endpoints"
)

// CLIMiss is one command line the CLI could not run, by its shape (CLI.md §7).
type CLIMiss = apitypes.CLIMiss

// ReportCLIMiss posts POST /v1/cli/misses. It works without a token.
func (c *Client) ReportCLIMiss(ctx context.Context, m CLIMiss) error {
	_, err := Send(ctx, c, endpoints.PostCLIMiss, m)
	return err
}
