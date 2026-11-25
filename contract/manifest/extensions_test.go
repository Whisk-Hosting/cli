package manifest

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// An app names the Postgres extensions Whisk turns on in its database, from the allowed list
// only, and only with a database (SKILL.md §5).
func TestExtensions(t *testing.T) {
	cases := []struct {
		name, yaml string
		want       []string
		problem    string
	}{
		{"none", "", []string{}, ""},
		{"pgvector and trigram search", "extensions: [vector, pg_trgm]\n", []string{"vector", "pg_trgm"}, ""},
		{"one not on the list", "extensions: [postgis]\n", nil, "/extensions/0"},
		{"twice", "extensions: [vector, vector]\n", nil, "/extensions"},
		{"without a database", "database: none\nextensions: [vector]\n", nil, "/extensions"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := Parse([]byte("whisk: 1\nname: shop\n" + tc.yaml))
			if tc.problem != "" {
				var ps Problems
				if !errors.As(err, &ps) || len(ps) == 0 || !strings.HasPrefix(ps[0].Path, tc.problem) {
					t.Fatalf("got %v, want a problem at %s", err, tc.problem)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(m.Extensions, tc.want) {
				t.Errorf("extensions %v, want %v", m.Extensions, tc.want)
			}
		})
	}
}

// The schema's list is AllowedExtensions.
func TestSchemaListsTheAllowedExtensions(t *testing.T) {
	for _, e := range AllowedExtensions {
		if _, err := Parse([]byte("whisk: 1\nname: shop\nextensions: [" + e + "]\n")); err != nil {
			t.Errorf("%s is allowed but the schema refuses it: %v", e, err)
		}
	}
}
