package doctor

import (
	"testing"

	"github.com/whisk-run/contract/naughty"
)

// A naughty rule id is not a rule, and a finding for one carries no level or fix of its own.
func TestNaughtyLookup(t *testing.T) {
	ids := map[string]bool{}
	for _, id := range IDs() {
		ids[id] = true
	}
	for _, s := range naughty.Strings() {
		r, ok := Lookup(s)
		if ok != ids[s] || (ok && r.ID != s) {
			t.Errorf("Lookup(%q) = %+v, %v", s, r, ok)
		}
		f := NewFinding(s, s, 1, s)
		if !ids[s] && (f.Level != "" || f.Fix != "") {
			t.Errorf("NewFinding(%q) took a level or fix from nowhere: %+v", s, f)
		}
		if f.Rule != s || f.File != s || f.Message != s {
			t.Errorf("NewFinding(%q) did not keep its input: %+v", s, f)
		}
		ok3 := NewFinding("W002", s, 0, s)
		if ok3.Level != Error || ok3.Fix == "" {
			t.Errorf("NewFinding(W002, %q) lost the rule's level or fix: %+v", s, ok3)
		}
	}
}
