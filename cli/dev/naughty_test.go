package dev

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/whisk-run/contract/manifest"
	"github.com/whisk-run/contract/naughty"
)

var reNaughtyInterpolation = regexp.MustCompile(`\$[^$]|\$$`)

// composeUnescape reads a value as Compose does after YAML: "$$" is a literal "$".
func composeUnescape(s string) string { return strings.ReplaceAll(s, "$$", "$") }

// A queue endpoint the manifest accepts reaches the dev server's command exactly: YAML keeps it
// one argument and Compose interpolates nothing in it.
func TestNaughtyCompose(t *testing.T) {
	ports := Ports{Postgres: 1, PgBouncer: 2, Inngest: 3, Valkey: 4, Storage: 5, Console: 6}
	for _, s := range naughty.Strings() {
		src, _ := yaml.Marshal(map[string]any{"whisk": 1, "name": "job-tracker", "queue": map[string]any{"endpoint": "/" + s}, "kv": true, "storage": true})
		m, err := manifest.Parse(src)
		if err != nil {
			continue
		}
		for _, host := range []bool{false, true} {
			plan := BuildPlan(m, ports, 3002, host)
			var doc struct {
				Services map[string]struct {
					Command []string `yaml:"command"`
				} `yaml:"services"`
			}
			if err := yaml.Unmarshal([]byte(plan.Compose), &doc); err != nil {
				t.Errorf("endpoint %q: compose is not YAML: %v", m.Queue.Endpoint, err)
				continue
			}
			cmd := doc.Services["inngest"].Command
			if len(cmd) < 4 || !strings.HasSuffix(composeUnescape(cmd[3]), ":3002"+m.Queue.Endpoint) {
				t.Errorf("endpoint %q: inngest command %q", m.Queue.Endpoint, cmd)
				continue
			}
			if reNaughtyInterpolation.MatchString(strings.ReplaceAll(cmd[3], "$$", "")) {
				t.Errorf("endpoint %q: Compose would interpolate %q", m.Queue.Endpoint, cmd[3])
			}
		}
	}
}

// Only verified deliveries not yet seen are forwarded, each once, oldest first.
func TestNaughtyDeliveries(t *testing.T) {
	var list []Delivery
	seen := map[string]bool{}
	at := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	for i, s := range naughty.Strings() {
		list = append(list, Delivery{ID: s, Verified: i%3 != 0, ReceivedAt: at.Add(-time.Duration(i) * time.Second), Body: s})
		if i%5 == 0 {
			seen[s] = true
		}
	}
	out := NewDeliveries(list, seen)
	for i, d := range out {
		if seen[d.ID] || !d.Verified {
			t.Errorf("forwarded %q: seen or unverified", d.ID)
		}
		if i > 0 && out[i-1].ReceivedAt.After(d.ReceivedAt) {
			t.Errorf("forwarded %q before an older delivery", d.ID)
		}
	}
}
