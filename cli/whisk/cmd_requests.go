package whisk

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/apitypes"
)

// Changes an agent asks a person to approve (CLI.md §5.11, CONTROL-PLANE.md §4.9): an invite,
// a removal or who can open an app, run with an agent token, is kept by the API until a person
// approves it on its page.

// requestPoll is how often a wait reads a request again.
var requestPoll = 3 * time.Second

// askedPerson is the answer while a person has not decided: the page to show them, exit 2.
func askedPerson(p api.PersonRequest) *output.Error {
	return &output.Error{
		Code:    "NEEDS_HUMAN",
		Message: "A person approves this before it happens: " + strings.Join(p.Summary, "; ") + ".",
		Fix: fmt.Sprintf("Show the person this link and ask them to approve it, signed in: %s. It waits 24 hours. Then run whisk requests show %s, or add --wait to wait for them.",
			p.URL, p.ID),
		Docs:    "https://skill.whisk.run/errors/NEEDS_HUMAN",
		Details: map[string]any{"url": p.URL, "request_id": p.ID, "summary": p.Summary, "expires_at": p.ExpiresAt},
	}
}

// requestOutcome is what a decided request means for the command: nil once approved, the
// change's own refusal once failed, REQUEST_CLOSED once declined or expired, and NEEDS_HUMAN
// while pending. Pure.
func requestOutcome(p api.PersonRequest) error {
	switch p.Status {
	case apitypes.PersonRequestApproved:
		return nil
	case apitypes.PersonRequestFailed:
		if p.Error != nil {
			return &output.Error{Code: p.Error.Code, Message: p.Error.Message, Fix: p.Error.Fix, Docs: p.Error.Docs, Details: p.Error.Details}
		}
		return output.New("PLATFORM_UNAVAILABLE", "The change was approved but could not be made.", "Run the command again for a new request.", map[string]any{"request_id": p.ID})
	case apitypes.PersonRequestDeclined, apitypes.PersonRequestExpired:
		why := "was declined"
		if p.Status == apitypes.PersonRequestExpired {
			why = "waited 24 hours without an answer"
		}
		return output.New("REQUEST_CLOSED", "This request "+why+", so nothing changed.",
			"If the change is still wanted, run the command again for a new request and show the person its link.", map[string]any{"request_id": p.ID, "status": p.Status})
	}
	return askedPerson(p)
}

// waitRequest reads a request until a person has decided it, or it expires.
func (s *session) waitRequest(client *api.Client, org, id string) (api.PersonRequest, error) {
	for {
		p, err := client.GetPersonRequest(s.ctx, org, id)
		if err != nil {
			return p, wrap(err)
		}
		if p.Status != apitypes.PersonRequestPending {
			return p, requestOutcome(p)
		}
		select {
		case <-s.ctx.Done():
			return p, s.ctx.Err()
		case <-time.After(requestPoll):
		}
	}
}

// awaitPerson follows a change the API kept for a person: off a terminal it answers
// NEEDS_HUMAN with the page, exit 2; on one it says so and waits for the person, answering
// the approved request, whose result is the change's own answer.
func (s *session) awaitPerson(client *api.Client, org string, p *api.PersonRequest) (api.PersonRequest, error) {
	if !s.env.IsTerminal {
		return *p, askedPerson(*p)
	}
	s.waiting("A person approves this first: %s. Open %s to approve it. Waiting; if this is interrupted, run whisk requests show %s --wait.",
		strings.Join(p.Summary, "; "), p.URL, p.ID)
	return s.waitRequest(client, org, p.ID)
}

// resultInto reads an approved request's result as the change's own answer.
func resultInto(p api.PersonRequest, out any) error {
	if len(p.Result) == 0 {
		return nil
	}
	if err := json.Unmarshal(p.Result, out); err != nil {
		return output.New("PLATFORM_UNAVAILABLE", "The approved change's answer could not be read.", "Run whisk requests show "+p.ID+" --json for it.", map[string]any{"request_id": p.ID})
	}
	return nil
}

func requestsCmd(s *session) *cobra.Command {
	requests := &cobra.Command{Use: "requests", Short: "Changes waiting for a person to approve them"}
	var wait bool
	show := &cobra.Command{
		Use:   "show <id> [--wait]",
		Short: "Where a change you asked a person to approve stands",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, err := s.orgClient()
			if err != nil {
				return err
			}
			var p api.PersonRequest
			if wait {
				p, err = s.waitRequest(client, org, args[0])
			} else {
				if p, err = client.GetPersonRequest(s.ctx, org, args[0]); err != nil {
					return wrap(err)
				}
				err = requestOutcome(p)
			}
			if err != nil {
				return err
			}
			s.printer.Result(map[string]any{"org": org, "request": p}, func(w io.Writer) {
				fmt.Fprintf(w, "Approved by %s: %s.\n", orDash(p.DecidedBy), strings.Join(p.Summary, "; "))
				var m api.Member
				if json.Unmarshal(p.Result, &m) == nil && m.InviteURL != "" {
					fmt.Fprintf(w, "Send them this link: %s\n", m.InviteURL)
				}
			})
			return nil
		},
	}
	show.Flags().BoolVar(&wait, "wait", false, "wait until a person approves or declines it")
	requests.AddCommand(show)
	return requests
}
