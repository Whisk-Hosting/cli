package apitypes

import "time"

// EmailSent is what the caller learns about a message that went.
type EmailSent struct {
	ID         string `json:"id"`
	ProviderID string `json:"provider_id,omitempty"`
	Recipients int    `json:"recipients"`
	// Held are the recipients a preview left out: everyone not on the business's team.
	Held   []string  `json:"held,omitempty"`
	SentAt time.Time `json:"sent_at"`
}

// SendEmailRequest is the body of POST /v1/orgs/{org}/apps/{app}/email/send. To is one address
// or a list of them.
type SendEmailRequest struct {
	To          any               `json:"to"`
	From        string            `json:"from"`
	Subject     string            `json:"subject"`
	HTML        string            `json:"html"`
	Text        string            `json:"text"`
	ReplyTo     string            `json:"reply_to"`
	Attachments []EmailAttachment `json:"attachments"`
}

var _ = more.addWire(EmailSent{}, SendEmailRequest{})
