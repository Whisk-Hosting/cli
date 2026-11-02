package whisk

import (
	"strings"
	"testing"

	"github.com/whisk-run/cli/doctor"
)

// A deploy names what the app does not send yet, from doctor's W140 and W141.
func TestObserveLine(t *testing.T) {
	cases := []struct {
		rules []string
		want  string
	}{
		{nil, ""},
		{[]string{"W030"}, ""},
		{[]string{"W140"}, "Not set up: errors."},
		{[]string{"W141"}, "Not set up: traces."},
		{[]string{"W030", "W140", "W141"}, "Not set up: errors and traces."},
	}
	for _, c := range cases {
		var findings []doctor.Finding
		for _, r := range c.rules {
			findings = append(findings, doctor.Finding{Rule: r})
		}
		got := observeLine(findings)
		if (c.want == "") != (got == "") || !strings.HasPrefix(got, c.want) {
			t.Errorf("observeLine(%v) = %q, want it to start %q", c.rules, got, c.want)
		}
	}
}
