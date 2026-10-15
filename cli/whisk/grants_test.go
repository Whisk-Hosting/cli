package whisk

import (
	"reflect"
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/apitypes"
)

// A deploy that waits on a shared secret or wider access (CLI.md §5.4): the block names each
// thing in the control plane's words and links the blocked deploy, where one tap approves it.
func TestGrantsNeededBlockApprove(t *testing.T) {
	grants := []api.GrantNeeded{
		{Kind: "connection", Name: "erp", Environment: "production", RecipeChanged: true},
		{Kind: "secret", Name: "MAPS_KEY", Hash: "s1", Change: "It reads the business's shared secret MAPS_KEY, which crm is not on the list of apps for."},
		{Kind: "access", Name: "calls:billing", Hash: "h1", Change: "It may call the app billing."},
		{Kind: "access", Name: "public:/api/**", Hash: "h1", Change: "Anyone may open /api/** without signing in."},
	}
	page := grantsPage("https://whisk.run", "acme", "crm", "d-1", grants)
	if page != "https://whisk.run/o/acme/apps/crm/deploys?approve=d-1" {
		t.Fatalf("page %s", page)
	}
	e := grantsNeededBlock("d-1", grants, []string{"SLACK_URL"}, page, "https://whisk.run/o/acme/apps/crm/secrets")
	if e.Code != "GRANT_NEEDED" || output.ExitCode(e) != output.ExitNeedsHuman {
		t.Fatalf("code %s exit %d", e.Code, output.ExitCode(e))
	}
	want := "Deploy d-1 waits for an owner or admin to approve: erp (production) with a new address or recipe; it reads the business's shared secret MAPS_KEY, which crm is not on the list of apps for; it may call the app billing; anyone may open /api/** without signing in."
	if e.Message != want {
		t.Errorf("message\n%q\nwant\n%q", e.Message, want)
	}
	for _, s := range []string{"approve it at " + page, "one tap", "Do not try to approve it yourself", "It also needs values for SLACK_URL"} {
		if !strings.Contains(e.Fix, s) {
			t.Errorf("fix %q lacks %q", e.Fix, s)
		}
	}
	if strings.Contains(e.Message+e.Fix, "—") {
		t.Error("an em dash in user-facing copy")
	}
	if e.Details["url"] != page {
		t.Errorf("details url %v", e.Details["url"])
	}
}

func TestGrantsPage(t *testing.T) {
	conn := api.GrantNeeded{Kind: apitypes.GrantConnection, Name: "erp"}
	acc := api.GrantNeeded{Kind: apitypes.GrantAccess, Name: "calls:billing"}
	if got := grantsPage("https://w", "acme", "crm", "d1", []api.GrantNeeded{conn}); got != "https://w/o/acme/apps/crm/connections" {
		t.Errorf("connections only: %s", got)
	}
	if got := grantsPage("https://w", "acme", "crm", "d1", []api.GrantNeeded{conn, acc}); got != "https://w/o/acme/apps/crm/deploys?approve=d1" {
		t.Errorf("mixed: %s", got)
	}
}

func TestSecretApps(t *testing.T) {
	all := []api.Secret{
		{Name: "MAPS_KEY", Scope: apitypes.ScopeApp, AppID: "a1"},
		{Name: "MAPS_KEY", Scope: apitypes.ScopeOrg, Apps: []string{"a1", "gone"}},
	}
	sec, ok := businessSecret(all, "MAPS_KEY")
	if !ok || sec.Scope != apitypes.ScopeOrg {
		t.Fatalf("business secret: %+v %v", sec, ok)
	}
	if _, ok := businessSecret(all, "OTHER"); ok {
		t.Error("found a secret nobody declared")
	}
	slugs := appSlugs(sec.Apps, []api.App{{ID: "a1", Slug: "crm"}})
	if !reflect.DeepEqual(slugs, []string{"crm", "gone"}) {
		t.Errorf("slugs %v", slugs)
	}
	if got := secretAppsSentence("MAPS_KEY", slugs); got != "MAPS_KEY reaches crm, gone." {
		t.Errorf("sentence %q", got)
	}
	if got := secretAppsSentence("MAPS_KEY", nil); got != "MAPS_KEY reaches no app yet." {
		t.Errorf("none %q", got)
	}
}
