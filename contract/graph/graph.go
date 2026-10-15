// Package graph parses and validates a declared workflow graph (*.graph.yaml) against
// graph.schema.json and the graph rules: unique ids, resolvable targets, reachability, and
// decisions with branches.
package graph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"gopkg.in/yaml.v3"

	"github.com/whisk-run/contract"
)

// End is the reserved target that finishes the workflow.
const End = "end"

// Graph is a parsed, validated declared graph.
type Graph struct {
	Steps []Step `json:"steps"`
}

// StepKind is what a step does.
type StepKind string

const (
	KindRun      StepKind = "run"
	KindApproval StepKind = "approval"
	KindSleep    StepKind = "sleep"
	KindWait     StepKind = "wait"
)

// Step is one node. Kind is run, approval, sleep or wait; decisions are ids ending in "?".
type Step struct {
	ID       string            `json:"id"`
	Kind     StepKind          `json:"kind,omitempty"`
	Next     string            `json:"next,omitempty"`
	Branches map[string]string `json:"branches,omitempty"`
	Label    string            `json:"label,omitempty"`
}

// IsDecision reports whether the step is a decision node.
func (s Step) IsDecision() bool { return strings.HasSuffix(s.ID, "?") }

// Problem is one validation finding with a JSON pointer into the graph file.
type Problem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (p Problem) String() string { return p.Path + ": " + p.Message }

// Problems is the error type returned by Parse.
type Problems []Problem

func (ps Problems) Error() string {
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = p.String()
	}
	return fmt.Sprintf("graph has %d problem(s): %s", len(ps), strings.Join(parts, "; "))
}

var compiled = func() *jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(contract.GraphSchema))
	if err != nil {
		panic("graph.schema.json: " + err.Error())
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("graph.schema.json", doc); err != nil {
		panic("graph.schema.json: " + err.Error())
	}
	s, err := c.Compile("graph.schema.json")
	if err != nil {
		panic("graph.schema.json: " + err.Error())
	}
	return s
}()

var printer = message.NewPrinter(language.English)

// Parse decodes and validates a graph file.
func Parse(src []byte) (Graph, error) {
	var raw any
	if err := yaml.Unmarshal(src, &raw); err != nil {
		return Graph{}, Problems{{Path: "", Message: "does not parse as YAML: " + err.Error()}}
	}
	if raw == nil {
		return Graph{}, Problems{{Path: "", Message: "the file is empty"}}
	}
	if err := compiled.Validate(raw); err != nil {
		if ve, ok := err.(*jsonschema.ValidationError); ok {
			return Graph{}, flatten(ve)
		}
		return Graph{}, Problems{{Path: "", Message: err.Error()}}
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return Graph{}, Problems{{Path: "", Message: err.Error()}}
	}
	var g Graph
	if err := json.Unmarshal(b, &g); err != nil {
		return Graph{}, Problems{{Path: "", Message: err.Error()}}
	}
	for i := range g.Steps {
		if g.Steps[i].Kind == "" {
			g.Steps[i].Kind = KindRun
		}
	}
	if ps := rules(g); ps != nil {
		return Graph{}, ps
	}
	return g, nil
}

func flatten(ve *jsonschema.ValidationError) Problems {
	var out Problems
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) > 0 {
			for _, c := range e.Causes {
				walk(c)
			}
			return
		}
		out = append(out, Problem{Path: "/" + strings.Join(e.InstanceLocation, "/"), Message: e.ErrorKind.LocalizedString(printer)})
	}
	walk(ve)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func rules(g Graph) Problems {
	var ps Problems
	add := func(path, msg string) { ps = append(ps, Problem{Path: path, Message: msg}) }

	index := map[string]int{}
	for i, s := range g.Steps {
		if j, dup := index[s.ID]; dup {
			add(fmt.Sprintf("/steps/%d/id", i), fmt.Sprintf("duplicate step id %s (also /steps/%d)", s.ID, j))
			continue
		}
		index[s.ID] = i
	}
	resolves := func(target string) bool {
		_, ok := index[target]
		return ok || target == End
	}
	for i, s := range g.Steps {
		if s.Next != "" && !resolves(s.Next) {
			add(fmt.Sprintf("/steps/%d/next", i), s.Next+" is not a step in this graph")
		}
		for _, label := range sortedLabels(s.Branches) {
			if t := s.Branches[label]; !resolves(t) {
				add(fmt.Sprintf("/steps/%d/branches/%s", i, label), t+" is not a step in this graph")
			}
		}
		if s.IsDecision() && s.Kind != KindRun {
			add(fmt.Sprintf("/steps/%d/kind", i), "a decision node has no kind")
		}
	}
	if len(ps) > 0 {
		return ps
	}
	for _, id := range unreachable(g) {
		add(fmt.Sprintf("/steps/%d", index[id]), id+" is not reachable from the first step")
	}
	if len(ps) == 0 {
		return nil
	}
	return ps
}

// Successors returns the ids a step can lead to, resolving the implicit "next step in the
// list" for steps without next or branches. End is excluded.
func (g Graph) Successors(i int) []string {
	s := g.Steps[i]
	var out []string
	switch {
	case s.IsDecision():
		for _, l := range sortedLabels(s.Branches) {
			out = append(out, s.Branches[l])
		}
	case s.Next != "":
		out = append(out, s.Next)
	case i+1 < len(g.Steps):
		out = append(out, g.Steps[i+1].ID)
	}
	return filter(out, func(id string) bool { return id != End })
}

func unreachable(g Graph) []string {
	index := map[string]int{}
	for i, s := range g.Steps {
		index[s.ID] = i
	}
	seen := map[string]bool{}
	var visit func(i int)
	visit = func(i int) {
		if seen[g.Steps[i].ID] {
			return
		}
		seen[g.Steps[i].ID] = true
		for _, id := range g.Successors(i) {
			visit(index[id])
		}
	}
	if len(g.Steps) > 0 {
		visit(0)
	}
	var out []string
	for _, s := range g.Steps {
		if !seen[s.ID] {
			out = append(out, s.ID)
		}
	}
	return out
}

// StepIDs returns the ids that must exist as step names in code: every step that is not a
// decision node.
func (g Graph) StepIDs() []string {
	var out []string
	for _, s := range g.Steps {
		if !s.IsDecision() {
			out = append(out, s.ID)
		}
	}
	return out
}

// Drift compares observed step names from runs with the declared graph. Unexpected are
// observed names not declared; neverRan are declared ids never observed. Names ending in
// "/request" belong to the approval step they prefix and are folded into it.
func (g Graph) Drift(observed []string) (unexpected, neverRan []string) {
	declared := map[string]bool{}
	for _, id := range g.StepIDs() {
		declared[id] = false
	}
	for _, name := range observed {
		name = strings.TrimSuffix(name, "/request")
		if _, ok := declared[name]; ok {
			declared[name] = true
		} else if !contains(unexpected, name) {
			unexpected = append(unexpected, name)
		}
	}
	for _, id := range g.StepIDs() {
		if !declared[id] {
			neverRan = append(neverRan, id)
		}
	}
	return unexpected, neverRan
}

func sortedLabels(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func filter(in []string, keep func(string) bool) []string {
	var out []string
	for _, s := range in {
		if keep(s) {
			out = append(out, s)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
