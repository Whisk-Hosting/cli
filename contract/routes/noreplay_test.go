package routes

import "testing"

func TestNoReplay(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/device/token", true},
		{"/tokens/git", true},
		{"/orgs/acme/tokens", true},
		{"/orgs/{org}/tokens", true},
		{"/orgs/acme/apps/stock/db/query", true},
		{"/orgs/acme/apps/stock/db/query?timeout=5", true},
		{"/orgs/acme/members/m1/invite", true},
		{"/app-demos/d1/copies", true},
		{"/orgs/acme/apps/stock/deploys", false},
		{"/orgs/acme/tokens/t1", false},
		{"/orgs//tokens", false},
		{"/orgs/acme/apps/stock/db", false},
		{"/v1/orgs/acme/tokens", false},
		{"", false},
	}
	for _, c := range cases {
		if got := NoReplay(c.path); got != c.want {
			t.Errorf("NoReplay(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestNoReplayRoutesIsACopy(t *testing.T) {
	r := NoReplayRoutes()
	r[0] = "/changed"
	if !NoReplay("/device/code") {
		t.Fatal("changing the returned slice changed the set")
	}
}

func TestMatchRoute(t *testing.T) {
	for _, c := range []struct {
		pattern, path string
		want          bool
	}{
		{"/orgs/{org}/tokens", "/orgs/acme/tokens", true},
		{"/orgs/{org}/tokens", "/orgs/{org}/tokens", true},
		{"/orgs/{org}/tokens", "/orgs//tokens", false},
		{"/orgs/{org}/tokens", "/orgs/acme/tokens/t1", false},
		{"/device/code", "/device/code", true},
		{"/device/code", "/device/token", false},
	} {
		if got := MatchRoute(c.pattern, c.path); got != c.want {
			t.Errorf("MatchRoute(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}
