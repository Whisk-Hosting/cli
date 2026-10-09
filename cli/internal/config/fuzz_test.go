package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whisk-run/contract/naughty"
)

// Fuzz targets for the CLI's own files and references (docs/HARNESS.md §8.3).

// FuzzParseRef: an accepted reference is exactly two non-empty parts that join back to it.
func FuzzParseRef(f *testing.F) {
	for _, s := range []string{"acme/app", "acme/", "/app", "a/b/c", ""} {
		f.Add(s)
	}
	for _, s := range naughty.Strings() {
		f.Add(s)
		f.Add("acme/" + s)
	}
	f.Fuzz(func(t *testing.T, ref string) {
		org, app, err := ParseRef(ref)
		if err != nil {
			if org != "" || app != "" {
				t.Fatalf("ParseRef(%q) failed but answered %q, %q", ref, org, app)
			}
			return
		}
		if org == "" || app == "" || strings.Contains(org, "/") || strings.Contains(app, "/") || org+"/"+app != ref {
			t.Fatalf("ParseRef(%q) = %q, %q", ref, org, app)
		}
	})
}

// FuzzLoad writes fuzzed config.json and .whisk/app.json: loading answers a value or an error,
// a loaded config has its defaults, a loaded binding names an org and an app, and what loaded
// saves and loads back the same.
func FuzzLoad(f *testing.F) {
	f.Add([]byte(`{"api":"https://api.whisk.run","profile":"default"}`))
	f.Add([]byte(`{"org":"acme","app":"crm","api":"https://api.whisk.run"}`))
	for _, s := range naughty.Strings() {
		f.Add([]byte(s))
		q, _ := json.Marshal(s)
		f.Add([]byte(`{"api":` + string(q) + `,"profile":` + string(q) + `,"org":` + string(q) + `,"app":` + string(q) + `}`))
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "config.json"), src, 0o600); err != nil {
			t.Fatal(err)
		}
		if cfg, err := Load(dir); err == nil {
			if cfg.API == "" || cfg.Profile == "" {
				t.Fatalf("Load left the defaults empty: %+v", cfg)
			}
			if err := Save(dir, cfg); err != nil {
				t.Fatal(err)
			}
			if again, err := Load(dir); err != nil || again != cfg {
				t.Fatalf("Save/Load changed %+v to %+v, %v", cfg, again, err)
			}
		}
		if err := os.MkdirAll(filepath.Join(dir, ".whisk"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".whisk", "app.json"), src, 0o600); err != nil {
			t.Fatal(err)
		}
		if b, ok, err := LoadBinding(dir); err == nil && ok {
			if b.Org == "" || b.App == "" {
				t.Fatalf("binding without org or app: %+v", b)
			}
			if err := SaveBinding(dir, b); err != nil {
				t.Fatal(err)
			}
			if again, ok, err := LoadBinding(dir); err != nil || !ok || again != b {
				t.Fatalf("SaveBinding/LoadBinding changed %+v to %+v, %v", b, again, err)
			}
		}
		fs := fileStore{path: filepath.Join(dir, "credentials.json")}
		if err := os.WriteFile(fs.path, src, 0o600); err != nil {
			t.Fatal(err)
		}
		_, _, _ = fs.Get("default")
	})
}
