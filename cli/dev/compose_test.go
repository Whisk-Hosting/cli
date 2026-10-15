package dev

import (
	"strings"
	"testing"

	"github.com/whisk-run/contract/manifest"
)

func TestBuildPlan(t *testing.T) {
	m, err := manifest.Parse([]byte("whisk: 1\nname: my-app\nkv: true\nstorage: true\n"))
	if err != nil {
		t.Fatal(err)
	}
	ports := Ports{Postgres: 50001, PgBouncer: 50002, Inngest: 50003, Valkey: 50004, Storage: 50005, Console: 50006}
	p := BuildPlan(m, ports, 3002, false)
	if p.Project != "whisk-my-app" || p.Database != "my_app" {
		t.Errorf("project %s database %s", p.Project, p.Database)
	}
	if p.DatabaseURL != "postgres://whisk:whisk@127.0.0.1:50002/my_app?sslmode=disable" {
		t.Errorf("database url %s", p.DatabaseURL)
	}
	for _, want := range []string{"WHISK_KV_URL=redis://127.0.0.1:50004/0", "WHISK_STORAGE_ENDPOINT=http://127.0.0.1:50005", "WHISK_STORAGE_PREFIX=my-app/"} {
		if !contains(p.ExtraEnv, want) {
			t.Errorf("missing %s in %v", want, p.ExtraEnv)
		}
	}
	for _, want := range []string{"host.docker.internal:3002/.whisk/inngest", `"127.0.0.1:50003:8288"`, "valkey:", "storage-init:", "extra_hosts"} {
		if !strings.Contains(p.Compose, want) {
			t.Errorf("compose lacks %q:\n%s", want, p.Compose)
		}
	}
	got, ok := portsFromCompose(p.Compose)
	if !ok || got != ports {
		t.Errorf("ports round trip: %+v (%v)", got, ok)
	}
	if !strings.Contains(p.Compose, "# inputs: "+Hash(m, ports, 3002)) {
		t.Error("compose does not carry its input hash")
	}

	linux := BuildPlan(m, ports, 3002, true)
	if !strings.Contains(linux.Compose, "network_mode: host") || !strings.Contains(linux.Compose, "http://127.0.0.1:3002/.whisk/inngest") || strings.Contains(linux.Compose, "extra_hosts") {
		t.Errorf("host network compose:\n%s", linux.Compose)
	}
	if got, ok := portsFromCompose(linux.Compose); !ok || got.Inngest != 50003 {
		t.Errorf("host network ports: %+v (%v)", got, ok)
	}

	plain, _ := manifest.Parse([]byte("whisk: 1\nname: plain\n"))
	minimal := BuildPlan(plain, ports, 3002, false)
	if strings.Contains(minimal.Compose, "valkey") || strings.Contains(minimal.Compose, "storage") || len(minimal.ExtraEnv) != 0 {
		t.Errorf("minimal manifest should only get postgres, pgbouncer and inngest:\n%s", minimal.Compose)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
