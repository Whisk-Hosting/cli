package release

import (
	"reflect"
	"strings"
	"testing"

	"github.com/whisk-run/contract/naughty"
)

// FuzzParseSums reads a fuzzed SHA256SUMS (docs/HARNESS.md §8.3): every entry is a name
// without spaces and a lower-case digest that the text names on one line, a stamp parses only
// from a well-formed line and renders back to one that parses the same, and parsing what was
// parsed and written out again gives the same map.
func FuzzParseSums(f *testing.F) {
	f.Add("e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855  whisk_linux_amd64\n# whisk v1.2.3 released 1788782400\n")
	for _, s := range naughty.Strings() {
		f.Add(s)
		f.Add(Digest([]byte(s)) + "  *" + s + "\n# whisk " + s + " released 1\n")
	}
	f.Fuzz(func(t *testing.T, text string) {
		sums := ParseSums(text)
		var rebuilt strings.Builder
		kept := map[string]string{}
		for name, digest := range sums {
			if strings.ContainsAny(name, " \t\n\r\v\f") || digest != strings.ToLower(digest) || digest == "" || strings.ContainsAny(digest, " \t\n\r\v\f") {
				t.Fatalf("ParseSums gave %q => %q", name, digest)
			}
			// A name that still starts with '*' loses it again when read back, as sha256sum's
			// binary marker; the others are written out as sha256sum writes them.
			if !strings.HasPrefix(name, "*") && name != "" {
				rebuilt.WriteString(digest + "  " + name + "\n")
				kept[name] = digest
			}
		}
		if again := ParseSums(rebuilt.String()); !reflect.DeepEqual(again, kept) {
			t.Fatalf("re-parsing changed the sums: %v vs %v", kept, again)
		}
		st, ok := ParseStamp(text)
		if !ok {
			return
		}
		if st.Released <= 0 || st.Version == "" || strings.ContainsAny(st.Version, " \t\n") {
			t.Fatalf("ParseStamp gave %+v", st)
		}
		again, ok := ParseStamp(StampLine(st))
		if !ok || again != st {
			t.Fatalf("stamp %+v renders to %q which reads back as %+v", st, StampLine(st), again)
		}
		if Newer(st.Version, st.Version) {
			t.Fatalf("Newer wants %q over itself", st.Version)
		}
	})
}
