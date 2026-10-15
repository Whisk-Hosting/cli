package manifest

import (
	"strings"
	"testing"
)

// A product's notices (MANAGED-APPS.md §6.1): a code in the error-code shape, declared once,
// with a one-line subject.
func TestManagedNotify(t *testing.T) {
	head := "whisk: 1\nname: erp-link\nmanaged:\n  product: erp-link\n  name: ERP link\n"
	cases := []struct {
		name, notify, want string
	}{
		{"none declared", "", ""},
		{"two codes", "  notify:\n    - {code: LINK_ERP_RECONNECT, subject: Sign in again}\n    - {code: LINK_PRICES_UNREADABLE, subject: Prices unreadable}\n", ""},
		{"a lower-case code", "  notify:\n    - {code: link_x, subject: S}\n", "notify/0/code"},
		{"a code too short", "  notify:\n    - {code: AB, subject: S}\n", "notify/0/code"},
		{"twice", "  notify:\n    - {code: LINK_X, subject: A}\n    - {code: LINK_X, subject: B}\n", "declared twice"},
		{"an empty subject", "  notify:\n    - {code: LINK_X, subject: \" \"}\n", "short sentence"},
		{"a subject too long", "  notify:\n    - {code: LINK_X, subject: " + strings.Repeat("a", 121) + "}\n", "120"},
		{"a subject over two lines", "  notify:\n    - {code: LINK_X, subject: \"a\\rb\"}\n", "one line"},
		{"a tab in the subject", "  notify:\n    - {code: LINK_X, subject: \"a\\tb\"}\n", "one line"},
		{"no subject", "  notify:\n    - {code: LINK_X}\n", "subject"},
	}
	for _, c := range cases {
		m, err := Parse([]byte(head + c.notify))
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: %v", c.name, err)
		case c.want == "" && m.Managed.Notify == nil:
			t.Errorf("%s: notify is nil, want an empty list", c.name)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: error %v, want one containing %q", c.name, err, c.want)
		}
	}
}

func TestManagedNotice(t *testing.T) {
	m := Managed{Notify: []Notice{{Code: "LINK_A", Subject: "A"}, {Code: "LINK_B", Subject: "B"}}}
	if n, ok := m.Notice("LINK_B"); !ok || n.Subject != "B" {
		t.Errorf("LINK_B: %+v %v", n, ok)
	}
	if _, ok := m.Notice("LINK_C"); ok {
		t.Error("LINK_C is not declared")
	}
	if _, ok := (Managed{}).Notice("LINK_A"); ok {
		t.Error("a product with no notices declares none")
	}
}
