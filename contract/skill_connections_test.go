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
	full := FullSkill()
	for _, name := range append(connect.FunctionNames(), connect.ReferenceNames()...) {
		if !strings.Contains(full, "`"+name) {
			t.Errorf("the skill never names %s in backticks; list it with the connection references and functions in connections.md", name)
		}
	}
}

// TestSkillCitesNoPrivateDocument keeps the public skill from sending an agent to a document
// only Whisk can read: docs/ is private, so its pages are never cited.
func TestSkillCitesNoPrivateDocument(t *testing.T) {
	private := regexp.MustCompile(`\b(ARCHITECTURE|BROKER|CADDY|CLI|COMMERCE-ENGINE|CONTENT-ENGINE|CONTRACT|CONTROL-PLANE|DASHBOARD|DEPLOYMENT|DESIGN|ENGINES|FORKS|HARNESS|INFRA|MANAGED-APPS|NODE-AGENT|SHOP-B2B)\.md\b`)
	if found := private.FindAllString(FullSkill(), -1); len(found) > 0 {
		t.Errorf("the skill cites %v, which agents cannot open; say what they need in the skill", found)
	}
}
