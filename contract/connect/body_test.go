package connect

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// odoo is Odoo 16 to 18's JSON-RPC: every call is POST /jsonrpc, and the API key is the third
// argument of execute_kw.
func odoo() Connection {
	return Connection{
		URL:        "https://erp.example.com",
		Auth:       Auth{Body: map[string]string{"/params/args/2": "{secret.ODOO_KEY}"}},
		Operations: []Operation{{Name: "Call the API", Method: "POST", Path: "/jsonrpc"}},
	}.WithDefaults()
}

func TestBodyPlacementsCheck(t *testing.T) {
	ps, secrets := Check(odoo())
	if len(ps) != 0 || strings.Join(secrets, ",") != "ODOO_KEY" {
		t.Fatalf("problems %v secrets %v", ps, secrets)
	}
	if u := Usage(odoo()); len(u) != 1 || !u[0].Sent || strings.Join(u[0].To, ",") != "erp.example.com" {
		t.Fatalf("a body-placed secret is not counted as sent: %+v", u)
	}
	if d := Describe(odoo()); strings.Join(d.BodyFields, ",") != "/params/args/2" {
		t.Fatalf("summary %+v", d)
	}
	bad := func(body map[string]string, extra func(*Connection), want string) {
		t.Helper()
		c := odoo()
		c.Auth.Body = body
		if extra != nil {
			extra(&c)
		}
		ps, _ := Check(c)
		for _, p := range ps {
			if strings.HasPrefix(p.Path, "/auth/body") && strings.Contains(p.Message, want) {
				return
			}
		}
		t.Errorf("%v: no problem %q in %v", body, want, ps)
	}
	bad(map[string]string{"params": "{secret.K}"}, nil, "JSON pointer")
	bad(map[string]string{"/a~2b": "{secret.K}"}, nil, "~0")
	bad(map[string]string{"/a": "{secret.K}", "/a/b": "{secret.K}"}, nil, "inside the placement at /a")
	bad(map[string]string{"/a": "{request.path}"}, nil, "")
	bad(map[string]string{"/a": "{secret.K}"}, func(c *Connection) {
		c.Auth.Headers = map[string]string{"X-Sig": "{hex(hmac_sha256(secret.K, request.body))}"}
	}, "cannot also sign")
	many := map[string]string{}
	for _, k := range []string{"/a", "/b", "/c", "/d", "/e", "/f", "/g", "/h", "/i"} {
		many[k] = "{secret.K}"
	}
	bad(many, nil, "at most 8")
}

func TestPlaceBody(t *testing.T) {
	env := Env{Secrets: map[string]string{"ODOO_KEY": "k3y-\"quoted\"<&>"}, Now: time.Unix(0, 0)}
	cases := []struct {
		name, in, out string
		err           string
	}{
		{"replaces the placeholder", `{"jsonrpc":"2.0","method":"call","params":{"service":"object","method":"execute_kw","args":["db",7,"x","res.partner","search",[[]]]}}`,
			`{"jsonrpc":"2.0","method":"call","params":{"args":["db",7,"k3y-\"quoted\"<&>","res.partner","search",[[]]],"method":"execute_kw","service":"object"}}`, ""},
		{"any placeholder type", `{"params":{"args":["db",7,null]}}`, `{"params":{"args":["db",7,"k3y-\"quoted\"<&>"]}}`, ""},
		{"a repeated key leaves one copy, the broker's", `{"params":{"args":["db",7,"a"]},"params":{"args":["db",7,"b"]}}`, `{"params":{"args":["db",7,"k3y-\"quoted\"<&>"]}}`, ""},
		{"keeps big numbers exactly", `{"n":12345678901234567890.5,"params":{"args":[1,2,3]}}`, `{"n":12345678901234567890.5,"params":{"args":[1,2,"k3y-\"quoted\"<&>"]}}`, ""},
		{"missing index", `{"params":{"args":["db",7]}}`, "", "no value at /params/args/2"},
		{"missing key", `{"params":{}}`, "", "no value at /params/args/2"},
		{"array where an object is", `{"params":["args"]}`, "", "no value"},
		{"not JSON", `params=1`, "", "not a JSON document"},
		{"two documents", `{"params":{"args":[1,2,3]}} {}`, "", "not a JSON document"},
		{"empty", ``, "", "not a JSON document"},
	}
	for _, c := range cases {
		got, err := PlaceBody(odoo(), env, []byte(c.in))
		if c.err != "" {
			if err == nil || !strings.Contains(err.Error(), c.err) {
				t.Errorf("%s: err %v, want %q", c.name, err, c.err)
			}
			continue
		}
		if err != nil || string(got) != c.out {
			t.Errorf("%s:\n got %s %v\nwant %s", c.name, got, err, c.out)
		}
	}
	if _, err := PlaceBody(odoo(), Env{}, []byte(`{"params":{"args":[1,2,3]}}`)); !errors.As(err, new(*MissingSecretError)) {
		t.Errorf("an unset secret: %v", err)
	}
	keyed := odoo()
	keyed.Auth.Body = map[string]string{"/a~1b": "{secret.ODOO_KEY}", "/c~0d": "x"}
	if got, err := PlaceBody(keyed, env, []byte(`{"a/b":1,"c~d":2}`)); err != nil || !strings.Contains(string(got), `"a/b":"k3y`) || !strings.Contains(string(got), `"c~d":"x"`) {
		t.Errorf("escaped pointers: %s %v", got, err)
	}
	plain := conn()
	if got, _ := PlaceBody(plain, env, []byte("raw")); string(got) != "raw" {
		t.Errorf("a recipe without placements changed the body")
	}
}
