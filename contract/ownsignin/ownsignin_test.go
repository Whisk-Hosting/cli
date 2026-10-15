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

func TestMedusaShop(t *testing.T) {
	cases := []struct {
		name string
		pkg  string
		want bool
	}{
		{"the shop template", `{"dependencies":{"@medusajs/medusa":"2.8.0","astro":"5.0.0"}}`, true},
		{"medusa only as a dev dependency", `{"devDependencies":{"@medusajs/medusa":"2.8.0"}}`, false},
		{"another app", `{"dependencies":{"express":"5.0.0"}}`, false},
		{"no dependencies", `{"name":"x"}`, false},
		{"not JSON", `{"dependencies":`, false},
		{"no package.json", "", false},
	}
	for _, c := range cases {
		var raw []byte
		if c.pkg != "" {
			raw = []byte(c.pkg)
		}
		if got := MedusaShop(raw); got != c.want {
			t.Errorf("%s: MedusaShop = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestWords(t *testing.T) {
	m := Message("crm")
	if !strings.Contains(m, "crm") || !strings.Contains(m, "only a Promoted app") || !strings.Contains(m, "/**") {
		t.Errorf("Message = %q", m)
	}
	f := Fix("crm", "https://whisk.run/o/acme/apps/crm")
	for _, want := range []string{"promoted: true", "https://whisk.run/o/acme/apps/crm", "charged", "routes.public"} {
		if !strings.Contains(f, want) {
			t.Errorf("Fix = %q, want it to say %q", f, want)
		}
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
