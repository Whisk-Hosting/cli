package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The customers routes.
var (
	ListCustomers       = New[None, apitypes.CustomerList]("GET /v1/orgs/{org}/apps/{app}/customers")
	EmailCustomerInvite = New[None, apitypes.Customer]("POST /v1/orgs/{org}/apps/{app}/customers/{id}/invite")
	DeleteCustomer      = New[None, None]("DELETE /v1/orgs/{org}/apps/{app}/customers/{id}")

	_ = public.Add(ListCustomers, EmailCustomerInvite, DeleteCustomer)
)
