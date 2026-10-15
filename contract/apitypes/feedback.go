package apitypes

import (
	"time"
)

// Feedback as its sender sends and reads it (CONTROL-PLANE.md §6.23).

// FeedbackKind is what a piece of feedback is (CONTROL-PLANE.md §6.23).
type FeedbackKind string

const (
	FeedbackBug        FeedbackKind = "bug"
	FeedbackDifficulty FeedbackKind = "difficulty"
	FeedbackIdea       FeedbackKind = "idea"
	FeedbackPraise     FeedbackKind = "praise"
)

// FeedbackStatus is whether the Whisk team has resolved a piece of feedback.
type FeedbackStatus string

const (
	FeedbackOpen     FeedbackStatus = "open"
	FeedbackResolved FeedbackStatus = "resolved"
)

// OwnFeedback is one piece of feedback as its sender reads it: what they said, where it stands
// and the Whisk team's note, without the address, client or other people's emails. SentByYou is
// whether the caller's person, or the calling token, sent it.
type OwnFeedback struct {
	ID         string            `json:"id"`
	Kind       FeedbackKind      `json:"kind"`
	Message    string            `json:"message"`
	Status     FeedbackStatus    `json:"status"`
	Org        string            `json:"org,omitempty"`
	App        string            `json:"app,omitempty"`
	SentByYou  bool              `json:"sent_by_you"`
	Context    map[string]string `json:"context"`
	Redacted   int               `json:"redacted"`
	CreatedAt  time.Time         `json:"created_at"`
	ResolvedAt *time.Time        `json:"resolved_at,omitempty"`
	Note       string            `json:"note,omitempty"`
}

// OwnFeedbackPage is a page of the caller's own feedback.
type OwnFeedbackPage struct {
	Items      []OwnFeedback `json:"items"`
	NextCursor string        `json:"next_cursor"`
}

// FeedbackReceipt is what the sender of feedback gets back: the id, and the thanks.
type FeedbackReceipt struct {
	ID        string       `json:"id"`
	Kind      FeedbackKind `json:"kind"`
	Org       string       `json:"org,omitempty"`
	App       string       `json:"app,omitempty"`
	Redacted  int          `json:"redacted"`
	CreatedAt time.Time    `json:"created_at"`
	Message   string       `json:"message"`
}
