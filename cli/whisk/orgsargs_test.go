package whisk

import "testing"

// whisk orgs takes whisk orgs list too, and answers any other word with what to run.
func TestOrgsArgs(t *testing.T) {
	for _, c := range []struct {
		args []string
		ok   bool
	}{{nil, true}, {[]string{"list"}, true}, {[]string{"acme"}, false}, {[]string{"list", "acme"}, false}} {
		if err := orgsArgs(nil, c.args); (err == nil) != c.ok {
			t.Errorf("orgsArgs(%q) = %v, want ok %v", c.args, err, c.ok)
		}
	}
}
