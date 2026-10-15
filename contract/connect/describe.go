package connect

import (
	"net"
	"net/url"
	"sort"
	"strings"
)

// Use says how a recipe uses one secret (BROKER.md §4).
type Use struct {
	Name string `json:"name"`
	// Sent is true when the secret, or something that turns back into it, leaves Whisk; false
	// when it only ever keys a MAC or signature or is hashed, so only a signature leaves.
	Sent bool `json:"sent"`
	// To lists the hosts it is sent to, sorted; empty when it only signs.
	To []string `json:"to"`
}

// oneWay reports whether argument i of fn can be read back from fn's result. A MAC, digest or
// signature reveals neither its key nor its message; a JWT's claims, algorithm and key id are
// readable in the token, its key is not.
func oneWay(fn string, i int) bool {
	switch fn {
	case "hmac_sha1", "hmac_sha256", "hmac_sha512", "sha1", "sha256", "sha512", "sign":
		return true
	case "jwt":
		return i == 1
	}
	return false
}

// Usage reports how the recipe uses each secret it reads, sorted by name.
func Usage(c Connection) []Use {
	c = c.WithDefaults()
	sent := map[string]map[string]bool{}
	seen := map[string]bool{}
	visit := func(src, host string, ctx Context) {
		t, _, err := ParseTemplate(src, ctx)
		if err != nil {
			return
		}
		for _, p := range t.parts {
			walk(p, true, func(name string, exposed bool) {
				seen[name] = true
				if exposed {
					if sent[name] == nil {
						sent[name] = map[string]bool{}
					}
					sent[name][host] = true
				}
			})
		}
	}
	call := Context{Request: true, Token: c.Auth.Token != nil}
	host := hostOf(c.URL)
	for _, k := range sortedKeys(c.Auth.Headers) {
		visit(c.Auth.Headers[k], host, call)
	}
	for _, k := range sortedKeys(c.Auth.Query) {
		visit(c.Auth.Query[k], host, call)
	}
	for _, k := range sortedKeys(c.Auth.Body) {
		visit(c.Auth.Body[k], host, Context{Token: c.Auth.Token != nil})
	}
	if t := c.Auth.Token; t != nil {
		th := hostOf(t.URL)
		for _, m := range []map[string]string{t.Headers, t.Form, t.JSON} {
			for _, k := range sortedKeys(m) {
				visit(m[k], th, Context{})
			}
		}
		if r := t.Revoke; r != nil {
			for _, m := range []map[string]string{r.Headers, r.Form} {
				for _, k := range sortedKeys(m) {
					visit(m[k], th, Context{Token: true})
				}
			}
		}
		if l := t.Logout; l != nil {
			for _, k := range sortedKeys(l.Headers) {
				visit(l.Headers[k], th, Context{Token: true})
			}
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Use, 0, len(names))
	for _, n := range names {
		to := make([]string, 0, len(sent[n]))
		for h := range sent[n] {
			to = append(to, h)
		}
		sort.Strings(to)
		out = append(out, Use{Name: n, Sent: len(to) > 0, To: to})
	}
	return out
}

func walk(n node, exposed bool, see func(string, bool)) {
	switch v := n.(type) {
	case ref:
		if v.path[0] == "secret" {
			see(v.path[1], exposed)
		}
	case call:
		for i, a := range v.args {
			walk(a, exposed && !oneWay(v.fn, i), see)
		}
	}
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// Summary is Whisk's own account of what granting a connection allows, drawn only from the
// rules the broker enforces (CONTROL-PLANE.md §6.8). Label on an operation is the app's own
// description and is never the basis of a decision.
type Summary struct {
	Host     string `json:"host"`
	Port     string `json:"port"`
	BasePath string `json:"base_path"`
	Insecure bool   `json:"insecure"`
	// Pin is the one certificate key the host must present, when the connection pins one.
	Pin string `json:"pin,omitempty"`
	// Keypair is the secret holding the key pair Whisk makes for the connection, when it asks
	// for one.
	Keypair string `json:"keypair,omitempty"`
	// VendorApp is Whisk's own developer app the connection signs in as, when it names one.
	VendorApp string `json:"vendor_app,omitempty"`
	TokenHost string `json:"token_host,omitempty"`
	// Revokes is true when revoking the grant also asks the token host to cancel the tokens the
	// broker holds, by revoking them or by logging their sessions out.
	Revokes    bool               `json:"revokes"`
	Operations []SummaryOperation `json:"operations"`
	Secrets    []Use              `json:"secrets"`
	SignsBody  bool               `json:"signs_body"`
	// BodyFields are the JSON pointers in each call's body where the broker places a value.
	BodyFields []string `json:"body_fields"`
}

// SummaryOperation is one operation as enforced, with the app's label beside it.
type SummaryOperation struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Verb   string `json:"verb"`
	Label  string `json:"label"`
	// Body is what the call's JSON body must hold, by JSON pointer.
	Body map[string]string `json:"body,omitempty"`
}

var verbs = map[string]string{"GET": "Read", "HEAD": "Read", "POST": "Create", "PUT": "Replace", "PATCH": "Change", "DELETE": "Delete", "*": "Any method"}

// Describe is the summary of a connection.
func Describe(c Connection) Summary {
	c = c.WithDefaults()
	u, _ := url.Parse(c.URL)
	s := Summary{Secrets: Usage(c), SignsBody: UsesBody(c), Operations: []SummaryOperation{}, BodyFields: sortedKeys(c.Auth.Body), Pin: c.Pin, VendorApp: c.Auth.VendorApp}
	if c.Keypair != nil {
		s.Keypair = c.Keypair.Secret
	}
	if u != nil {
		s.Host, s.Port, s.Insecure = u.Hostname(), u.Port(), u.Scheme != "https"
		if s.Port == "" {
			s.Port = map[bool]string{true: "443", false: "80"}[u.Scheme == "https"]
		}
		s.BasePath = strings.TrimSuffix(u.EscapedPath(), "/")
		if s.BasePath == "" {
			s.BasePath = "/"
		}
	}
	if c.Auth.Token != nil {
		s.TokenHost = hostOf(c.Auth.Token.URL)
		s.Revokes = c.Auth.Token.Revoke != nil || c.Auth.Token.Logout != nil
	}
	for _, o := range c.Operations {
		s.Operations = append(s.Operations, SummaryOperation{Method: o.Method, Path: o.Path, Verb: verbs[o.Method], Label: o.Name, Body: o.Body})
	}
	return s
}

// Upstream joins a connection's base URL and a call's escaped path below it.
func Upstream(base, path, rawQuery string) (*url.URL, error) {
	u, err := url.Parse(base)
	if err != nil {
		return nil, err
	}
	out := *u
	joined := strings.TrimSuffix(u.EscapedPath(), "/") + path
	p, err := url.PathUnescape(joined)
	if err != nil {
		return nil, err
	}
	out.Path, out.RawPath = p, joined
	out.RawQuery = rawQuery
	return &out, nil
}

// HostPort is the host and port a URL dials.
func HostPort(u *url.URL) string {
	port := u.Port()
	if port == "" {
		port = map[bool]string{true: "443", false: "80"}[u.Scheme == "https"]
	}
	return net.JoinHostPort(u.Hostname(), port)
}
