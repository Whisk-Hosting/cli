package contract

import (
	"encoding/json"
	"image"
	_ "image/png"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The plugin package OpenAI's directory takes (plugin/README.md), checked against the limits the
// directory sets for public submission, so an upload is not turned back for a field we can check
// here.

type pluginCase struct {
	Description      string `json:"description"`
	Prompt           string `json:"prompt"`
	ToolsTriggered   string `json:"tools_triggered"`
	ExpectedBehavior string `json:"expected_behavior"`
}

type pluginManifest struct {
	Schema     string `json:"$schema"`
	Name       string `json:"name"`
	Version    string `json:"version"`
	Extensions struct {
		OpenAI struct {
			Interface struct {
				DisplayName       string   `json:"displayName"`
				ShortDescription  string   `json:"shortDescription"`
				LongDescription   string   `json:"longDescription"`
				DeveloperName     string   `json:"developerName"`
				Category          string   `json:"category"`
				Capabilities      []string `json:"capabilities"`
				WebsiteURL        string   `json:"websiteURL"`
				SupportURL        string   `json:"supportURL"`
				PrivacyPolicyURL  string   `json:"privacyPolicyURL"`
				TermsOfServiceURL string   `json:"termsOfServiceURL"`
				DefaultPrompt     []string `json:"defaultPrompt"`
				BrandColor        string   `json:"brandColor"`
				BrandColorDark    string   `json:"brandColorDark"`
				ComposerIcon      string   `json:"composerIcon"`
				Logo              string   `json:"logo"`
				LogoDark          string   `json:"logoDark"`
			} `json:"interface"`
			Review struct {
				TestCases struct {
					Positive []pluginCase `json:"positive"`
					Negative []pluginCase `json:"negative"`
				} `json:"test_cases"`
			} `json:"review"`
		} `json:"com.openai"`
	} `json:"extensions"`
}

func readPluginFile(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("plugin", name))
	if err != nil {
		t.Fatalf("plugin/%s: %v", name, err)
	}
	return b
}

func runes(s string) int { return len([]rune(s)) }

func TestPluginListing(t *testing.T) {
	raw := readPluginFile(t, "plugin.json")
	for _, banned := range []string{`"test_credentials"`, `"reviewer_instructions"`, `"apps"`, `"hooks"`, `"clientSecret"`} {
		if strings.Contains(string(raw), banned) {
			t.Errorf("plugin.json has %s, which public submission refuses", banned)
		}
	}
	var m pluginManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("plugin.json: %v", err)
	}
	if m.Schema != "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json" {
		t.Errorf("$schema = %q", m.Schema)
	}
	if !regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`).MatchString(m.Name) || len(m.Name) > 64 {
		t.Errorf("name %q must be kebab-case, at most 64 characters", m.Name)
	}
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(m.Version) {
		t.Errorf("version %q must be a semantic version", m.Version)
	}
	in := m.Extensions.OpenAI.Interface
	limits := []struct {
		field, value string
		max          int
	}{
		{"displayName", in.DisplayName, 30},
		{"shortDescription", in.ShortDescription, 30},
		{"longDescription", in.LongDescription, 4000},
		{"developerName", in.DeveloperName, 80},
		{"category", in.Category, 80},
	}
	for _, l := range limits {
		if strings.TrimSpace(l.value) == "" || runes(l.value) > l.max {
			t.Errorf("%s is %d characters; it must be set and at most %d", l.field, runes(l.value), l.max)
		}
	}
	if len(in.Capabilities) > 20 {
		t.Errorf("%d capabilities; at most 20", len(in.Capabilities))
	}
	for _, c := range in.Capabilities {
		if runes(c) > 120 {
			t.Errorf("capability %q is over 120 characters", c)
		}
	}
	seen := map[string]bool{}
	if len(in.DefaultPrompt) > 3 {
		t.Errorf("%d starter prompts; at most 3", len(in.DefaultPrompt))
	}
	for _, p := range in.DefaultPrompt {
		if runes(p) > 128 || seen[p] || strings.Contains(p, "@") {
			t.Errorf("starter prompt %q must be unique, at most 128 characters, with no @mention", p)
		}
		seen[p] = true
	}
	for field, u := range map[string]string{"websiteURL": in.WebsiteURL, "supportURL": in.SupportURL,
		"privacyPolicyURL": in.PrivacyPolicyURL, "termsOfServiceURL": in.TermsOfServiceURL} {
		p, err := url.Parse(u)
		if err != nil || p.Scheme != "https" || p.Host == "" || p.User != nil {
			t.Errorf("%s %q must be an https address", field, u)
		}
	}
	if c := contrast(in.BrandColor, "#ffffff"); c < 2 {
		t.Errorf("brandColor %s has contrast %.2f against white; at least 2", in.BrandColor, c)
	}
	if c := contrast(in.BrandColorDark, "#212121"); c < 2 {
		t.Errorf("brandColorDark %s has contrast %.2f against #212121; at least 2", in.BrandColorDark, c)
	}
	for _, icon := range []string{in.Logo, in.LogoDark, in.ComposerIcon} {
		checkIcon(t, icon)
	}

	cases := m.Extensions.OpenAI.Review.TestCases
	if len(cases.Positive) != 5 || len(cases.Negative) != 3 {
		t.Errorf("review needs exactly 5 positive and 3 negative cases, not %d and %d", len(cases.Positive), len(cases.Negative))
	}
	for _, c := range cases.Positive {
		if c.Description == "" || c.Prompt == "" || c.ToolsTriggered == "" || c.ExpectedBehavior == "" {
			t.Errorf("positive case %q needs a description, prompt, tools and expected behaviour", c.Description)
		}
	}
	for _, c := range cases.Negative {
		if c.Description == "" || c.Prompt == "" {
			t.Errorf("negative case %q needs a description and a prompt", c.Description)
		}
	}
}

func checkIcon(t *testing.T, rel string) {
	t.Helper()
	if !strings.HasPrefix(rel, "./") {
		t.Errorf("icon path %q must start with ./", rel)
		return
	}
	b := readPluginFile(t, strings.TrimPrefix(rel, "./"))
	if len(b) > 5<<20 {
		t.Errorf("%s is over 5 MiB", rel)
	}
	cfg, _, err := image.DecodeConfig(strings.NewReader(string(b)))
	if err != nil {
		t.Errorf("%s: %v", rel, err)
		return
	}
	if cfg.Width != cfg.Height || cfg.Width < 48 || cfg.Width > 4096 {
		t.Errorf("%s is %dx%d; icons are square, 48 to 4096 pixels", rel, cfg.Width, cfg.Height)
	}
}

// contrast is the WCAG contrast ratio of two #RRGGBB colours, 0 when either is malformed. Pure.
func contrast(a, b string) float64 {
	la, oka := luminance(a)
	lb, okb := luminance(b)
	if !oka || !okb {
		return 0
	}
	return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
}

func luminance(hex string) (float64, bool) {
	if !regexp.MustCompile(`^#[0-9a-fA-F]{6}$`).MatchString(hex) {
		return 0, false
	}
	channel := func(s string) float64 {
		var v int
		for _, r := range strings.ToLower(s) {
			v = v*16 + strings.IndexRune("0123456789abcdef", r)
		}
		c := float64(v) / 255
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(hex[1:3]) + 0.7152*channel(hex[3:5]) + 0.0722*channel(hex[5:7]), true
}

func TestPluginServer(t *testing.T) {
	var m struct {
		Schema  string `json:"$schema"`
		Servers map[string]struct {
			Type       string            `json:"type"`
			URL        string            `json:"url"`
			Headers    map[string]string `json:"headers"`
			Extensions struct {
				OpenAI struct {
					Auth struct {
						Type string `json:"type"`
					} `json:"auth"`
				} `json:"com.openai"`
			} `json:"extensions"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(readPluginFile(t, "mcp.json"), &m); err != nil {
		t.Fatalf("mcp.json: %v", err)
	}
	if m.Schema != "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json" {
		t.Errorf("$schema = %q", m.Schema)
	}
	if len(m.Servers) != 1 {
		t.Fatalf("%d servers; review cases need exactly one", len(m.Servers))
	}
	for name, s := range m.Servers {
		if s.Type != "streamable-http" || !strings.HasPrefix(s.URL, "https://") || len(s.Headers) > 0 {
			t.Errorf("server %s must be streamable-http at an https address with no headers", name)
		}
		if s.Extensions.OpenAI.Auth.Type != "oauth" {
			t.Errorf("server %s signs in with OAuth; auth.type is %q", name, s.Extensions.OpenAI.Auth.Type)
		}
	}
}

func TestPluginSkills(t *testing.T) {
	dirs, err := os.ReadDir(filepath.Join("plugin", "skills"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("plugin/skills: %v (%d skills)", err, len(dirs))
	}
	front := regexp.MustCompile(`(?s)^---\nname: ([a-z0-9-]+)\ndescription: ([^\n]+)\n---\n`)
	for _, d := range dirs {
		body := string(readPluginFile(t, filepath.Join("skills", d.Name(), "SKILL.md")))
		got := front.FindStringSubmatch(body)
		if got == nil || got[1] != d.Name() {
			t.Errorf("skill %s needs a header naming it %q and describing when to use it", d.Name(), d.Name())
		}
		// The directory serves several assistants; a skill names none of them.
		if regexp.MustCompile(`(?i)\b(claude|chatgpt|codex|gpt)\b`).MatchString(body) {
			t.Errorf("skill %s names an assistant; say the model or leave it out", d.Name())
		}
	}
}
