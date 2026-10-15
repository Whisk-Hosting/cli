package connect

import (
	"strings"
	"testing"
)

// rpc is Odoo 16 to 18's JSON-RPC with one operation per model and method, matched inside the
// body as execute_kw names them.
func rpc() Connection {
	c := odoo()
	c.Operations = []Operation{
		{Name: "Read products", Method: "POST", Path: "/jsonrpc",
			Body: map[string]string{"/params/service": "object", "/params/args/3": "product.product", "/params/args/4": "search_read"}},
		{Name: "Create sales orders", Method: "POST", Path: "/jsonrpc",
			Body: map[string]string{"/params/service": "object", "/params/args/3": "sale.order", "/params/args/4": "create"}},
	}
	return c
}

func TestBodyMatchCheck(t *testing.T) {
	if ps, _ := Check(rpc()); len(ps) != 0 {
		t.Fatalf("problems %v", ps)
	}
	bad := func(op Operation, want string) {
		t.Helper()
		c := rpc()
		c.Operations = []Operation{op}
		ps, _ := Check(c.WithDefaults())
		for _, p := range ps {
			if strings.HasPrefix(p.Path, "/operations/0/body") && strings.Contains(p.Message, want) {
				return
			}
		}
		t.Errorf("%+v: no problem %q in %v", op, want, ps)
	}
	op := func(method string, body map[string]string) Operation {
		return Operation{Name: "Call", Method: method, Path: "/jsonrpc", Body: body}
	}
	bad(op("GET", map[string]string{"/a": "x"}), "a call with a body")
	bad(op("*", map[string]string{"/a": "x"}), "a call with a body")
	bad(op("POST", map[string]string{"a": "x"}), "JSON pointer")
	bad(op("POST", map[string]string{"/a": ""}), "1 to 200 characters")
	bad(op("POST", map[string]string{"/a": strings.Repeat("x", 201)}), "1 to 200 characters")
	bad(op("POST", map[string]string{"/a": "1", "/b": "2", "/c": "3", "/d": "4", "/e": "5"}), "at most 4")
}

func TestMatchBody(t *testing.T) {
	ops := rpc().Operations
	call := func(model, method string) any {
		doc, err := DecodeBody([]byte(`{"jsonrpc":"2.0","method":"call","params":{"service":"object","method":"execute_kw",` +
			`"args":["db",2,"KEY","` + model + `","` + method + `",[[]],{}]}}`))
		if err != nil {
			t.Fatal(err)
		}
		return doc
	}
	cases := []struct {
		name string
		doc  any
		want string
	}{
		{"read products", call("product.product", "search_read"), "Read products"},
		{"create orders", call("sale.order", "create"), "Create sales orders"},
		{"another method", call("sale.order", "unlink"), ""},
		{"another model", call("res.users", "search_read"), ""},
		{"no body", nil, ""},
		{"a number where a string is matched", mustDecode(t, `{"params":{"service":"object","args":[0,0,0,1,"create"]}}`), ""},
		{"an index written 03", mustDecode(t, `{"params":{"service":"object","args":{"03":"sale.order"}}}`), ""},
		{"the last of a repeated key", mustDecode(t, `{"params":{"service":"common","service":"object","args":["d",2,"K","sale.order","create"]}}`), "Create sales orders"},
	}
	for _, c := range cases {
		got, ok := Match(ops, "POST", "/jsonrpc", c.doc)
		if ok != (c.want != "") || got.Name != c.want {
			t.Errorf("%s: got %q %v, want %q", c.name, got.Name, ok, c.want)
		}
	}
	if _, ok := Match(ops, "GET", "/jsonrpc", call("product.product", "search_read")); ok {
		t.Error("a GET matched a POST operation")
	}
}

func TestBodyMatchIsPartOfTheGrant(t *testing.T) {
	granted := rpc()
	wanted := rpc()
	wanted.Operations[0].Body = map[string]string{"/params/service": "object", "/params/args/3": "res.users", "/params/args/4": "write"}
	if _, ops := Gaps(&granted, wanted); len(ops) != 1 || ops[0].Name != "Read products" {
		t.Fatalf("a changed body condition under the same name needs no grant: %+v", ops)
	}
	if e := Effective(&granted, wanted); e == nil || len(e.Operations) != 1 || e.Operations[0].Name != "Create sales orders" {
		t.Fatalf("effective %+v", e)
	}
	if Hash(granted) == Hash(wanted) {
		t.Fatal("the hash ignores body conditions")
	}
}

func TestPlaceBodyReencodesAMatchedBody(t *testing.T) {
	c := rpc()
	c.Auth.Body = nil
	out, err := PlaceBody(c, Env{}, []byte(`{"params":{"service":"common","service":"object","n":1.50}}`))
	if err != nil || string(out) != `{"params":{"n":1.50,"service":"object"}}` {
		t.Fatalf("got %s %v", out, err)
	}
	if _, err := PlaceBody(c, Env{}, []byte(`not json`)); err != ErrBodyNotJSON {
		t.Fatalf("got %v", err)
	}
}

func mustDecode(t *testing.T, s string) any {
	t.Helper()
	doc, err := DecodeBody([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}
