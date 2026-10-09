package graph

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/whisk-run/contract/naughty"
)

var reNaughtyStepID = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?\??$`)

// checkGraph asserts what Parse promises: Problems, or a graph whose ids are unique and well
// formed and whose every target resolves.
func checkGraph(t *testing.T, where, s string, src []byte) {
	t.Helper()
	g, err := Parse(src)
	if err != nil {
		var ps Problems
		if !errors.As(err, &ps) || len(ps) == 0 {
			t.Errorf("%s %q: error is not Problems: %v", where, s, err)
		}
		return
	}
	ids := map[string]bool{}
	for _, st := range g.Steps {
		if !reNaughtyStepID.MatchString(st.ID) || ids[st.ID] {
			t.Errorf("%s %q: accepted step id %q", where, s, st.ID)
		}
		ids[st.ID] = true
	}
	resolves := func(id string) bool { return id == End || ids[id] }
	for i, st := range g.Steps {
		if st.Next != "" && !resolves(st.Next) {
			t.Errorf("%s %q: next %q does not resolve", where, s, st.Next)
		}
		for _, target := range st.Branches {
			if !resolves(target) {
				t.Errorf("%s %q: branch target %q does not resolve", where, s, target)
			}
		}
		for _, id := range g.Successors(i) {
			if !ids[id] {
				t.Errorf("%s %q: successor %q is not a step", where, s, id)
			}
		}
	}
}

func TestNaughtyGraph(t *testing.T) {
	shapes := []struct {
		name  string
		embed func(s string) any
	}{
		{"id", func(s string) any { return map[string]any{"steps": []any{map[string]any{"id": s}}} }},
		{"next", func(s string) any {
			return map[string]any{"steps": []any{map[string]any{"id": "a", "next": s}, map[string]any{"id": "b"}}}
		}},
		{"kind", func(s string) any { return map[string]any{"steps": []any{map[string]any{"id": "a", "kind": s}}} }},
		{"label", func(s string) any { return map[string]any{"steps": []any{map[string]any{"id": "a", "label": s}}} }},
		{"branch label", func(s string) any {
			return map[string]any{"steps": []any{map[string]any{"id": "ok?", "branches": map[string]any{s: "a", "no": "end"}}, map[string]any{"id": "a"}}}
		}},
		{"branch target", func(s string) any {
			return map[string]any{"steps": []any{map[string]any{"id": "ok?", "branches": map[string]any{"yes": s, "no": "end"}}, map[string]any{"id": "a"}}}
		}},
		{"top-level key", func(s string) any { return map[string]any{"steps": []any{map[string]any{"id": "a"}}, s: 1} }},
	}
	for _, shape := range shapes {
		for _, s := range naughty.Strings() {
			src, err := yaml.Marshal(shape.embed(s))
			if err != nil {
				t.Fatal(err)
			}
			checkGraph(t, shape.name, s, src)
		}
	}
	for _, s := range naughty.Strings() {
		checkGraph(t, "file", s, []byte(s))
		checkGraph(t, "text id", s, []byte("steps:\n  - id: "+s+"\n"))
	}
}

// Drift reports every observed name that is not declared, once, and nothing else.
func TestNaughtyDrift(t *testing.T) {
	g, err := Parse([]byte("steps:\n  - id: receive\n  - id: ok?\n    branches: {yes: approve, no: end}\n  - id: approve\n    kind: approval\n"))
	if err != nil {
		t.Fatal(err)
	}
	observed := append(naughty.Strings(), "receive", "approve/request")
	unexpected, neverRan := g.Drift(observed)
	if len(neverRan) != 0 {
		t.Errorf("neverRan = %q, want none", neverRan)
	}
	seen := map[string]bool{}
	for _, u := range unexpected {
		if seen[u] || u == "receive" || u == "approve" {
			t.Errorf("unexpected holds %q twice or a declared step", u)
		}
		seen[u] = true
	}
	for _, s := range naughty.Strings() {
		name := strings.TrimSuffix(s, "/request")
		if name != "receive" && name != "approve" && !seen[name] {
			t.Errorf("observed %q is missing from unexpected", s)
		}
	}
}
