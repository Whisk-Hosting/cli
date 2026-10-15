package manifest

import (
	"strings"
	"testing"
)

// The inbox (CONTRACT.md §3, §7 "Inbound email"): a handler that is a service route, an
// optional list of senders, and no webhook source under the inbox's own name.
func TestInbox(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		problem string
	}{
		{"a handler alone", "inbox:\n  handler: /inbound/email\n", ""},
		{"senders by address and by domain", "inbox:\n  handler: /inbound/email\n  allow_from: [results@lab.example, \"@lab.example\"]\n", ""},
		{"no handler", "inbox:\n  allow_from: [\"@lab.example\"]\n", "/inbox"},
		{"a handler that is not a path", "inbox:\n  handler: inbound\n", "/inbox/handler"},
		{"a sender that is not an address", "inbox:\n  handler: /in\n  allow_from: [lab.example]\n", "/inbox/allow_from/0"},
		{"an unknown key", "inbox:\n  handler: /in\n  address: x@y.z\n", "/inbox"},
		{"the queue endpoint", "inbox:\n  handler: /.whisk/inngest\n", "/inbox/handler"},
		{"a webhook named inbox", "inbox:\n  handler: /in\nwebhooks:\n  - name: inbox\n    preset: token\n    handler: /hooks/inbox\n", "/webhooks/0/name"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m, err := Parse([]byte("whisk: 1\nname: lab-results\n" + c.yaml))
			if c.problem == "" {
				if err != nil {
					t.Fatal(err)
				}
				if m.Inbox == nil || m.Inbox.AllowFrom == nil || !m.IsService(m.Inbox.Handler) {
					t.Fatalf("inbox not read as a service route: %+v", m.Inbox)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.problem) {
				t.Fatalf("want a problem at %s, got %v", c.problem, err)
			}
		})
	}
	m, err := Parse([]byte("whisk: 1\nname: lab-results\nwebhooks:\n  - name: inbox\n    preset: token\n    handler: /hooks/inbox\n"))
	if err != nil || m.Inbox != nil {
		t.Fatalf("a webhook named inbox without an inbox is fine: %v %+v", err, m.Inbox)
	}
}
