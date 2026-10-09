package gitcmd

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseSideband(t *testing.T) {
	stderr := "remote: whisk: scanning 3 commits\x1b[K\nremote: whisk: deploy 01JDEPLOY\nTo https://git.whisk.run/acme/crm.git\n"
	sb := ParseSideband(stderr)
	if sb.DeployID != "01JDEPLOY" || sb.Error != nil || len(sb.Lines) != 2 || sb.Lines[0] != "scanning 3 commits" {
		t.Fatalf("sideband: %+v", sb)
	}

	rejected := "remote: whisk: SECRET_IN_COMMIT: A value that looks like a Stripe secret key is in src/config.ts line 12.\r\nremote: whisk: fix: Remove the value and declare STRIPE_SECRET_KEY.\nremote: error: hook declined\n ! [remote rejected] main -> main (pre-receive hook declined)\n"
	sb = ParseSideband(rejected)
	if sb.Error == nil || sb.Error.Code != "SECRET_IN_COMMIT" || !strings.HasPrefix(sb.Error.Message, "A value") || sb.Error.Fix != "Remove the value and declare STRIPE_SECRET_KEY." || sb.Error.Docs == "" {
		t.Fatalf("rejection: %+v", sb.Error)
	}

	asJSON := `remote: whisk: {"error":{"code":"MANIFEST_INVALID","message":"whisk.yaml has 1 problem.","fix":"Fix it.","docs":"https://skill.whisk.run/errors/MANIFEST_INVALID","details":{"problems":[{"path":"/name","message":"required"}]}}}` + "\n"
	sb = ParseSideband(asJSON)
	if sb.Error == nil || sb.Error.Code != "MANIFEST_INVALID" || sb.Error.Fix != "Fix it." || sb.Error.Details["problems"] == nil {
		t.Fatalf("json rejection: %+v", sb.Error)
	}
}

func TestParsePush(t *testing.T) {
	cases := map[string]PushOutcome{
		"To https://x\n*\tHEAD:refs/heads/main\t[new branch]\nDone\n":                                  PushUpdated,
		"To https://x\n \tmain:refs/heads/main\ta1b2c3d..e4f5a6b\nDone\n":                              PushUpdated,
		"To https://x\n=\tmain:refs/heads/main\t[up to date]\nDone\n":                                  PushUpToDate,
		"To https://x\n!\tmain:refs/heads/main\t[remote rejected] (pre-receive hook declined)\nDone\n": PushRejected,
		"Everything up-to-date\n": PushUpToDate,
		"":                        PushUnknown,
	}
	for in, want := range cases {
		if got, _ := ParsePush(in); got != want {
			t.Errorf("%q: got %v want %v", in, got, want)
		}
	}
	if _, summary := ParsePush("!\tmain:refs/heads/main\t[remote rejected] (pre-receive hook declined)\n"); summary != "[remote rejected] (pre-receive hook declined)" {
		t.Errorf("summary %q", summary)
	}
}

func TestParseStatusAndLastLines(t *testing.T) {
	got := ParseStatus(" M src/app.ts\n?? whisk.yaml\nA  new file.txt\n")
	if !reflect.DeepEqual(got, []string{"src/app.ts", "whisk.yaml", "new file.txt"}) {
		t.Fatalf("status: %v", got)
	}
	if got := ParseStatus(""); len(got) != 0 {
		t.Fatalf("clean: %v", got)
	}
	lines := LastLines("a\nb\n\nc\r\nd\n", 2)
	if !reflect.DeepEqual(lines, []string{"c", "d"}) {
		t.Fatalf("last lines: %v", lines)
	}
}

func TestCredentialNeverCarriesTheToken(t *testing.T) {
	env, args, err := Credential("whsk_agent_secret", "https://git.whisk.run/01J/a1.git")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(args, " ") != `-c credential.helper= -c credential.https://git.whisk.run.helper=!f() { echo username=whisk; echo "password=$WHISK_GIT_TOKEN"; }; f` {
		t.Fatalf("args: %q", args)
	}
	for _, a := range args {
		if strings.Contains(a, "whsk_agent_secret") {
			t.Fatalf("token on the command line: %q", a)
		}
	}
	if env[0] != "WHISK_GIT_TOKEN=whsk_agent_secret" || env[1] != "GIT_TERMINAL_PROMPT=0" {
		t.Fatalf("env: %q", env)
	}
}

// The token is only ever offered to the platform's git host, over https.
func TestCredentialOrigin(t *testing.T) {
	cases := []struct {
		url, want string
		ok        bool
	}{
		{"https://git.whisk.run/01J/a1.git", "https://git.whisk.run", true},
		{"https://git.whisk.test:8443/a.git", "https://git.whisk.test:8443", true},
		{"http://127.0.0.1:3000/a.git", "http://127.0.0.1:3000", true},
		{"http://localhost/a.git", "http://localhost", true},
		{"http://git.whisk.run/a.git", "", false},
		// http only to the machine itself: never to another address, private or not.
		{"http://[::1]:3000/a.git", "http://[::1]:3000", true},
		{"http://10.0.0.1/a.git", "", false},
		{"http://192.168.1.10:3000/a.git", "", false},
		{"https://user:pw@git.whisk.run/a.git", "", false},
		{"ssh://git@git.whisk.run/a.git", "", false},
		{"ext::sh -c id", "", false},
		{"/tmp/repo.git", "", false},
	}
	for _, c := range cases {
		got, err := CredentialOrigin(c.url)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("CredentialOrigin(%q) = %q, %v; want %q ok=%v", c.url, got, err, c.want, c.ok)
		}
	}
}

func TestFailure(t *testing.T) {
	for _, stderr := range []string{
		"fatal: unable to access 'https://git.whisk.run/01J/a1.git/': SSL certificate problem: unable to get local issuer certificate",
		"fatal: unable to access 'https://git.whisk.run/': server certificate verification failed. CAfile: none CRLfile: none",
		"fatal: unable to access 'https://git.whisk.run/': SSL certificate problem: self-signed certificate in certificate chain",
	} {
		if k := Failure(stderr, PushUnknown); k != "tls" {
			t.Errorf("tls: %s for %q", k, stderr)
		}
	}
	if k := Failure("fatal: unable to access 'https://x/': Could not resolve host: x", PushUnknown); k != "network" {
		t.Errorf("network: %s", k)
	}
	if k := Failure("fatal: Authentication failed for 'https://x/'", PushUnknown); k != "auth" {
		t.Errorf("auth: %s", k)
	}
	if k := Failure("error: failed to push some refs", PushRejected); k != "rejected" {
		t.Errorf("rejected: %s", k)
	}
	for _, stderr := range []string{
		"error: RPC failed; curl 56 Recv failure: Connection was reset\nsend-pack: unexpected disconnect while reading sideband packet\nfatal: the remote end hung up unexpectedly",
		"error: RPC failed; HTTP 502 curl 22 The requested URL returned error: 502\nsend-pack: unexpected disconnect while reading sideband packet\nfatal: the remote end hung up unexpectedly",
		"fatal: the remote end hung up unexpectedly",
		"fatal: early EOF",
	} {
		if k := Failure(stderr, PushUnknown); k != "network" {
			t.Errorf("dropped connection: %s for %q", k, stderr)
		}
	}
	if k := Failure("fatal: Authentication failed for 'https://x/'\nfatal: the remote end hung up unexpectedly", PushUnknown); k != "auth" {
		t.Errorf("a refused credential that hangs up is still auth: %s", k)
	}
	if k := Failure("something else", PushUnknown); k != "other" {
		t.Errorf("other: %s", k)
	}
}
