package whisk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/apitypes"
)

// movePoll is how often whisk apps move reads how the move is going.
var movePoll = 3 * time.Second

// moveCmd moves an app to another business the person runs (CLI.md §5.2, CONTROL-PLANE.md §6.1
// "Moving an app"). Only a person, signed in, may start one, so from an agent's login it prints
// the link where the person does, as whisk apps delete does.
func moveCmd(s *session) *cobra.Command {
	var to string
	var noWait bool
	c := &cobra.Command{
		Use:   "move <app> --to <business>",
		Short: "Move an app, its data, files, keys and address, to another business you run",
		Long: `Moves the app (<app> in the current org, or <org>/<app>) to another business the same person
owns or administers: its code with history, production database, uploaded files with their ids,
secrets with their values, custom domains and address. The old address sends visitors on to the
new one, and the app in the old business is deleted once the new one is live (restorable for 7
days). Members and grants stay with the old business.

Only a person, signed in, moves an app: from an agent's login this prints the link where they do
(NEEDS_HUMAN, exit 2). Signed in as the person, it starts the move and follows it until it is done
or failed; --no-wait prints the move's page instead.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if to == "" {
				return output.New("INVALID_REQUEST", "Name the business the app moves to.", "Run whisk apps move <app> --to <business>, the business by its slug.", nil)
			}
			refs, err := parseAppRefs(args, s.org)
			if err != nil {
				return err
			}
			org := refs[0].org
			client, _, err := s.client()
			if err != nil {
				return err
			}
			a, err := client.GetApp(s.ctx, org, refs[0].app)
			if err != nil {
				return wrap(err)
			}
			m, err := client.MoveApp(s.ctx, org, a.ID, to)
			var ae *api.Error
			if errors.As(err, &ae) && refusedAsAgent(ae) {
				return needsHumanMove(s.dashboard(), org, a.Slug, a.ID, to)
			}
			if err != nil {
				return wrap(err)
			}
			page := movePage(s.dashboard(), org, a.ID)
			if !noWait {
				if m, err = followMove(s.ctx, client, org, a.ID, s.env.Stderr, movePoll); err != nil {
					return wrap(err)
				}
			}
			if m.Status == apitypes.AppMoveFailed {
				return moveFailed(m, page)
			}
			s.printer.Result(map[string]any{"org": org, "app": a.Slug, "move": m, "url": page}, func(w io.Writer) {
				for _, line := range moveLines(m, page) {
					fmt.Fprintln(w, line)
				}
			})
			return nil
		},
	}
	c.Flags().StringVar(&to, "to", "", "the business the app moves to, by slug")
	c.Flags().BoolVar(&noWait, "no-wait", false, "print the move's page instead of following it")
	return c
}

// refusedAsAgent reports whether the platform refused the move because the caller is not a person
// signed in: an agent token's limit or an agent identity's. Pure.
func refusedAsAgent(e *api.Error) bool {
	switch e.Code {
	case "FORBIDDEN_ROLE":
		token, _ := e.Details["token"].(string)
		return token != "" && token != "user"
	case "AGENT_CATEGORY":
		return e.Details["required"] == "human"
	}
	return false
}

// moveURL is the app's settings page where a person moves it, with the business picked. Pure.
func moveURL(dash, org, app, to string) string {
	return dash + "/o/" + url.PathEscape(org) + "/apps/" + url.PathEscape(app) + "/move?to=" + url.QueryEscape(to)
}

// movePage is the page that follows an app's move by the app's ID, which still answers once it
// moved. Pure.
func movePage(dash, org, appID string) string {
	return dash + "/o/" + url.PathEscape(org) + "/moves/" + url.PathEscape(appID)
}

// needsHumanMove is the block whisk apps move prints for an agent: the one link where a person
// moves the app. Pure.
func needsHumanMove(dash, org, app, id, to string) *output.Error {
	link := moveURL(dash, org, app, to)
	return &output.Error{
		Code:    "NEEDS_HUMAN",
		Message: fmt.Sprintf("Moving %s/%s to %s needs a person who runs both businesses, signed in.", org, app, to),
		Fix:     "Ask an owner or admin of both to open " + link + ", check the new address and confirm. whisk apps move " + org + "/" + app + " --to " + to + " run by them follows it.",
		Docs:    "https://skill.whisk.run/errors/NEEDS_HUMAN",
		Details: map[string]any{"url": link, "reason": "move_app", "org": org, "app": app, "id": id, "to": to},
	}
}

// moveEnded reports whether a move is over or stopped for a person. Pure.
func moveEnded(s apitypes.AppMoveStatus) bool {
	return s == apitypes.AppMoveDone || s == apitypes.AppMoveFailed || s == apitypes.AppMoveCancelled
}

// followMove reads the move until it ends, naming each new step on log.
func followMove(ctx context.Context, client *api.Client, org, appID string, log io.Writer, every time.Duration) (api.AppMove, error) {
	last := ""
	for {
		m, err := client.GetMove(ctx, org, appID)
		if err != nil {
			return m, err
		}
		if now := string(m.Status) + " " + string(m.Step); now != last {
			fmt.Fprintf(log, "move: %s, %s\n", m.Status, m.Step)
			last = now
		}
		if moveEnded(m.Status) {
			return m, nil
		}
		select {
		case <-ctx.Done():
			return m, ctx.Err()
		case <-time.After(every):
		}
	}
}

// moveFailed is the error a failed move recorded, with the page where a person retries or
// cancels it. Pure.
func moveFailed(m api.AppMove, page string) error {
	details := map[string]any{"move": m.ID, "step": string(m.Step), "url": page}
	if m.Error == nil {
		return output.New("PLATFORM_UNAVAILABLE", "The move stopped at its "+string(m.Step)+" step without saying why.",
			"Open "+page+" and try it again; it carries on from that step.", details)
	}
	for k, v := range m.Error.Details {
		details[k] = v
	}
	return &output.Error{Code: m.Error.Code, Message: m.Error.Message, Fix: m.Error.Fix + " The move's page: " + page + ".", Details: details}
}

// moveLines is what whisk apps move prints for a move that is going or done. Pure.
func moveLines(m api.AppMove, page string) []string {
	switch m.Status {
	case apitypes.AppMoveDone:
		out := []string{fmt.Sprintf("Moved %s/%s to %s/%s: %s", m.From, m.App, m.To, m.NewApp, m.URL),
			m.OldURL + " now sends visitors there. The app left in " + m.From + " is deleted and can be restored for 7 days."}
		if m.SecretsURL != "" {
			out = append(out, "Some keys still need a value: "+m.SecretsURL)
		}
		return out
	case apitypes.AppMoveCancelled:
		return []string{fmt.Sprintf("The move of %s/%s was cancelled; the app stays in %s.", m.From, m.App, m.From)}
	}
	return []string{fmt.Sprintf("Moving %s/%s to %s (%s, %s). Follow it at %s", m.From, m.App, m.To, m.Status, m.Step, page)}
}
