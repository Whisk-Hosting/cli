package gitcmd

import (
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/whisk-run/contract/naughty"
)

// A naughty token or git address never becomes an argument of its own: the arguments are
// exactly the two -c settings, so the token goes only into the environment, and the helper is
// scoped to an https (or loopback http) origin that parses back to itself and holds no '=',
// which would end git's -c key early.
func TestNaughtyCredential(t *testing.T) {
	for _, s := range naughty.Strings() {
		for _, gitURL := range []string{s, "https://" + s, "https://git.whisk.run/" + s, "http://" + s + "/x", "https://" + s + "@git.whisk.run/x"} {
			env, args, err := Credential(s, gitURL)
			if err != nil {
				continue
			}
			origin, _ := CredentialOrigin(gitURL)
			u, perr := url.Parse(origin)
			if perr != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.Path != "" || u.User != nil || strings.ContainsAny(origin, "= \t\n") {
				t.Errorf("Credential(%q) scoped the helper to %q", gitURL, origin)
			}
			if len(args) != 4 || args[0] != "-c" || args[2] != "-c" || args[1] != "credential.helper=" || args[3] != "credential."+origin+`.helper=!f() { echo username=whisk; echo "password=$`+TokenVar+`"; }; f` {
				t.Errorf("Credential(%q) args %q", gitURL, args)
			}
			if len(env) != 2 || env[0] != TokenVar+"="+s || env[1] != "GIT_TERMINAL_PROMPT=0" {
				t.Errorf("Credential(%q) env %q", gitURL, env)
			}
		}
	}
}

var (
	reNaughtyDeploy = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	reNaughtyCode   = regexp.MustCompile(`^[A-Z][A-Z0-9_]+$`)
)

// git's output is text from a server; whatever it says, the parsers answer with values of the
// shapes they document.
func TestNaughtyParsers(t *testing.T) {
	kinds := map[string]bool{"tls": true, "network": true, "auth": true, "rejected": true, "other": true}
	for _, s := range naughty.Strings() {
		for _, stderr := range []string{s, "remote: whisk: " + s, "remote: whisk: deploy " + s, "whisk: {\"error\":{\"code\":\"" + s + "\"}}", "whisk: INVALID_REQUEST: " + s + "\nwhisk: fix: " + s, "\x1b[31mremote: whisk: " + s + "\x1b[0m\r"} {
			sb := ParseSideband(stderr)
			if sb.DeployID != "" && !reNaughtyDeploy.MatchString(sb.DeployID) {
				t.Errorf("ParseSideband(%q) deploy id %q", stderr, sb.DeployID)
			}
			if sb.Error != nil && sb.Error.Code == "" {
				t.Errorf("ParseSideband(%q) gave an error without a code", stderr)
			}
			if sb.Error != nil && !strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(stderr), "\x1b[31m")), "whisk: {") && !reNaughtyCode.MatchString(sb.Error.Code) {
				t.Errorf("ParseSideband(%q) code %q", stderr, sb.Error.Code)
			}
			for _, l := range sb.Lines {
				if l == "" || strings.ContainsAny(l, "\r\n") {
					t.Errorf("ParseSideband(%q) line %q", stderr, l)
				}
			}
			if k := Failure(stderr, PushUnknown); !kinds[k] {
				t.Errorf("Failure(%q) = %q", stderr, k)
			}
			for _, l := range LastLines(stderr, 3) {
				if strings.TrimSpace(l) == "" || strings.Contains(l, "\n") {
					t.Errorf("LastLines(%q) gave %q", stderr, l)
				}
			}
		}
		for _, stdout := range []string{s, "=\t" + s + "\t" + s, "!\t" + s, s + "\t" + s} {
			outcome, _ := ParsePush(stdout)
			if outcome < PushUnknown || outcome > PushRejected {
				t.Errorf("ParsePush(%q) = %d", stdout, outcome)
			}
		}
		for _, p := range ParseStatus(" M " + s + "\n?? " + s) {
			if p != strings.TrimSpace(p) {
				t.Errorf("ParseStatus(%q) gave %q", s, p)
			}
		}
	}
}
