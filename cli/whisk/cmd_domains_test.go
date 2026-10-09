package whisk

import (
	"reflect"
	"testing"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/contract/apitypes"
)

// The records printed are the API's own list when it sends one, so a bare domain shows its A and
// AAAA records rather than a CNAME it cannot have; nothing for a verified or platform name.
func TestDNSRecords(t *testing.T) {
	bare := api.Domain{Hostname: "lab.example", Kind: apitypes.DomainCustom, TXTRecord: "_whisk-verify.lab.example", TXTValue: "whisk-verify-1", CNAMETarget: "results--acme.whisk.page",
		Records: []apitypes.DNSRecord{{Type: "TXT", Name: "_whisk-verify.lab.example", Value: "whisk-verify-1"}, {Type: "A", Name: "lab.example", Value: "95.217.38.236"}}}
	older := bare
	older.Records = nil
	verified := bare
	verified.Verified = true
	cases := []struct {
		name string
		d    api.Domain
		want []map[string]string
	}{
		{"the API's records", bare, []map[string]string{{"type": "TXT", "name": "_whisk-verify.lab.example", "value": "whisk-verify-1"}, {"type": "A", "name": "lab.example", "value": "95.217.38.236"}}},
		{"an API without records", older, []map[string]string{{"type": "TXT", "name": "_whisk-verify.lab.example", "value": "whisk-verify-1"}, {"type": "CNAME", "name": "lab.example", "value": "results--acme.whisk.page"}}},
		{"verified", verified, []map[string]string{}},
		{"a platform name", api.Domain{Kind: apitypes.DomainPlatform}, []map[string]string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := dnsRecords(c.d)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("dnsRecords = %v, want %v", got, c.want)
			}
			if pointsByAddress(got) != (c.name == "the API's records") {
				t.Errorf("pointsByAddress(%v) = %v", got, pointsByAddress(got))
			}
		})
	}
}
