package contract

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestSkillFilesAreNamedAndWhole holds SKILL.md and contract/skill/ to each other: every file
// there is named by a section's last line and in the table at the top, every named file exists,
// a pointer's link and name agree, and each file opens with its title and one line saying what
// it is, so FullSkill can put its body back.
func TestSkillFilesAreNamedAndWhole(t *testing.T) {
	onDisk, err := fs.Glob(skillFiles, "skill/*.md")
	if err != nil || len(onDisk) == 0 {
		t.Fatalf("contract/skill holds no files: %v", err)
	}
	named := map[string]bool{}
	for _, m := range skillPointer.FindAllStringSubmatch(Skill, -1) {
		if m[1] != m[2] {
			t.Errorf("SKILL.md names %s but links %s", m[1], m[2])
		}
		named[m[1]] = true
	}
	for _, path := range onDisk {
		name := strings.TrimPrefix(path, "skill/")
		if !named[name] {
			t.Errorf("%s is not named by any section of SKILL.md; end the section it continues with its More in line", name)
		}
		if !strings.Contains(Skill, "| `"+name+"` |") {
			t.Errorf("%s is missing from the table at the top of SKILL.md", name)
		}
	}
	for _, f := range SkillFiles() {
		if f.Text == "" {
			t.Errorf("SKILL.md names %s, which contract/skill does not hold", f.Name)
			continue
		}
		lines := strings.SplitN(f.Text, "\n", 5)
		if len(lines) < 5 || !strings.HasPrefix(lines[0], "# Whisk skill §") || lines[1] != "" || lines[2] == "" || lines[3] != "" || skillFileBody(f.Text) == "" {
			t.Errorf("%s must open with '# Whisk skill §N: title', a blank line, one line saying what it is and a blank line", f.Name)
		}
	}
}

// TestFullSkillHoldsEverything: the whole skill has every file's body and no More in line left.
func TestFullSkillHoldsEverything(t *testing.T) {
	full := FullSkill()
	if skillPointer.MatchString(full) {
		t.Fatal("FullSkill left a More in line")
	}
	for _, f := range SkillFiles() {
		if !strings.Contains(full, skillFileBody(f.Text)) {
			t.Errorf("FullSkill is missing %s", f.Name)
		}
	}
}

// coreBudget is the most SKILL.md may hold. Every agent reads all of it before its first line
// of code, so what only some apps need goes in a file of its own.
const coreBudget = 64 << 10

// TestSkillCoreStaysSmall keeps SKILL.md to what every app needs.
func TestSkillCoreStaysSmall(t *testing.T) {
	if len(Skill) > coreBudget {
		t.Fatalf("SKILL.md is %d bytes, over its %d; move what only some apps need into a file in contract/skill and end its section with a More in line", len(Skill), coreBudget)
	}
}

// yamlKeys answers every key path in a whisk.yaml: top-level keys and the keys of the maps
// under them, and of maps inside lists (a function's or a webhook's fields). Pure.
func yamlKeys(prefix string, v any, out map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		for k, sub := range x {
			out[prefix+k] = true
			if prefix == "" {
				yamlKeys(k+".", sub, out)
			}
		}
	case []any:
		for _, item := range x {
			if m, ok := item.(map[string]any); ok {
				for k := range m {
					out[prefix+k] = true
				}
			}
		}
	}
}

// TestSkillCoreShipsEveryStarter is the one-page test: an agent that has read SKILL.md alone,
// and none of its files, meets every key the starters' whisk.yaml use, so it can make one of
// them live and change it without fetching anything more.
func TestSkillCoreShipsEveryStarter(t *testing.T) {
	starters := []string{"typescript", "python", "go", "next", "astro"}
	for _, name := range starters {
		raw, err := os.ReadFile(filepath.Join("..", "templates", name, "whisk.yaml"))
		if err != nil {
			t.Fatalf("templates/%s/whisk.yaml: %v", name, err)
		}
		var doc map[string]any
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("templates/%s/whisk.yaml: %v", name, err)
		}
		keys := map[string]bool{}
		yamlKeys("", doc, keys)
		for key := range keys {
			leaf := key[strings.LastIndex(key, ".")+1:]
			if !keyWritten(leaf).MatchString(Skill) {
				t.Errorf("templates/%s uses %s, which SKILL.md never names; keep it on the page every agent reads", name, key)
			}
		}
	}
}
