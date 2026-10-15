package output

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/whisk-run/contract/errors"
)

// codeLiteral matches the ways the CLI names an error code in source: output.New("X", ...),
// &output.Error{Code: "X"}, e.Code = "X", and the exit-code table's keys.
var codeLiteral = regexp.MustCompile(`(?:output\.New\(|\bNew\(|Code:\s*|\.Code\s*=\s*|^\s*)"([A-Z][A-Z0-9_]{3,})"`)

// codesIn returns the error codes a Go source names, sorted and without repeats.
func codesIn(src string) []string {
	seen := map[string]bool{}
	for _, line := range strings.Split(src, "\n") {
		for _, m := range codeLiteral.FindAllStringSubmatch(line, -1) {
			seen[m[1]] = true
		}
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

func TestCodesIn(t *testing.T) {
	cases := []struct {
		src  string
		want []string
	}{
		{`return output.New("DOCTOR_FAILED", msg, fix, nil)`, []string{"DOCTOR_FAILED"}},
		{`e := &Error{Code: "CLI_ERROR", Message: m}`, []string{"CLI_ERROR"}},
		{`e.Code = "DEPLOY_FAILED"`, []string{"DEPLOY_FAILED"}},
		{"\t\"MIGRATE_FAILED\":   ExitDeploy,", []string{"MIGRATE_FAILED"}},
		{`fmt.Println("HELLO world")`, nil},
		{`x := "OK"`, nil},
	}
	for _, c := range cases {
		got := codesIn(c.src)
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%q: got %v, want %v", c.src, got, c.want)
		}
	}
}

// Every code the CLI can print has a section in contract/errors.md, so its docs link resolves
// and an agent can look it up (CLAUDE.md: new codes go in the catalogue in the same change).
func TestEveryCLICodeCatalogued(t *testing.T) {
	root := filepath.Join("..", "..")
	var missing []string
	found := map[string]bool{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "templates" || d.Name() == "testdata") {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, code := range codesIn(string(src)) {
			found[code] = true
			if _, ok := errors.Lookup(code); !ok {
				missing = append(missing, p+": "+code)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !found["DOCTOR_FAILED"] || !found["LOCAL_DOCKER_UNAVAILABLE"] || len(found) < 30 {
		t.Fatalf("the scan found only %d codes; it no longer reads the CLI's sources", len(found))
	}
	if len(missing) > 0 {
		t.Errorf("codes with no section in contract/errors.md:\n%s", strings.Join(missing, "\n"))
	}
}
