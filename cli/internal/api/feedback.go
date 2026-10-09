package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"

	"github.com/whisk-run/contract/apitypes"
)

// FeedbackInput is POST /v1/feedback (CONTROL-PLANE.md §6.23).
type FeedbackInput struct {
	Kind    string            `json:"kind,omitempty"`
	Message string            `json:"message"`
	Org     string            `json:"org,omitempty"`
	App     string            `json:"app,omitempty"`
	Context map[string]string `json:"context,omitempty"`
}

// FeedbackReceipt is the platform's answer to a submission.
type FeedbackReceipt = apitypes.FeedbackReceipt

// Feedback is one piece of feedback as an operator reads it.
type Feedback = apitypes.Feedback

// OwnFeedback is one piece of feedback as its sender reads it: where it stands and the Whisk
// team's note.
type OwnFeedback = apitypes.OwnFeedback

// OwnFeedbackPage is a page of the caller's own feedback.
type OwnFeedbackPage = apitypes.OwnFeedbackPage

// FeedbackPage is a page of feedback and how much is still open.
type FeedbackPage = apitypes.FeedbackPage

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
	var out FeedbackReceipt
	err := c.Do(ctx, http.MethodPost, "/feedback", in, &out)
	return out, err
}

// ListOwnFeedback reads a page of the caller's own feedback: what they sent and what was sent
// about their org.
func (c *Client) ListOwnFeedback(ctx context.Context, q FeedbackQuery) (OwnFeedbackPage, error) {
	var out OwnFeedbackPage
	err := c.Do(ctx, http.MethodGet, "/feedback"+feedbackQuery(q), nil, &out)
	return out, err
}

// ShowFeedback reads one piece by id: the caller's own, or one sent without signing in.
func (c *Client) ShowFeedback(ctx context.Context, id string) (OwnFeedback, error) {
	var out OwnFeedback
	err := c.Do(ctx, http.MethodGet, "/feedback/"+pathSeg(id), nil, &out)
	return out, err
}

// ListFeedback reads a page of everyone's feedback: an operator, or a token scoped to
// feedback:read.
func (c *Client) ListFeedback(ctx context.Context, q FeedbackQuery) (FeedbackPage, error) {
	var out FeedbackPage
	err := c.Do(ctx, http.MethodGet, "/operator/feedback"+feedbackQuery(q), nil, &out)
	return out, err
}

// feedbackQuery is a list's filters as a query string, empty when none is set.
func feedbackQuery(q FeedbackQuery) string {
	v := url.Values{}
	for k, val := range map[string]string{"status": q.Status, "kind": q.Kind, "org": q.Org, "cursor": q.Cursor} {
		if val != "" {
			v.Set(k, val)
		}
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	if len(v) == 0 {
		return ""
	}
	return "?" + v.Encode()
}

// SetFeedbackStatus resolves or reopens a piece of feedback, with a note for its sender when one
// is given: an operator, or a token scoped to feedback:resolve.
func (c *Client) SetFeedbackStatus(ctx context.Context, id, status, note string) (Feedback, error) {
	body := map[string]string{"status": status}
	if note != "" {
		body["note"] = note
	}
	var out Feedback
	err := c.Do(ctx, http.MethodPost, "/operator/feedback/"+pathSeg(id), body, &out)
	return out, err
}
