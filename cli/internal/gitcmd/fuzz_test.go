package gitcmd

import (
	"net/url"
	"strings"
	"testing"

	"github.com/whisk-run/contract/naughty"
)

// Fuzz targets for what git and the git server print (docs/HARNESS.md §8.3).

// FuzzParseSideband: whatever the server writes on the sideband, the deploy id and error code
// have their documented shapes, lines are whole and trimmed, and the classifiers answer one of
// their kinds.
func FuzzParseSideband(f *testing.F) {
	f.Add("remote: whisk: deploy 01J8Z3WQ9XN4M6P8R0T2V4X6Y8\n")
	f.Add("remote: whisk: INVALID_MANIFEST: name is required\nremote: whisk: fix: add a name\nremote: whisk: docs: https://x\n")
	f.Add(`remote: whisk: {"error":{"code":"QUOTA","message":"m","fix":"f"}}`)
	for _, s := range naughty.Strings() {
		f.Add(s)
		f.Add("\x1b[31mremote: whisk: " + s + "\x1b[0m\r")
		f.Add("whisk: X: " + s + "\nwhisk: fix: " + s)
	}
	kinds := map[string]bool{"tls": true, "network": true, "auth": true, "rejected": true, "other": true}
	f.Fuzz(func(t *testing.T, stderr string) {
		sb := ParseSideband(stderr)
		if sb.DeployID != "" && !reNaughtyDeploy.MatchString(sb.DeployID) {
			t.Fatalf("deploy id %q", sb.DeployID)
		}
		if sb.Error != nil && sb.Error.Code == "" {
			t.Fatalf("an error without a code")
		}
		for _, l := range sb.Lines {
			if l == "" || l != strings.TrimSpace(l) || strings.ContainsAny(l, "\r\n") {
				t.Fatalf("line %q", l)
			}
		}
		if sb.DeployID != "" && !contains(sb.Lines, "deploy "+sb.DeployID) {
			t.Fatalf("deploy id %q is not on a line", sb.DeployID)
		}
		// Parsing the lines it kept, each written as whisk: <line>, finds the same deploy.
		again := ParseSideband("whisk: " + strings.Join(sb.Lines, "\nwhisk: "))
		if again.DeployID != sb.DeployID || len(again.Lines) != len(sb.Lines) {
			t.Fatalf("re-parsing the lines gave %+v, want %+v", again, sb)
		}
		for _, o := range []PushOutcome{PushUnknown, PushRejected} {
			if k := Failure(stderr, o); !kinds[k] {
				t.Fatalf("Failure = %q", k)
			}
		}
		lines := LastLines(stderr, 3)
		if len(lines) > 3 {
			t.Fatalf("LastLines gave %d lines", len(lines))
		}
		for _, l := range lines {
			if strings.TrimSpace(l) == "" || strings.Contains(l, "\n") || !strings.Contains(stderr, l) {
				t.Fatalf("LastLines gave %q", l)
			}
		}
	})
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// FuzzParsePush reads fuzzed git push --porcelain and git status output: the outcome is one of
// the four, a summary comes only with an outcome, and status paths are trimmed and non-empty
// only when the line had room for one.
func FuzzParsePush(f *testing.F) {
	f.Add("To https://git.whisk.run/acme/crm.git\n \trefs/heads/main:refs/heads/main\tabc..def\nDone\n")
	f.Add("!\trefs/heads/main:refs/heads/main\t[remote rejected] (pre-receive hook declined)\n")
	f.Add("Everything up-to-date\n")
	f.Add(" M a.go\n?? b c\nR  x -> y\n")
	for _, s := range naughty.Strings() {
		f.Add(s)
		f.Add("=\t" + s + "\t" + s)
	}
	f.Fuzz(func(t *testing.T, stdout string) {
		outcome, summary := ParsePush(stdout)
		if outcome < PushUnknown || outcome > PushRejected {
			t.Fatalf("outcome %d", outcome)
		}
		if outcome == PushUnknown && summary != "" {
			t.Fatalf("summary %q without an outcome", summary)
		}
		if summary != strings.TrimSpace(summary) {
			t.Fatalf("summary %q is not trimmed", summary)
		}
		for _, p := range ParseStatus(stdout) {
			if p != strings.TrimSpace(p) || strings.ContainsAny(p, "\n") {
				t.Fatalf("status path %q", p)
			}
		}
	})
}

// FuzzCredential: the token only ever reaches the environment, and the helper is scoped to an
// https (or loopback http) origin with no '=' or space, which would change git's -c key.
func FuzzCredential(f *testing.F) {
	f.Add("tok", "https://git.whisk.run/acme/crm.git")
	f.Add("tok", "http://127.0.0.1:8080/x")
	for _, s := range naughty.Strings() {
		f.Add(s, s)
		f.Add(s, "https://"+s+"/x")
	}
	f.Fuzz(func(t *testing.T, token, gitURL string) {
		env, args, err := Credential(token, gitURL)
		if err != nil {
			return
		}
		origin, err := CredentialOrigin(gitURL)
		if err != nil {
			t.Fatalf("Credential accepted %q that CredentialOrigin refuses: %v", gitURL, err)
		}
		u, perr := url.Parse(origin)
		if perr != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.Path != "" || u.User != nil || strings.ContainsAny(origin, "= \t\n\r") {
			t.Fatalf("helper scoped to %q", origin)
		}
		want := []string{"-c", "credential.helper=", "-c", "credential." + origin + `.helper=!f() { echo username=whisk; echo "password=$` + TokenVar + `"; }; f`}
		if strings.Join(args, "\x00") != strings.Join(want, "\x00") {
			t.Fatalf("args %q", args)
		}
		if len(env) != 2 || env[0] != TokenVar+"="+token {
			t.Fatalf("env %q", env)
		}
	})
}
