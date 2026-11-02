package whisk

import (
	"reflect"
	"testing"

	"github.com/whisk-run/contract/apitypes"
)

func TestFlagReach(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    apitypes.AppFlag
		want string
	}{
		{"never set", apitypes.AppFlag{}, "nobody"},
		{"everyone wins over the rest", apitypes.AppFlag{Team: true, Percent: 100}, "everyone"},
		{"team and one person", apitypes.AppFlag{Team: true, People: []string{"ann@example.com"}}, "the team, 1 person"},
		{"groups, a share and previews", apitypes.AppFlag{People: []string{"a@example.com", "b@example.com"}, Groups: []apitypes.FlagGroup{{ID: "g1", Name: "beta"}}, Percent: 10, Previews: true},
			"2 people, group beta, 10% of everyone else, previews"},
	} {
		if got := flagReach(tc.f); got != tc.want {
			t.Errorf("%s: flagReach = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// Setting one option keeps the others as they were, so --percent 10 after --team keeps the team.
func TestRequestOfFlag(t *testing.T) {
	f := apitypes.AppFlag{Name: "new-checkout", People: []string{"ann@example.com"}, Groups: []apitypes.FlagGroup{{ID: "g1", Name: "beta"}}, Team: true, Percent: 5}
	got := requestOfFlag(f)
	want := apitypes.AppFlagRequest{People: []string{"ann@example.com"}, Groups: []string{"beta"}, Team: true, Percent: 5}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("requestOfFlag = %+v, want %+v", got, want)
	}
	got.People[0] = "changed@example.com"
	if f.People[0] != "ann@example.com" {
		t.Error("requestOfFlag shares the flag's people list")
	}
}

func TestSplitFlagList(t *testing.T) {
	if got := splitFlagList([]string{" a@example.com", "", "b@example.com "}); !reflect.DeepEqual(got, []string{"a@example.com", "b@example.com"}) {
		t.Errorf("splitFlagList = %v", got)
	}
	if got := splitFlagList(nil); got == nil {
		t.Error("splitFlagList(nil) is nil; the API takes an empty list")
	}
}
