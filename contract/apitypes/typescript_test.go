package apitypes

import (
	"encoding/json"
	"errors"
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite the dashboard's generated wire types")

// dashboardTypes is the dashboard's copy of TypeScript(), committed so the dashboard builds
// without Go. contract/ is published without the dashboard beside it; there the check has
// nothing to compare.
const dashboardTypes = "../../dashboard/src/lib/api/wire.gen.ts"

func TestDashboardWireTypesAreCurrent(t *testing.T) {
	want, err := TypeScript()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(dashboardTypes)); errors.Is(err, fs.ErrNotExist) {
		t.Skip("no dashboard beside contract/ (the published copy)")
	}
	if *update {
		if err := os.WriteFile(dashboardTypes, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(dashboardTypes)
	if err != nil || string(got) != want {
		t.Fatalf("%s is stale: run (cd contract && go test ./apitypes -update)", dashboardTypes)
	}
}

// sources is every file of the package but the generator and its tests: the wire types, their
// enums and their constants.
func sources(t *testing.T) []*ast.File {
	t.Helper()
	pkgs, err := parser.ParseDir(token.NewFileSet(), ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go") && fi.Name() != "typescript.go"
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var out []*ast.File
	for _, p := range pkgs {
		for _, f := range p.Files {
			out = append(out, f)
		}
	}
	return out
}

// Every constant the package declares is among the values the generator writes, in order, so a
// new value reaches the dashboard's union.
func TestEnumsListEveryConstant(t *testing.T) {
	declared := map[string][]string{}
	for _, file := range sources(t) {
		for _, d := range file.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.CONST {
				continue
			}
			for _, s := range g.Specs {
				v := s.(*ast.ValueSpec)
				typ := v.Type.(*ast.Ident).Name
				lit, err := strconv.Unquote(v.Values[0].(*ast.BasicLit).Value)
				if err != nil {
					t.Fatal(err)
				}
				declared[typ] = append(declared[typ], lit)
			}
		}
	}
	if len(declared) == 0 {
		t.Fatal("the package declares no constants")
	}
	for typ, values := range declared {
		var listed *enumSet
		for rt, e := range enums {
			if rt.PkgPath() == reflect.TypeFor[Role]().PkgPath() && rt.Name() == typ {
				listed = &e
			}
		}
		if listed == nil {
			t.Errorf("%s is not in enums", typ)
			continue
		}
		if !slices.Equal(listed.values, values) {
			t.Errorf("enums lists %s as %v; enums.go declares %v", typ, listed.values, values)
		}
	}
}

type colour string

type sample struct {
	Name     string            `json:"name"`
	Nick     string            `json:"nick,omitempty"`
	Count    int64             `json:"count"`
	Colour   colour            `json:"colour"`
	When     time.Time         `json:"when"`
	Ends     *time.Time        `json:"ends"`
	Starts   *time.Time        `json:"starts,omitempty"`
	Tags     []string          `json:"tags"`
	Shades   []colour          `json:"shades,omitempty"`
	Extra    map[string]any    `json:"extra"`
	Rows     []map[string]any  `json:"rows"`
	Any      any               `json:"any,omitempty"`
	Inner    struct{ A bool }  `json:"inner"`
	Child    *child            `json:"child,omitempty"`
	Children []child           `json:"children"`
	Skipped  string            `json:"-"`
	Counts   map[string]uint16 `json:"counts"`
	Raw      json.RawMessage   `json:"raw"`
	Due      time.Time         `json:"due,omitzero"`
	Pairs    [][2]float64      `json:"pairs"`
}

type child struct {
	ID string `json:"id"`
}

type bare struct {
	Free string `json:"free"`
}

type loose struct {
	C colour
}

func TestRender(t *testing.T) {
	colours := map[reflect.Type]enumSet{reflect.TypeFor[colour](): {name: "Colour", list: "COLOURS", values: []string{"red", "blue"}}}
	got, err := render([]any{sample{}, child{}}, colours)
	if err != nil {
		t.Fatal(err)
	}
	want := header + `
export const COLOURS = ["red", "blue"] as const;
export type Colour = (typeof COLOURS)[number];

export type sample = {
  name: string;
  nick?: string;
  count: number;
  colour: Colour;
  when: string;
  ends: string | null;
  starts?: string;
  tags: string[];
  shades?: Colour[];
  extra: Record<string, unknown>;
  rows: Array<Record<string, unknown>>;
  any?: unknown;
  inner: { A: boolean; };
  child?: child;
  children: child[];
  counts: Record<string, number>;
  raw: unknown;
  due?: string;
  pairs: Array<[number, number]>;
};

export type child = {
  id: string;
};
`
	if got != want {
		t.Errorf("render:\n%s\nwant:\n%s", got, want)
	}

	for _, c := range []struct {
		name  string
		types []any
		enums map[reflect.Type]enumSet
		err   string
	}{
		{"a struct not written out", []any{sample{}}, colours, "child is not among the types written out"},
		{"a named string with no values", []any{loose{}}, nil, "colour is a named string with no values listed"},
		{"everything listed", []any{bare{}}, nil, ""},
	} {
		_, err := render(c.types, c.enums)
		if c.err == "" && err != nil || c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.err)
		}
	}
}

func TestGenericsTakeTheirParameter(t *testing.T) {
	got, err := render([]any{generic{name: "Page", param: "T", sample: Page[typeParam]{}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "export type Page<T> = {\n  items: T[];\n  next_cursor: string;\n};") {
		t.Errorf("Page rendered as\n%s", got)
	}
}

// Every struct the package declares is written out, so none reaches the dashboard by hand. The
// named strings are TestEnumsListEveryConstant's.
func TestEveryWireTypeIsWritten(t *testing.T) {
	written := map[string]bool{}
	for _, w := range wire {
		switch v := w.(type) {
		case generic:
			written[v.name] = true
		case named:
		default:
			written[reflect.TypeOf(v).Name()] = true
		}
	}
	for _, file := range sources(t) {
		for _, d := range file.Decls {
			g, ok := d.(*ast.GenDecl)
			if !ok || g.Tok != token.TYPE {
				continue
			}
			for _, s := range g.Specs {
				ts := s.(*ast.TypeSpec)
				if id, ok := ts.Type.(*ast.Ident); ok && id.Name == "string" {
					continue
				}
				if !written[ts.Name.Name] {
					t.Errorf("%s is not in wire", ts.Name.Name)
				}
			}
		}
	}
}
