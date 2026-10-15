package errors

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/whisk-run/contract/naughty"
)

// A naughty code is not in the catalogue, and an error body built from naughty text is valid
// JSON that reads back as what was written.
func TestNaughtyCatalogue(t *testing.T) {
	codes := map[string]bool{}
	for _, c := range Codes() {
		codes[c] = true
	}
	for _, s := range naughty.Strings() {
		if _, ok := Lookup(s); ok != codes[s] {
			t.Errorf("Lookup(%q) = %v", s, ok)
		}
		if section, ok := Section(s); ok != codes[s] || (ok && !strings.HasPrefix(section, "## "+s)) {
			t.Errorf("Section(%q) = %v", s, ok)
		}
		for _, code := range []string{s, "INVALID_REQUEST"} {
			raw := New(code, s, s, map[string]any{"input": s}).JSON()
			var back Body
			if err := json.Unmarshal(raw, &back); err != nil {
				t.Errorf("New(%q) is not JSON: %v", code, err)
				continue
			}
			if !utf8.ValidString(s) {
				continue
			}
			want := s
			if want == "" {
				e, _ := Lookup(code)
				want = e.When
			}
			if back.Error.Code != code || back.Error.Message != want || back.Error.Details["input"] != s {
				t.Errorf("New(%q, %q) read back as %+v", code, s, back.Error)
			}
		}
	}
}
