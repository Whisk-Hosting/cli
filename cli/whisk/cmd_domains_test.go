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
		Records: []apitypes.DNSRecord{{Type: "TXT", Name: "_whisk-verify.lab.example", Value: "whisk-verify-1", Host: "_whisk-verify"}, {Type: "A", Name: "lab.example", Value: "95.217.38.236", Host: "@"}}}
	older := bare
	older.Records = nil
	verified := bare
	verified.Verified = true
	cases := []struct {
		name string
		d    api.Domain
		want []map[string]string
	}{
		{"the API's records", bare, []map[string]string{{"type": "TXT", "name": "_whisk-verify.lab.example", "host": "_whisk-verify", "value": "whisk-verify-1"}, {"type": "A", "name": "lab.example", "host": "@", "value": "95.217.38.236"}}},
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

// Under the records, a bare domain is told that an ALIAS, ANAME or flattened CNAME to the
// domain's apex target is better where its DNS host offers one; an API that sends no apex
// target gets the A and AAAA advice alone.
func TestApexAdvice(t *testing.T) {
	byAddress := []map[string]string{{"type": "TXT"}, {"type": "A", "value": "95.217.38.236"}}
	byCNAME := []map[string]string{{"type": "TXT"}, {"type": "CNAME"}}
	flatten := "If its DNS host offers an ALIAS or ANAME record, or CNAME flattening, at the domain itself, pointing it at domains.whisk.page that way is better: it follows Whisk if the addresses change."
	bareWords := "A bare domain such as example.com cannot have a CNAME: point it with A (IPv4) or AAAA (IPv6) records to 95.217.38.236, 2a01:4f9::1 instead."
	cases := []struct {
		name    string
		d       api.Domain
		records []map[string]string
		want    []string
	}{
		{"a bare domain", api.Domain{Addresses: []string{"95.217.38.236"}, ApexTarget: "domains.whisk.page"}, byAddress, []string{flatten}},
		{"a bare domain without an apex target", api.Domain{Addresses: []string{"95.217.38.236"}}, byAddress, nil},
		{"a name with a CNAME", api.Domain{Addresses: []string{"95.217.38.236", "2a01:4f9::1"}, ApexTarget: "domains.whisk.page"}, byCNAME,
			[]string{bareWords, "Where" + flatten[2:]}},
		{"a name with a CNAME, no apex target", api.Domain{Addresses: []string{"95.217.38.236", "2a01:4f9::1"}}, byCNAME, []string{bareWords}},
		{"nothing to say", api.Domain{}, byCNAME, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := apexAdvice(c.d, c.records); !reflect.DeepEqual(got, c.want) {
				t.Errorf("apexAdvice = %q, want %q", got, c.want)
			}
		})
	}
}
