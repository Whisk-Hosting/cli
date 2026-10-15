package webhook

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
)

// Handshake is how a provider checks a webhook address before it sends deliveries: it calls the
// address with a value in a query parameter and wants the value back as text/plain with a 200
// (Business Central and Microsoft Graph POST ?validationToken=…, on creating a subscription and
// on every renewal). A handshake is answered, never stored or delivered to the app.
type Handshake struct {
	// Method is GET or POST (the default).
	Method string `yaml:"method,omitempty" json:"method,omitempty"`
	// Query is the query parameter carrying the value to send back.
	Query string `yaml:"query" json:"query"`
}

var queryName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,63}$`)

// echoable reports whether a handshake may send v back: 1 to 1024 characters of printable
// ASCII without spaces, so the answer can never carry a header.
func echoable(v string) bool {
	if len(v) == 0 || len(v) > 1024 {
		return false
	}
	for i := 0; i < len(v); i++ {
		if v[i] < 0x21 || v[i] > 0x7e {
			return false
		}
	}
	return true
}

// Check reports whether the handshake can be answered.
func (h Handshake) Check() error {
	if h.Method != "" && h.Method != http.MethodGet && h.Method != http.MethodPost {
		return errors.New("handshake.method is GET or POST")
	}
	if !queryName.MatchString(h.Query) {
		return errors.New("handshake.query is a query parameter name such as validationToken")
	}
	return nil
}

// Echo is the text to answer a request with when it is this handshake: the request's method
// is the handshake's and its query carries exactly one printable value under Query. ok is false
// for anything else, which is then an ordinary delivery. Pure.
func (h Handshake) Echo(method string, q url.Values) (text string, ok bool) {
	want := h.Method
	if want == "" {
		want = http.MethodPost
	}
	vs := q[h.Query]
	if method != want || len(vs) != 1 || !echoable(vs[0]) {
		return "", false
	}
	return vs[0], true
}
