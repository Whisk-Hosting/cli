package connect

import (
	"errors"
	"testing"
	"time"
)

// exo is MYOB Exo's shape: Whisk's developer key in a header beside the business's own secrets.
func exo(vendorApp string) Connection {
	return Connection{URL: "https://exo.api.myob.com", Operations: []Operation{{Name: "Read", Method: "GET", Path: "/stockitem"}},
		Auth: Auth{VendorApp: vendorApp, Headers: map[string]string{
			"Authorization":      "{basic(secret.EXO_USER, secret.EXO_PASSWORD)}",
			"x-myobapi-key":      "{vendor.client_id}",
			"x-myobapi-exotoken": "{secret.EXO_TOKEN}",
		}}}.WithDefaults()
}

func TestVendorApp(t *testing.T) {
	c := exo("myobexo")
	ps, secrets := Check(c)
	if len(ps) != 0 {
		t.Fatalf("problems %v", ps)
	}
	if len(secrets) != 3 {
		t.Errorf("the vendor app counted as a secret: %v", secrets)
	}
	env := NewEnv(map[string]string{"EXO_USER": "u", "EXO_PASSWORD": "p", "EXO_TOKEN": "t"}, &Request{Method: "GET"}, "", time.Unix(1_800_000_000, 0))
	if _, err := Apply(c, env); !errors.As(err, new(*MissingSecretError)) {
		t.Errorf("no vendor values: %v", err)
	}
	env.Vendor = map[string]string{"client_id": "whisk-key", "client_secret": "whisk-secret"}
	placed, err := Apply(c, env)
	if err != nil || placed.Headers.Get("X-Myobapi-Key") != "whisk-key" {
		t.Fatalf("placed %+v %v", placed.Headers, err)
	}
	if Hash(c) == Hash(exo("myob")) {
		t.Error("the vendor app is not part of the granted version")
	}
}

func TestVendorAppCheck(t *testing.T) {
	cases := map[string]struct {
		c    Connection
		path string
	}{
		"vendor values without a vendor app": {exo(""), "/auth/headers/x-myobapi-key"},
		"a bad name":                         {exo("MYOB Exo"), "/auth/vendor_app"},
		"one letter":                         {exo("m"), "/auth/vendor_app"},
	}
	bad := exo("myobexo")
	bad.Auth.Headers = map[string]string{"x-myobapi-key": "{vendor.key}"}
	cases["an unknown vendor field"] = struct {
		c    Connection
		path string
	}{bad, "/auth/headers/x-myobapi-key"}
	inToken := Connection{URL: "https://api.example.com", Operations: []Operation{{Name: "Read", Method: "GET", Path: "/**"}},
		Auth: Auth{Headers: map[string]string{"Authorization": "Bearer {token}"}, Token: &TokenStep{URL: "https://login.example.com/token",
			Headers: map[string]string{"Authorization": "{basic(vendor.client_id, vendor.client_secret)}"}}}}.WithDefaults()
	cases["vendor values in a token step without a vendor app"] = struct {
		c    Connection
		path string
	}{inToken, "/auth/token/headers/Authorization"}
	for name, tc := range cases {
		ps, _ := Check(tc.c)
		found := false
		for _, p := range ps {
			found = found || p.Path == tc.path
		}
		if !found {
			t.Errorf("%s: want a problem at %s, got %v", name, tc.path, ps)
		}
	}
	inToken.Auth.VendorApp = "microsoft"
	if ps, _ := Check(inToken); len(ps) != 0 {
		t.Errorf("a token step signed with the vendor app: %v", ps)
	}
}
