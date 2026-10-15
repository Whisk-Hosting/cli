package release

import (
	"strings"
	"testing"

	"github.com/whisk-run/contract/naughty"
)

// SHA256SUMS and its stamp come from the download server; whatever they hold, parsing answers
// names and digests of the documented shape, and a stamp renders and parses back.
func TestNaughtySums(t *testing.T) {
	kp, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range naughty.Strings() {
		text := s + "\n" + Digest([]byte(s)) + "  " + s + "\n# whisk " + s + " released 1788782400\n"
		for name, digest := range ParseSums(text) {
			if name == "" || strings.ContainsAny(name, " \t\n") || digest != strings.ToLower(digest) {
				t.Errorf("ParseSums(%q) gave %q => %q", text, name, digest)
			}
		}
		if f := strings.Fields(s); len(f) == 1 && f[0] == s {
			st, ok := ParseStamp(StampLine(Stamp{Version: s, Released: 1788782400}))
			if !ok || st.Version != s || st.Released != 1788782400 {
				t.Errorf("stamp for version %q read back as %+v, %v", s, st, ok)
			}
		}
		if st, ok := ParseStamp(s); ok && (st.Released <= 0 || st.Version == "") {
			t.Errorf("ParseStamp(%q) = %+v", s, st)
		}
		if Newer(s, s) || Newer(s, "") {
			t.Errorf("Newer(%q) wants an update to itself or to nothing", s)
		}
		sig, err := Sign(kp.Private, []byte(s))
		if err != nil {
			t.Fatal(err)
		}
		if ok, err := Verify(kp.Public, []byte(s), sig); !ok || err != nil {
			t.Errorf("signature over %q did not verify: %v", s, err)
		}
		if ok, _ := Verify(kp.Public, []byte(s+"x"), sig); ok {
			t.Errorf("signature over %q verified other data", s)
		}
		if ok, _ := Verify(kp.Public, []byte(s), s); ok {
			t.Errorf("signature %q verified", s)
		}
		if _, err := Sign(s, []byte(s)); err == nil {
			t.Errorf("Sign accepted the key %q", s)
		}
		if _, err := PublicKey(s); err == nil {
			t.Errorf("PublicKey accepted %q", s)
		}
		if strings.Contains(AssetName(s, s), "/") && !strings.Contains(s, "/") {
			t.Errorf("AssetName(%q) added a slash", s)
		}
	}
}
