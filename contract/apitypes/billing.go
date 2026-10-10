package apitypes

import (
	"time"

	"github.com/whisk-run/contract/status"
)

// Billing (CONTROL-PLANE.md §6.15): plans, the billing page, Whisk's own payment pages, an
// agency's client businesses and its rebate, and the operator's Stripe account.

// Interval is how often a plan is paid for.
type Interval string

const (
	Monthly Interval = "month"
	Yearly  Interval = "year"
)

// Plan is one plan as a person choosing one reads it. Intervals are the ones it is sold at here;
// empty for Free and for a plan without prices. ClientMonthlyUSD and ClientAnnualUSD are what
// each client business an agency adds costs on the plan (Team, and Agency for the client
// businesses already on it), and ClientIntervals the intervals client businesses are sold at
// here; zero and empty on every other plan.
type Plan struct {
	ID               string         `json:"id"`
	Name             string         `json:"name"`
	MonthlyUSD       int            `json:"monthly_usd"`
	AnnualUSD        int            `json:"annual_usd"`
	Limits           map[string]any `json:"limits"`
	Overage          map[string]any `json:"overage"`
	Intervals        []Interval     `json:"intervals"`
	ClientMonthlyUSD int            `json:"client_monthly_usd"`
	ClientAnnualUSD  int            `json:"client_annual_usd"`
	ClientIntervals  []Interval     `json:"client_intervals"`
}

// InvoiceStatus is where an invoice stands at Stripe.
type InvoiceStatus string

const (
	InvoiceDraft         InvoiceStatus = "draft"
	InvoiceOpen          InvoiceStatus = "open"
	InvoicePaid          InvoiceStatus = "paid"
	InvoiceUncollectible InvoiceStatus = "uncollectible"
	InvoiceVoid          InvoiceStatus = "void"
)

// Invoice is one invoice on the billing page. URL is Stripe's page for paying it; PDFURL is the
// invoice in the Whisk design.
type Invoice struct {
	ID          string        `json:"id"`
	Number      string        `json:"number"`
	Period      string        `json:"period"`
	AmountCents int64         `json:"amount_cents"`
	Currency    string        `json:"currency"`
	Status      InvoiceStatus `json:"status"`
	URL         string        `json:"url,omitempty"`
	PDFURL      string        `json:"pdf_url,omitempty"`
	PaidAt      *time.Time    `json:"paid_at,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
}

// NextInvoice is Stripe's preview of the next invoice.
type NextInvoice struct {
	AmountCents int64      `json:"amount_cents"`
	Currency    string     `json:"currency"`
	At          *time.Time `json:"at,omitempty"`
}

// Card is the customer's default payment method, as far as a person needs to recognise it.
type Card struct {
	Brand    string `json:"brand"`
	Last4    string `json:"last4"`
	ExpMonth int    `json:"exp_month"`
	ExpYear  int    `json:"exp_year"`
}

// Trial is the org's one Starter trial: whether the person reading may start one, and while one
// runs, when it ends. Starter is charged then unless the trial is ended first. UsedElsewhere
// names the business the reader had their one trial on, when that is why there is none here.
type Trial struct {
	Plan          string     `json:"plan"`
	Days          int        `json:"days"`
	Available     bool       `json:"available"`
	Active        bool       `json:"active"`
	EndsAt        *time.Time `json:"ends_at,omitempty"`
	UsedAt        *time.Time `json:"used_at,omitempty"`
	UsedElsewhere string     `json:"used_elsewhere,omitempty"`
}

// FreeApp is whether the business has the free app; HeldBy names the business that has it in
// its place when it does not (CONTROL-PLANE.md §6.1, §6.15).
type FreeApp struct {
	Included bool   `json:"included"`
	HeldBy   string `json:"held_by,omitempty"`
}

// Billing is GET /orgs/:org/billing. PlanEndsAt is when the plan ends and the business moves to
// Free, while it is ending: a move to Free waits for the end of the period paid for; null when
// the plan is not ending. ClientOf is the agency a client business is billed through; HandedOver
// is set on a client business its agency has handed over, whose owners may choose a plan of
// their own; Comp is set while Whisk gives the business its plan free of charge.
type Billing struct {
	Plan         Plan             `json:"plan"`
	Interval     Interval         `json:"interval"`
	Period       string           `json:"period"`
	Usage        map[string]int64 `json:"usage"`
	Limits       map[string]int64 `json:"limits"`
	OverageCents int64            `json:"overage_cents"`
	NextInvoice  *NextInvoice     `json:"next_invoice,omitempty"`
	Card         *Card            `json:"card,omitempty"`
	Invoices     []Invoice        `json:"invoices"`
	Payments     bool             `json:"payments"`
	Canary       bool             `json:"canary,omitempty"`
	Timeline     Timeline         `json:"timeline"`
	Trial        Trial            `json:"trial"`
	PlanEndsAt   *time.Time       `json:"plan_ends_at"`
	FreeApp      FreeApp          `json:"free_app"`
	ClientOf     *OrgRef          `json:"client_of,omitempty"`
	HandedOver   bool             `json:"handed_over,omitempty"`
	Comp         *BillingComp     `json:"comp,omitempty"`
	Promoted     BillingPromoted  `json:"promoted"`
	// Boosted is the business's boosted apps in force and the monthly price of Boost on each.
	Boosted BillingPromoted `json:"boosted"`
}

// BillingPromoted is the business's Promoted (or boosted) apps in force and the monthly price of
// each, 0 when the plan has no price for them (CONTROL-PLANE.md §6.15).
type BillingPromoted struct {
	Apps      int   `json:"apps"`
	UnitCents int64 `json:"unit_cents"`
}

// BillingComp is a comp as the business reads it: the plan is free, until ends_at or for good.
// The operator's reason is theirs and is not shown.
type BillingComp struct {
	EndsAt *time.Time `json:"ends_at"`
}

// OrgRef names another business.
type OrgRef struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// PlanCheckout answers PUT /orgs/:org/billing/plan when the card is given on Stripe's checkout
// first (202); a change that needs no card answers with the org.
type PlanCheckout struct {
	CheckoutURL string `json:"checkout_url"`
	Plan        string `json:"plan"`
}

// TrialStart answers POST /orgs/:org/billing/trial: Stripe's checkout where the card for the
// trial is given.
type TrialStart struct {
	CheckoutURL string `json:"checkout_url"`
	Plan        string `json:"plan"`
	Days        int    `json:"days"`
}

// PayKind is what one of Whisk's payment pages is for: a subscription, a card for later, or an
// open invoice.
type PayKind string

const (
	PaySubscribe PayKind = "subscribe"
	PayCard      PayKind = "card"
	PayInvoice   PayKind = "invoice"
)

// PayInterval is a payment page's interval: a plan's, or none for a card or an invoice.
type PayInterval string

const (
	PayMonthly    PayInterval = "month"
	PayYearly     PayInterval = "year"
	PayNoInterval PayInterval = ""
)

// PayPage is GET /orgs/:org/billing/pay/:id: one of Whisk's own payment pages.
type PayPage struct {
	ID             string            `json:"id"`
	Kind           PayKind           `json:"kind"`
	Plan           string            `json:"plan"`
	PlanName       string            `json:"plan_name"`
	Interval       PayInterval       `json:"interval"`
	Days           int               `json:"days"`
	AmountCents    int64             `json:"amount_cents"`
	Currency       string            `json:"currency"`
	For            string            `json:"for"`
	Invoice        string            `json:"invoice"`
	PublishableKey string            `json:"publishable_key"`
	Details        CustomerDetails   `json:"details"`
	TaxTypes       map[string]string `json:"tax_types"`
	CancelURL      string            `json:"cancel_url"`
	Done           bool              `json:"done"`
}

// PostalAddress is a postal address as Stripe keeps it.
type PostalAddress struct {
	Line1      string `json:"line1"`
	Line2      string `json:"line2"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
}

// CustomerDetails is what an invoice names of the business paying it.
type CustomerDetails struct {
	Name    string          `json:"name"`
	Email   string          `json:"email"`
	Address PostalAddress   `json:"address"`
	TaxIDs  []CustomerTaxID `json:"tax_ids"`
}

// CustomerTaxID is one of the customer's tax numbers.
type CustomerTaxID struct {
	ID    string `json:"id"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// PayDetails is POST /orgs/:org/billing/pay/:id/start: the name and address invoices are made
// out to, and a tax number where the country has one Stripe knows.
type PayDetails struct {
	Name    string        `json:"name"`
	Address PostalAddress `json:"address"`
	TaxID   string        `json:"tax_id"`
}

// PayStepKind is what a payment page's browser does next.
type PayStepKind string

const (
	StepSetup   PayStepKind = "setup"
	StepPayment PayStepKind = "payment"
	StepDone    PayStepKind = "done"
)

// PayStep is what the page's browser does next: confirm a card (setup), confirm a payment (with
// PaymentMethod, the card already given, when there is one), or go to ReturnURL.
type PayStep struct {
	Step          PayStepKind `json:"step"`
	ClientSecret  string      `json:"client_secret,omitempty"`
	PaymentMethod string      `json:"payment_method,omitempty"`
	ReturnURL     string      `json:"return_url,omitempty"`
}

// ClientBilling is where a client business stands with its billing.
type ClientBilling string

const (
	ClientUnbilled ClientBilling = "unbilled"
	ClientTrialing ClientBilling = "trialing"
	ClientPaying   ClientBilling = "active"
	ClientPastDue  ClientBilling = "past_due"
	ClientFrozen   ClientBilling = "frozen"
	ClientDeleting ClientBilling = "shredding"
)

// ClientBusiness is one client business as its agency's billing page lists it
// (GET /orgs/:org/clients). PriceCents is what it costs each interval once paid for; zero while
// unbilled. Handover is who it will be handed to, the link while it has not been, and when it
// was.
type ClientBusiness struct {
	ID          string        `json:"id"`
	Slug        string        `json:"slug"`
	Name        string        `json:"name"`
	Status      status.Org    `json:"status"`
	Plan        string        `json:"plan"`
	Interval    Interval      `json:"interval"`
	Billing     ClientBilling `json:"billing"`
	PriceCents  int64         `json:"price_cents"`
	TrialEndsAt *time.Time    `json:"trial_ends_at,omitempty"`
	PastDueAt   *time.Time    `json:"past_due_at,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
	Handover    Handover      `json:"handover"`
}

// Handover is a client business's handover (CONTROL-PLANE.md §6.15).
type Handover struct {
	Contact      string     `json:"contact,omitempty"`
	ClaimURL     string     `json:"claim_url,omitempty"`
	HandedOverAt *time.Time `json:"handed_over_at,omitempty"`
}

// ClientTrial is the agency's one free client month.
type ClientTrial struct {
	Days          int        `json:"days"`
	Available     bool       `json:"available"`
	UsedAt        *time.Time `json:"used_at,omitempty"`
	UsedOn        string     `json:"used_on,omitempty"`
	UsedElsewhere string     `json:"used_elsewhere,omitempty"`
}

// AgencyBlock is why a business may not add client businesses now.
type AgencyBlock string

const (
	AgencyPending  AgencyBlock = "pending"
	AgencyInactive AgencyBlock = "inactive"
	AgencyIsClient AgencyBlock = "client"
	AgencyCanary   AgencyBlock = "canary"
	AgencyUnpaid   AgencyBlock = "unpaid"
)

// Clients is GET /orgs/:org/clients. Prices is what a client business costs, in cents, at each
// interval sold here; MonthlyCents what the client businesses being paid for cost a month, a
// yearly one counted as a twelfth. CanAdd is whether the org may add a client business now;
// Blocked says why not when it may not.
type Clients struct {
	Items        []ClientBusiness `json:"items"`
	Prices       map[string]int64 `json:"prices"`
	MonthlyCents int64            `json:"monthly_cents"`
	Limits       map[string]any   `json:"limits"`
	Trial        ClientTrial      `json:"trial"`
	CanAdd       bool             `json:"can_add"`
	Blocked      AgencyBlock      `json:"blocked,omitempty"`
	Payments     bool             `json:"payments"`
}

// ClientAdded is the answer to adding or paying for a client business: 201 (or 200) when it is
// paid for at once on the agency's card, 202 with Stripe's checkout when the card is given first.
type ClientAdded struct {
	Client      Org    `json:"client"`
	CheckoutURL string `json:"checkout_url,omitempty"`
	Trial       bool   `json:"trial"`
}

// HandoverMade is the answer to making a client business's handover link.
type HandoverMade struct {
	ClaimURL string `json:"claim_url"`
	Contact  string `json:"contact"`
}

// ClientOverview is one client business on the agency's all-clients screen
// (GET /orgs/:org/clients/overview): no prices. Apps is its live apps, Problems how many of them
// are broken or blocked, and FailedDeploys its failed deploys in the last 24 hours.
type ClientOverview struct {
	ID            string        `json:"id"`
	Slug          string        `json:"slug"`
	Name          string        `json:"name"`
	Status        status.Org    `json:"status"`
	Billing       ClientBilling `json:"billing"`
	HandedOver    bool          `json:"handed_over"`
	Apps          int           `json:"apps"`
	Problems      int           `json:"problems"`
	FailedDeploys int           `json:"failed_deploys"`
	Usage         ClientUsage   `json:"usage"`
	Limits        ClientUsage   `json:"limits"`
}

// ClientUsage is a client business's storage and this month's runs, or the plan's allowance of
// each.
type ClientUsage struct {
	StorageBytes int64 `json:"storage_bytes"`
	Runs         int64 `json:"runs"`
}

// RebateSettlement is one month's rebate settled: what came off the bill and what was paid out.
type RebateSettlement struct {
	Month            string    `json:"month"`
	EarnedCents      int64     `json:"earned_cents"`
	CreditCents      int64     `json:"credit_cents"`
	PayoutCents      int64     `json:"payout_cents"`
	PayoutCurrency   string    `json:"payout_currency,omitempty"`
	PayoutLocalCents int64     `json:"payout_local_cents,omitempty"`
	PaidOut          bool      `json:"paid_out"`
	SettledAt        time.Time `json:"settled_at"`
}

// PayoutAccount is where the agency's payout account stands at Stripe.
type PayoutAccount string

const (
	PayoutNone    PayoutAccount = "none"
	PayoutPending PayoutAccount = "pending"
	PayoutReady   PayoutAccount = "ready"
)

// Payouts is where the agency's payouts stand: the one country Stripe pays out to, and the
// account.
type Payouts struct {
	Country string        `json:"country,omitempty"`
	Account PayoutAccount `json:"account"`
}

// Rebates is GET /orgs/:org/billing/rebates: an agency's rebate, 20% of what the clients it
// built pay for themselves, off its own bill first and the rest paid out.
type Rebates struct {
	Percent         int                `json:"percent"`
	Built           int                `json:"built"`
	ThisMonthCents  int64              `json:"this_month_cents"`
	Settlements     []RebateSettlement `json:"settlements"`
	Payouts         Payouts            `json:"payouts"`
	PaymentsEnabled bool               `json:"payments"`
}

// StripeSource is where the Stripe key in use comes from.
type StripeSource string

const (
	StripeFromOperator    StripeSource = "operator"
	StripeFromEnvironment StripeSource = "environment"
	StripeNone            StripeSource = "none"
)

// StripeMode is whether a Stripe key is live or test; empty while none is set.
type StripeMode string

const (
	StripeLive   StripeMode = "live"
	StripeTest   StripeMode = "test"
	StripeNoMode StripeMode = ""
)

// StripeAccount is which Stripe account billing uses, as the operator page shows it: the key the
// operator pasted, else the server's environment's. The key is never part of it. StandIn is
// whether the environment's key talks to a stand-in (stripe-mock), not Stripe; Charges whether
// Stripe lets the account take payments now, null when it could not be read; OwnPages whether
// people give cards on Whisk's own pages rather than Stripe's. Drift is what the last daily
// comparison with Stripe found, DriftCheckedAt when it ran (absent before the first) and
// DriftOrgs how many businesses it compared.
type StripeAccount struct {
	Source         StripeSource   `json:"source"`
	StandIn        bool           `json:"stand_in"`
	Set            bool           `json:"set"`
	Mode           StripeMode     `json:"mode"`
	KeyHint        string         `json:"key_hint"`
	AccountID      string         `json:"account_id"`
	AccountName    string         `json:"account_name"`
	Webhook        bool           `json:"webhook"`
	Charges        *bool          `json:"charges_enabled"`
	Prices         int            `json:"prices"`
	OwnPages       bool           `json:"own_pages"`
	SetBy          string         `json:"set_by"`
	SetAt          *time.Time     `json:"set_at,omitempty"`
	Drift          []BillingDrift `json:"drift"`
	DriftCheckedAt *time.Time     `json:"drift_checked_at,omitempty"`
	DriftOrgs      int            `json:"drift_orgs"`
}

// StripeKeyRequest is PUT /operator/payments/stripe.
type StripeKeyRequest struct {
	APIKey         string `json:"api_key"`
	PublishableKey string `json:"publishable_key"`
}

// DriftKind is one kind of difference between Stripe and Whisk's records.
type DriftKind string

const (
	DriftCustomerMissing     DriftKind = "customer_missing"
	DriftSubscriptionUnknown DriftKind = "subscription_unknown"
	DriftSubscriptionMissing DriftKind = "subscription_missing"
	DriftSubscriptionPlan    DriftKind = "subscription_plan"
	DriftSubscriptionItems   DriftKind = "subscription_items"
	DriftSubscriptionPaused  DriftKind = "subscription_paused"
	DriftInvoiceUnrecorded   DriftKind = "invoice_unrecorded"
	DriftInvoiceStale        DriftKind = "invoice_stale"
)

// BillingDrift is one difference the daily comparison found between Stripe and Whisk's records
// (CONTROL-PLANE.md §6.15): Kind names it, Ref is Stripe's id it is about, Detail says it in a
// sentence.
type BillingDrift struct {
	OrgID   string    `json:"org_id"`
	OrgSlug string    `json:"org"`
	Kind    DriftKind `json:"kind"`
	Ref     string    `json:"ref,omitempty"`
	Detail  string    `json:"detail"`
	FoundAt time.Time `json:"found_at"`
}
