package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The domains routes.
var (
	ListDomains       = New[None, apitypes.Page[apitypes.Domain]]("GET /v1/orgs/{org}/apps/{app}/domains")
	VerifyDomain      = New[None, apitypes.Domain]("POST /v1/orgs/{org}/apps/{app}/domains/{id}/verify")
	SetDomainRedirect = New[apitypes.DomainRedirectRequest, apitypes.Domain]("PUT /v1/orgs/{org}/apps/{app}/domains/{id}/redirect")
	SetMainAddress    = New[apitypes.MainAddressRequest, apitypes.Page[apitypes.Domain]]("PUT /v1/orgs/{org}/apps/{app}/main-address")

	_ = public.Add(ListDomains, VerifyDomain, SetDomainRedirect, SetMainAddress)
)
