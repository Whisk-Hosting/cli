// Package endpoints is the Whisk API's routes with what each one takes and answers
// (CONTROL-PLANE.md §5): one typed declaration per route, which the control plane registers its
// handler under, and the CLI and the harness call through Call and Send instead of a hand-written
// method. A route's input and answer are the wire types of contract/apitypes, so the server, its
// clients and the API description cannot disagree about a route's shape. Whisk's own routes are
// declared the same way in its private code, which is never public.
package endpoints

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/whisk-run/contract/apitypes"
)

// None is a route's input when it takes no body and no query, or its answer when it answers
// with no body.
type None struct{}

// Route is one API route: its method and path, with the input In it takes and the answer Out it
// gives. An In whose fields are tagged `query:"name"` is sent as the query; any other as the
// JSON body, which a GET or HEAD route never has. Path holds {name} for each path value, in order.
type Route[In, Out any] struct {
	Method string
	Path   string
}

// New declares a route from its pattern, "METHOD /v1/path". It panics on a malformed pattern, so
// a mistake stops the program at start rather than at the first call.
func New[In, Out any](pattern string) Route[In, Out] {
	method, path, ok := strings.Cut(pattern, " ")
	if !ok || method == "" || !strings.HasPrefix(path, "/v1/") {
		panic("endpoints: a route's pattern is \"METHOD /v1/path\", got " + strconv.Quote(pattern))
	}
	if q := reflect.TypeFor[In](); !hasBody(method) && !hasQuery(q) && q != reflect.TypeFor[None]() {
		panic("endpoints: " + pattern + " takes " + q.String() + ", and a GET or HEAD sends no body: tag its fields query")
	}
	return Route[In, Out]{Method: method, Path: path}
}

// Pattern is the route as the router matches it: "METHOD /v1/path".
func (r Route[In, Out]) Pattern() string { return r.Method + " " + r.Path }

// Wire is the route's input and answer types.
func (r Route[In, Out]) Wire() (in, out reflect.Type) {
	return reflect.TypeFor[In](), reflect.TypeFor[Out]()
}

// URL is the route's path with each {name} filled, in order, by one of values, escaped so a
// value is always exactly one segment. A count of values that does not match is an error. Pure.
func (r Route[In, Out]) URL(values ...string) (string, error) {
	return fill(r.Path, values)
}

// Spec is any route, whatever it takes and answers: what a table of routes holds.
type Spec interface {
	Pattern() string
	Wire() (in, out reflect.Type)
}

// Doer performs one JSON request: in is nil when the route sends no body, and out is nil when its
// answer is not read. A client's own Do is one, with its credential, retries and error type.
type Doer func(ctx context.Context, method, path string, in, out any) error

// Call sends a route that takes no input and returns its answer.
func Call[Out any](ctx context.Context, do Doer, r Route[None, Out], values ...string) (Out, error) {
	return Send(ctx, do, r, None{}, values...)
}

// Send sends a route with its input and returns its answer.
func Send[In, Out any](ctx context.Context, do Doer, r Route[In, Out], in In, values ...string) (Out, error) {
	var out Out
	path, err := r.URL(values...)
	if err != nil {
		return out, err
	}
	var body any
	switch t := reflect.TypeFor[In](); {
	case t == reflect.TypeFor[None]():
	case hasQuery(t):
		path += Query(in)
	default:
		body = in
	}
	var into any = &out
	if reflect.TypeFor[Out]() == reflect.TypeFor[None]() {
		into = nil
	}
	return out, do(ctx, r.Method, path, body, into)
}

// Query is in's fields tagged `query:"name"` as a query string, with its leading "?", or "" when
// every one is empty. A slice is sent as one value per item. Pure.
func Query(in any) string {
	v := reflect.Indirect(reflect.ValueOf(in))
	if v.Kind() != reflect.Struct {
		return ""
	}
	q := url.Values{}
	for i := range v.NumField() {
		name, ok := v.Type().Field(i).Tag.Lookup("query")
		if !ok || name == "-" || v.Field(i).IsZero() {
			continue
		}
		q[name] = queryValues(v.Field(i))
	}
	if len(q) == 0 {
		return ""
	}
	return "?" + q.Encode()
}

// queryValues is one field's values as the query carries them. Pure.
func queryValues(f reflect.Value) []string {
	if f.Kind() == reflect.Slice {
		out := make([]string, f.Len())
		for i := range out {
			out[i] = fmt.Sprint(f.Index(i).Interface())
		}
		return out
	}
	return []string{fmt.Sprint(f.Interface())}
}

// QueryNames is the query parameter names a type's fields are tagged with, sorted. Pure.
func QueryNames(t reflect.Type) []string {
	var out []string
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	for i := range t.NumField() {
		if name, ok := t.Field(i).Tag.Lookup("query"); ok && name != "-" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return slices.Compact(out)
}

// hasBody reports whether a method carries its input as a body rather than a query.
func hasBody(method string) bool {
	return method != http.MethodGet && method != http.MethodHead
}

// hasQuery reports whether a type has a field tagged for the query.
func hasQuery(t reflect.Type) bool { return len(QueryNames(t)) > 0 }

// fill puts values into a path's {name} segments in order. "." and ".." are escaped too, so a
// client or server that resolves dot segments cannot climb out of the segment. Pure.
func fill(path string, values []string) (string, error) {
	segs := strings.Split(path, "/")
	n := 0
	for i, s := range segs {
		if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
			continue
		}
		if n == len(values) {
			return "", fmt.Errorf("endpoints: %s needs a value for %s, and only %d were given", path, s, len(values))
		}
		segs[i] = Segment(values[n])
		n++
	}
	if n != len(values) {
		return "", fmt.Errorf("endpoints: %s takes %d values, and %d were given", path, n, len(values))
	}
	return strings.Join(segs, "/"), nil
}

// Segment escapes a value for one segment of an API path, "." and ".." included. Pure.
func Segment(s string) string {
	switch s {
	case ".":
		return "%2E"
	case "..":
		return "%2E%2E"
	}
	return url.PathEscape(s)
}

// Table is a set of route declarations that files add to as they declare them (Add), so no one
// list is edited by every change. Its zero value is empty and ready.
type Table struct {
	mu    sync.Mutex
	specs []Spec
}

// Add puts routes in the table. It returns nothing worth keeping; a file calls it as
// `var _ = table.Add(...)` beside its declarations.
func (t *Table) Add(specs ...Spec) struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.specs = append(t.specs, specs...)
	return struct{}{}
}

// Sorted is every route in the table, sorted by pattern.
func (t *Table) Sorted() []Spec {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := slices.Clone(t.specs)
	slices.SortFunc(out, func(a, b Spec) int { return strings.Compare(a.Pattern(), b.Pattern()) })
	return out
}

// public is every public route, each file adding its own.
var public = &Table{}

// Public is every public route declared here, sorted by pattern.
func Public() []Spec { return public.Sorted() }

// Exec sends a route that takes no input and answers with no body.
func Exec(ctx context.Context, do Doer, r Route[None, None], values ...string) error {
	_, err := Send(ctx, do, r, None{}, values...)
	return err
}

// All sends a list route page by page, from in's first page to the end, and returns every item.
// in's query field "cursor" carries each next page's cursor, and its "limit", when it has one
// and in leaves it 0, asks for 100 at a time. A list that takes no cursor is one page.
func All[In, T any](ctx context.Context, do Doer, r Route[In, apitypes.Page[T]], in In, values ...string) ([]T, error) {
	all := []T{}
	q := reflect.ValueOf(&in).Elem()
	cursor, limit := queryField(q, "cursor"), queryField(q, "limit")
	if !cursor.IsValid() {
		// A list that takes no cursor answers in one page.
		page, err := Send(ctx, do, r, in, values...)
		switch {
		case err != nil:
			return nil, err
		case page.NextCursor != "":
			return nil, fmt.Errorf("endpoints: %s answered a next page and takes no cursor to ask for it", r.Pattern())
		}
		return append(all, page.Items...), nil
	}
	if limit.IsValid() && limit.CanInt() && limit.Int() == 0 {
		limit.SetInt(100)
	}
	for {
		page, err := Send(ctx, do, r, in, values...)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Items...)
		if page.NextCursor == "" {
			return all, nil
		}
		cursor.SetString(page.NextCursor)
	}
}

// queryField is the field of a struct value tagged `query:"name"`, invalid when it has none.
func queryField(v reflect.Value, name string) reflect.Value {
	if v.Kind() != reflect.Struct {
		return reflect.Value{}
	}
	for i := range v.NumField() {
		if v.Type().Field(i).Tag.Get("query") == name {
			return v.Field(i)
		}
	}
	return reflect.Value{}
}
