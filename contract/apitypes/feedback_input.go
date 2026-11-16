package apitypes

// FeedbackInput is a submission as it arrives.
type FeedbackInput struct {
	Kind    string            `json:"kind"`
	Message string            `json:"message"`
	Org     string            `json:"org"`
	App     string            `json:"app"`
	Context map[string]string `json:"context"`
}

var _ = more.addWire(FeedbackInput{})
