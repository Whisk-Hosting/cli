package endpoints

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/whisk-run/contract/apitypes"
)

func TestURL(t *testing.T) {
	r := New[None, None]("GET /v1/orgs/{org}/apps/{app}")
	cases := []struct {
		values []string
		want   string
		err    bool
	}{
		{[]string{"acme", "crm"}, "/v1/orgs/acme/apps/crm", false},
		{[]string{"a b", "x/y"}, "/v1/orgs/a%20b/apps/x%2Fy", false},
		{[]string{".", ".."}, "/v1/orgs/%2E/apps/%2E%2E", false},
		{[]string{"acme"}, "", true},
		{[]string{"acme", "crm", "extra"}, "", true},
	}
	for _, c := range cases {
		got, err := r.URL(c.values...)
		if (err != nil) != c.err || got != c.want {
			t.Errorf("URL(%q) = %q, %v; want %q, error %v", c.values, got, err, c.want, c.err)
		}
	}
}

type query struct {
	User  string   `query:"user"`
	Limit int      `query:"limit"`
	Tags  []string `query:"tag"`
	Skip  string   `query:"-"`
	Plain string
}

func TestQuery(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{query{}, ""},
		{query{User: "u 1"}, "?user=u+1"},
		{query{Limit: 5, Tags: []string{"a", "b"}, Skip: "x", Plain: "y"}, "?limit=5&tag=a&tag=b"},
		{&query{User: "u"}, "?user=u"},
		{None{}, ""},
		{"not a struct", ""},
	}
	for _, c := range cases {
		if got := Query(c.in); got != c.want {
			t.Errorf("Query(%+v) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := QueryNames(reflect.TypeFor[query]()); !reflect.DeepEqual(got, []string{"limit", "tag", "user"}) {
		t.Errorf("QueryNames = %v", got)
	}
}

type body struct {
	Name string `json:"name"`
}

type sent struct {
	method, path string
	in           any
}

// record is a Doer that keeps what it was asked and answers with answer.
func record(got *sent, answer string) Doer {
	return func(_ context.Context, method, path string, in, out any) error {
		*got = sent{method, path, in}
		if out == nil {
			return nil
		}
		return json.Unmarshal([]byte(answer), out)
	}
}

func TestSend(t *testing.T) {
	ctx := context.Background()
	var got sent

	post := New[body, body]("POST /v1/orgs/{org}/things")
	out, err := Send(ctx, record(&got, `{"name":"back"}`), post, body{Name: "new"}, "acme")
	if err != nil || out.Name != "back" {
		t.Fatalf("Send = %+v, %v", out, err)
	}
	if got.method != "POST" || got.path != "/v1/orgs/acme/things" || got.in != (body{Name: "new"}) {
		t.Errorf("a body route sends its input as the body: %+v", got)
	}

	get := New[query, body]("GET /v1/things")
	if _, err := Send(ctx, record(&got, `{}`), get, query{User: "u"}); err != nil {
		t.Fatal(err)
	}
	if got.path != "/v1/things?user=u" || got.in != nil {
		t.Errorf("a GET sends its input as the query: %+v", got)
	}

	del := New[None, None]("DELETE /v1/things/{id}")
	if _, err := Call(ctx, record(&got, `not json`), del, "t1"); err != nil {
		t.Errorf("an answer of None is not read: %v", err)
	}
	if got.in != nil || got.path != "/v1/things/t1" {
		t.Errorf("None sends no body: %+v", got)
	}

	if _, err := Call(ctx, record(&got, `{}`), del); err == nil {
		t.Errorf("a missing path value is an error, not a request")
	}
}

func TestNewRefusesAMalformedRoute(t *testing.T) {
	for name, declare := range map[string]func(){
		"no method":        func() { New[None, None]("/v1/x") },
		"not below /v1":    func() { New[None, None]("GET /x") },
		"a body on a GET":  func() { New[body, None]("GET /v1/x") },
		"a body on a HEAD": func() { New[body, None]("HEAD /v1/x") },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: New did not panic", name)
				}
			}()
			declare()
		}()
	}
}

// Every public route is declared once.
func TestTable(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range Public() {
		p := s.Pattern()
		if seen[p] {
			t.Errorf("%s is declared twice", p)
		}
		seen[p] = true
	}
	if len(seen) == 0 {
		t.Fatal("no routes are declared")
	}
}

func TestExecAndAll(t *testing.T) {
	ctx := context.Background()
	var got sent
	del := New[None, None]("DELETE /v1/things/{id}")
	if err := Exec(ctx, record(&got, ``), del, "t1"); err != nil || got.path != "/v1/things/t1" {
		t.Errorf("Exec sent %+v: %v", got, err)
	}

	type listQuery struct {
		Cursor string `query:"cursor"`
		Limit  int    `query:"limit"`
	}
	list := New[listQuery, apitypes.Page[string]]("GET /v1/things")
	var paths []string
	pages := map[string]string{
		"/v1/things?limit=100":           `{"items":["a","b"],"next_cursor":"c2"}`,
		"/v1/things?cursor=c2&limit=100": `{"items":["c"],"next_cursor":""}`,
	}
	do := func(_ context.Context, _, path string, _, out any) error {
		paths = append(paths, path)
		return json.Unmarshal([]byte(pages[path]), out)
	}
	items, err := All(ctx, do, list, listQuery{})
	if err != nil || strings.Join(items, ",") != "a,b,c" || len(paths) != 2 {
		t.Errorf("All = %v, %v after %v", items, err, paths)
	}

	once := New[None, apitypes.Page[string]]("GET /v1/once")
	pages["/v1/once"] = `{"items":["x"],"next_cursor":""}`
	if items, err := All(ctx, do, once, None{}); err != nil || strings.Join(items, ",") != "x" {
		t.Errorf("a list with no cursor is one page: %v, %v", items, err)
	}
	pages["/v1/once"] = `{"items":["x"],"next_cursor":"c2"}`
	if _, err := All(ctx, do, once, None{}); err == nil {
		t.Errorf("a list with no cursor that answers a next page is an error, not a short list")
	}
}
