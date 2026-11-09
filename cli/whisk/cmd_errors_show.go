package whisk

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	werrors "github.com/whisk-run/contract/errors"
)

// errorsArg is what the one argument of whisk errors names.
type errorsArg int

const (
	// errorsArgCode is a platform error code, in any case.
	errorsArgCode errorsArg = iota
	// errorsArgGroup is the id of one of the app's error groups, as the list shows it.
	errorsArgGroup
	// errorsArgUnknown is neither, explained as an unknown code.
	errorsArgUnknown
)

// errorsArgOf reads the argument of whisk errors: a code from errors.md, a group id (a whole
// number, as the error store counts its groups), or neither. Pure.
func errorsArgOf(arg string) errorsArg {
	if _, ok := werrors.Lookup(strings.ToUpper(arg)); ok {
		return errorsArgCode
	}
	if arg != "" && strings.Trim(arg, "0123456789") == "" {
		return errorsArgGroup
	}
	return errorsArgUnknown
}

// notAnErrorCode is whisk errors given a group's id where it explains a code.
func notAnErrorCode(arg string) error {
	show := "whisk errors show " + arg
	return output.New("NOT_AN_ERROR_CODE", arg+" is an error group's id, not a platform error code.",
		"Run "+show+" to open the group: its stack, the request it was in and its recent occurrences. Platform error codes are upper snake case, like BUILD_FAILED.",
		map[string]any{"argument": arg, "show": show})
}

// errorShowCmd opens one of the app's error groups by the id whisk errors lists (CLI.md §5.5).
func errorShowCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Open one error group: its stack, the request it was in, and its recent occurrences",
		Long: `Opens one of the app's error groups by the id whisk errors lists: what the group is and how
often it happened, its newest occurrence with the stack that threw, the request id it was in
(which whisk traces show and whisk logs --grep open), and its recent occurrences.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			id := args[0]
			sample, err := client.ErrorGroupSample(s.ctx, org, app, id)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(errorShowJSON(org, app, id, sample), func(w io.Writer) {
				for _, line := range errorShowLines(id, sample, time.Now()) {
					fmt.Fprintln(w, line)
				}
			})
			return nil
		},
	}
}

// errorShowJSON is whisk errors show --json: the group, its newest occurrence with the stack,
// the request it was in, and its recent occurrences, never null. Pure.
func errorShowJSON(org, app, id string, s api.ErrorSample) map[string]any {
	group := any(map[string]any{"id": id})
	if s.Group != nil {
		group = s.Group
	}
	frames := s.Frames
	if frames == nil {
		frames = []api.ErrorFrame{}
	}
	recent := s.Recent
	if recent == nil {
		recent = []api.ErrorOccurrence{}
	}
	latest := map[string]any{"id": s.ID, "at": s.At, "message": s.Message, "request_id": s.RequestID, "frames": frames, "tags": s.Tags}
	return map[string]any{"org": org, "app": app, "id": id, "group": group, "latest": latest, "request_id": s.RequestID, "recent": recent}
}

// errorShowLines is whisk errors show for a person: what the group is, the newest occurrence's
// request and stack, the recent occurrences, and what to do with the group. Pure.
func errorShowLines(id string, s api.ErrorSample, now time.Time) []string {
	title, status := s.Message, ""
	var out []string
	if g := s.Group; g != nil {
		if g.Title != "" {
			title = g.Title
		}
		status = g.Status
		out = append(out, title,
			fmt.Sprintf("Group %s, %s: %d time%s (%d user%s), first seen %s, last seen %s.", id, orWord(g.Status, "status unknown"),
				g.Count, plural(g.Count), g.Users, plural(g.Users), agoAt(g.FirstSeen, now), agoAt(g.LastSeen, now)))
		if g.Culprit != "" {
			out = append(out, "Where: "+g.Culprit)
		}
	} else {
		out = append(out, orWord(title, "Error group "+id))
	}

	out = append(out, "", fmt.Sprintf("Newest occurrence %s, %s (%s).", orDash(s.ID), agoAt(s.At, now), stamp(s.At)))
	if s.Message != "" && s.Message != title {
		out = append(out, "Message: "+s.Message)
	}
	if s.RequestID != "" {
		out = append(out, fmt.Sprintf("Request: %s. Its trace: whisk traces show %s. Its log lines: whisk logs --grep %s.", s.RequestID, s.RequestID, s.RequestID))
	} else {
		out = append(out, "Request: none recorded. Tag the error with request_id from the X-Whisk-Request-Id header, as the templates' error handlers do.")
	}

	out = append(out, "")
	if len(s.Frames) == 0 {
		out = append(out, "No stack: the error carried none.")
	} else {
		out = append(out, "Stack, innermost last (> is the app's own code):")
		for _, f := range s.Frames {
			mark := " "
			if f.InApp {
				mark = ">"
			}
			out = append(out, "  "+mark+" "+frameText(f))
		}
	}

	if len(s.Recent) > 0 {
		out = append(out, "", "Recent occurrences, newest first:")
		for _, o := range s.Recent {
			out = append(out, fmt.Sprintf("  %-20s  %-26s  %s", stamp(o.At), orDash(o.RequestID), truncate(o.Message, 60)))
		}
	}

	out = append(out, "")
	if status == "resolved" {
		out = append(out, fmt.Sprintf("It is resolved: it comes back as unresolved if it happens again. whisk errors --ignore %s stops it showing.", id))
	} else {
		out = append(out, fmt.Sprintf("Once it is fixed: whisk errors --resolve %s. To stop it showing: whisk errors --ignore %s.", id, id))
	}
	return out
}

// frameText is one frame as a stack trace reads: the function and where it is. Pure.
func frameText(f api.ErrorFrame) string {
	where := f.File
	if where != "" && f.Line > 0 {
		where = fmt.Sprintf("%s:%d", where, f.Line)
	}
	switch {
	case f.Function != "" && where != "":
		return f.Function + "  " + where
	case f.Function != "":
		return f.Function
	case where != "":
		return where
	}
	return "(a frame with no name)"
}

// stamp is a time in UTC to the second, or - when there is none. Pure.
func stamp(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.UTC().Format("2006-01-02 15:04:05Z")
}

// orWord is s, or the word for nothing when it is empty. Pure.
func orWord(s, none string) string {
	if s == "" {
		return none
	}
	return s
}
