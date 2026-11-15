package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The org domains routes.
var (
	ListOrgDomains  = New[None, apitypes.Page[apitypes.OrgDomain]]("GET /v1/orgs/{org}/domains")
	AddOrgDomain    = New[apitypes.OrgDomainRequest, apitypes.OrgDomain]("POST /v1/orgs/{org}/domains")
	VerifyOrgDomain = New[None, apitypes.OrgDomain]("POST /v1/orgs/{org}/domains/{id}/verify")
	DeleteOrgDomain = New[None, None]("DELETE /v1/orgs/{org}/domains/{id}")

	_ = public.Add(ListOrgDomains, AddOrgDomain, VerifyOrgDomain, DeleteOrgDomain)
)
