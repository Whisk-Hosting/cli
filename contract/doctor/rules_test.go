package doctor

import (
	"strings"
	"testing"
)

var specified = strings.Fields(`W001 W002 W003 W004 W005 W006 W010 W011 W020 W021 W022 W023 W024 W030 W031 W040 W041 W042 W043 W050 W051 W052 W053 W060 W061 W062 W063 W064 W070 W080 W090 W091 W092 W093 W094 W095 W096 W097 W098 W099 W100 W101 W102 W103 W104 W105 W110 W111`)

func TestEveryRuleDocumented(t *testing.T) {
	for _, id := range specified {
		r, ok := Lookup(id)
		if !ok {
			t.Errorf("%s: no section in doctor-rules.md", id)
			continue
		}
		if r.Level != Error && r.Level != Warning {
			t.Errorf("%s: level %q", id, r.Level)
		}
		if r.Check == "" || r.Fix == "" {
			t.Errorf("%s: needs Check and Fix lines", id)
		}
	}
	for _, id := range IDs() {
		if !contains(specified, id) {
			t.Errorf("%s: in doctor-rules.md but not in the specified list; add it to the test and CONTRACT.md §9", id)
		}
	}
	for _, id := range ManifestRules {
		if _, ok := Lookup(id); !ok {
			t.Errorf("manifest rule %s is not documented", id)
		}
	}
}

func TestFindingCarriesRuleText(t *testing.T) {
	f := NewFinding("W030", "src/xero.ts", 8, "XERO_TENANT_ID is read from the environment but not declared.")
	if f.Level != Warning || f.Fix == "" || f.Rule != "W030" {
		t.Fatalf("finding = %+v", f)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
