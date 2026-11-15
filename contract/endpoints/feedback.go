package endpoints

import (
	"github.com/whisk-run/contract/apitypes"
)

// The feedback routes.
var (
	OwnFeedback  = New[OwnFeedbackQuery, apitypes.OwnFeedbackPage]("GET /v1/feedback")
	ShowFeedback = New[None, apitypes.OwnFeedback]("GET /v1/feedback/{id}")

	_ = public.Add(OwnFeedback, ShowFeedback)
)

// OwnFeedbackQuery is what OwnFeedback reads from the query.
type OwnFeedbackQuery struct {
	Cursor string `query:"cursor"`
	Kind   string `query:"kind"`
	Limit  int    `query:"limit"`
	Org    string `query:"org"`
	Status string `query:"status"`
}
