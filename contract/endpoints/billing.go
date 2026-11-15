package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The billing routes.
var (
	ListPlans     = New[None, apitypes.Page[apitypes.Plan]]("GET /v1/billing/plans")
	GetBilling    = New[None, apitypes.Billing]("GET /v1/orgs/{org}/billing")
	QuotePlan     = New[QuotePlanQuery, apitypes.PlanQuote]("GET /v1/orgs/{org}/billing/plan/quote")
	GetUsage      = New[None, map[string]any]("GET /v1/orgs/{org}/billing/usage")
	BillingPortal = New[None, map[string]any]("POST /v1/orgs/{org}/billing/portal")
	EndTrial      = New[None, apitypes.Org]("DELETE /v1/orgs/{org}/billing/trial")
	CancelLicence = New[None, apitypes.BillingLicence]("DELETE /v1/orgs/{org}/billing/licences/{id}")

	_ = public.Add(ListPlans, GetBilling, QuotePlan, GetUsage, BillingPortal, EndTrial, CancelLicence)
)

// QuotePlanQuery is what QuotePlan reads from the query.
type QuotePlanQuery struct {
	Interval string `query:"interval"`
	Plan     string `query:"plan"`
}
