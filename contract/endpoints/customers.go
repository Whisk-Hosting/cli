package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The customers routes.
var (
	ListCustomers       = New[None, apitypes.CustomerList]("GET /v1/orgs/{org}/apps/{app}/customers")
	EmailCustomerInvite = New[None, apitypes.Customer]("POST /v1/orgs/{org}/apps/{app}/customers/{id}/invite")
	DeleteCustomer      = New[None, None]("DELETE /v1/orgs/{org}/apps/{app}/customers/{id}")
	InviteCustomer      = New[apitypes.InviteCustomerRequest, apitypes.Customer]("POST /v1/orgs/{org}/apps/{app}/customers")
	SetCustomerSignup   = New[apitypes.SetCustomerSignupRequest, apitypes.CustomerSignupSet]("PUT /v1/orgs/{org}/apps/{app}/customers/signup")
	PatchCustomer       = New[apitypes.PatchCustomerRequest, apitypes.Customer]("PATCH /v1/orgs/{org}/apps/{app}/customers/{id}")

	_ = public.Add(ListCustomers, EmailCustomerInvite, DeleteCustomer, InviteCustomer, SetCustomerSignup, PatchCustomer)
)
