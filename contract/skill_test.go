package contract

import (
	"encoding/json"
	"regexp"
	"sort"
	"testing"
)

// manifestKeys answers every key whisk.yaml takes that an agent writes by hand: the top-level
// properties, the properties of the top-level objects, and the fields of a function and a
// webhook. The fields of hmac settings are a preset's, documented in webhook-presets.yaml.
func manifestKeys(t *testing.T) []string {
	t.Helper()
	type node struct {
		Properties map[string]node `json:"properties"`
	}
	var schema struct {
		node
		Defs map[string]node `json:"$defs"`
	}
	if err := json.Unmarshal(ManifestSchema, &schema); err != nil {
		t.Fatalf("whisk.schema.json: %v", err)
	}
	keys := []string{}
	for name, prop := range schema.Properties {
		keys = append(keys, name)
		for sub := range prop.Properties {
			keys = append(keys, name+"."+sub)
		}
	}
	for _, def := range []string{"function", "webhook"} {
		for sub := range schema.Defs[def].Properties {
			keys = append(keys, def+"."+sub)
		}
	}
	sort.Strings(keys)
	return keys
}

// keyWritten matches a manifest key written as YAML or in backticks.
func keyWritten(leaf string) *regexp.Regexp {
	return regexp.MustCompile("(?m)(^|[\\s{,`.-])" + regexp.QuoteMeta(leaf) + "(:|`)")
}

// TestSkillNamesEveryManifestKey holds the skill to CONTRACT.md §12: an agent learns every
// feature from the skill, so every key the manifest takes appears in SKILL.md or one of its
// files, written as YAML or in backticks.
func TestSkillNamesEveryManifestKey(t *testing.T) {
	full := FullSkill()
	for _, key := range manifestKeys(t) {
		leaf := regexp.MustCompile(`[^.]+$`).FindString(key)
		if !keyWritten(leaf).MatchString(full) {
			t.Errorf("the skill never names %s; add it where an agent would look for it", key)
		}
	}
}
