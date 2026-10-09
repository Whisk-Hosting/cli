package whisk

import (
	"testing"

	werrors "github.com/whisk-run/contract/errors"

	"github.com/whisk-run/cli/internal/api"
)

// A listing says why a run or deploy failed and whether it was Whisk's side.
func TestWhyNamesWhoseSide(t *testing.T) {
	platform := &werrors.Detail{Code: "PLATFORM_RUN_INTERRUPTED", Details: map[string]any{"fault": "platform"}}
	app := &werrors.Detail{Code: "RUN_STEP_FAILED", Details: map[string]any{"fault": "app"}}
	cases := map[string]struct {
		got, want string
	}{
		"interrupted and re-run": {runWhy(api.Run{Error: platform, RerunRunID: "01NEW"}), "PLATFORM_RUN_INTERRUPTED (Whisk's side), re-run as 01NEW"},
		"the app's":              {runWhy(api.Run{Error: app}), "RUN_STEP_FAILED"},
		"a re-run":               {runWhy(api.Run{RerunOf: "01OLD"}), "re-run of 01OLD"},
		"fine":                   {runWhy(api.Run{}), ""},
		"deploy on Whisk's side": {deployWhy(&werrors.Detail{Code: "PLATFORM_DEPLOY_FAILED"}), "PLATFORM_DEPLOY_FAILED (Whisk's side)"},
		"deploy on the app's":    {deployWhy(&werrors.Detail{Code: "MIGRATE_FAILED", Details: map[string]any{"fault": "app"}}), "MIGRATE_FAILED"},
	}
	for name, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: %q, want %q", name, tc.got, tc.want)
		}
	}
}
