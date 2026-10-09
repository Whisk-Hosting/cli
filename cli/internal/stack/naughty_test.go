package stack

import (
	"encoding/json"
	"testing"

	"github.com/whisk-run/contract/naughty"
)

// Detection reads file names and manifests from the repository; with naughty ones it answers
// only the values it documents.
func TestNaughtyDetect(t *testing.T) {
	migrates := map[string]bool{"": true, "npm run migrate": true, "npx prisma migrate deploy": true, "npx drizzle-kit migrate": true, "alembic upgrade head": true, "python manage.py migrate": true}
	langs := map[Lang]bool{JS: true, PY: true, GO: true}
	for _, s := range naughty.Strings() {
		pkg, _ := json.Marshal(map[string]any{"dependencies": map[string]string{s: s}, "scripts": map[string]string{"migrate": s, s: s}})
		files := []string{s, "package.json", "pyproject.toml", "go.mod", s + "/" + s}
		contents := map[string][]byte{"package.json": pkg, "pyproject.toml": []byte(s), "go.mod": []byte(s)}
		for _, read := range []func(string) []byte{func(p string) []byte { return contents[p] }, func(string) []byte { return []byte(s) }} {
			st := Detect(files, read)
			if !migrates[st.Migrate] {
				t.Errorf("Detect with %q guessed migrate %q", s, st.Migrate)
			}
			if !langs[st.Primary] {
				t.Errorf("Detect with %q gave primary %q", s, st.Primary)
			}
		}
		if l := LangOf(s); l != "" && !langs[l] {
			t.Errorf("LangOf(%q) = %q", s, l)
		}
		IsTest(s)
		IsVendored(s)
	}
}
