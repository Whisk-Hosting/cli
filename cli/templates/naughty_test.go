package templates

import (
	"testing"

	"github.com/whisk-run/cli/scaffold"
	"github.com/whisk-run/contract/manifest"
	"github.com/whisk-run/contract/naughty"
)

// Each template's manifest renamed to any slug CheckSlug accepts parses as that app, and a
// naughty template name is refused without writing anything.
func TestNaughtyTemplates(t *testing.T) {
	dir := t.TempDir()
	for _, s := range naughty.Strings() {
		if _, err := Write(dir, s, "job-tracker"); err == nil {
			t.Errorf("Write accepted the template name %q", s)
		}
		if scaffold.CheckSlug(s) != nil {
			continue
		}
		for _, name := range List() {
			tf, _ := Files(name)
			m, err := manifest.Parse([]byte(RenameManifest(tf["whisk.yaml"].Content, s)))
			if err != nil || m.Name != s {
				t.Errorf("template %s renamed to %q: %v", name, s, err)
			}
		}
	}
}
