package doctor

import (
	"strings"
	"testing"
)

func TestSignInRules(t *testing.T) {
	manifest := passing["whisk.yaml"] + "customer_identity: app\n"
	png := "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"
	cases := []struct {
		name  string
		rule  string
		files map[string]string
		want  bool
	}{
		{"logo missing", "W130", map[string]string{"whisk.yaml": manifest + "sign_in: {logo: brand/logo.png}\n"}, true},
		{"logo an svg", "W130", map[string]string{"whisk.yaml": manifest + "sign_in: {logo: brand/logo.svg}\n", "brand/logo.svg": "<svg xmlns=\"http://www.w3.org/2000/svg\"/>"}, true},
		{"logo a png named jpg", "W130", map[string]string{"whisk.yaml": manifest + "sign_in: {logo: brand/logo.jpg}\n", "brand/logo.jpg": png}, false},
		{"logo too big", "W130", map[string]string{"whisk.yaml": manifest + "sign_in: {logo: brand/logo.png}\n", "brand/logo.png": png + strings.Repeat("\x00", 101<<10)}, true},
		{"logo a png", "W130", map[string]string{"whisk.yaml": manifest + "sign_in: {logo: brand/logo.png}\n", "brand/logo.png": png}, false},
		{"no logo", "W130", map[string]string{"whisk.yaml": manifest + "sign_in: {colour: '#0b6e4f'}\n"}, false},

		{"grey on white", "W131", map[string]string{"whisk.yaml": manifest + "sign_in: {text: '#bbbbbb', background: '#ffffff'}\n"}, true},
		{"white on Whisk's background", "W131", map[string]string{"whisk.yaml": manifest + "sign_in: {text: '#ffffff'}\n"}, true},
		{"dark on white", "W131", map[string]string{"whisk.yaml": manifest + "sign_in: {text: '#1a1a1a', background: '#ffffff'}\n"}, false},
		{"colour only", "W131", map[string]string{"whisk.yaml": manifest + "sign_in: {colour: '#ffff00'}\n"}, false},

		{"no way out", "W132", map[string]string{"whisk.yaml": manifest}, true},
		{"only a test signs out", "W132", map[string]string{"whisk.yaml": manifest, "src/app.test.ts": "fetch('/.whisk/logout')\n"}, true},
		{"an account link", "W132", map[string]string{"whisk.yaml": manifest, "src/views/layout.html": "<a href=\"/.whisk/account\">Account</a>\n"}, false},
		{"a sign-out link", "W132", map[string]string{"whisk.yaml": manifest, "src/nav.tsx": "<a href=\"/.whisk/logout?return=/\">Sign out</a>\n"}, false},
		{"no customers", "W132", map[string]string{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rep := runDoctor(t, with(passing, c.files), false)
			if got := has(rep, c.rule); got != c.want {
				t.Errorf("%s = %v, want %v; findings %+v; skipped %v", c.rule, got, c.want, rep.Findings, rep.Skipped)
			}
		})
	}
}
