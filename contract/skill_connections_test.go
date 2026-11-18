package contract

import (
	"regexp"
	"strings"
	"testing"

	"github.com/whisk-run/contract/connect"
)

// TestSkillNamesEveryTemplateFunction holds the skill to connect's templates: an agent writing a
// connection recipe reads every reference and function there, since the skill is all it has.
func TestSkillNamesEveryTemplateFunction(t *testing.T) {
	for _, name := range append(connect.FunctionNames(), connect.ReferenceNames()...) {
		if !strings.Contains(Skill, "`"+name) {
			t.Errorf("SKILL.md never names %s in backticks; list it with the connection references and functions", name)
		}
	}
}

// TestSkillCitesNoPrivateDocument keeps the public skill from sending an agent to a document
// only Whisk can read: docs/ is private, so its pages are never cited.
func TestSkillCitesNoPrivateDocument(t *testing.T) {
	private := regexp.MustCompile(`\b(ARCHITECTURE|BROKER|CADDY|CLI|COMMERCE-ENGINE|CONTENT-ENGINE|CONTRACT|CONTROL-PLANE|DASHBOARD|DEPLOYMENT|DESIGN|FORKS|HARNESS|INFRA|MANAGED-APPS|NODE-AGENT|SHOP-B2B)\.md\b`)
	if found := private.FindAllString(Skill, -1); len(found) > 0 {
		t.Errorf("SKILL.md cites %v, which agents cannot open; say what they need in the skill", found)
	}
}
