package apitypes

// ClientErrorReport is what a person's browser sends when a Whisk page fails in it (POST /v1/client-errors):
// the error's message and stack, the page's path and, for a server render, Next's digest.
type ClientErrorReport struct {
	Message string `json:"message"`
	Stack   string `json:"stack"`
	Page    string `json:"page"`
	Digest  string `json:"digest"`
}

var _ = more.addWire(ClientErrorReport{})
