package gitcmd

import (
	"strings"
	"testing"
)

// A transfer StallArgs ended is a lost connection, as git reports it with and without a hang-up.
func TestStalledTransferIsNetwork(t *testing.T) {
	for _, stderr := range []string{
		"error: RPC failed; curl 28 Operation too slow. Less than 1000 bytes/sec transferred the last 60 seconds\nfatal: the remote end hung up unexpectedly",
		"error: RPC failed; curl 28 Operation too slow. Less than 1000 bytes/sec transferred the last 60 seconds",
	} {
		if k := Failure(stderr, PushUnknown); k != "network" {
			t.Errorf("Failure(%q) = %q", stderr, k)
		}
	}
}

func TestNetworked(t *testing.T) {
	cred := []string{"-c", "credential.helper="}
	cases := []struct {
		name    string
		cred    []string
		command []string
		want    string
	}{
		{"push", cred, []string{"push", "--porcelain", "whisk", "main:refs/heads/main"},
			"-c credential.helper= -c http.lowSpeedLimit=1000 -c http.lowSpeedTime=60 push --porcelain whisk main:refs/heads/main"},
		{"clone", cred, []string{"clone", "--quiet", "--", "https://git.whisk.run/a.git", "dir"},
			"-c credential.helper= -c http.lowSpeedLimit=1000 -c http.lowSpeedTime=60 clone --quiet -- https://git.whisk.run/a.git dir"},
		{"no credential", nil, []string{"fetch", "whisk"}, "-c http.lowSpeedLimit=1000 -c http.lowSpeedTime=60 fetch whisk"},
	}
	for _, tc := range cases {
		backing := make([]string, len(tc.cred), len(tc.cred)+8)
		copy(backing, tc.cred)
		got := Networked(backing, tc.command...)
		if strings.Join(got, " ") != tc.want {
			t.Errorf("%s: %q", tc.name, got)
		}
		if backing[:cap(backing)][len(tc.cred)] != "" {
			t.Errorf("%s: Networked wrote into the credential's backing array", tc.name)
		}
	}
}
