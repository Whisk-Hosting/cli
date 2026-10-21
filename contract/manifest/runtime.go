package manifest

import (
	"maps"
	"slices"
)

// File is the manifest's path in an app's repository.
const File = "whisk.yaml"

// SameBuild reports whether two manifests ask the build for the same thing: the same Dockerfile,
// build secrets, env, static folders and trade ordering. Everything else in a manifest is a
// run-time setting the platform applies to an image already built, so a commit whose only change
// is to those settings runs the image the earlier commit built (CONTRACT.md §3.1 "Settings that
// apply without a build"). The order of build secrets and of static folders does not matter.
// Pure.
func SameBuild(a, b Manifest) bool {
	return a.Build.Dockerfile == b.Build.Dockerfile &&
		slices.Equal(sortedSet(a.Build.Secrets), sortedSet(b.Build.Secrets)) &&
		maps.Equal(a.Env, b.Env) &&
		slices.Equal(sortedSet(staticDirs(a.Static)), sortedSet(staticDirs(b.Static))) &&
		a.B2B == b.B2B
}

// SettingsOnly reports whether a commit can run the image built from an earlier one: changed,
// the paths that differ between the two commits, names nothing but the manifest, and the two
// manifests ask the build for the same thing (SameBuild). Pure.
func SettingsOnly(changed []string, before, after Manifest) bool {
	return !slices.ContainsFunc(changed, func(p string) bool { return p != File }) && SameBuild(before, after)
}

func staticDirs(s []Static) []string {
	out := make([]string, 0, len(s))
	for _, st := range s {
		out = append(out, st.Dir)
	}
	return out
}

func sortedSet(in []string) []string {
	out := slices.Clone(in)
	slices.Sort(out)
	return slices.Compact(out)
}
