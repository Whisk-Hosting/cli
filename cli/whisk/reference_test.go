package whisk

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The public build only: the operator's build adds commands the public reference leaves out.
// The public CLI reference (reference.json, which /docs/cli draws) is the command tree as it is.
// After changing a command, write it again: WHISK_UPDATE_REFERENCE=1 go test ./whisk -run TestReferenceIsTheCommandTree
func TestReferenceIsTheCommandTree(t *testing.T) {
	ref := Reference(newRoot(&session{env: Env{}}))
	got, err := json.MarshalIndent(ref, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	if os.Getenv("WHISK_UPDATE_REFERENCE") != "" {
		if err := os.WriteFile("../reference.json", got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile("../reference.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("cli/reference.json is not the command tree; write it again with WHISK_UPDATE_REFERENCE=1 go test ./whisk -run TestReferenceIsTheCommandTree")
	}
	for _, g := range ref {
		if g.Name == "operator" {
			t.Fatal("the public reference lists the operator's commands")
		}
		for _, c := range g.Commands {
			if !strings.HasPrefix(c.Usage, "whisk ") || strings.TrimSpace(c.Short) == "" {
				t.Errorf("%q has no usage or no words: %+v", g.Name, c)
			}
		}
	}
}
