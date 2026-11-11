package custom

import "testing"

func TestLineSubject(t *testing.T) {
	for _, c := range []struct {
		subject string
		product string
		number  int
	}{
		{LineSubject("CRM", 12), "CRM", 12},
		{LineSubject("ERP link", 1), "ERP link", 1},
		{LineSubject("Harness H160", 3), "Harness H160", 3},
		{LineSubject("Acme release 2", 7), "Acme release 2", 7},
		{"CRM release 0", "", 0},
		{"CRM release 2b", "", 0},
		{"CRM release 1234567890", "", 0},
		{"release 2", "", 0},
		{" CRM release 2", "", 0},
		{"Prepare the release", "", 0},
	} {
		product, number, ok := LineRelease(c.subject)
		if product != c.product || number != c.number || ok != (c.number > 0) || IsLineSubject(c.subject) != ok {
			t.Errorf("LineRelease(%q) = %q, %d, %v; want %q, %d", c.subject, product, number, ok, c.product, c.number)
		}
	}
}
