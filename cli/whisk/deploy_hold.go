package whisk

import (
	"time"

	"github.com/whisk-run/contract/apitypes"

	"github.com/whisk-run/cli/internal/api"
)

// An update of a fork whose rehearsal timed its migration as short switches with the app's
// requests waiting at the edge, so nothing is written in between (CONTROL-PLANE.md §6.5). The
// deploy carries how that went, which whisk deploys info, whisk managed update and a failed
// deploy say.

// followed is what the deploy a command follows did to the app's data and its requests, for its
// summary once it is live: a rollback's data (whisk rollback --data) and an update's switch
// (whisk managed update), each into result too. Nothing for any other deploy.
func (s *session) followed(client *api.Client, org, app, deployID string, result map[string]any) []string {
	if !s.followsData && s.forkUpdate == nil {
		return nil
	}
	d, err := client.GetDeploy(s.ctx, org, app, deployID)
	if err != nil {
		return nil
	}
	var lines []string
	if d.Data != nil {
		result["data"] = d.Data
		lines = append(lines, dataLines(d.Data)...)
	}
	if d.Hold != nil {
		result["hold"] = d.Hold
		lines = append(lines, holdLines(d.Hold)...)
	}
	return lines
}

// unsettled reports whether a failed deploy is still putting back what it changed: a rollback's
// data, or a held update's. Pure.
func unsettled(d api.Deploy) bool {
	return (d.Data != nil && d.Data.EndedAt == nil) || (d.Hold != nil && d.Hold.EndedAt == nil)
}

// holdLines is how an update switched with the app's requests held, for a person: how long they
// waited, or, for an update that did not go live, what the version that was live answers on.
// Pure.
func holdLines(h *apitypes.UpdateHold) []string {
	if h == nil {
		return nil
	}
	switch h.State {
	case apitypes.UpdateHoldHeld:
		return []string{"switch: the app's requests wait while the update switches"}
	case apitypes.UpdateHoldSwitched:
		return []string{"switch: the app's requests waited " + waited(h) + " while the update switched, so nothing was written in between"}
	case apitypes.UpdateHoldPutBack:
		point := ""
		if h.PointAt != nil {
			point = " at " + when(h.PointAt)
		}
		return []string{"switch: the update did not go live, so its data was put back as it was" + point + ", and the version that was live answers again"}
	case apitypes.UpdateHoldUnchanged:
		return []string{"switch: the update did not go live and changed no data, and the version that was live answers again"}
	case apitypes.UpdateHoldReleased:
		return []string{"switch: the update went on while the version that was live served"}
	}
	return []string{"switch: " + string(h.State)}
}

// waited is how long a held update's requests waited, in words: from its hold to its end, a
// second at the least. Pure.
func waited(h *apitypes.UpdateHold) string {
	if h.EndedAt == nil {
		return "until it ends"
	}
	return spanWords(max(h.EndedAt.Sub(h.StartedAt), time.Second))
}
