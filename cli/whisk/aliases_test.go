package whisk

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The words agents reached for reach the commands that do the work (CLI.md §7): get, info and
// show name one another's commands, whisk restart is whisk apps restart, whisk usage is whisk
// billing, whisk cache is whisk cdn, whisk storage is whisk uploads, whisk products is whisk
// managed, -q is --quiet, and whisk call takes --body for --data and saves with -o.
func TestWordsAgentsReachFor(t *testing.T) {
	root := newRoot(&session{ctx: context.Background()})
	for typed, want := range map[string]string{
		"apps get":     "whisk apps info",
		"apps show":    "whisk apps info",
		"uploads info": "whisk uploads show",
		"uploads get":  "whisk uploads show",
		"deploys get":  "whisk deploys info",
		"restart":      "whisk restart",
		"usage":        "whisk billing",
		"cache":        "whisk cdn",
		"cache purge":  "whisk cdn purge",
		"storage":      "whisk uploads",
		"products":     "whisk managed",
		"env":          "whisk envs",
		"audit":        "whisk audit",
	} {
		c, _, err := root.Find(strings.Fields(typed))
		if err != nil || c.CommandPath() != want {
			t.Errorf("whisk %s: reached %v (%v), want %s", typed, c.CommandPath(), err, want)
		}
	}
	if f := root.PersistentFlags().ShorthandLookup("q"); f == nil || f.Name != "quiet" {
		t.Errorf("-q is %v, want --quiet", f)
	}
	call, _, _ := root.Find([]string{"call"})
	if err := call.ParseFlags([]string{"--body", `{"a":1}`, "-o", "out.json"}); err != nil {
		t.Fatal(err)
	}
	if v, _ := call.Flags().GetString("data"); v != `{"a":1}` {
		t.Errorf("--body set --data to %q", v)
	}
	if v, _ := call.Flags().GetString("output"); v != "out.json" {
		t.Errorf("-o set --output to %q", v)
	}
}

// A lookup name another command already has is left to it.
func TestLookupAliasesNeverShadow(t *testing.T) {
	root := newRoot(&session{ctx: context.Background()})
	seen := map[string]string{}
	for _, g := range root.Commands() {
		for _, sub := range g.Commands() {
			for _, n := range append([]string{sub.Name()}, sub.Aliases...) {
				key := g.Name() + " " + n
				if prev, ok := seen[key]; ok {
					t.Errorf("whisk %s names both %s and %s", key, prev, sub.Name())
				}
				seen[key] = sub.Name()
			}
		}
	}
}

// whisk call -o saves the body for this user alone and refuses to pass a cut answer off as whole.
func TestSaveCallAnswer(t *testing.T) {
	dir := t.TempDir()
	s := &session{env: Env{Dir: dir, Stdout: &strings.Builder{}, Stderr: &strings.Builder{}}}
	s.printer.Out, s.printer.Err = s.env.Stdout, s.env.Stderr
	if err := s.saveCallAnswer("page.json", "acme", "crm", "production", "GET", "https://crm--acme.whisk.page/x", callAnswer{Status: 200, Body: []byte(`{"ok":true}`)}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "page.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("saved %v %v", info, err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "page.json")); string(b) != `{"ok":true}` {
		t.Errorf("saved %q", b)
	}
	if !strings.Contains(s.env.Stdout.(*strings.Builder).String(), "Saved 11 bytes to page.json.") {
		t.Errorf("stdout %q", s.env.Stdout)
	}
	err = s.saveCallAnswer("big.bin", "acme", "crm", "production", "GET", "https://crm--acme.whisk.page/x", callAnswer{Status: 200, Body: []byte("cut"), Truncated: true})
	if err == nil || !strings.Contains(err.Error(), "holds only its first") {
		t.Errorf("a cut answer: %v", err)
	}
}
