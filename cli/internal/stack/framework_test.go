package stack

import "testing"

func TestJSFramework(t *testing.T) {
	cases := []struct{ name, pkg, want string }{
		{"hono", `{"dependencies":{"hono":"4"}}`, "hono"},
		{"next beside hono's types is next", `{"dependencies":{"next":"16"},"devDependencies":{"hono":"4"}}`, "next"},
		{"express", `{"dependencies":{"express":"5"}}`, "express"},
		{"none", `{}`, ""},
	}
	for _, c := range cases {
		if got, _ := jsFramework([]byte(c.pkg)); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}
