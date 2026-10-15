package whisk

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/api"
)

func TestDeliveryWhy(t *testing.T) {
	cases := []struct {
		name string
		err  *api.DeliveryError
		want string
	}{
		{"delivered", nil, ""},
		{"handler error", &api.DeliveryError{Status: 500, Error: "Internal\nServer Error"}, "HTTP 500 Internal Server Error"},
		{"connection", &api.DeliveryError{Error: "dial tcp 10.0.0.2:8080: connection refused"}, "dial tcp 10.0.0.2:8080: connection refused"},
		{"status only", &api.DeliveryError{Status: 404}, "HTTP 404"},
		{"long", &api.DeliveryError{Status: 400, Error: strings.Repeat("x", 200)}, "HTTP 400 " + strings.Repeat("x", 70) + "…"},
	}
	for _, c := range cases {
		if got := deliveryWhy(api.WebhookEvent{LastError: c.err}); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// The API's last_error is {status, error}, as the ingress records a failed attempt.
func TestWebhookEventDecodesLastError(t *testing.T) {
	var e api.WebhookEvent
	if err := json.Unmarshal([]byte(`{"id":"e1","delivery_status":"retrying","attempts":2,"last_error":{"status":502,"error":"bad gateway"}}`), &e); err != nil {
		t.Fatal(err)
	}
	if e.LastError == nil || e.LastError.Status != 502 || deliveryWhy(e) != "HTTP 502 bad gateway" {
		t.Fatalf("%+v", e.LastError)
	}
}
