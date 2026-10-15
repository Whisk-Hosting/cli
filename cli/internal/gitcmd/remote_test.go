package gitcmd

import "testing"

func TestRemoteDisagrees(t *testing.T) {
	cases := []struct {
		remote, gitURL string
		want           bool
	}{
		{"", "https://git.whisk.run/01J/a1.git", false},
		{"https://git.whisk.run/01J/a1.git", "https://git.whisk.run/01J/a1.git", false},
		{"https://git.whisk.run/01J/a1.git/", "https://git.whisk.run/01J/a1.git", false},
		{" https://git.whisk.run/01J/a1.git\n", "https://git.whisk.run/01J/a1.git", false},
		{"https://git.whisk.run/01J/a2.git", "https://git.whisk.run/01J/a1.git", true},
		{"https://git.whisk.run/02K/a1.git", "https://git.whisk.run/01J/a1.git", true},
		{"https://evil.example/01J/a1.git", "https://git.whisk.run/01J/a1.git", true},
	}
	for _, c := range cases {
		if got := RemoteDisagrees(c.remote, c.gitURL); got != c.want {
			t.Errorf("RemoteDisagrees(%q, %q) = %v, want %v", c.remote, c.gitURL, got, c.want)
		}
	}
}

func TestRedact(t *testing.T) {
	cases := map[string]string{
		"https://user:secret@git.whisk.run/01J/a1.git": "https://git.whisk.run/01J/a1.git",
		"https://git.whisk.run/01J/a1.git":             "https://git.whisk.run/01J/a1.git",
		"git@github.com:acme/crm.git":                  "git@github.com:acme/crm.git",
	}
	for in, want := range cases {
		if got := Redact(in); got != want {
			t.Errorf("Redact(%q) = %q, want %q", in, got, want)
		}
	}
}
