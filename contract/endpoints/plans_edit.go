package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The plans edit routes.
var (
	PublicPricing = New[None, apitypes.Pricing]("GET /v1/pricing")

	_ = public.Add(PublicPricing)
)
