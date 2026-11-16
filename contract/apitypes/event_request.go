package apitypes

// EventRequest is what an app asks to send.
type EventRequest struct {
	Name      string         `json:"name"`
	Data      map[string]any `json:"data"`
	DedupeKey string         `json:"dedupe_key"`
}

var _ = more.addWire(EventRequest{})
