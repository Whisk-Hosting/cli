package endpoints

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite testdata/wire.json from the declarations")

// The public routes' wire format, field by field, is testdata/wire.json. A change to any route's
// input or answer, or to a type either uses, changes the document, so it is made on purpose:
// go test ./endpoints -run TestWireIsCurrent -update rewrites it, and the diff is the change.
func TestWireIsCurrent(t *testing.T) {
	got, err := json.MarshalIndent(Wire(All()), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "wire.json")
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v; run go test ./endpoints -run TestWireIsCurrent -update", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s is not what the declarations send and answer. If the change to the wire is meant, run go test ./endpoints -run TestWireIsCurrent -update and commit it", path)
	}
}

type inner struct {
	N int `json:"n"`
}

type Embedded struct {
	E string `json:"e"`
}

type kind string

type wireBody struct {
	Embedded
	Name   string            `json:"name"`
	Tags   []string          `json:"tags,omitempty"`
	Labels map[string]string `json:"labels"`
	Inner  *inner            `json:"inner"`
	At     time.Time         `json:"at"`
	Kind   kind              `json:"kind"`
	Count  int64             `json:"count,string"`
	Skip   string            `json:"-"`
	hidden string
}

var _ = wireBody{hidden: "never sent"}

func TestSchemaOf(t *testing.T) {
	defs := map[string]any{}
	got := schemaOf(reflect.TypeFor[wireBody](), defs)
	if got["$ref"] != "#/$defs/endpoints.wireBody" {
		t.Fatalf("a named struct is a reference: %v", got)
	}
	body := defs["endpoints.wireBody"].(map[string]any)
	props := body["properties"].(map[string]any)
	want := []string{"at", "count", "e", "inner", "kind", "labels", "name", "tags"}
	if len(props) != len(want) {
		t.Fatalf("fields: got %v, want %v", props, want)
	}
	for _, k := range want {
		if props[k] == nil {
			t.Errorf("%s is missing", k)
		}
	}
	if r := body["required"].([]string); len(r) != 7 {
		t.Errorf("every field but the omitempty one is required: %v", r)
	}
	if props["kind"].(map[string]any)["x-type"] != "endpoints.kind" {
		t.Errorf("a named string keeps its name: %v", props["kind"])
	}
	if props["at"].(map[string]any)["format"] != "date-time" {
		t.Errorf("a time is a date-time string: %v", props["at"])
	}
	if props["count"].(map[string]any)["type"] != "string" {
		t.Errorf(",string sends a number as a string: %v", props["count"])
	}
	if _, ok := defs["endpoints.inner"]; !ok {
		t.Errorf("a nested struct is its own definition")
	}
}
