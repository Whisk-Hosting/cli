package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The inbox routes.
var (
	GetInbox          = New[None, apitypes.Inbox]("GET /v1/orgs/{org}/apps/{app}/inbox")
	AddInboxDomain    = New[apitypes.InboxDomainRequest, apitypes.InboxDomain]("POST /v1/orgs/{org}/apps/{app}/inbox/domains")
	VerifyInboxDomain = New[None, apitypes.InboxDomain]("POST /v1/orgs/{org}/apps/{app}/inbox/domains/{id}/verify")
	DeleteInboxDomain = New[None, None]("DELETE /v1/orgs/{org}/apps/{app}/inbox/domains/{id}")

	_ = public.Add(GetInbox, AddInboxDomain, VerifyInboxDomain, DeleteInboxDomain)
)
