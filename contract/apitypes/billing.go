package apitypes

import "time"

// Billing (CONTROL-PLANE.md §6.15): plans, the billing page and Whisk's own payment pages.

// Interval is how often a plan is paid for.
type Interval string

const (
	Monthly Interval = "month"
	Yearly  Interval = "year"
)

// Plan is one plan as a person choosing one reads it. Intervals are the ones it is sold at here;
// empty for Free and for a plan without prices.
type Plan struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	MonthlyUSD int            `json:"monthly_usd"`
	AnnualUSD  int            `json:"annual_usd"`
	Limits     map[string]any `json:"limits"`
	Overage    map[string]any `json:"overage"`
	Intervals  []Interval     `json:"intervals"`
	// Public is whether the public pricing page shows the plan.
	Public bool `json:"public"`
}

// PricingRow is one row of the pricing page's plan table: its label and its words for Starter,
// Team, Business and Enterprise, in that order.
type PricingRow struct {
	Label  string   `json:"label"`
	Values []string `json:"values"`
}

// Pricing is GET /v1/pricing: the plans the pricing page shows and the rows of its plan table.
type Pricing struct {
	Items      []Plan       `json:"items"`
	Rows       []PricingRow `json:"rows"`
	NextCursor string       `json:"next_cursor"`
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

// NextInvoice is the next invoice the business can expect: its amount and its day.
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

// Trial is the org's running trial, if it has one, and when it ends: the plan is charged then
// unless the trial is ended first. Whisk starts no new trials.
type Trial struct {
	Active bool       `json:"active"`
	EndsAt *time.Time `json:"ends_at,omitempty"`
}

// FreeApp is whether the business has the free app; HeldBy names the business that has it in
// its place when it does not (CONTROL-PLANE.md §6.1, §6.15).
type FreeApp struct {
	Included bool   `json:"included"`
	HeldBy   string `json:"held_by,omitempty"`
}

// Billing is GET /orgs/:org/billing. PlanEndsAt is when the plan ends and the business moves to
// Free, while it is ending: a move to Free waits for the end of the period paid for; null when
// the plan is not ending. Comp is set while Whisk gives the business its plan free of charge.
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
	// Own is set on one of Whisk's own businesses, which is never billed and has the top
	// plan's features without its counts (CONTROL-PLANE.md §6.15).
	Own        bool            `json:"own,omitempty"`
	Timeline   Timeline        `json:"timeline"`
	Trial      Trial           `json:"trial"`
	PlanEndsAt *time.Time      `json:"plan_ends_at"`
	FreeApp    FreeApp         `json:"free_app"`
	Comp       *BillingComp    `json:"comp,omitempty"`
	Promoted   BillingPromoted `json:"promoted"`
	// Boosted is the business's boosted apps in force and the monthly price of Boost on each.
	Boosted BillingPromoted `json:"boosted"`
	// Licences is the monthly licences of the business's copies of priced demos that have not
	// ended (CONTROL-PLANE.md §6.1 Demos, "A price").
	Licences []BillingLicence `json:"licences"`
	// Referrals is the business's referral link and where its referrals stand; absent for a
	// business that takes no part in referrals (CONTROL-PLANE.md §6.15 "Referrals").
	Referrals *BillingReferrals `json:"referrals,omitempty"`
}

// BillingReferrals is a business's referrals on its billing page: the link to share, the live
// apps its referrals add now (BonusApps, at most CapApps, AppsEach for each), how many count and
// how many wait for their business's first payment. The bonus applies on a paid plan.
type BillingReferrals struct {
	Link      string `json:"link"`
	BonusApps int64  `json:"bonus_apps"`
	Counted   int    `json:"counted"`
	Waiting   int    `json:"waiting"`
	AppsEach  int    `json:"apps_each"`
	CapApps   int    `json:"cap_apps"`
}

// BillingLicence is one copy's monthly licence on the billing page: the copy's app, the
// business it came from and the company it was made for, its price a month, where it stands and
// the day it is paid to or, once cancelled, ends.
type BillingLicence struct {
	ID           string      `json:"id"`
	App          string      `json:"app,omitempty"`
	AppName      string      `json:"app_name"`
	From         string      `json:"from"`
	Company      string      `json:"company"`
	MonthlyCents int64       `json:"monthly_cents"`
	Status       DemoLicence `json:"status"`
	Until        *time.Time  `json:"until,omitempty"`
}

// BillingPromoted is the business's Promoted (or boosted) apps in force and the monthly price of
// each, 0 when the plan has no price for them (CONTROL-PLANE.md §6.15).
type BillingPromoted struct {
	Apps      int   `json:"apps"`
	UnitCents int64 `json:"unit_cents"`
	// NowCents is what switching it on for an app charges at once, for the days to the business's
	// next billing day (NowUntil); both absent for a business that is not charged for its extras.
	NowCents int64      `json:"now_cents,omitempty"`
	NowUntil *time.Time `json:"now_until,omitempty"`
}

// BillingComp is a comp as the business reads it: the plan is free, until ends_at or for good.
// Whisk's reason for it is not shown.
type BillingComp struct {
	EndsAt *time.Time `json:"ends_at"`
}

// PlanCheckout answers PUT /orgs/:org/billing/plan when the card is given on Stripe's checkout
// first (202); a change that needs no card answers with the org.
type PlanCheckout struct {
	CheckoutURL string `json:"checkout_url"`
	Plan        string `json:"plan"`
}

// PlanQuote answers GET /orgs/:org/billing/plan/quote?plan=&interval=: what moving to the plan
// costs. DueCents is charged today; the next invoice is NextCents on NextAt (null when nothing
// renews); from then on the plan costs ThenCents each interval. Amounts are in Currency's
// smallest unit. Sending QuotedAt back as PUT /orgs/:org/billing/plan's quoted_at, within an
// hour, makes what is charged exactly what was quoted.
type PlanQuote struct {
	Plan      string     `json:"plan"`
	Interval  string     `json:"interval"`
	Currency  string     `json:"currency"`
	DueCents  int64      `json:"due_cents"`
	NextCents int64      `json:"next_cents"`
	NextAt    *time.Time `json:"next_at"`
	ThenCents int64      `json:"then_cents"`
	QuotedAt  time.Time  `json:"quoted_at"`
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
	AmountCents    int64             `json:"amount_cents"`
	Currency       string            `json:"currency"`
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
