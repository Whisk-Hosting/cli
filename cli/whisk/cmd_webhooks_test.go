package whisk

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/whisk-run/contract/manifest"
)

// TestAddWebhookEntry covers the shapes a manifest arrives in: no webhooks block, one that
// already has entries, secrets written inline and as a list, and none at all.
func TestAddWebhookEntry(t *testing.T) {
	base := "whisk: 1\nname: my-app\n\nroutes:\n  public: [\"/\"]\n"
	cases := []struct {
		name        string
		manifest    string
		wantSecrets []string
		added       bool
	}{
		{
			name:        "no webhooks block",
			manifest:    base + "\nsecrets: [OTHER]\n",
			wantSecrets: []string{"OTHER", "STRIPE_WEBHOOK_SECRET"},
			added:       true,
		},
		{
			name:        "an existing block gains an entry",
			manifest:    base + "\nsecrets: [STRIPE_WEBHOOK_SECRET]\n\nwebhooks:\n  - name: github\n    preset: github\n    secret: STRIPE_WEBHOOK_SECRET\n    handler: /hooks/github\n",
			wantSecrets: []string{"STRIPE_WEBHOOK_SECRET"},
		},
		{
			name:        "secrets as a list",
			manifest:    base + "\nsecrets:\n  - OTHER\n",
			wantSecrets: []string{"STRIPE_WEBHOOK_SECRET", "OTHER"},
			added:       true,
		},
		{
			name:        "no secrets at all",
			manifest:    base,
			wantSecrets: []string{"STRIPE_WEBHOOK_SECRET"},
			added:       true,
		},
		{
			name:        "a trailing block that is not webhooks",
			manifest:    base + "\nsecrets: [STRIPE_WEBHOOK_SECRET]\n\nwebhooks:\n  - name: github\n    preset: github\n    secret: STRIPE_WEBHOOK_SECRET\n    handler: /hooks/github\n\nstorage: true\n",
			wantSecrets: []string{"STRIPE_WEBHOOK_SECRET"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "whisk.yaml")
			if err := os.WriteFile(path, []byte(c.manifest), 0o644); err != nil {
				t.Fatal(err)
			}
			added, err := addWebhookEntry(path, "stripe", "stripe", "STRIPE_WEBHOOK_SECRET", "/hooks/stripe")
			if err != nil {
				t.Fatalf("addWebhookEntry: %v", err)
			}
			if added != c.added {
				t.Errorf("added secret = %v, want %v", added, c.added)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			m, err := manifest.Parse(raw)
			if err != nil {
				t.Fatalf("the result does not parse: %v\n%s", err, raw)
			}
			src, ok := m.WebhookByName("stripe")
			if !ok {
				t.Fatalf("stripe was not added:\n%s", raw)
			}
			if src.Preset != "stripe" || src.Handler != "/hooks/stripe" || src.Secret != "STRIPE_WEBHOOK_SECRET" {
				t.Errorf("source = %+v", src)
			}
			for _, want := range c.wantSecrets {
				if !namesInclude(m.Secrets, want) {
					t.Errorf("secrets %v missing %s", m.Secrets, want)
				}
			}
			if strings.Contains(c.manifest, "github") {
				if _, ok := m.WebhookByName("github"); !ok {
					t.Errorf("the existing source was lost:\n%s", raw)
				}
			}
			if strings.Contains(c.manifest, "storage: true") && !m.Storage {
				t.Errorf("the block after webhooks was lost:\n%s", raw)
			}
		})
	}
}

// TestAddWebhookEntryRefusesBrokenManifest keeps a manifest that does not parse untouched.
func TestAddWebhookEntryRefusesBrokenManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "whisk.yaml")
	broken := "whisk: 1\nname: [unterminated\n"
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := addWebhookEntry(path, "stripe", "stripe", "S", "/hooks/stripe"); err == nil {
		t.Fatal("expected a refusal")
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != broken {
		t.Errorf("the file was changed:\n%s", raw)
	}
}
