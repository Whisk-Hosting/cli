package api

import (
	"context"

	"github.com/whisk-run/contract/apitypes"
	"github.com/whisk-run/contract/endpoints"
)

// FeedbackInput is POST /v1/feedback (CONTROL-PLANE.md §6.23).
type FeedbackInput = apitypes.FeedbackInput

// FeedbackReceipt is the platform's answer to a submission.
type FeedbackReceipt = apitypes.FeedbackReceipt

// OwnFeedback is one piece of feedback as its sender reads it: where it stands and the Whisk
// team's note.
type OwnFeedback = apitypes.OwnFeedback

// OwnFeedbackPage is a page of the caller's own feedback.
type OwnFeedbackPage = apitypes.OwnFeedbackPage

// FeedbackQuery filters the list; empty fields do not filter.
type FeedbackQuery struct {
	Status string
	Kind   string
	Org    string
	Cursor string
	Limit  int
}

// SendFeedback posts feedback. It works without a token.
func (c *Client) SendFeedback(ctx context.Context, in FeedbackInput) (FeedbackReceipt, error) {
	return Send(ctx, c, endpoints.PostFeedback, in)
}

// ListOwnFeedback reads a page of the caller's own feedback: what they sent and what was sent
// about their org.
func (c *Client) ListOwnFeedback(ctx context.Context, q FeedbackQuery) (OwnFeedbackPage, error) {
	return Send(ctx, c, endpoints.OwnFeedback, endpoints.OwnFeedbackQuery{Status: q.Status, Kind: q.Kind, Org: q.Org, Cursor: q.Cursor, Limit: max(q.Limit, 0)})
}

// feedbackQuery is a list's filters as a query string, empty when none is set.
