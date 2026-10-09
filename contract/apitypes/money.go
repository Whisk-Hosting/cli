package apitypes

import "time"

// The operator's money page, GET /operator/money (CONTROL-PLANE.md §6.20, "Money"): what the
// businesses pay, what is coming, what needs a person, and what reached Stripe's balance. Charges
// are in US cents; what reached the balance is in the account's payout currency, null when Stripe
// could not be read for it.

// MoneyStripeState is how the page read Stripe: in full, not at all because it did not answer,
// or not at all because no key is set.
type MoneyStripeState string

const (
	MoneyStripeOK          MoneyStripeState = "ok"
	MoneyStripeUnavailable MoneyStripeState = "unavailable"
	MoneyStripeNotSet      MoneyStripeState = "not_set"
)

// MoneyAttentionKind is what needs a person.
type MoneyAttentionKind string

const (
	AttentionPaymentFailed MoneyAttentionKind = "payment_failed"
	AttentionDispute       MoneyAttentionKind = "dispute"
	AttentionCardExpiring  MoneyAttentionKind = "card_expiring"
	AttentionTrialEnding   MoneyAttentionKind = "trial_ending"
	AttentionDrift         MoneyAttentionKind = "drift"
)

// MoneyUpcomingWhy is why a charge is coming: a trial ends, a period renews, or a plan ends,
// which charges nothing.
type MoneyUpcomingWhy string

const (
	UpcomingTrialEnds MoneyUpcomingWhy = "trial_ends"
	UpcomingRenews    MoneyUpcomingWhy = "renews"
	UpcomingPlanEnds  MoneyUpcomingWhy = "plan_ends"
)

// MoneySubscriptionState is where a business with a subscription, or on a comp, stands.
type MoneySubscriptionState string

const (
	SubTrial         MoneySubscriptionState = "trial"
	SubPaying        MoneySubscriptionState = "paying"
	SubPaymentFailed MoneySubscriptionState = "payment_failed"
	SubEnding        MoneySubscriptionState = "ending"
	SubComped        MoneySubscriptionState = "comped"
)

// MoneyEventKind is a kind of movement of money.
type MoneyEventKind string

const (
	EventCharge       MoneyEventKind = "charge"
	EventTrialInvoice MoneyEventKind = "trial_invoice"
	EventRefund       MoneyEventKind = "refund"
	EventFailed       MoneyEventKind = "failed"
	EventDispute      MoneyEventKind = "dispute"
	EventCredit       MoneyEventKind = "credit"
	EventRebate       MoneyEventKind = "rebate"
)

// MoneyPayoutStatus is where a payout to the bank stands. A cancelled payout is not shown.
type MoneyPayoutStatus string

const (
	MoneyPayoutPaid      MoneyPayoutStatus = "paid"
	MoneyPayoutInTransit MoneyPayoutStatus = "in_transit"
	MoneyPayoutPending   MoneyPayoutStatus = "pending"
	MoneyPayoutFailed    MoneyPayoutStatus = "failed"
)

// Money is GET /operator/money.
type Money struct {
	Currency          string              `json:"currency"`
	PayoutCurrency    string              `json:"payout_currency"`
	Stripe            MoneyStripeState    `json:"stripe"`
	MonthlyCents      int64               `json:"monthly_cents"`
	Paying            int                 `json:"paying"`
	Trialing          int                 `json:"trialing"`
	CollectedCents    int64               `json:"collected_cents"`
	CollectedNetCents *int64              `json:"collected_net_cents"`
	FeesCents         *int64              `json:"fees_cents"`
	UpcomingCents     int64               `json:"upcoming_cents"`
	OwedCents         int64               `json:"owed_cents"`
	Attention         []MoneyAttention    `json:"attention"`
	Upcoming          []MoneyUpcoming     `json:"upcoming"`
	Subscriptions     []MoneySubscription `json:"subscriptions"`
	Events            []MoneyEvent        `json:"events"`
	Payouts           []MoneyPayout       `json:"payouts"`
	Months            []MoneyMonth        `json:"months"`
}

// MoneyAttention is one thing that needs a person.
type MoneyAttention struct {
	Kind    MoneyAttentionKind `json:"kind"`
	Org     string             `json:"org"`
	OrgName string             `json:"org_name"`
	Detail  string             `json:"detail"`
	At      time.Time          `json:"at"`
}

// MoneyUpcoming is one charge in the next 30 days.
type MoneyUpcoming struct {
	Org      string           `json:"org"`
	OrgName  string           `json:"org_name"`
	Plan     string           `json:"plan"`
	Interval Interval         `json:"interval"`
	Cents    int64            `json:"cents"`
	At       time.Time        `json:"at"`
	Why      MoneyUpcomingWhy `json:"why"`
}

// MoneySubscription is one business that pays, tries a plan, or is comped.
type MoneySubscription struct {
	Org       string                 `json:"org"`
	OrgName   string                 `json:"org_name"`
	Plan      string                 `json:"plan"`
	Interval  Interval               `json:"interval"`
	State     MoneySubscriptionState `json:"state"`
	Since     time.Time              `json:"since"`
	EndsAt    *time.Time             `json:"ends_at,omitempty"`
	NextAt    *time.Time             `json:"next_at,omitempty"`
	NextCents *int64                 `json:"next_cents,omitempty"`
	Card      *Card                  `json:"card,omitempty"`
	PaidCents int64                  `json:"paid_cents"`
	StripeURL string                 `json:"stripe_url,omitempty"`
}

// MoneyEvent is one movement of money.
type MoneyEvent struct {
	ID            string         `json:"id"`
	At            time.Time      `json:"at"`
	Org           string         `json:"org"`
	OrgName       string         `json:"org_name"`
	Kind          MoneyEventKind `json:"kind"`
	Cents         int64          `json:"cents"`
	NetCents      *int64         `json:"net_cents"`
	FeeCents      *int64         `json:"fee_cents"`
	InvoiceNumber string         `json:"invoice_number,omitempty"`
	PDFURL        string         `json:"pdf_url,omitempty"`
	StripeURL     string         `json:"stripe_url,omitempty"`
}

// MoneyPayout is one payout from Stripe's balance to the bank.
type MoneyPayout struct {
	ID        string            `json:"id"`
	At        time.Time         `json:"at"`
	ArrivesAt time.Time         `json:"arrives_at"`
	Cents     int64             `json:"cents"`
	Status    MoneyPayoutStatus `json:"status"`
	StripeURL string            `json:"stripe_url,omitempty"`
}

// MoneyMonth is one month's figures, the month as "2006-01" in New Zealand time.
type MoneyMonth struct {
	Month          string `json:"month"`
	MonthlyCents   int64  `json:"monthly_cents"`
	CollectedCents int64  `json:"collected_cents"`
}
