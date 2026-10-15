package webhook

import (
	"net/url"
	"strings"
	"testing"
)

func TestHandshake(t *testing.T) {
	bc := Handshake{Query: "validationToken"}
	if err := bc.Check(); err != nil {
		t.Fatal(err)
	}
	for _, h := range []Handshake{{Query: ""}, {Query: "a b"}, {Query: "x", Method: "PUT"}} {
		if h.Check() == nil {
			t.Errorf("%+v passes its check", h)
		}
	}
	cases := []struct {
		method, query string
		want          string
		ok            bool
	}{
		{"POST", "validationToken=abc-123_XYZ", "abc-123_XYZ", true},
		{"POST", "validationToken=" + url.QueryEscape("a<b>&\"c"), "a<b>&\"c", true},
		{"GET", "validationToken=abc", "", false},
		{"POST", "", "", false},
		{"POST", "validationToken=", "", false},
		{"POST", "validationToken=a&validationToken=b", "", false},
		{"POST", "validationToken=" + url.QueryEscape("a b"), "", false},
		{"POST", "validationToken=" + url.QueryEscape("a\r\nSet-Cookie: x"), "", false},
		{"POST", "validationToken=" + strings.Repeat("a", 1025), "", false},
	}
	for _, c := range cases {
		q, _ := url.ParseQuery(c.query)
		got, ok := bc.Echo(c.method, q)
		if got != c.want || ok != c.ok {
			t.Errorf("%s ?%.40s: %q %v", c.method, c.query, got, ok)
		}
	}
	sap := Handshake{Method: "GET", Query: "challenge"}
	if got, ok := sap.Echo("GET", url.Values{"challenge": {"n0nce"}}); !ok || got != "n0nce" {
		t.Errorf("a GET handshake: %q %v", got, ok)
	}
}
