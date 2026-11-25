package whisk

import (
	"errors"
	"testing"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
)

// whisk deploys log with no id reads the app's latest deploy, and says so when there is none.
func TestLatestDeployID(t *testing.T) {
	if id, err := latestDeployID([]api.Deploy{{ID: "d2"}, {ID: "d1"}}); err != nil || id != "d2" {
		t.Errorf("latest = %q, %v; want d2", id, err)
	}
	_, err := latestDeployID(nil)
	var e *output.Error
	if !errors.As(err, &e) || e.Code != "NOT_FOUND" {
		t.Errorf("no deploys gave %v, want NOT_FOUND", err)
	}
}
