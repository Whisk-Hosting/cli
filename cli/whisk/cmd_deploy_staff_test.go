package whisk

import (
	"strings"
	"testing"

	"github.com/whisk-run/cli/doctor"
)

func TestStaffLine(t *testing.T) {
	if staffLine([]doctor.Finding{{Rule: "W140"}}) != "" {
		t.Error("no W180, no line")
	}
	got := staffLine([]doctor.Finding{{Rule: "W180", File: "src/admin.ts"}, {Rule: "W180", File: "app/staff.py"}})
	if !strings.Contains(got, "src/admin.ts, app/staff.py") || !strings.Contains(got, "W180") {
		t.Errorf("staffLine = %q", got)
	}
}
