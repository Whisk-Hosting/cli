package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"pgregory.net/rapid"
)

// TestCredentialStoreStateMachine runs the credentials file and the in-memory store side by
// side against a map, with config.json and the directory binding saved and loaded between
// (CLI.md §7): a profile's credential is what was last set for it, a pending login lives beside
// its profile and never over it, a delete forgets one profile only, a missing config file is
// the defaults and an empty API or profile reads as the default, and a binding without an org
// or an app does not count as one (HARNESS.md §8.5).
func TestCredentialStoreStateMachine(t *testing.T) {
	root := t.TempDir()
	n := 0
	rapid.Check(t, func(t *rapid.T) {
		n++
		dir := filepath.Join(root, strconv.Itoa(n))
		file := fileStore{path: filepath.Join(dir, "credentials.json")}
		mem := &MemoryStore{m: map[string]Credential{}}
		creds := map[string]Credential{}
		var cfg *Config
		var binding *Binding

		profiles := rapid.SampledFrom([]string{"default", "work", PendingProfile("default"), PendingProfile("work")})
		credential := rapid.Custom(func(t *rapid.T) Credential {
			c := Credential{Token: "whsk_agent_" + rapid.StringMatching(`[a-z0-9]{0,6}`).Draw(t, "token"),
				API: rapid.SampledFrom([]string{DefaultAPI, "https://api.whisk.test"}).Draw(t, "api"),
				Org: rapid.SampledFrom([]string{"", "acme"}).Draw(t, "org")}
			if rapid.Bool().Draw(t, "scoped") {
				c.Scopes = rapid.SliceOfN(rapid.SampledFrom([]string{"deploy", "logs:read", "secrets:declare"}), 1, 3).Draw(t, "scopes")
			}
			if rapid.Bool().Draw(t, "expires") {
				c.ExpiresAt = time.Unix(rapid.Int64Range(1_700_000_000, 1_900_000_000).Draw(t, "at"), 0).UTC()
			}
			if rapid.Bool().Draw(t, "bound") {
				c.PrivateKey = "c2VlZA=="
			}
			return c
		})

		t.Repeat(map[string]func(*rapid.T){
			"set": func(t *rapid.T) {
				p, c := profiles.Draw(t, "profile"), credential.Draw(t, "credential")
				for _, s := range []Store{file, mem} {
					if err := s.Set(p, c); err != nil {
						t.Fatalf("%s Set: %v", s.Where(), err)
					}
				}
				creds[p] = c
			},
			"delete": func(t *rapid.T) {
				p := profiles.Draw(t, "profile")
				for _, s := range []Store{file, mem} {
					if err := s.Delete(p); err != nil {
						t.Fatalf("%s Delete: %v", s.Where(), err)
					}
				}
				delete(creds, p)
			},
			"saveConfig": func(t *rapid.T) {
				c := Config{API: rapid.SampledFrom([]string{"", DefaultAPI, "https://api.whisk.test"}).Draw(t, "api"),
					Profile:   rapid.SampledFrom([]string{"", "default", "work"}).Draw(t, "profile"),
					Telemetry: rapid.Bool().Draw(t, "telemetry"),
					Dashboard: rapid.SampledFrom([]string{"", "https://whisk.test"}).Draw(t, "dashboard")}
				if err := Save(dir, c); err != nil {
					t.Fatalf("Save: %v", err)
				}
				cfg = &c
			},
			"saveBinding": func(t *rapid.T) {
				b := Binding{Org: rapid.SampledFrom([]string{"", "acme"}).Draw(t, "org"), App: rapid.SampledFrom([]string{"", "jobs", "crm"}).Draw(t, "app"), API: DefaultAPI}
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := SaveBinding(dir, b); err != nil {
					t.Fatalf("SaveBinding: %v", err)
				}
				binding = &b
			},
			"get": func(t *rapid.T) {
				p := profiles.Draw(t, "profile")
				want, held := creds[p]
				for _, s := range []Store{file, mem} {
					got, ok, err := s.Get(p)
					if err != nil || ok != held || !reflect.DeepEqual(got, want) {
						t.Fatalf("%s Get(%s) = %+v %v %v, the model says %+v %v", s.Where(), p, got, ok, err, want, held)
					}
				}
			},
			"load": func(t *rapid.T) {
				got, err := Load(dir)
				want := Config{API: DefaultAPI, Profile: "default"}
				if cfg != nil {
					want = *cfg
					if want.API == "" {
						want.API = DefaultAPI
					}
					if want.Profile == "" {
						want.Profile = "default"
					}
				}
				if err != nil || got != want {
					t.Fatalf("Load = %+v %v, the model says %+v", got, err, want)
				}
			},
			"loadBinding": func(t *rapid.T) {
				b, ok, err := LoadBinding(dir)
				switch {
				case binding == nil:
					if ok || err != nil {
						t.Fatalf("LoadBinding of an unbound folder = %+v %v %v", b, ok, err)
					}
				case binding.Org == "" || binding.App == "":
					if ok || err == nil {
						t.Fatalf("LoadBinding of %+v = %+v %v %v, the model says it is no binding", *binding, b, ok, err)
					}
				default:
					if !ok || err != nil || b != *binding {
						t.Fatalf("LoadBinding = %+v %v %v, the model says %+v", b, ok, err, *binding)
					}
				}
			},
		})
	})
}
