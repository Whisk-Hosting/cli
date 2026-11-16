package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The cdn routes.
var (
	AppCDN      = New[None, apitypes.AppCdn]("GET /v1/orgs/{org}/apps/{app}/cdn")
	PurgeAppCDN = New[apitypes.CdnPurgeRequest, apitypes.CdnPurge]("POST /v1/orgs/{org}/apps/{app}/cdn/purge")
	SetAppCDN   = New[apitypes.AppCDNRequest, apitypes.AppCdn]("PUT /v1/orgs/{org}/apps/{app}/cdn")

	_ = public.Add(AppCDN, PurgeAppCDN, SetAppCDN)
)
