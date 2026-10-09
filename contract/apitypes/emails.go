package apitypes

import "time"

// What Whisk sent (CONTROL-PLANE.md §6.14): the emails Whisk sent as itself, on the operator's
// Emails page.

// SentEmailStatus is what became of an email Whisk sent.
type SentEmailStatus string

const (
	SentEmailSent       SentEmailStatus = "sent"
	SentEmailFailed     SentEmailStatus = "failed"
	SentEmailBounced    SentEmailStatus = "bounced"
	SentEmailComplained SentEmailStatus = "complained"
)

// SentEmailOrg names the business an email concerns.
type SentEmailOrg struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// SentEmail is one email Whisk sent as itself. The list leaves out the bodies; one email by id
// carries them.
type SentEmail struct {
	ID      string          `json:"id"`
	SentAt  time.Time       `json:"sent_at"`
	Kind    string          `json:"kind"`
	Org     *SentEmailOrg   `json:"org"`
	To      []string        `json:"to"`
	Subject string          `json:"subject"`
	Status  SentEmailStatus `json:"status"`
	Detail  string          `json:"detail,omitempty"`
	Text    string          `json:"text,omitempty"`
	HTML    string          `json:"html,omitempty"`
}

// SentEmailPage is a page of the emails Whisk sent, newest first.
type SentEmailPage struct {
	Items      []SentEmail `json:"items"`
	NextCursor string      `json:"next_cursor"`
}
