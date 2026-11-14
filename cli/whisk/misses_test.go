package whisk

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"github.com/whisk-run/contract/apitypes"
)

var errStopHere = stringError("stopped before the command ran")

type stringError string

func (e stringError) Error() string { return string(e) }

// missFor runs args through the real command tree, as Run does, and answers the report.
func missFor(t *testing.T, args ...string) (apitypes.CLIMiss, bool) {
	t.Helper()
	s := &session{ctx: context.Background(), env: Env{Getenv: func(string) string { return "" }, Stdout: io.Discard, Stderr: io.Discard, Stdin: strings.NewReader("")}}
	root := newRoot(s)
	markUsage(root)
	root.SetArgs(args)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	// prepare must not run: these all fail before it, and anything that got that far is
	// a command's own error.
	root.PersistentPreRunE = func(*cobra.Command, []string) error { return errStopHere }
	var helped []string
	help := root.HelpFunc()
	root.SetHelpFunc(func(c *cobra.Command, a []string) { helped = a; help(c, a) })
	err := root.ExecuteContext(context.Background())
	if err == nil {
		return helpMiss(root, helped)
	}
	return missOf(err, root, args)
}

func TestMissOf(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want apitypes.CLIMiss
	}{
		{"unknown top-level command", []string{"logs-tail", "--json"},
			apitypes.CLIMiss{Kind: apitypes.MissUnknownCommand, Command: "", Word: "logs-tail", Flags: []string{"--json"}}},
		{"unknown subcommand", []string{"deploys", "lst"},
			apitypes.CLIMiss{Kind: apitypes.MissUnknownCommand, Command: "deploys", Word: "lst", Flags: []string{}}},
		{"a word that is not shaped like a command is not sent", []string{"sk_live_abc123", "x"},
			apitypes.CLIMiss{Kind: apitypes.MissUnknownCommand, Command: "", Word: "", Flags: []string{}, Args: 1}},
		{"unknown long flag, its value never read", []string{"deploy", "--branchx=secret-value", "--json"},
			apitypes.CLIMiss{Kind: apitypes.MissUnknownFlag, Command: "deploy", Word: "--branchx", Flags: []string{"--branchx", "--json"}}},
		{"unknown shorthand flag", []string{"status", "-Z"},
			apitypes.CLIMiss{Kind: apitypes.MissUnknownFlag, Command: "status", Word: "-z", Flags: []string{"-z"}}},
		{"unknown word after a command that groups others", []string{"secrets", "ls", "--json"},
			apitypes.CLIMiss{Kind: apitypes.MissUnknownCommand, Command: "secrets", Word: "ls", Flags: []string{"--json"}}},
		{"flag without its value", []string{"status", "--org"},
			apitypes.CLIMiss{Kind: apitypes.MissFlagValue, Command: "status", Word: "--org", Flags: []string{"--org"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := missFor(t, c.args...)
			if !ok {
				t.Fatalf("not a miss")
			}
			if got.Kind != c.want.Kind || got.Command != c.want.Command || got.Word != c.want.Word || !slices.Equal(got.Flags, c.want.Flags) || got.Args != c.want.Args {
				t.Errorf("got %+v, want %+v", got, c.want)
			}
			if b, _ := json.Marshal(got); bytes.Contains(b, []byte("secret-value")) || bytes.Contains(b, []byte("sk_live")) {
				t.Errorf("a value reached the report: %s", b)
			}
		})
	}
}

// Asking a grouping command for its help, or naming none, is not a miss.
func TestHelpIsNotAMiss(t *testing.T) {
	root := newRoot(&session{})
	for _, args := range [][]string{nil, {"secrets"}, {"secrets", "--help"}, {"secrets", "-h"}, {"secrets", "--org", "acme"}} {
		if m, ok := helpMiss(root, args); ok {
			t.Errorf("%v: reported %+v", args, m)
		}
	}
}

// A command's own error is not a miss.
func TestCommandErrorsAreNotMisses(t *testing.T) {
	if _, ok := missOf(errStopHere, newRoot(&session{}), []string{"status"}); ok {
		t.Error("a command's error was reported")
	}
}

// Arguments a command's Args refuses are a miss of kind args, counted and never sent.
func TestWrongArgumentsAreAMiss(t *testing.T) {
	s := &session{ctx: context.Background()}
	root := newRoot(s)
	markUsage(root)
	status, _, _ := root.Find([]string{"status"})
	m, ok := missOf(&usageFault{cmd: status, kind: apitypes.MissArgs, err: errStopHere}, root, []string{"status", "my-app", "--org", "acme", "other"})
	if !ok || m.Kind != apitypes.MissArgs || m.Command != "status" || m.Word != "" || m.Args != 2 || !slices.Equal(m.Flags, []string{"--org"}) {
		t.Errorf("got %+v %v", m, ok)
	}
}

func TestMissMessages(t *testing.T) {
	cases := []struct {
		msg  string
		kind apitypes.CLIMissKind
		word string
	}{
		{`required flag(s) "app", "org" not set`, apitypes.MissMissingFlag, "--app --org"},
		{`if any flags in the group [at data] are set none of the others can be; [at data] were all set`, apitypes.MissFlagGroup, "--at --data"},
		{`unknown command "Logs" for "whisk"`, apitypes.MissUnknownCommand, "logs"},
		{`unknown command "a/b" for "whisk"`, apitypes.MissUnknownCommand, ""},
		{`something a command said`, "", ""},
	}
	for _, c := range cases {
		kind, word, _ := classify(stringError(c.msg))
		if kind != c.kind || word != c.word {
			t.Errorf("%q: got %q %q, want %q %q", c.msg, kind, word, c.kind, c.word)
		}
	}
}

func TestAgentOf(t *testing.T) {
	cases := []struct {
		env  map[string]string
		want string
	}{
		{nil, ""},
		{map[string]string{"CLAUDECODE": "1"}, "claude-code"},
		{map[string]string{"CURSOR_AGENT": "1"}, "cursor"},
		{map[string]string{"CLAUDECODE": "0"}, ""},
		{map[string]string{"WHISK_AGENT": "My Agent v2!", "CLAUDECODE": "1"}, "my-agent-v2"},
		{map[string]string{"WHISK_AGENT": strings.Repeat("a", 50)}, strings.Repeat("a", 32)},
	}
	for _, c := range cases {
		if got := agentOf(func(k string) string { return c.env[k] }); got != c.want {
			t.Errorf("%v: got %q, want %q", c.env, got, c.want)
		}
	}
}

func TestReportsOff(t *testing.T) {
	cases := []struct {
		env  map[string]string
		want bool
	}{
		{nil, false},
		{map[string]string{"WHISK_NO_TELEMETRY": "1"}, true},
		{map[string]string{"DO_NOT_TRACK": "1"}, true},
		{map[string]string{"DO_NOT_TRACK": "true"}, true},
		{map[string]string{"DO_NOT_TRACK": "0"}, false},
		{map[string]string{"WHISK_NO_TELEMETRY": "false"}, false},
	}
	for _, c := range cases {
		if got := reportsOff(func(k string) string { return c.env[k] }); got != c.want {
			t.Errorf("%v: got %v, want %v", c.env, got, c.want)
		}
	}
}

// The binary sends one report per failed command line to the API it is pointed at, with no
// value from it, and sends none when turned off or when driven as a library.
func TestRunReportsAMiss(t *testing.T) {
	var mu sync.Mutex
	var got []apitypes.CLIMiss
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/cli/misses" {
			http.NotFound(w, r)
			return
		}
		var m apitypes.CLIMiss
		_ = json.NewDecoder(r.Body).Decode(&m)
		mu.Lock()
		got = append(got, m)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"kept":true}`))
	}))
	defer srv.Close()
	run := func(report bool, env map[string]string, args ...string) int {
		vars := map[string]string{"WHISK_API": srv.URL, "CLAUDECODE": "1"}
		for k, v := range env {
			vars[k] = v
		}
		e := Env{Stdout: io.Discard, Stderr: io.Discard, ConfigDir: t.TempDir(), Store: &memStore{m: map[string]Credential{}}, Dir: t.TempDir(),
			Getenv: func(k string) string { return vars[k] }, ReportMisses: report}
		return Run(context.Background(), args, e)
	}
	if code := run(true, nil, "deploys", "tail", "--json"); code != 0 {
		t.Fatalf("exit %d: a grouping command's help still succeeds", code)
	}
	if code := run(true, nil, "status", "--follow=yes"); code != 1 {
		t.Fatalf("exit %d", code)
	}
	run(true, map[string]string{"WHISK_NO_TELEMETRY": "1"}, "nosuch")
	run(true, map[string]string{"DO_NOT_TRACK": "1"}, "nosuch")
	run(false, nil, "nosuch")
	run(true, nil, "version")
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("%d reports, want 2: %+v", len(got), got)
	}
	m := got[0]
	if m.Kind != apitypes.MissUnknownCommand || m.Command != "deploys" || m.Word != "tail" || m.Agent != "claude-code" || m.Version != Version || m.OS == "" || m.Arch == "" {
		t.Errorf("report %+v", m)
	}
	m = got[1]
	if m.Kind != apitypes.MissUnknownFlag || m.Command != "status" || m.Word != "--follow" {
		t.Errorf("report %+v", m)
	}
	if b, _ := json.Marshal(got); bytes.Contains(b, []byte("yes")) {
		t.Errorf("a value reached the report: %s", b)
	}
}
