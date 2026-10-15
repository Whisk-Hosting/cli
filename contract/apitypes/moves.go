package apitypes

import (
	"time"

	werrors "github.com/whisk-run/contract/errors"
)

// Moving an app to another business the same person runs (CONTROL-PLANE.md §6.1 "Moving an
// app"): its code, production database, files, secrets and address go; its members and grants
// stay with the business it leaves.

// AppMoveStatus is where a move is. A failed move is retried or cancelled; cancelled deletes the
// half-made app and leaves the app where it was.
type AppMoveStatus string

const (
	AppMoveQueued    AppMoveStatus = "queued"
	AppMoveRunning   AppMoveStatus = "running"
	AppMoveDone      AppMoveStatus = "done"
	AppMoveFailed    AppMoveStatus = "failed"
	AppMoveCancelled AppMoveStatus = "cancelled"
)

// AppMoveStep is the step a move is on, or done. Switch is the one step that changes the app
// that moves: its uploads and addresses go to the new app and it is deleted.
type AppMoveStep string

const (
	MoveStepApp      AppMoveStep = "app"
	MoveStepSecrets  AppMoveStep = "secrets"
	MoveStepCode     AppMoveStep = "code"
	MoveStepFiles    AppMoveStep = "files"
	MoveStepDatabase AppMoveStep = "database"
	MoveStepSwitch   AppMoveStep = "switch"
	MoveStepDone     AppMoveStep = "done"
)

// AppMove is one move: the app that moves (App, AppID) from the business From, the business To it
// goes to, and once made the new app there (NewApp) at URL. OldURL is the address it had, which
// sends visitors on to URL once the move is done. SecretsURL is the new app's secrets page while a
// name it reads has no value.
type AppMove struct {
	ID         string          `json:"id"`
	From       string          `json:"from"`
	App        string          `json:"app"`
	AppID      string          `json:"app_id"`
	OldURL     string          `json:"old_url"`
	To         string          `json:"to"`
	ToName     string          `json:"to_name"`
	NewApp     string          `json:"new_app,omitempty"`
	URL        string          `json:"url,omitempty"`
	MadeBy     string          `json:"made_by"`
	Status     AppMoveStatus   `json:"status"`
	Step       AppMoveStep     `json:"step"`
	Error      *werrors.Detail `json:"error,omitempty"`
	SecretsURL string          `json:"secrets_url,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	DoneAt     *time.Time      `json:"done_at,omitempty"`
}

// AppMoveRequest is POST /orgs/:org/apps/:app/move: ToOrg is the business it goes to, by slug.
type AppMoveRequest struct {
	ToOrg string `json:"to_org"`
}

// AppMoveTarget is a business the person runs that the app could move to: the app's slug and
// address there, and the refusal when it cannot go.
type AppMoveTarget struct {
	Org     string          `json:"org"`
	Name    string          `json:"name"`
	App     string          `json:"app"`
	URL     string          `json:"url"`
	Refusal *werrors.Detail `json:"refusal,omitempty"`
}

// AppMoveTargets is GET /orgs/:org/apps/:app/move/targets: every other business the person is an
// owner or admin of, by name.
type AppMoveTargets struct {
	Items []AppMoveTarget `json:"items"`
}

// The move's types join the wire types the TypeScript is written from.
func init() {
	wire = append(wire, AppMove{}, AppMoveRequest{}, AppMoveTarget{}, AppMoveTargets{})
	t, e := enumOf("AppMoveStatus", "APP_MOVE_STATUSES", AppMoveQueued, AppMoveRunning, AppMoveDone, AppMoveFailed, AppMoveCancelled)
	enums[t] = e
	t, e = enumOf("AppMoveStep", "APP_MOVE_STEPS", MoveStepApp, MoveStepSecrets, MoveStepCode, MoveStepFiles, MoveStepDatabase, MoveStepSwitch, MoveStepDone)
	enums[t] = e
}
