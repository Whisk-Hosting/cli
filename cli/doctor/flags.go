package doctor

import (
	"maps"
	"slices"
	"time"

	"github.com/whisk-run/contract/manifest"
)

// w150 warns about a release flag with no remove_by, or one whose day has passed (CONTRACT.md
// §3.3).
func w150(r Repo, _ Context) outcome { return w150On(r, time.Now().UTC().Format(time.DateOnly)) }

// w150On (pure) is w150 on the given day, as YYYY-MM-DD.
func w150On(r Repo, today string) outcome {
	var out outcome
	for _, name := range slices.Sorted(maps.Keys(r.Manifest.Flags)) {
		f := r.Manifest.Flags[name]
		if f.Kind != manifest.FlagRelease {
			continue
		}
		switch {
		case f.RemoveBy == "":
			out = out.add("W150", manifestFile, yamlLine(r.ManifestNode, "/flags/"+name),
				"The release flag "+name+" has no remove_by, so nothing says when it should be gone.")
		case f.RemoveBy < today:
			out = out.add("W150", manifestFile, yamlLine(r.ManifestNode, "/flags/"+name+"/remove_by"),
				"The release flag "+name+" was to be gone by "+f.RemoveBy+".")
		}
	}
	return out
}
