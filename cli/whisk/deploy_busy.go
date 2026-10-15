package whisk

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/status"
)

// deployStallLimit is how long a deploy may stay queued or building before whisk deploy calls
// it stuck rather than waiting on it: the build's own limit (BUILD_TIMEOUT, 20 minutes) and
// a margin. The platform ends a deploy nothing is working on well before this
// (CONTROL-PLANE.md §6.5); this is what the CLI does when one is still there.
const deployStallLimit = 25 * time.Minute

// phaseSince is when the deploy entered its current status: its last recorded phase, or its
// creation when none is recorded.
func phaseSince(d api.Deploy) time.Time {
	if n := len(d.Phases); n > 0 && !d.Phases[n-1].At.IsZero() {
		return d.Phases[n-1].At
	}
	return d.CreatedAt
}

// stalled reports whether a deploy has sat queued or building past deployStallLimit, and for
// how long it has been in that status.
func stalled(d api.Deploy, now time.Time) (time.Duration, bool) {
	if d.Status != status.DeployQueued && d.Status != status.DeployBuilding {
		return 0, false
	}
	since := phaseSince(d)
	if since.IsZero() {
		return 0, false
	}
	waited := now.Sub(since)
	return waited, waited >= deployStallLimit
}

// stuckDeploy is the deploy in environment that has sat queued or building the longest past
// deployStallLimit, if any.
func stuckDeploy(deploys []api.Deploy, environment string, now time.Time) (api.Deploy, time.Duration, bool) {
	var found api.Deploy
	var longest time.Duration
	ok := false
	for _, d := range deploys {
		if environment != "" && d.Environment != environment {
			continue
		}
		if waited, is := stalled(d, now); is && waited > longest {
			found, longest, ok = d, waited, true
		}
	}
	return found, longest, ok
}

// spanWords is a duration as an agent reads it: "1 minute", "47 minutes", "3 hours".
func spanWords(d time.Duration) string {
	unit := func(n int, one string) string {
		if n == 1 {
			return "1 " + one
		}
		return fmt.Sprintf("%d %ss", n, one)
	}
	switch {
	case d < time.Minute:
		return unit(int(d.Seconds()), "second")
	case d < 2*time.Hour:
		return unit(int(d.Minutes()), "minute")
	case d < 48*time.Hour:
		return unit(int(d.Hours()), "hour")
	}
	return unit(int(d.Hours()/24), "day")
}

// deployInProgress is DEPLOY_IN_PROGRESS: deploy d has sat queued or building for waited, and
// whisk deploy will not wait on it. lead, when set, opens the sentence with what happened
// first (a push that lost its connection); extra details are kept.
func deployInProgress(d api.Deploy, waited time.Duration, lead string, extra map[string]any) *output.Error {
	details := map[string]any{"deploy_id": d.ID, "status": d.Status, "environment": d.Environment, "since": phaseSince(d).UTC().Format(time.RFC3339), "waited_seconds": int(waited.Seconds())}
	if d.CommitSHA != "" {
		details["commit_sha"] = d.CommitSHA
	}
	for k, v := range extra {
		details[k] = v
	}
	what := fmt.Sprintf("deploy %s has been %s for %s without finishing", d.ID, d.Status, spanWords(waited))
	var msg string
	if lead != "" {
		msg = lead + ", and " + what + "."
	} else {
		msg = "D" + what[1:] + "."
	}
	fix := fmt.Sprintf("Wait for it with whisk deploys info %s, or cancel it with whisk deploys cancel %s and run whisk deploy again.", d.ID, d.ID)
	return output.New("DEPLOY_IN_PROGRESS", msg, fix, details)
}

// pushFailed explains a push that lost its connection. When the environment has a deploy
// stuck queued or building, that is what the agent can act on, so the answer is
// DEPLOY_IN_PROGRESS naming it, with git's lines kept; otherwise the push error stands. A
// listing that fails leaves the push error as it was.
func (s *session) pushFailed(client *api.Client, org, app, environment string, err error) error {
	var e *output.Error
	if !errors.As(err, &e) || e.Code != "PLATFORM_UNAVAILABLE" || e.Details["git"] == nil {
		return err
	}
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	deploys, lerr := client.ListDeploys(ctx, org, app, 20)
	if lerr != nil {
		return err
	}
	d, waited, ok := stuckDeploy(deploys, environment, s.now())
	if !ok {
		return err
	}
	return deployInProgress(d, waited, "git push lost its connection to the platform", map[string]any{"git": e.Details["git"], "summary": e.Details["summary"]})
}
