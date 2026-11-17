package gitcmd

import (
	"reflect"
	"testing"
)

func TestHelperPlan(t *testing.T) {
	key := HelperKey("https://git.whisk.run")
	want := HelperValues("/usr/local/bin/whisk")
	for _, tc := range []struct {
		name    string
		current []string
		plan    [][]string
	}{
		{"none yet", nil, [][]string{{"--add", key, ""}, {"--add", key, `!"/usr/local/bin/whisk" git-credential`}}},
		{"already whisk", []string{"", `!"/usr/local/bin/whisk" git-credential`}, nil},
		{"whisk moved", []string{"", `!"/opt/whisk" git-credential`},
			[][]string{{"--unset-all", key}, {"--add", key, ""}, {"--add", key, `!"/usr/local/bin/whisk" git-credential`}}},
		{"another helper", []string{"manager"},
			[][]string{{"--unset-all", key}, {"--add", key, ""}, {"--add", key, `!"/usr/local/bin/whisk" git-credential`}}},
		{"whisk without the reset", []string{`!"/usr/local/bin/whisk" git-credential`},
			[][]string{{"--unset-all", key}, {"--add", key, ""}, {"--add", key, `!"/usr/local/bin/whisk" git-credential`}}},
	} {
		if got := HelperPlan(key, tc.current, want); !reflect.DeepEqual(got, tc.plan) {
			t.Errorf("%s: %q", tc.name, got)
		}
	}
	if key != "credential.https://git.whisk.run.helper" {
		t.Fatalf("key %q", key)
	}
}

func TestConfigValues(t *testing.T) {
	for in, want := range map[string][]string{
		"":                           nil,
		"\n":                         {""},
		"\n!\"/w\" git-credential\n": {"", `!"/w" git-credential`},
		"\r\nmanager\r\n":            {"", "manager"},
		"manager":                    {"manager"},
	} {
		if got := ConfigValues(in); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: %q", in, got)
		}
	}
}
