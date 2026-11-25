package ownsignin

import (
	"strings"
	"testing"
)

func TestWarn(t *testing.T) {
	cases := []struct {
		name     string
		public   []string
		password bool
		promoted bool
		shop     bool
		want     bool
	}{
		{"every route public with a login, not promoted", []string{"/**"}, true, false, false, true},
		{"a public website with no login", []string{"/**"}, false, false, false, false},
		{"a catch-all beside other routes", []string{"/", "/**"}, true, false, false, true},
		{"an equivalent catch-all", []string{"/", "/*", "/*/**"}, true, false, false, true},
		{"a Promoted app", []string{"/**"}, true, true, false, false},
		{"a shop", []string{"/**"}, true, false, true, false},
		{"a promoted shop", []string{"/**"}, true, true, true, false},
		{"some routes public", []string{"/", "/health", "/blog/**"}, true, false, false, false},
		{"everything but home", []string{"/*/**"}, true, false, false, false},
		{"nothing public", nil, true, false, false, false},
	}
	for _, c := range cases {
		if got := Warn(c.public, c.password, c.promoted, c.shop); got != c.want {
			t.Errorf("%s: Warn(%q, %v, %v, %v) = %v, want %v", c.name, c.public, c.password, c.promoted, c.shop, got, c.want)
		}
	}
}

func TestShop(t *testing.T) {
	for commerce, want := range map[string]bool{"medusa": true, "": false, "Medusa": false, "shopify": false} {
		if got := Shop(commerce); got != want {
			t.Errorf("Shop(%q) = %v, want %v", commerce, got, want)
		}
	}
}

func TestWords(t *testing.T) {
	m := Message("crm")
	if !strings.Contains(m, "crm") || !strings.Contains(m, "only a Promoted app") || !strings.Contains(m, "/**") {
		t.Errorf("Message = %q", m)
	}
	f := Fix("crm", "https://whisk.run/o/acme/apps/crm")
	for _, want := range []string{"make it a Promoted app", "https://whisk.run/o/acme/apps/crm", "charged", "routes.public"} {
		if !strings.Contains(f, want) {
			t.Errorf("Fix = %q, want it to say %q", f, want)
		}
	}
	// Only the platform says whether an app is a Promoted app (CONTRACT.md §3), so the fix never
	// sends an agent to whisk.yaml for it.
	if strings.Contains(f, "promoted:") || strings.Contains(f, "whisk.yaml") {
		t.Errorf("Fix = %q, names whisk.yaml's promoted key", f)
	}
	if strings.Contains(m+f, "—") {
		t.Errorf("no em dashes in user-facing copy")
	}
}

func TestAsksPassword(t *testing.T) {
	cases := map[string]bool{
		`<input type="password" name="pw">`:   true,
		`<input type=password>`:               true,
		`<Input type={"password"} />`:         true,
		`el.type = "password"`:                true,
		`pw = PasswordField("Password")`:      true,
		`<input type="text" name="password">`: false,
		`const passwordReset = "/reset"`:      false,
	}
	for src, want := range cases {
		if got := AsksPassword([]byte(src)); got != want {
			t.Errorf("AsksPassword(%q) = %v, want %v", src, got, want)
		}
	}
}

func TestPage(t *testing.T) {
	cases := map[string]bool{
		"src/login.tsx":              true,
		"templates/login.html":       true,
		"app/views.py":               true,
		"main.go":                    true,
		"README.md":                  false,
		"src/login.test.tsx":         false,
		"tests/test_login.py":        false,
		"node_modules/x/login.html":  false,
		"dist/index.html":            false,
		"static/logo.svg":            false,
		"internal/web/login_test.go": false,
	}
	for p, want := range cases {
		if got := Page(p); got != want {
			t.Errorf("Page(%q) = %v, want %v", p, got, want)
		}
	}
}
