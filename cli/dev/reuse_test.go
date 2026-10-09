package dev

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whisk-run/contract/manifest"
)

// A compose file on disk is run again only when it is exactly the file whisk dev writes; a
// file that came with the repository carrying a matching inputs line is regenerated.
func TestReusableOnlyTheCLIsOwnFile(t *testing.T) {
	m, err := manifest.Parse([]byte("whisk: 1\nname: my-app\nkv: true\n"))
	if err != nil {
		t.Fatal(err)
	}
	ports := usedPorts(m, Ports{Postgres: 50001, PgBouncer: 50002, Inngest: 50003, Valkey: 50004, Storage: 50005, Console: 50006})
	if ports.Storage != 0 || ports.Console != 0 || ports.Valkey == 0 {
		t.Fatalf("usedPorts kept %+v", ports)
	}
	own := BuildPlan(m, ports, 3002, false).Compose
	forged := own + "  evil:\n    image: alpine\n    volumes: [\"/:/host\"]\n"
	mounted := strings.Replace(own, `volumes: ["postgres:/var/lib/postgresql/data"]`, `volumes: ["/home:/var/lib/postgresql/data"]`, 1)
	cases := []struct {
		name    string
		src     string
		hostNet bool
		reuse   bool
	}{
		{"the CLI's own file", own, false, true},
		{"an extra service with the same inputs line", forged, false, false},
		{"a host mount with the same inputs line", mounted, false, false},
		{"written for the other network mode", own, true, false},
		{"no ports", "# inputs: " + Hash(m, ports, 3002) + "\nservices: {}\n", false, false},
		{"empty", "", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(c.src, "# inputs: "+Hash(m, ports, 3002)) && c.src != "" {
				t.Fatalf("case should carry the matching inputs line")
			}
			got, ok := reusable(c.src, m, 3002, c.hostNet)
			if ok != c.reuse {
				t.Fatalf("reusable = %v, want %v", ok, c.reuse)
			}
			if ok && got != ports {
				t.Errorf("ports %+v, want %+v", got, ports)
			}
		})
	}
	// A different internal port or manifest makes the file stale too.
	if _, ok := reusable(own, m, 3003, false); ok {
		t.Error("reused a file written for another internal port")
	}
	plain, _ := manifest.Parse([]byte("whisk: 1\nname: my-app\n"))
	if _, ok := reusable(own, plain, 3002, false); ok {
		t.Error("reused a file written for another manifest")
	}
}

// Down names every service and volume whisk dev can create for the project, whatever the
// manifest declares now.
func TestDownPlan(t *testing.T) {
	cases := []string{"whisk: 1\nname: plain\n", "whisk: 1\nname: plain\nkv: true\nstorage: true\n"}
	for _, src := range cases {
		m, err := manifest.Parse([]byte(src))
		if err != nil {
			t.Fatal(err)
		}
		p := DownPlan(m)
		if p.Project != "whisk-plain" {
			t.Errorf("project %s", p.Project)
		}
		for _, want := range []string{"postgres:", "pgbouncer:", "valkey:", "storage:"} {
			if !strings.Contains(p.Compose, want) {
				t.Errorf("%q: down plan lacks %s", src, want)
			}
		}
	}
}

func TestEnsureIgnored(t *testing.T) {
	cases := []struct {
		name     string
		existing *string
		added    int
		link     bool
	}{
		{name: "no .gitignore", added: len(gitignoreAll())},
		{name: "everything there", existing: ptr(strings.Join(gitignoreAll(), "\n") + "\n")},
		{name: "only node_modules", existing: ptr("node_modules/\n"), added: len(gitignoreAll())},
		{name: "a link", link: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			outside := filepath.Join(t.TempDir(), "target")
			if c.existing != nil {
				if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(*c.existing), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if c.link {
				if err := os.Symlink(outside, filepath.Join(dir, ".gitignore")); err != nil {
					t.Fatal(err)
				}
			}
			added, err := EnsureIgnored(dir)
			if c.link {
				if err == nil {
					t.Error("followed a .gitignore link")
				}
				if _, err := os.Lstat(outside); err == nil {
					t.Error("wrote through the link")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(added) != c.added {
				t.Errorf("added %v, want %d entries", added, c.added)
			}
			got, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
			if !strings.Contains(string(got), ".whisk/dev/\n") {
				t.Errorf(".gitignore lacks .whisk/dev/:\n%s", got)
			}
			if c.existing != nil && !strings.HasPrefix(string(got), *c.existing) {
				t.Errorf("existing lines were not kept:\n%s", got)
			}
		})
	}
}

func gitignoreAll() []string { return []string{".whisk/dev/", ".env", ".env.*", "!.env.example"} }

func ptr(s string) *string { return &s }
