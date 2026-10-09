package graph

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/whisk-run/contract/naughty"
)

// FuzzParse feeds graph files to Parse (docs/HARNESS.md §8.3): the answer is Problems or a graph
// whose ids are unique and whose targets resolve, and an accepted graph written back out parses
// to the same graph.
func FuzzParse(f *testing.F) {
	for _, dir := range []string{"valid", "invalid"} {
		files, _ := filepath.Glob("../fixtures/graphs/" + dir + "/*.yaml")
		for _, file := range files {
			b, err := os.ReadFile(file)
			if err != nil {
				f.Fatal(err)
			}
			f.Add(b)
		}
	}
	for _, s := range naughty.Strings() {
		f.Add([]byte(s))
		b, _ := json.Marshal(map[string]any{"steps": []any{map[string]any{"id": s}, map[string]any{"id": "b?", "branches": map[string]any{s: "end"}}}})
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		checkGraph(t, "fuzz", "", src)
		g, err := Parse(src)
		if err != nil {
			return
		}
		for i := range g.Steps {
			_ = g.Successors(i)
		}
		ids := g.StepIDs()
		unexpected, neverRan := g.Drift(ids)
		if len(unexpected) != 0 || len(neverRan) != 0 {
			t.Fatalf("a graph drifts from its own step ids: %v %v", unexpected, neverRan)
		}
		again, err := json.Marshal(g)
		if err != nil {
			t.Fatal(err)
		}
		g2, err := Parse(again)
		if err != nil {
			t.Fatalf("accepted graph does not parse again: %v\n%s", err, again)
		}
		if !reflect.DeepEqual(g, g2) {
			t.Fatalf("round trip changed the graph:\n%+v\n%+v", g, g2)
		}
	})
}
