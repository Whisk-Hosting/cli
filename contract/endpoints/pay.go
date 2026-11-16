package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The pay routes.
var (
	GetPay     = New[None, apitypes.PayPage]("GET /v1/orgs/{org}/billing/pay/{id}")
	StartPay   = New[apitypes.PayDetails, apitypes.PayStep]("POST /v1/orgs/{org}/billing/pay/{id}/start")
	FinishPay  = New[None, apitypes.PayStep]("POST /v1/orgs/{org}/billing/pay/{id}/finish")
	PayInvoice = New[None, apitypes.PageURL]("POST /v1/orgs/{org}/billing/invoices/{invoice}/pay")

	_ = public.Add(GetPay, StartPay, FinishPay, PayInvoice)
)
