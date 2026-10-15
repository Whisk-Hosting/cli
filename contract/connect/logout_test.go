package connect

import "testing"

// sessionLogin is SAP Business One's shape: a token sent back as a cookie, and a logout.
func sessionLogin(l *LogoutStep) Connection {
	return Connection{URL: "https://b1.example.com:50000/b1s/v2", Operations: []Operation{{Name: "Read", Method: "GET", Path: "/Items"}},
		Auth: Auth{Headers: map[string]string{"Cookie": "B1SESSION={token}"}, Token: &TokenStep{URL: "https://b1.example.com:50000/b1s/v2/Login",
			JSON: map[string]string{"UserName": "{secret.USER}", "Password": "{secret.PASS}"}, Field: "SessionId", Lifetime: 1500,
			Logout: l}}}.WithDefaults()
}

func TestLogoutStep(t *testing.T) {
	c := sessionLogin(&LogoutStep{URL: "https://b1.example.com:50000/b1s/v2/Logout"})
	if ps, _ := Check(c); len(ps) != 0 {
		t.Fatalf("problems %v", ps)
	}
	if !Describe(c).Revokes {
		t.Error("the summary does not say revoking ends the session")
	}
	req, err := RenderLogout(c, env(nil, nil), "sess-1")
	if err != nil || req.Method != "POST" || req.URL != "https://b1.example.com:50000/b1s/v2/Logout" || req.Headers.Get("Cookie") != "B1SESSION=sess-1" || len(req.Body) != 0 {
		t.Fatalf("logout %+v %v", req, err)
	}
	own := sessionLogin(&LogoutStep{URL: "https://b1.example.com:50000/b1s/v2/Logout", Method: "delete",
		Headers: map[string]string{"Authorization": "Session {token}", "X-Client": "{secret.USER}"}})
	if ps, secrets := Check(own); len(ps) != 0 || len(secrets) != 2 {
		t.Fatalf("problems %v, secrets %v", ps, secrets)
	}
	req, err = RenderLogout(own, env(map[string]string{"USER": "link"}, nil), "sess-2")
	if err != nil || req.Method != "DELETE" || req.Headers.Get("Authorization") != "Session sess-2" || req.Headers.Get("X-Client") != "link" || req.Headers.Get("Cookie") != "" {
		t.Fatalf("logout with its own headers %+v %v", req, err)
	}
}

func TestLogoutStepCheck(t *testing.T) {
	cases := map[string]struct {
		c    Connection
		path string
	}{
		"another host":   {sessionLogin(&LogoutStep{URL: "https://evil.example.net/Logout"}), "/auth/token/logout/url"},
		"a PUT":          {sessionLogin(&LogoutStep{URL: "https://b1.example.com:50000/b1s/v2/Logout", Method: "PUT"}), "/auth/token/logout/method"},
		"a bad template": {sessionLogin(&LogoutStep{URL: "https://b1.example.com:50000/b1s/v2/Logout", Headers: map[string]string{"X": "{nope}"}}), "/auth/token/logout/headers/X"},
	}
	readsCall := sessionLogin(&LogoutStep{URL: "https://b1.example.com:50000/b1s/v2/Logout"})
	readsCall.Auth.Headers = map[string]string{"Cookie": "B1SESSION={token}", "X-Sig": "{hex(sha256(request.body))}"}
	cases["call headers that read the call"] = struct {
		c    Connection
		path string
	}{readsCall, "/auth/token/logout/headers"}
	queryOnly := sessionLogin(&LogoutStep{URL: "https://b1.example.com:50000/b1s/v2/Logout"})
	queryOnly.Auth.Headers, queryOnly.Auth.Query = nil, map[string]string{"session": "{token}"}
	cases["no headers to send"] = struct {
		c    Connection
		path string
	}{queryOnly, "/auth/token/logout/headers"}
	for name, tc := range cases {
		ps, _ := Check(tc.c)
		found := false
		for _, p := range ps {
			found = found || p.Path == tc.path
		}
		if !found {
			t.Errorf("%s: want a problem at %s, got %v", name, tc.path, ps)
		}
	}
}
