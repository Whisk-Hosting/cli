package apitypes

import (
	"encoding/json"
	"time"

	werrors "github.com/whisk-run/contract/errors"
	"github.com/whisk-run/contract/graph"
)

// Workflows (CONTROL-PLANE.md §6.9): an app's functions, their runs and the approvals runs wait
// on.

// List wraps a list the API answers whole, with no cursor.
type List[T any] struct {
	Items []T `json:"items"`
}

// Run is one run of one of an app's functions. App and AppID are set on org-wide listings and
// details. Status is the engine's word, lower-cased (queued, running, completed, failed,
// cancelled and whatever else it reports), so it is not a closed set. Trigger is what started
// the run: the event's bare name, "cron" or "replay". Manual marks a run of a cron function
// someone started by hand (whisk cron run), and Whisk's re-run of one. StepDetails carries each
// step on a run's own page; listings leave it out. Error says why a failed run failed, its
// details.fault whose side (CONTRACT.md §10). Parked explains the guard that parked the run's
// function. RerunRunID is the run Whisk started after it interrupted this one; RerunOf marks a
// run that is itself such a re-run.
type Run struct {
	ID          string          `json:"id"`
	App         string          `json:"app,omitempty"`
	AppID       string          `json:"app_id,omitempty"`
	Function    string          `json:"function"`
	Status      string          `json:"status"`
	Trigger     string          `json:"trigger,omitempty"`
	Manual      bool            `json:"manual,omitempty"`
	Steps       []string        `json:"steps"`
	StepDetails []RunStep       `json:"step_details,omitempty"`
	EventID     string          `json:"event_id,omitempty"`
	QueuedAt    *time.Time      `json:"queued_at,omitempty"`
	StartedAt   *time.Time      `json:"started_at,omitempty"`
	EndedAt     *time.Time      `json:"ended_at,omitempty"`
	DurationMS  int64           `json:"duration_ms,omitempty"`
	Output      any             `json:"output,omitempty"`
	Error       *werrors.Detail `json:"error,omitempty"`
	Attempts    int             `json:"attempts,omitempty"`
	Parked      *Parked         `json:"parked,omitempty"`
	RerunRunID  string          `json:"rerun_run_id,omitempty"`
	RerunOf     string          `json:"rerun_of,omitempty"`
}

// RunStep is one step of a run with what it took and produced. Input and output are JSON,
// truncated at 10 KB, which the truncated flags say. Status is the engine's word, lower-cased.
type RunStep struct {
	Name            string          `json:"name"`
	Status          string          `json:"status"`
	StartedAt       *time.Time      `json:"started_at,omitempty"`
	EndedAt         *time.Time      `json:"ended_at,omitempty"`
	DurationMS      int64           `json:"duration_ms,omitempty"`
	Retries         int             `json:"retries"`
	Input           json.RawMessage `json:"input,omitempty"`
	Output          json.RawMessage `json:"output,omitempty"`
	InputTruncated  bool            `json:"input_truncated,omitempty"`
	OutputTruncated bool            `json:"output_truncated,omitempty"`
	Error           *werrors.Detail `json:"error,omitempty"`
}

// Parked explains the guard that parked a run's function.
type Parked struct {
	Guard   string `json:"guard"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
}

// RunsHold explains why an org's event-triggered runs are not starting.
type RunsHold struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Fix        string `json:"fix"`
	HeldEvents int    `json:"held_events"`
}

// RunList is one page of runs, failures first, with the hold on the org when there is one.
type RunList struct {
	Items      []Run     `json:"items"`
	NextCursor string    `json:"next_cursor"`
	Total      int64     `json:"total"`
	Hold       *RunsHold `json:"hold,omitempty"`
}

// WorkflowFunction is a declared function with what its runs have done.
type WorkflowFunction struct {
	Name     string    `json:"name"`
	Triggers []Trigger `json:"triggers"`
	Declared []string  `json:"declared_steps"`
	Observed []string  `json:"observed_steps"`
	Drift    Drift     `json:"drift"`
	Ran      bool      `json:"ran"`
}

// Trigger is what starts a function, with the event name the app wrote. A cron trigger carries
// the next time it is due. On a plan that spaces scheduled functions out (cron_min_minutes),
// RunsAs is the schedule it really runs on when that differs from the one the app wrote, and
// NextRun follows it.
type Trigger struct {
	Event   string     `json:"event,omitempty"`
	Cron    string     `json:"cron,omitempty"`
	RunsAs  string     `json:"runs_as,omitempty"`
	TZ      string     `json:"tz,omitempty"`
	NextRun *time.Time `json:"next_run,omitempty"`
}

// Drift is the difference between the declared graph and what ran.
type Drift struct {
	Unexpected []string `json:"unexpected"`
	NeverRan   []string `json:"never_ran"`
}

// FunctionGraph is GET /orgs/:org/apps/:app/functions/:name/graph: the function, the paths its
// runs took, and the declared graph when it has one (CONTROL-PLANE.md §8).
type FunctionGraph struct {
	Function WorkflowFunction `json:"function"`
	Paths    [][]string       `json:"paths"`
	Graph    *graph.Graph     `json:"graph,omitempty"`
}

// ApprovalStatus is where a decision a workflow waits on stands.
type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "pending"
	ApprovalApproved ApprovalStatus = "approved"
	ApprovalRejected ApprovalStatus = "rejected"
	ApprovalExpired  ApprovalStatus = "expired"
)

// Approval is one decision a workflow is waiting on. Org and App are the slugs of the business
// and app it belongs to, so a decided approval links to its run. Body is Markdown the workflow
// supplied, empty when it sent none, rendered without scripts or raw HTML.
type Approval struct {
	ID        string          `json:"id"`
	Org       string          `json:"org"`
	App       string          `json:"app"`
	AppID     string          `json:"app_id"`
	RunID     string          `json:"run_id"`
	To        string          `json:"to"`
	Title     string          `json:"title"`
	Body      string          `json:"body"`
	Data      json.RawMessage `json:"data,omitempty"`
	Status    ApprovalStatus  `json:"status"`
	DecidedBy string          `json:"decided_by,omitempty"`
	DecidedAt *time.Time      `json:"decided_at,omitempty"`
	Note      string          `json:"note,omitempty"`
	ExpiresAt time.Time       `json:"expires_at"`
	CreatedAt time.Time       `json:"created_at"`
	URL       string          `json:"url"`
}
