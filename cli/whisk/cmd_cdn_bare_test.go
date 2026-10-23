package whisk

import (
	"strings"
	"testing"

	"github.com/whisk-run/cli/internal/api"
)

func TestCDNBareText(t *testing.T) {
	if got := cdnBareText([]api.CDNHostname{{Hostname: "www.example.com", ThroughCDN: true}, {Hostname: "a.example.com", Reason: "business"}}, "app.whisk.page"); got != "" {
		t.Errorf("no bare domain: %q, want nothing", got)
	}
	bare := []api.CDNHostname{
		{Hostname: "example.com", Reason: "bare"},
		{Hostname: "example.co.nz", Reason: "bare"},
		{Hostname: "example.au", Reason: "bare", ThroughCDN: true},
	}
	got := cdnBareText(bare, "site.acme.whisk.page")
	if !strings.HasPrefix(got, "example.com, example.co.nz can't use the CDN") || !strings.Contains(got, "point each at site.acme.whisk.page with an ALIAS or ANAME record") {
		t.Errorf("cdnBareText = %q", got)
	}
	if got := cdnBareText(bare, ""); !strings.Contains(got, "point each at the app's address with") {
		t.Errorf("cdnBareText without the address = %q", got)
	}
}
