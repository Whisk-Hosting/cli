package contract

import (
	"strings"
	"testing"
)

// TestSkillQuickStartDeploysFirst holds SKILL.md to getting the person an address before the app
// is built: the quick start comes before everything else, signs in before it deploys, and
// deploys the starter before it builds.
func TestSkillQuickStartDeploysFirst(t *testing.T) {
	start := strings.Index(Skill, "## Quick start")
	first := strings.Index(Skill, "## 1. What Whisk is")
	if start < 0 || first < 0 || start > first {
		t.Fatalf("SKILL.md must open with ## Quick start before ## 1")
	}
	qs := Skill[start:first]
	steps := []string{"whisk login", "whisk init --template", "whisk deploy", "Build what they asked for"}
	at := 0
	for _, s := range steps {
		i := strings.Index(qs[at:], s)
		if i < 0 {
			t.Fatalf("the quick start must name %q after the steps before it", s)
		}
		at += i
	}
}
