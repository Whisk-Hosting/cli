package whisk

import (
	"encoding/json"
	"strings"
	"testing"
)

// Arguments a command refuses name the command and show how to run it (CLI.md §7): whisk clone
// with no app says it wants <org>/<app>, not only how many arguments it takes.
func TestRefusedArgumentsShowTheUsage(t *testing.T) {
	r := runCLI(t, t.TempDir(), map[string]string{"WHISK_NO_TELEMETRY": "1"}, nil, "clone", "--json")
	var body struct {
		Error struct {
			Code, Message, Fix string
			Details            map[string]any
		}
	}
	if err := json.Unmarshal([]byte(r.Stdout), &body); err != nil {
		t.Fatalf("stdout is not JSON: %q", r.Stdout)
	}
	e := body.Error
	if r.Code != 1 || e.Code != "CLI_ERROR" || !strings.HasPrefix(e.Message, "whisk clone accepts between 1 and 2 arg(s)") ||
		!strings.Contains(e.Fix, "whisk clone <org>/<app> [directory]") || !strings.Contains(e.Fix, "whisk clone --help") || e.Details["usage"] == nil {
		t.Errorf("code %d, %+v", r.Code, e)
	}
}
