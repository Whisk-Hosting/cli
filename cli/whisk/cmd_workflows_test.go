package whisk

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/whisk-run/cli/internal/output"
)

func TestEventData(t *testing.T) {
	files := map[string][]byte{
		"plain.json": []byte(`{"id":42}`),
		"bom.json":   append([]byte{0xEF, 0xBB, 0xBF}, `{"id":42}`...),
		// What Windows PowerShell 5.1 writes for '{"id":42}' > utf16.json.
		"utf16.json":   {0xFF, 0xFE, '{', 0, '"', 0, 'i', 0, 'd', 0, '"', 0, ':', 0, '4', 0, '2', 0, '}', 0, '\r', 0, '\n', 0},
		"utf16be.json": {0xFE, 0xFF, 0, '{', 0, '"', 0, 'i', 0, 'd', 0, '"', 0, ':', 0, '4', 0, '2', 0, '}'},
		"array.json":   []byte(`[1,2]`),
	}
	read := func(path string) ([]byte, error) {
		if b, ok := files[path]; ok {
			return b, nil
		}
		return nil, errors.New("no such file")
	}
	want := map[string]any{"id": float64(42)}
	cases := []struct {
		name, data, file string
		want             map[string]any
		code             string
	}{
		{name: "nothing", want: map[string]any{}},
		{name: "inline", data: `{"id":42}`, want: want},
		{name: "inline not JSON", data: `{id:42}`, code: "INVALID_REQUEST"},
		{name: "file", file: "plain.json", want: want},
		{name: "at file", data: "@plain.json", want: want},
		{name: "utf-8 bom", file: "bom.json", want: want},
		{name: "utf-16le from powershell", file: "utf16.json", want: want},
		{name: "utf-16be", data: "@utf16be.json", want: want},
		{name: "not an object", file: "array.json", code: "INVALID_REQUEST"},
		{name: "missing file", file: "gone.json", code: "INVALID_REQUEST"},
		{name: "both", data: `{"id":1}`, file: "plain.json", code: "INVALID_REQUEST"},
		{name: "at file and file", data: "@plain.json", file: "plain.json", code: "INVALID_REQUEST"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := eventData(c.data, c.file, read)
			if c.code != "" {
				var oe *output.Error
				if !errors.As(err, &oe) || oe.Code != c.code || oe.Fix == "" {
					t.Fatalf("want %s, got %v", c.code, err)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v, %v; want %v", got, err, c.want)
			}
		})
	}
}

func TestEventsSendDataFile(t *testing.T) {
	st := &remoteState{}
	srv := remoteAPI(t, st)
	defer srv.Close()
	dir := boundDir(t, srv.URL)
	must(t, os.WriteFile(filepath.Join(dir, "call.json"), []byte(`{"call_id":"c-1","tags":["x"]}`), 0o644))

	for _, args := range [][]string{
		{"events", "send", "call.ended", "--data-file", "call.json", "--json"},
		{"events", "send", "call.ended", "--data", "@call.json", "--json"},
	} {
		r := runRemote(t, dir, srv.URL, nil, args...)
		if r.Code != 0 || r.JSON["name"] != "call.ended" {
			t.Fatalf("%v: %+v", args, r)
		}
	}
	want := map[string]any{"call_id": "c-1", "tags": []any{"x"}}
	if len(st.events) != 2 || !reflect.DeepEqual(st.events[0]["data"], want) || !reflect.DeepEqual(st.events[1]["data"], want) {
		t.Fatalf("events: %v", st.events)
	}

	r := runRemote(t, dir, srv.URL, nil, "events", "send", "call.ended", "--data-file", "missing.json", "--json")
	if r.Code != 1 || r.JSON["error"].(map[string]any)["code"] != "INVALID_REQUEST" || len(st.events) != 2 {
		t.Fatalf("missing file: %+v", r)
	}
}
