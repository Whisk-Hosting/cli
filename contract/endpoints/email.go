package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The email routes.
var (
	GetEmail          = New[None, apitypes.EmailStatus]("GET /v1/orgs/{org}/email")
	ListEmailDomains  = New[None, map[string]any]("GET /v1/orgs/{org}/email/domains")
	VerifyEmailDomain = New[None, apitypes.EmailDomain]("POST /v1/orgs/{org}/email/domains/{id}/verify")
	ResumeEmailDomain = New[None, apitypes.EmailDomain]("POST /v1/orgs/{org}/email/domains/{id}/resume")
	DeleteEmailDomain = New[None, None]("DELETE /v1/orgs/{org}/email/domains/{id}")
	SetEmailProvider  = New[apitypes.EmailProviderRequest, apitypes.EmailProviderSet]("PUT /v1/orgs/{org}/email/provider")
	GetEmailSend      = New[None, apitypes.EmailSend]("GET /v1/orgs/{org}/apps/{app}/email/sends/{id}")

	_ = public.Add(GetEmail, ListEmailDomains, VerifyEmailDomain, ResumeEmailDomain, DeleteEmailDomain, SetEmailProvider, GetEmailSend)
)
