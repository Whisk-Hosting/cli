package manifest

import (
	"strings"
	"testing"
)

func TestSignIn(t *testing.T) {
	m, err := Parse([]byte("whisk: 1\nname: portal\ncustomer_identity: app\nsign_in:\n  logo: brand/logo.png\n  colour: '#0B6E4F'\n"))
	if err != nil {
		t.Fatal(err)
	}
	if m.SignIn == nil || m.SignIn.Logo != "brand/logo.png" || m.SignIn.Colour != "#0B6E4F" ||
		m.SignIn.Font != FontWhisk || m.SignIn.Corners != CornersRounded || m.SignIn.LogoPath() != "brand/logo.png" {
		t.Fatalf("sign_in = %+v", m.SignIn)
	}
	if none, _ := Parse([]byte("whisk: 1\nname: portal\n")); none.SignIn != nil {
		t.Fatalf("no sign_in = %+v", none.SignIn)
	}
	for _, tc := range []struct{ name, yaml, problem string }{
		{"without customers", "sign_in: {colour: '#000'}", "/sign_in"},
		{"a shared pool", "customer_identity: org\nsign_in: {colour: '#000'}", "/sign_in"},
		{"a colour by name", "customer_identity: app\nsign_in: {colour: green}", "colour"},
		{"a colour without its hash", "customer_identity: app\nsign_in: {colour: '0b6e4f'}", "colour"},
		{"a font of its own", "customer_identity: app\nsign_in: {font: Comic Sans}", "font"},
		{"round corners", "customer_identity: app\nsign_in: {corners: round}", "corners"},
		{"the logo's hash", "customer_identity: app\nsign_in: {logo_sha256: abc}", "sign_in"},
		{"a logo outside", "customer_identity: app\nsign_in: {logo: ../logo.png}", "/sign_in/logo"},
		{"a logo from the root", "customer_identity: app\nsign_in: {logo: /etc/logo.png}", "/sign_in/logo"},
		{"color", "customer_identity: app\nsign_in: {color: '#000'}", "sign_in"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte("whisk: 1\nname: portal\n" + tc.yaml + "\n"))
			if err == nil || !strings.Contains(err.Error(), tc.problem) {
				t.Fatalf("Parse = %v, want a problem naming %s", err, tc.problem)
			}
		})
	}
}

func TestLogoPath(t *testing.T) {
	cases := map[string]string{
		"brand/logo.png": "brand/logo.png", "./logo.png": "logo.png", "a/../logo.png": "logo.png",
		"../logo.png": "", "/logo.png": "", "": "", "a\\logo.png": "", "..": "",
	}
	for in, want := range cases {
		if got := (SignIn{Logo: in}).LogoPath(); got != want {
			t.Errorf("LogoPath(%q) = %q, want %q", in, got, want)
		}
	}
}
