package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/whisk-run/contract/naughty"
)

// ParseRef answers two non-empty parts that join back to the input, or an error.
func TestNaughtyParseRef(t *testing.T) {
	for _, s := range naughty.Strings() {
		for _, ref := range []string{s, s + "/" + s, "acme/" + s, s + "/app"} {
			org, app, err := ParseRef(ref)
			if err != nil {
				continue
			}
			if org == "" || app == "" || strings.Contains(org, "/") || strings.Contains(app, "/") || org+"/"+app != ref {
				t.Errorf("ParseRef(%q) = %q, %q", ref, org, app)
			}
		}
	}
}

// Whatever config.json, credentials.json or .whisk/app.json hold, loading answers a value or
// an error, and what the CLI saves loads back as it was.
func TestNaughtyFiles(t *testing.T) {
	dir := t.TempDir()
	for _, s := range naughty.Strings() {
		for _, src := range []string{s, `{"api":` + quote(s) + `,"profile":` + quote(s) + `}`} {
			if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(src), 0o600); err != nil {
				t.Fatal(err)
			}
			if cfg, err := Load(dir); err == nil && (cfg.API == "" || cfg.Profile == "") {
				t.Errorf("Load of %q left the defaults empty: %+v", src, cfg)
			}
			if err := os.MkdirAll(filepath.Join(dir, ".whisk"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, ".whisk", "app.json"), []byte(src), 0o600); err != nil {
				t.Fatal(err)
			}
			if b, ok, err := LoadBinding(dir); err == nil && ok && (b.Org == "" || b.App == "") {
				t.Errorf("LoadBinding of %q answered a binding without org or app: %+v", src, b)
			}
			fs := fileStore{path: filepath.Join(dir, "credentials.json")}
			if err := os.WriteFile(fs.path, []byte(src), 0o600); err != nil {
				t.Fatal(err)
			}
			_, _, _ = fs.Get(s)
		}
		if !utf8.ValidString(s) || s == "" {
			continue
		}
		cfg := Config{API: s, Profile: s, Dashboard: s}
		if err := Save(dir, cfg); err != nil {
			t.Fatal(err)
		}
		if got, err := Load(dir); err != nil || got != cfg {
			t.Errorf("Save/Load of %q gave %+v, %v", s, got, err)
		}
		b := Binding{Org: s, App: s, API: s}
		if err := SaveBinding(dir, b); err != nil {
			t.Fatal(err)
		}
		if got, ok, err := LoadBinding(dir); err != nil || !ok || got != b {
			t.Errorf("SaveBinding/LoadBinding of %q gave %+v, %v, %v", s, got, ok, err)
		}
		fs := fileStore{path: filepath.Join(dir, "credentials.json")}
		_ = os.Remove(fs.path)
		if err := fs.Set(s, Credential{Token: s, API: s}); err != nil {
			t.Fatal(err)
		}
		if c, ok, err := fs.Get(s); err != nil || !ok || c.Token != s {
			t.Errorf("credential for profile %q read back as %+v, %v, %v", s, c, ok, err)
		}
	}
}

func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "\t", `\t`).Replace(s) + `"`
}
