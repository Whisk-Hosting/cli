package whisk

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/whisk-run/cli/internal/output"
	werrors "github.com/whisk-run/contract/errors"
	"github.com/whisk-run/contract/naughty"
)

// gitRefOK is git check-ref-format's rule for a branch name, the part a refspec depends on.
func gitRefOK(b string) bool {
	if b == "" || b == "@" || strings.HasPrefix(b, "-") || strings.HasPrefix(b, "/") || strings.HasSuffix(b, "/") ||
		strings.HasSuffix(b, ".") || strings.HasSuffix(b, ".lock") || strings.Contains(b, "..") || strings.Contains(b, "@{") ||
		strings.Contains(b, "//") || strings.ContainsAny(b, " ~^:?*[\\") {
		return false
	}
	for _, seg := range strings.Split(b, "/") {
		if strings.HasPrefix(seg, ".") {
			return false
		}
	}
	return !strings.ContainsFunc(b, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

// A branch from --branch or from git becomes one refspec: never an option of git push, never a
// second refspec, never a ref git would refuse.
func TestNaughtyPlanPush(t *testing.T) {
	for _, s := range naughty.Strings() {
		for _, c := range [][3]string{{"", s, "main"}, {"preview", s, "main"}, {"production", s, "main"}, {"", "", s}, {s, "", "main"}} {
			plan, err := planPush(c[0], c[1], c[2])
			if err != nil {
				if e, ok := err.(*output.Error); !ok || (e.Code != "INVALID_REQUEST" && e.Code != "PRODUCTION_NEEDS_MAIN") {
					t.Errorf("planPush(%q) error %v", c, err)
				}
				continue
			}
			if plan.Src != "HEAD" && !gitRefOK(plan.Src) || !gitRefOK(plan.Dst) {
				t.Errorf("planPush(%q) = %+v, which git push would read as an option or refuse", c, plan)
			}
			if plan.Environment != "production" && plan.Environment != "preview:"+plan.Dst {
				t.Errorf("planPush(%q) environment %q", c, plan.Environment)
			}
		}
	}
}

// A positional word or flag value an MCP client sends reaches the command as exactly that word
// or value: none becomes a flag such as --yes or --api.
func TestNaughtyToolArgv(t *testing.T) {
	root := newRoot(&session{env: Env{Stdout: io.Discard, Stderr: io.Discard}})
	tools := mcpTools(root)
	if len(tools) < 30 {
		t.Fatalf("only %d tools", len(tools))
	}
	list := naughty.Strings()
	for _, tl := range tools {
		flag := ""
		for name, typ := range tl.flagTypes {
			if typ == "string" && (flag == "" || name < flag) {
				flag = name
			}
		}
		for i, s := range list {
			if i%7 != len(tl.Name)%7 {
				continue // a seventh of the strings per tool keeps this fast; every string meets many tools
			}
			args := map[string]any{"args": []any{s, "--yes", "--api=https://evil.example", "-h"}}
			if flag != "" {
				args[flag] = s
			}
			argv, err := toolArgv(tl, args)
			if err != nil {
				t.Errorf("%s with %q: %v", tl.Name, s, err)
				continue
			}
			cmd, rest, err := root.Find(argv)
			if err != nil {
				t.Errorf("%s with %q: %v", tl.Name, s, err)
				continue
			}
			if err := cmd.ParseFlags(rest); err != nil {
				t.Errorf("%s with %q: argv %q does not parse: %v", tl.Name, s, argv, err)
				continue
			}
			got := cmd.Flags().Args()
			if len(got) != 4 || got[0] != s || got[1] != "--yes" || got[2] != "--api=https://evil.example" || got[3] != "-h" {
				t.Errorf("%s with %q: positional words arrived as %q", tl.Name, s, got)
			}
			if flag != "" {
				if v, _ := cmd.Flags().GetString(flag); v != s {
					t.Errorf("%s with %q: --%s arrived as %q", tl.Name, s, flag, v)
				}
			}
			if f := cmd.Flags().Lookup("yes"); f != nil && f.Changed {
				t.Errorf("%s with %q: a positional word set --yes", tl.Name, s)
			}
			if f := cmd.Flags().Lookup("help"); f != nil && f.Changed {
				t.Errorf("%s with %q: a positional word set --help", tl.Name, s)
			}
		}
	}
}

// --since is a duration ago or an instant; a naughty one is refused or is a time not after now.
func TestNaughtyParseSince(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	for _, s := range append(naughty.Strings(), "-5m", "-7d", "9223372036854775807d", "106752d", "-9223372036854775808d") {
		got, err := parseSince(s, now)
		if err != nil {
			continue
		}
		if _, rfc := time.Parse(time.RFC3339, strings.TrimSpace(s)); rfc != nil && got.After(now) {
			t.Errorf("parseSince(%q) = %s, after now", s, got)
		}
	}
}

// normalizeRepo answers owner/name with nothing but those two words.
func TestNaughtyNormalizeRepo(t *testing.T) {
	for _, s := range naughty.Strings() {
		for _, in := range []string{s, "acme/" + s, s + "/repo", "https://github.com/" + s + "/x.git", "git@github.com:acme/" + s} {
			repo, ok := normalizeRepo(in)
			if !ok {
				continue
			}
			owner, name, _ := strings.Cut(repo, "/")
			if owner == "" || name == "" || strings.Contains(name, "/") || strings.ContainsFunc(repo, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
				t.Errorf("normalizeRepo(%q) = %q", in, repo)
			}
		}
	}
}

// git asks whisk git-credential with key=value lines; whatever it sends, the answer is a
// username and password for https only, and a naughty line cannot add a field.
func TestNaughtyCredentialHelper(t *testing.T) {
	for _, s := range naughty.Strings() {
		req := parseCredentialRequest(strings.NewReader("protocol=" + s + "\nhost=" + s + "\n" + s + "\n\nprotocol=http\n"))
		ans := credentialAnswer(req, "tok")
		if ans != "" && (req["protocol"] != "" && req["protocol"] != "https") {
			t.Errorf("answered protocol %q", req["protocol"])
		}
		if ans != "" && ans != "username=whisk\npassword=tok\n" {
			t.Errorf("answer for %q: %q", s, ans)
		}
	}
}

// Whole commands with naughty names: the CLI answers with an exit code from CLI.md §3 and, in
// JSON mode, one object whose error code is catalogued, never a crash.
func TestNaughtyCommands(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		_, _ = w.Write([]byte(`{"error":{"code":"NOT_FOUND","message":"nothing here","fix":"check the name","docs":"https://skill.whisk.run/errors/NOT_FOUND"}}`))
	}))
	defer srv.Close()
	env := map[string]string{"WHISK_TOKEN": "whsk_test_token", "WHISK_API": srv.URL}
	cliCodes := regexp.MustCompile(`^[A-Z][A-Z0-9_]+$`)
	for i, s := range naughty.Strings() {
		if len(s) > 4096 {
			continue
		}
		commands := [][]string{
			{"apps", "info", "--org", s, "--app", s, "--json"},
			{"apps", "create", s, "--org", "acme", "--json"},
			{"runs", "replay", s, "--org", "acme", "--app", "crm", "--json"},
			{"deploys", "info", s, "--org", "acme", "--app", "crm", "--json"},
			{"errors", s, "--json"},
			{"logs", "--org", "acme", "--app", "crm", "--since", s, "--json"},
		}
		for j, args := range commands {
			if (i+j)%3 != 0 {
				continue // a third per string keeps the package fast; each command still meets every kind of string
			}
			r := runCLI(t, t.TempDir(), env, &memStore{}, args...)
			if r.Code < 0 || r.Code > 6 {
				t.Errorf("%q: exit %d", args, r.Code)
			}
			if r.Code == 0 || (s == "--" && r.JSON == nil) {
				continue // after "--" even --json is a positional word, so the error is for a human
			}
			e, _ := r.JSON["error"].(map[string]any)
			code, _ := e["code"].(string)
			if _, known := werrors.Lookup(code); !known && !cliCodes.MatchString(code) {
				t.Errorf("%q: error %v, stdout %q", args, e, r.Stdout)
			}
		}
	}
}
