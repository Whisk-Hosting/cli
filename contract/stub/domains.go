package stub

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/whisk-run/contract/apitypes"
	werrors "github.com/whisk-run/contract/errors"
)

// domains stands in for the platform's custom domain routes as the app's own service token calls
// them (CONTRACT.md §8, "Custom domains"):
//
//	GET    /v1/orgs/{org}/apps/{app}/domains
//	POST   /v1/orgs/{org}/apps/{app}/domains               {hostname}
//	POST   /v1/orgs/{org}/apps/{app}/domains/{id}/verify
//	DELETE /v1/orgs/{org}/apps/{app}/domains/{id}
//
// The stub reads no DNS, so a name under the reserved .test or .example top-level domains
// verifies at once with its certificate issued, and every other name answers DOMAIN_UNVERIFIED
// with its records, so both of an app's screens can be built locally. The plan's cap and the
// hourly limits are not emulated.
type domains struct {
	mu    sync.Mutex
	items map[string]*apitypes.Domain
}

func newDomains() *domains { return &domains{items: map[string]*apitypes.Domain{}} }

// stubVerifies reports whether the stub verifies a name: one under a reserved top-level domain
// that can never be anyone's (RFC 2606, RFC 6761).
func stubVerifies(hostname string) bool {
	return strings.HasSuffix(hostname, ".test") || strings.HasSuffix(hostname, ".example")
}

// validHostname is a lower-case DNS name of at least two labels of letters, digits and hyphens,
// whose last label has a letter, as the platform takes one.
func validHostname(h string) bool {
	if len(h) == 0 || len(h) > 253 || !strings.Contains(h, ".") || !strings.ContainsAny(h[strings.LastIndex(h, ".")+1:], "abcdefghijklmnopqrstuvwxyz") {
		return false
	}
	for _, label := range strings.Split(h, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}

// target is the name a custom domain points at in a local run.
func (a *api) domainTarget() string {
	name := strings.ToLower(a.stub.manifest.Name)
	if name == "" {
		name = "app"
	}
	return name + ".whisk.localhost"
}

func (a *api) domains(w http.ResponseWriter, r *http.Request, rest string) {
	if !a.bearerOK(r) {
		writeError(w, r, 401, authRequired("The domain routes need the app's service token.", "Send Authorization: Bearer $WHISK_SERVICE_TOKEN."))
		return
	}
	d := a.stub.store.domains
	id, verb, _ := strings.Cut(strings.TrimPrefix(rest, "/"), "/")
	switch {
	case id == "" && r.Method == http.MethodGet:
		writeJSON(w, 200, apitypes.List[apitypes.Domain]{Items: d.list()})
	case id == "" && r.Method == http.MethodPost:
		var in apitypes.DomainRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<14)).Decode(&in); err != nil {
			writeError(w, r, 400, werrors.New("INVALID_REQUEST", "The body is not JSON.", `Send {"hostname": "results.lab.example"}.`, nil))
			return
		}
		host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(in.Hostname), "."))
		if !validHostname(host) {
			writeError(w, r, 400, werrors.New("INVALID_REQUEST", "That is not a hostname.", "Pass a hostname like app.acme.com: letters, digits and hyphens, with a non-ASCII name in its xn-- form.", nil))
			return
		}
		if under(host, "whisk.run") || under(host, "whisk.page") || under(host, "localhost") {
			writeError(w, r, 400, werrors.New("INVALID_REQUEST", host+" is one of Whisk's own names, not a domain you own.", "Pass a hostname on a domain you control, like app.acme.com.", nil))
			return
		}
		dom, ok := d.add(host, a.domainTarget())
		if !ok {
			writeError(w, r, 409, werrors.New("DOMAIN_TAKEN", "That hostname is already registered on Whisk.", "Use a hostname you control that is not already added.", map[string]any{"hostname": host}))
			return
		}
		a.stub.logf("custom domain %s added (%s); names under .test or .example verify at once here", host, dom.ID)
		writeJSON(w, 201, dom)
	case id != "" && verb == "verify" && r.Method == http.MethodPost:
		dom, ok := d.verify(id)
		if !ok {
			writeError(w, r, 404, werrors.New("NOT_FOUND", "That domain was not found.", "List the app's domains with GET .../domains.", nil))
			return
		}
		if !dom.Verified {
			writeError(w, r, 409, werrors.New("DOMAIN_UNVERIFIED", dom.Hostname+" is not verified: the TXT record and a CNAME (or A records) pointing at the app were not found yet.",
				"The local stub reads no DNS: names under .test or .example verify at once, any other answers this. On Whisk, create the records, then POST .../domains/"+dom.ID+"/verify again.",
				map[string]any{"hostname": dom.Hostname, "records": dom.Records, "addresses": dom.Addresses, "missing": []string{"TXT", "CNAME"}}))
			return
		}
		writeJSON(w, 200, dom)
	case id != "" && verb == "" && r.Method == http.MethodDelete:
		if !d.remove(id) {
			writeError(w, r, 404, werrors.New("NOT_FOUND", "That domain was not found.", "List the app's domains with GET .../domains.", nil))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, r, 405, werrors.New("INVALID_REQUEST", r.Method+" is not a domain route.", "GET or POST .../domains, POST .../domains/{id}/verify, DELETE .../domains/{id}.", nil))
	}
}

func under(host, domain string) bool { return host == domain || strings.HasSuffix(host, "."+domain) }

func (d *domains) list() []apitypes.Domain {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]apitypes.Domain, 0, len(d.items))
	for _, x := range d.items {
		out = append(out, *x)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Hostname < out[j].Hostname })
	return out
}

func (d *domains) add(host, target string) (apitypes.Domain, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, x := range d.items {
		if x.Hostname == host {
			return apitypes.Domain{}, false
		}
	}
	token := "whisk-verify-" + strings.ToLower(randomToken(9))
	dom := &apitypes.Domain{ID: newULID(), Hostname: host, Kind: apitypes.DomainCustom, CertStatus: "pending",
		TXTRecord: "_whisk-verify." + host, TXTValue: token, CNAMETarget: target, Addresses: []string{"127.0.0.1"},
		Records: []apitypes.DNSRecord{{Type: "TXT", Name: "_whisk-verify." + host, Value: token}, {Type: "CNAME", Name: host, Value: target}},
		Status:  apitypes.DomainPendingDNS, AddedBy: apitypes.DomainAddedByApp, CreatedAt: time.Now().UTC()}
	d.items[dom.ID] = dom
	return *dom, true
}

func (d *domains) verify(id string) (apitypes.Domain, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	x, ok := d.items[id]
	if !ok {
		return apitypes.Domain{}, false
	}
	if stubVerifies(x.Hostname) {
		x.Verified, x.CertStatus, x.Status = true, "issued", apitypes.DomainActive
		x.TXTRecord, x.TXTValue, x.CNAMETarget, x.Addresses, x.Records = "", "", "", nil, nil
	}
	return *x, true
}

func (d *domains) remove(id string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.items[id]; !ok {
		return false
	}
	delete(d.items, id)
	return true
}
