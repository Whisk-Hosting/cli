package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The app email domains routes.
var (
	ListAppEmailDomains  = New[None, map[string]any]("GET /v1/orgs/{org}/apps/{app}/email/domains")
	GetAppEmailDomain    = New[None, apitypes.EmailDomain]("GET /v1/orgs/{org}/apps/{app}/email/domains/{id}")
	VerifyAppEmailDomain = New[None, apitypes.EmailDomain]("POST /v1/orgs/{org}/apps/{app}/email/domains/{id}/verify")
	DeleteAppEmailDomain = New[None, None]("DELETE /v1/orgs/{org}/apps/{app}/email/domains/{id}")

	_ = public.Add(ListAppEmailDomains, GetAppEmailDomain, VerifyAppEmailDomain, DeleteAppEmailDomain)
)
