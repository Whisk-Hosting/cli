package apitypes

import (
	"encoding/json"
	"time"
)

// Notification is one thing the platform told the caller about this org.
type Notification struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	AppID     string          `json:"app_id,omitempty"`
	Title     string          `json:"title"`
	Body      string          `json:"body"`
	URL       string          `json:"url,omitempty"`
	Payload   json.RawMessage `json:"payload,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}

var _ = more.addWire(Notification{})
