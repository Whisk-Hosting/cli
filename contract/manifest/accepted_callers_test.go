package manifest

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// accepted_callers left out, empty and listed are three different things (CONTRACT.md §3.1):
// left out declares nothing, [] accepts no caller, a list accepts those apps. The difference
// survives storing the manifest as JSON and reading it back, as the control plane does.
func TestAcceptedCallers(t *testing.T) {
	cases := []struct {
		name     string
		yaml     string
		declared bool
		accepted []string
		problem  string
	}{
		{name: "left out", yaml: "", declared: false, accepted: []string{}},
		{name: "empty", yaml: "accepted_callers: []\n", declared: true, accepted: []string{}},
		{name: "listed", yaml: "accepted_callers: [website, shop]\n", declared: true, accepted: []string{"website", "shop"}},
		{name: "itself", yaml: "accepted_callers: [website, job-tracker]\n", problem: "/accepted_callers/1"},
		{name: "twice", yaml: "accepted_callers: [website, website]\n", problem: "/accepted_callers"},
		{name: "not a slug", yaml: "accepted_callers: [Website]\n", problem: "/accepted_callers/0"},
		{name: "null", yaml: "accepted_callers:\n", problem: "/accepted_callers"},
	}
	for _, c := range cases {
		m, err := Parse([]byte("whisk: 1\nname: job-tracker\n" + c.yaml))
		if c.problem != "" {
			if err == nil || !strings.Contains(err.Error(), c.problem) {
				t.Errorf("%s: err = %v, want a problem at %s", c.name, err, c.problem)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		raw, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		var stored Manifest
		if err := json.Unmarshal(raw, &stored); err != nil {
			t.Fatal(err)
		}
		for _, got := range []Manifest{m, stored} {
			accepted, declared := got.AcceptsCallers()
			if declared != c.declared || !slices.Equal(accepted, c.accepted) {
				t.Errorf("%s: AcceptsCallers = %v, %v; want %v, %v", c.name, accepted, declared, c.accepted, c.declared)
			}
		}
		if !c.declared && strings.Contains(string(raw), "accepted_callers") {
			t.Errorf("%s: stored JSON names accepted_callers: %s", c.name, raw)
		}
	}
}
