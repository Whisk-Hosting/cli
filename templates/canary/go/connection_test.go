package main

import (
	"testing"
)

func TestConnectionTarget(t *testing.T) {
	env := map[string]string{"WHISK_CONNECTION_ERP_URL": "http://connect.internal.whisk:8443/erp"}
	lookup := func(k string) string { return env[k] }
	cases := []struct {
		name, conn, path, want string
		fails                  bool
	}{
		{"a read", "erp", "/stock/42", "http://connect.internal.whisk:8443/erp/stock/42", false},
		{"an encoded dot segment is kept as given", "erp", "/stock/%2e%2e/payroll", "http://connect.internal.whisk:8443/erp/stock/%2e%2e/payroll", false},
		{"a query is kept", "erp", "/stock?sku=1", "http://connect.internal.whisk:8443/erp/stock?sku=1", false},
		{"a connection the app does not declare", "billing", "/x", "", true},
		{"a name that is not a connection name", "ERP", "/x", "", true},
		{"a name that escapes the variable", "erp_URL;", "/x", "", true},
		{"a path without a slash", "erp", "stock", "", true},
		{"no name", "", "/x", "", true},
	}
	for _, c := range cases {
		got, err := connectionTarget(lookup, c.conn, c.path)
		if (err != nil) != c.fails || got != c.want {
			t.Errorf("%s: got %q, %v; want %q, fails %v", c.name, got, err, c.want, c.fails)
		}
	}
}

func TestConnectionTargetTrailingSlash(t *testing.T) {
	got, err := connectionTarget(func(string) string { return "http://connect.internal.whisk:8443/erp/" }, "erp", "/orders")
	if err != nil || got != "http://connect.internal.whisk:8443/erp/orders" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestEnvPresence(t *testing.T) {
	env := map[string]string{"WHISK_CONNECTION_ERP_URL": "http://x", "EMPTY": ""}
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	got, err := envPresence("ERP_KEY, ERP_ID,ERP_SECRET,WHISK_CONNECTION_ERP_URL,EMPTY", lookup)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"ERP_KEY": false, "ERP_ID": false, "ERP_SECRET": false, "WHISK_CONNECTION_ERP_URL": true, "EMPTY": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %v, want %v", k, got[k], v)
		}
	}
	for _, bad := range []string{"", " , ", "erp_key", "A=B", "PATH,../x"} {
		if _, err := envPresence(bad, lookup); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
}
