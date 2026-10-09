// Package inbound is what an app's inbox means (CONTRACT.md §7, "Inbound email"): which address
// leads to which app, whether a message is taken, how a raw message is read into the fields a
// handler receives, and the shape of that delivery. Everything here is a pure function of its
// inputs, so the platform and the stub read a message the same way and the rules are
// table-tested.
package inbound

import (
	"net/mail"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Limits. A message larger than MaxMessageBytes is not stored; the platform keeps a record that
// it arrived and why it was dropped.
// PlanFeature is the plan setting that includes an inbox: Team, Agency, Business and
// Enterprise have it, Free and Starter do not.
const PlanFeature = "inbox"

const (
	// MaxMessageBytes is the largest message an inbox takes, attachments included: 25 MB, the
	// common limit of mail servers, so a sender that can send it at all is under it.
	MaxMessageBytes = 25 << 20
	// MaxAttachments is the most attachments one message may carry; the rest are left in the
	// raw message, which is stored whole, and the delivery says so.
	MaxAttachments = 50
	// MaxBodyBytes bounds the text and the html each carry in a delivery; the raw message
	// holds the rest and the delivery marks the cut.
	MaxBodyBytes = 1 << 20
	// PerMinute and PerDay bound the messages one app takes; BytesPerDay bounds what they
	// weigh. A message over them is dropped and recorded (Admit).
	PerMinute   = 30
	PerDay      = 1000
	BytesPerDay = 1 << 30
	// MaxApps is the most apps one message is delivered to, whatever it is addressed to.
	MaxApps = 5
)

// Reasons a message is dropped rather than delivered. Each is recorded on the stored event.
const (
	ReasonTooLarge         = "too_large"
	ReasonRateLimited      = "rate_limited"
	ReasonDailyBytes       = "daily_size_limit"
	ReasonSenderNotAllowed = "sender_not_allowed"
	ReasonUnreadable       = "unreadable"
	ReasonPaused           = "business_paused"
	// ReasonNotOnPlan is a business whose plan does not include an inbox (PlanFeature).
	ReasonNotOnPlan = "not_on_plan"
)

// Dropped reports whether a reason is one of the inbox's own, as opposed to a webhook
// signature's verdict.
func Dropped(reason string) bool {
	switch reason {
	case ReasonTooLarge, ReasonRateLimited, ReasonDailyBytes, ReasonSenderNotAllowed, ReasonUnreadable, ReasonPaused, ReasonNotOnPlan:
		return true
	}
	return false
}

// SourceName is the webhook source an inbox's deliveries are stored and replayed under:
// `whisk webhooks events inbox` lists them.
const SourceName = "inbox"

// Preset is the source's preset. It names no signature: the platform verifies the mail
// provider's webhook itself, and the source has no URL of its own.
const Preset = "email"

// Address is an app's own address on the platform's receiving domain: "<app>.<org>@<domain>".
// Slugs carry no dots, so the address names exactly one app. Anything may follow a plus:
// "<app>.<org>+lab-a@<domain>" reaches the same app with the tag "lab-a".
func Address(app, org, domain string) string {
	return app + "." + org + "@" + strings.ToLower(domain)
}

// Target is where one recipient leads: an app by its org and app slugs on the receiving domain,
// or a business's own domain, which the platform looks up.
type Target struct {
	// Address is the recipient as it was written, lower-cased.
	Address string
	// Org and App are set for an address on the receiving domain.
	Org, App string
	// Domain is set for an address on any other domain.
	Domain string
	// Tag is what followed a plus in the local part.
	Tag string
}

var slug = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

// Route reads every recipient into a target, once each, in the order given. An address that is
// not one address, or a local part on the receiving domain that names no app, leads nowhere and
// is left out.
func Route(recipients []string, receiving string) []Target {
	receiving = strings.ToLower(strings.TrimSuffix(receiving, "."))
	out := []Target{}
	for _, r := range recipients {
		addr := bare(r)
		at := strings.LastIndexByte(addr, '@')
		if at <= 0 || at == len(addr)-1 {
			continue
		}
		local, domain := addr[:at], addr[at+1:]
		tag := ""
		if plus := strings.IndexByte(local, '+'); plus >= 0 {
			local, tag = local[:plus], local[plus+1:]
		}
		t := Target{Address: addr, Tag: tag}
		if domain == receiving {
			app, org, ok := strings.Cut(local, ".")
			if !ok || !slug.MatchString(app) || !slug.MatchString(org) {
				continue
			}
			t.App, t.Org = app, org
		} else {
			t.Domain = domain
		}
		if slices.ContainsFunc(out, func(o Target) bool { return o.Address == t.Address }) {
			continue
		}
		out = append(out, t)
	}
	return out
}

// bare is the address alone, lower-cased: "Ana <ANA@x.com>" is "ana@x.com".
func bare(s string) string {
	s = strings.TrimSpace(s)
	if a, err := mail.ParseAddress(s); err == nil {
		s = a.Address
	}
	return strings.ToLower(strings.Trim(s, "<>"))
}

// Allowed reports whether a sender may reach an inbox whose allow_from lists these entries. An
// entry is a whole address ("results@lab.example") or a domain after an at sign
// ("@lab.example"), which allows every address at exactly that domain. An empty list allows
// everyone. A message whose DMARC check failed never passes a list: the From it claims is what
// the list matches, and a failed check says the claim is false.
func Allowed(allow []string, from, dmarc string) bool {
	if len(allow) == 0 {
		return true
	}
	if strings.EqualFold(dmarc, "fail") {
		return false
	}
	addr := bare(from)
	at := strings.LastIndexByte(addr, '@')
	if at <= 0 {
		return false
	}
	for _, a := range allow {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == addr || (strings.HasPrefix(a, "@") && a == addr[at:]) {
			return true
		}
	}
	return false
}

// Counts is what an app has taken lately, for Admit.
type Counts struct {
	LastMinute int
	Today      int
	BytesToday int64
}

// Admission is one message about to reach one app.
type Admission struct {
	Bytes     int64
	From      string
	DMARC     string
	AllowFrom []string
	// Paused is a business whose apps are paused (frozen or under review).
	Paused bool
	// NotOnPlan is a business whose plan does not include an inbox.
	NotOnPlan bool
	Counts    Counts
}

// Admit decides whether a message reaches the app, and if not, why: the reason is one of the
// Reason constants, and empty when it is taken. Size first, then the business, then the
// sender, then the rates, so a message that would never be taken does not count against them.
func Admit(a Admission) string {
	switch {
	case a.Bytes > MaxMessageBytes:
		return ReasonTooLarge
	case a.Paused:
		return ReasonPaused
	case a.NotOnPlan:
		return ReasonNotOnPlan
	case !Allowed(a.AllowFrom, a.From, a.DMARC):
		return ReasonSenderNotAllowed
	case a.Counts.LastMinute >= PerMinute, a.Counts.Today >= PerDay:
		return ReasonRateLimited
	case a.Counts.BytesToday+a.Bytes > BytesPerDay:
		return ReasonDailyBytes
	}
	return ""
}

// File is one stored object a delivery names: the raw message or an attachment. UploadID is
// what the app reads it back with (GET /v1/orgs/<org>/apps/<app>/uploads/<id>, which answers a
// link), Key where it lives under the app's storage prefix.
type File struct {
	UploadID    string `json:"upload_id"`
	Key         string `json:"key"`
	Bytes       int64  `json:"bytes"`
	ContentType string `json:"content_type"`
}

// AttachmentFile is an attachment as a delivery names it.
type AttachmentFile struct {
	File
	Filename  string `json:"filename"`
	ContentID string `json:"content_id,omitempty"`
	Inline    bool   `json:"inline"`
}

// Delivery is the JSON body an inbox's handler receives (CONTRACT.md §7). It arrives like any
// webhook delivery: signed by the platform, with X-Whisk-Webhook-Source: inbox and an
// X-Whisk-Webhook-Id to deduplicate on.
type Delivery struct {
	MessageID      string            `json:"message_id"`
	From           string            `json:"from"`
	FromAddress    string            `json:"from_address"`
	To             []string          `json:"to"`
	Cc             []string          `json:"cc"`
	ReplyTo        string            `json:"reply_to,omitempty"`
	Recipient      string            `json:"recipient"`
	Tag            string            `json:"tag,omitempty"`
	Subject        string            `json:"subject"`
	Date           *time.Time        `json:"date,omitempty"`
	InReplyTo      string            `json:"in_reply_to,omitempty"`
	References     []string          `json:"references"`
	Text           string            `json:"text"`
	HTML           string            `json:"html"`
	TextTruncated  bool              `json:"text_truncated"`
	HTMLTruncated  bool              `json:"html_truncated"`
	Authentication map[string]string `json:"authentication,omitempty"`
	Raw            File              `json:"raw"`
	Attachments    []AttachmentFile  `json:"attachments"`
	// AttachmentsOmitted counts attachments past MaxAttachments, which only the raw message holds.
	AttachmentsOmitted int       `json:"attachments_omitted,omitempty"`
	ReceivedAt         time.Time `json:"received_at"`
}

// Build is the delivery for one parsed message reaching one recipient, with the files the
// platform stored for it: raw is the message, files the attachments in the message's order
// (len(files) may be less than the attachments when MaxAttachments cut them). Pure.
func Build(m Message, t Target, raw File, files []File, auth map[string]string, receivedAt time.Time) Delivery {
	d := Delivery{
		MessageID: m.MessageID, From: m.From, FromAddress: bare(m.From), To: orEmpty(m.To), Cc: orEmpty(m.Cc),
		ReplyTo: m.ReplyTo, Recipient: t.Address, Tag: t.Tag, Subject: m.Subject, InReplyTo: m.InReplyTo,
		References: orEmpty(m.References), Raw: raw, Attachments: []AttachmentFile{}, ReceivedAt: receivedAt.UTC(),
	}
	if !m.Date.IsZero() {
		at := m.Date.UTC()
		d.Date = &at
	}
	d.Text, d.TextTruncated = cut(m.Text, MaxBodyBytes)
	d.HTML, d.HTMLTruncated = cut(m.HTML, MaxBodyBytes)
	for i, a := range m.Attachments {
		if i >= len(files) {
			d.AttachmentsOmitted = len(m.Attachments) - len(files)
			break
		}
		d.Attachments = append(d.Attachments, AttachmentFile{File: files[i], Filename: a.Filename, ContentID: a.ContentID, Inline: a.Inline})
	}
	if len(auth) > 0 {
		d.Authentication = auth
	}
	return d
}

// cut keeps at most n bytes of s without splitting a character.
func cut(s string, n int) (string, bool) {
	if len(s) <= n {
		return s, false
	}
	for n > 0 && n < len(s) && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n], true
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
