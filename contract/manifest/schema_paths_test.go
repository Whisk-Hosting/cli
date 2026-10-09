package manifest

import (
	"fmt"
	"testing"
)

// The repository paths in whisk.yaml (functions[].graph, static[].dir) stay inside the
// repository: no segment is "..", none is absolute.
func TestRepositoryPathsStayInside(t *testing.T) {
	cases := []struct {
		path     string
		graphOK  bool
		staticOK bool
	}{
		{path: "workflows/a.graph.yaml", graphOK: true, staticOK: true},
		{path: "a.graph.yml", graphOK: true, staticOK: true},
		{path: "./w/a.graph.yaml", graphOK: true, staticOK: true},
		{path: ".github/w/a.graph.yaml", graphOK: true, staticOK: true},
		{path: "..hidden/a.graph.yaml", graphOK: true, staticOK: true},
		{path: "w/.../a.graph.yaml", graphOK: true, staticOK: true},
		{path: "public/", graphOK: false, staticOK: true},
		{path: "../a.graph.yaml", graphOK: false, staticOK: false},
		{path: "w/../../a.graph.yaml", graphOK: false, staticOK: false},
		{path: "w/../a.graph.yaml", graphOK: false, staticOK: false},
		{path: "..", graphOK: false, staticOK: false},
		{path: "public/..", graphOK: false, staticOK: false},
		{path: "public/../", graphOK: false, staticOK: false},
		{path: "/etc/a.graph.yaml", graphOK: false, staticOK: false},
		{path: "w//a.graph.yaml", graphOK: false, staticOK: false},
	}
	for _, c := range cases {
		graph := fmt.Sprintf("whisk: 1\nname: job-tracker\nfunctions:\n  - name: nightly\n    cron: \"0 6 * * *\"\n    graph: %q\n", c.path)
		if _, err := Parse([]byte(graph)); (err == nil) != c.graphOK {
			t.Errorf("graph %q: accepted=%v, want %v (%v)", c.path, err == nil, c.graphOK, err)
		}
		static := fmt.Sprintf("whisk: 1\nname: job-tracker\nstatic:\n  - dir: %q\n    path: /assets\n", c.path)
		if _, err := Parse([]byte(static)); (err == nil) != c.staticOK {
			t.Errorf("static %q: accepted=%v, want %v (%v)", c.path, err == nil, c.staticOK, err)
		}
	}
}
