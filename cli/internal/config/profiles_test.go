package config

import (
	"slices"
	"testing"
)

// The keychain's list of profiles is sorted, without repeats, and never names a login waiting
// for --resume.
func TestProfileList(t *testing.T) {
	cases := []struct {
		name string
		got  []string
		want []string
	}{
		{"add", withProfile([]string{"jane", "default"}, "contoso"), []string{"contoso", "default", "jane"}},
		{"add again", withProfile([]string{"default"}, "default"), []string{"default"}},
		{"a waiting login is not a profile", withProfile([]string{"default"}, "default:pending"), []string{"default"}},
		{"take off", withoutProfile([]string{"default", "jane"}, "jane"), []string{"default"}},
		{"take off one not there", withoutProfile([]string{"default"}, "x"), []string{"default"}},
		{"clean", withoutProfile([]string{"b", "", "a", "a", "a:pending"}, ""), []string{"a", "b"}},
	}
	for _, c := range cases {
		if !slices.Equal(c.got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, c.got, c.want)
		}
	}
}

// The credentials file and the in-memory store list the profiles they hold the same way.
func TestStoresList(t *testing.T) {
	f := fileStore{path: t.TempDir() + "/credentials.json"}
	m := NewMemoryStore("default", Credential{Token: "a"})
	for _, s := range []Store{f, m} {
		for _, p := range []string{"default", "contoso", "contoso:pending"} {
			if err := s.Set(p, Credential{Token: p}); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.Delete("contoso"); err != nil {
			t.Fatal(err)
		}
		if got, err := s.List(); err != nil || !slices.Equal(got, []string{"default"}) {
			t.Errorf("%s lists %v %v", s.Where(), got, err)
		}
	}
}
