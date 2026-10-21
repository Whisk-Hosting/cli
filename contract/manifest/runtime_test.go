package manifest

import "testing"

func TestSettingsOnly(t *testing.T) {
	base := Manifest{
		Name: "crm", Routes: Routes{Public: []string{"/"}}, Health: Health{Path: "/health", Timeout: 60},
		Build:  Build{Dockerfile: "Dockerfile", Secrets: []string{"NPM_TOKEN", "SENTRY_AUTH"}},
		Env:    map[string]string{"NODE_ENV": "production"},
		Static: []Static{{Dir: "public", Path: "/assets"}},
	}
	with := func(f func(*Manifest)) Manifest {
		m := base
		m.Build.Secrets = append([]string(nil), base.Build.Secrets...)
		m.Env = map[string]string{"NODE_ENV": "production"}
		m.Static = append([]Static(nil), base.Static...)
		f(&m)
		return m
	}
	yaml := []string{"whisk.yaml"}
	for _, tc := range []struct {
		name    string
		changed []string
		after   Manifest
		want    bool
	}{
		{"a route made public", yaml, with(func(m *Manifest) { m.Routes.Public = append(m.Routes.Public, "/pricing") }), true},
		{"always on", yaml, with(func(m *Manifest) { m.AlwaysOn = true }), true},
		{"health timeout", yaml, with(func(m *Manifest) { m.Health.Timeout = 30 }), true},
		{"a challenge route and headers", yaml, with(func(m *Manifest) {
			m.Routes.Challenge = []string{"/signup"}
			m.Routes.Headers = map[string]string{"X-Frame-Options": "DENY"}
		}), true},
		{"a secret the app reads at run time", yaml, with(func(m *Manifest) { m.Secrets = []string{"STRIPE_KEY"} }), true},
		{"a static folder served elsewhere", yaml, with(func(m *Manifest) { m.Static[0].Path = "/static" }), true},
		{"build secrets in another order", yaml, with(func(m *Manifest) { m.Build.Secrets = []string{"SENTRY_AUTH", "NPM_TOKEN"} }), true},
		{"the same manifest, nothing changed", nil, base, true},
		{"env changed", yaml, with(func(m *Manifest) { m.Env["NODE_ENV"] = "development" }), false},
		{"env added", yaml, with(func(m *Manifest) { m.Env["NEXT_PUBLIC_API"] = "https://api.example.com" }), false},
		{"a build secret added", yaml, with(func(m *Manifest) { m.Build.Secrets = append(m.Build.Secrets, "GH_TOKEN") }), false},
		{"another Dockerfile", yaml, with(func(m *Manifest) { m.Build.Dockerfile = "Dockerfile.prod" }), false},
		{"another static folder", yaml, with(func(m *Manifest) { m.Static[0].Dir = "dist" }), false},
		{"trade ordering asked for", yaml, with(func(m *Manifest) { m.B2B = true }), false},
		{"code changed too", []string{"whisk.yaml", "main.go"}, with(func(m *Manifest) { m.AlwaysOn = true }), false},
		{"code alone", []string{"main.go"}, base, false},
		{"a manifest in a folder is code", []string{"docs/whisk.yaml"}, base, false},
	} {
		if got := SettingsOnly(tc.changed, base, tc.after); got != tc.want {
			t.Errorf("%s: SettingsOnly = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSameBuildEmptyEqualsAbsent(t *testing.T) {
	if !SameBuild(Manifest{Env: map[string]string{}, Build: Build{Secrets: []string{}}}, Manifest{}) {
		t.Fatal("an empty env and empty build secrets ask the build for what leaving them out does")
	}
}
