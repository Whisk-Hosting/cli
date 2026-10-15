package stub

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	werrors "github.com/whisk-run/contract/errors"
)

// An app's own sending domains, the stand-in for /v1/orgs/{org}/apps/{app}/email/domains
// (CONTROL-PLANE.md §6.12, "An app's own sending domains"), so an app that registers a domain for
// each of its customers runs locally as it does on Whisk. A registered domain is answered with
// records shaped like the provider's (DKIM, SPF and the return path's MX) and verifies when it is
// checked, because a local run has no DNS to wait for. Memory only.

type emailDNSRecord struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

type emailDomain struct {
	ID            string           `json:"id"`
	Domain        string           `json:"domain"`
	App           string           `json:"app"`
	Status        string           `json:"status"`
	DNSRecords    []emailDNSRecord `json:"dns_records"`
	BounceRate    float64          `json:"bounce_rate"`
	ComplaintRate float64          `json:"complaint_rate"`
	VerifiedAt    *time.Time       `json:"verified_at,omitempty"`
	CheckedAt     *time.Time       `json:"checked_at,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
}

type emailDomains struct {
	mu   sync.Mutex
	byID map[string]*emailDomain
}

func newEmailDomains() *emailDomains { return &emailDomains{byID: map[string]*emailDomain{}} }

// stubDomainRecords is the DNS a domain's owner publishes, in the provider's shape. Pure.
func stubDomainRecords(domain string) []emailDNSRecord {
	return []emailDNSRecord{
		{Type: "TXT", Name: "resend._domainkey." + domain, Value: "p=MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQC-local-stub"},
		{Type: "MX", Name: "send." + domain, Value: "10 feedback-smtp.eu-west-1.amazonses.com"},
		{Type: "TXT", Name: "send." + domain, Value: "v=spf1 include:amazonses.com ~all"},
	}
}

// stubDomainName is a sending domain as the platform takes it: lower case, without a leading @,
// two or more labels of a-z, 0-9 and -, none starting or ending with a hyphen. Pure.
func stubDomainName(raw string) (string, bool) {
	name := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "@")))
	labels := strings.Split(name, ".")
	if len(name) > 253 || len(labels) < 2 {
		return name, false
	}
	for _, l := range labels {
		if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' || strings.Trim(l, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
			return name, false
		}
	}
	return name, true
}

// emailDomains serves the routes under .../email/domains; rest is what follows that path.
func (a *api) emailDomains(w http.ResponseWriter, r *http.Request, rest string) {
	if !a.bearerOK(r) {
		writeError(w, r, 401, authRequired("Sending domains need the service token.", "Send Authorization: Bearer $WHISK_SERVICE_TOKEN."))
		return
	}
	d := a.sendingDomains
	id, action, _ := strings.Cut(strings.TrimPrefix(rest, "/"), "/")
	switch {
	case id == "" && r.Method == http.MethodGet:
		d.mu.Lock()
		items := make([]emailDomain, 0, len(d.byID))
		for _, e := range d.byID {
			items = append(items, *e)
		}
		d.mu.Unlock()
		sort.Slice(items, func(i, j int) bool { return items[i].Domain < items[j].Domain })
		writeJSON(w, 200, map[string]any{"items": items})
	case id == "" && r.Method == http.MethodPost:
		a.addEmailDomain(w, r)
	case id != "" && action == "" && r.Method == http.MethodGet:
		if e, ok := d.get(id); ok {
			writeJSON(w, 200, e)
			return
		}
		writeError(w, r, 404, emailDomainNotFound(id))
	case id != "" && action == "verify" && r.Method == http.MethodPost:
		d.mu.Lock()
		e, ok := d.byID[id]
		if ok {
			now := time.Now().UTC()
			e.CheckedAt = &now
			if e.Status == "pending" {
				e.Status, e.VerifiedAt = "verified", &now
			}
		}
		var out emailDomain
		if ok {
			out = *e
		}
		d.mu.Unlock()
		if !ok {
			writeError(w, r, 404, emailDomainNotFound(id))
			return
		}
		a.stub.logf("sending domain %s verified (locally there is no DNS to wait for)", out.Domain)
		writeJSON(w, 200, out)
	case id != "" && action == "" && r.Method == http.MethodDelete:
		d.mu.Lock()
		e, ok := d.byID[id]
		delete(d.byID, id)
		d.mu.Unlock()
		if !ok {
			writeError(w, r, 404, emailDomainNotFound(id))
			return
		}
		a.stub.logf("sending domain %s removed", e.Domain)
		w.WriteHeader(http.StatusNoContent)
	default:
		writeError(w, r, 404, werrors.New("NOT_FOUND", "No such route.", "See CONTROL-PLANE.md §6.12 for the sending domain routes.", nil))
	}
}

func (a *api) addEmailDomain(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Domain string `json:"domain"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&in); err != nil {
		writeError(w, r, 400, werrors.New("INVALID_REQUEST", "The request body is not JSON.", "Send {domain}.", nil))
		return
	}
	name, ok := stubDomainName(in.Domain)
	if !ok {
		writeError(w, r, 400, werrors.New("INVALID_REQUEST", "That is not a domain.", "Send the domain your customer sends from, like mail.theirbusiness.com.", map[string]any{"domain": name}))
		return
	}
	for _, own := range []string{"whisk.run", "whisk.page"} {
		if name == own || strings.HasSuffix(name, "."+own) {
			writeError(w, r, 409, werrors.New("EMAIL_DOMAIN_TAKEN", name+" is one of Whisk's own domains, so no business can send from it.",
				"Send from a domain your business owns, such as mail.yourbusiness.com.", map[string]any{"domain": name}))
			return
		}
	}
	d := a.sendingDomains
	d.mu.Lock()
	for _, e := range d.byID {
		if e.Domain == name {
			out := *e
			d.mu.Unlock()
			writeJSON(w, 200, out)
			return
		}
	}
	e := &emailDomain{ID: newULID(), Domain: name, App: a.stub.manifest.Name, Status: "pending", DNSRecords: stubDomainRecords(name), CreatedAt: time.Now().UTC()}
	d.byID[e.ID] = e
	out := *e
	d.mu.Unlock()
	a.stub.logf("sending domain %s registered; publish its records, then check it", name)
	writeJSON(w, 201, out)
}

func (d *emailDomains) get(id string) (emailDomain, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	e, ok := d.byID[id]
	if !ok {
		return emailDomain{}, false
	}
	return *e, true
}

func emailDomainNotFound(id string) werrors.Body {
	return werrors.New("NOT_FOUND", "The sending domain was not found.", "List the app's sending domains and use an id from there.", map[string]any{"id": id})
}
