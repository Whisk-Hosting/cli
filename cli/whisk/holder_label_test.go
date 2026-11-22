package whisk

import "testing"

func TestHolderLabel(t *testing.T) {
	slugs := map[string]string{"01A": "crm"}
	for _, tc := range []struct{ id, want string }{
		{"", "business"},
		{"01A", "crm"},
		{"01B", "01B"},
	} {
		if got := holderLabel(tc.id, slugs); got != tc.want {
			t.Errorf("holderLabel(%q) = %q, want %q", tc.id, got, tc.want)
		}
	}
}
