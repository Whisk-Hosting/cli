package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The routes of control-plane/internal/api/handlers_aikeys.go.
var (
	ReleaseSecretHold = New[ReleaseSecretHoldQuery, apitypes.Secret]("DELETE /v1/orgs/{org}/secrets/{name}/hold")

	_ = public.Add(ReleaseSecretHold)
)

// ReleaseSecretHoldQuery is what ReleaseSecretHold reads from the query.
type ReleaseSecretHoldQuery struct {
	App string `query:"app"`
}
