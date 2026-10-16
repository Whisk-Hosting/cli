package whisk

import (
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/apitypes"
)

// A request asked of a person shows them the link and waits 24 hours; one asked of the owners
// says they were emailed and waits 7 days (CONTROL-PLANE.md §4.9, §4.10).
func TestRequestOutcomeByWhomItIsAsked(t *testing.T) {
	base := api.PersonRequest{ID: "r1", URL: "https://whisk.run/o/acme/requests/r1", Summary: []string{"Delete the app crm (crm--acme.whisk.page): it stops at once"}}
	person, owners := base, base
	person.AskedOf, owners.AskedOf = apitypes.AskedOfPerson, apitypes.AskedOfOwners
	cases := map[string]struct {
		p      api.PersonRequest
		status apitypes.PersonRequestStatus
		code   string
		says   string
	}{
		"pending, a person": {person, apitypes.PersonRequestPending, "NEEDS_HUMAN", "It waits 24 hours"},
		"pending, owners":   {owners, apitypes.PersonRequestPending, "NEEDS_HUMAN", "They were emailed a link to approve it, https://whisk.run/o/acme/requests/r1, and it waits 7 days"},
		"expired, a person": {person, apitypes.PersonRequestExpired, "REQUEST_CLOSED", "waited 24 hours"},
		"expired, owners":   {owners, apitypes.PersonRequestExpired, "REQUEST_CLOSED", "waited 7 days"},
		"declined, owners":  {owners, apitypes.PersonRequestDeclined, "REQUEST_CLOSED", "was declined"},
		"approved, owners":  {owners, apitypes.PersonRequestApproved, "", ""},
	}
	for name, c := range cases {
		c.p.Status = c.status
		err := requestOutcome(c.p)
		if c.code == "" {
			if err != nil {
				t.Errorf("%s: %v", name, err)
			}
			continue
		}
		oe, ok := err.(*output.Error)
		if !ok || oe.Code != c.code || !strings.Contains(oe.Message+" "+oe.Fix, c.says) {
			t.Errorf("%s: %#v, want %s saying %q", name, err, c.code, c.says)
		}
	}
}
