package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The route that makes an app paused for want of room its business's active app.
var (
	MakeAppActive = New[None, apitypes.App]("POST /v1/orgs/{org}/apps/{app}/active")

	_ = public.Add(MakeAppActive)
)
