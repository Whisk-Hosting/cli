package whisk

import (
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/config"
	"github.com/whisk-run/cli/internal/output"
)

// feedbackCmd tells Whisk what got in the way (CLI.md §5.12, CONTROL-PLANE.md §6.23). Sending is
// the command itself and `send`, so an agent can type `whisk feedback "..."` and an MCP client
// sees a feedback_send tool; `list` and `show` let a sender see where its feedback stands, and
// `list --everyone`, `resolve` and `reopen` are for operators and the agents they trust to read
// it or to set its status.
func feedbackCmd(s *session) *cobra.Command {
	var f feedbackFlags
	root := &cobra.Command{
		Use:   `feedback "<what happened>" [--kind bug|difficulty|idea|praise] [--code CODE] [--command "whisk deploy"]`,
		Short: "Tell the Whisk team what got in your way; every piece is read",
		Long:  feedbackLong,
		Args:  cobra.ArbitraryArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return sendFeedback(s, f, args) },
	}
	f.bind(root)

	var sf feedbackFlags
	send := &cobra.Command{
		Use:   `send "<what happened>" [--kind bug|difficulty|idea|praise] [--code CODE] [--command "whisk deploy"]`,
		Short: "Tell the Whisk team what got in your way; every piece is read",
		Long:  feedbackLong,
		Args:  cobra.ArbitraryArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return sendFeedback(s, sf, args) },
	}
	sf.bind(send)

	var q api.FeedbackQuery
	var everyone bool
	list := &cobra.Command{
		Use:   "list [--status open|resolved|all] [--kind bug] [--org slug] [--limit 50] [--cursor id] [--everyone]",
		Short: "See the feedback you and your org sent, where each stands and what the Whisk team said",
		Long:  feedbackListLong,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := s.client()
			if err != nil {
				return err
			}
			q.Org = s.orgFlag
			if everyone {
				return listEveryonesFeedback(s, client, q)
			}
			page, err := client.ListOwnFeedback(s.ctx, q)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"items": page.Items, "next_cursor": page.NextCursor}, func(w io.Writer) {
				if len(page.Items) == 0 {
					fmt.Fprintln(w, "No feedback from you or your org matches. Send some with whisk feedback \"...\".")
					return
				}
				s.printer.Table(w, []string{"ID", "WHEN", "KIND", "STATUS", "WHERE", "FROM", "MESSAGE"}, ownFeedbackRows(page.Items))
				for _, f := range page.Items {
					if f.Note != "" {
						fmt.Fprintf(w, "%s, from the Whisk team: %s\n", f.ID, strings.Join(strings.Fields(f.Note), " "))
					}
				}
				if page.NextCursor != "" {
					fmt.Fprintf(w, "More with --cursor %s. ", page.NextCursor)
				}
				fmt.Fprintln(w, "whisk feedback show <id> prints one in full.")
			})
			return nil
		},
	}
	list.Flags().StringVar(&q.Status, "status", "", "open, resolved or all (default all; open with --everyone)")
	list.Flags().StringVar(&q.Kind, "kind", "", "bug, difficulty, idea or praise")
	list.Flags().IntVar(&q.Limit, "limit", 0, "how many, up to 200 (default 50)")
	list.Flags().StringVar(&q.Cursor, "cursor", "", "the next_cursor of the previous page")
	list.Flags().BoolVar(&everyone, "everyone", false, "every sender's feedback (Whisk operators, or a token with feedback:read)")

	show := &cobra.Command{
		Use:   "show <id>",
		Short: "See one piece of feedback you sent: where it stands and what the Whisk team said",
		Args:  cobra.ExactArgs(1),
		RunE:  func(cmd *cobra.Command, args []string) error { return showFeedback(s, strings.TrimSpace(args[0])) },
	}

	resolve := feedbackStatusCmd(s, "resolve", "resolved", "Mark a piece of feedback resolved, with a note for its sender (Whisk operators, or a token with feedback:resolve)")
	reopen := feedbackStatusCmd(s, "reopen", "open", "Reopen a piece of feedback (Whisk operators, or a token with feedback:resolve)")

	root.AddCommand(send, list, show, resolve, reopen)
	return root
}

const feedbackListLong = `Lists the feedback you sent and the feedback sent about your org, newest first, open and
resolved alike: its status, and the Whisk team's note on what changed when there is one. Signed
in as an agent token, it is what that token sent and what was sent about its org's apps that the
token reaches. Check it at the start of a session: a resolved piece usually means the thing that
got in your way is fixed, and the note says how.

--everyone lists every sender's feedback, open by default, for Whisk operators and the agents they
give a token with feedback:read.`

// listEveryonesFeedback is list --everyone: the operator's view of every sender's feedback.
func listEveryonesFeedback(s *session, client *api.Client, q api.FeedbackQuery) error {
	page, err := client.ListFeedback(s.ctx, q)
	if err != nil {
		return wrap(err)
	}
	s.printer.Result(map[string]any{"items": page.Items, "next_cursor": page.NextCursor, "open": page.Open}, func(w io.Writer) {
		if len(page.Items) == 0 {
			fmt.Fprintf(w, "No feedback matches. %d open in all.\n", page.Open)
			return
		}
		s.printer.Table(w, []string{"ID", "WHEN", "KIND", "STATUS", "WHERE", "FROM", "MESSAGE"}, feedbackRows(page.Items))
		fmt.Fprintf(w, "%d open in all.", page.Open)
		if page.NextCursor != "" {
			fmt.Fprintf(w, " More with --cursor %s.", page.NextCursor)
		}
		fmt.Fprintln(w, " --json prints every message in full with its context.")
	})
	return nil
}

// showFeedback prints one piece in full. Like sending, it works without a login, because
// feedback sent without one is found by its id alone.
func showFeedback(s *session, id string) error {
	client, _, err := s.client()
	if err != nil {
		client = s.anonymousClient()
	}
	f, err := client.ShowFeedback(s.ctx, id)
	if err != nil && client.Token != "" && isAuthFailure(err) {
		f, err = s.anonymousClient().ShowFeedback(s.ctx, id)
	}
	if err != nil {
		return wrap(err)
	}
	s.printer.Result(map[string]any{"id": f.ID, "kind": f.Kind, "message": f.Message, "status": f.Status, "org": f.Org, "app": f.App,
		"sent_by_you": f.SentByYou, "context": f.Context, "redacted": f.Redacted, "created_at": f.CreatedAt, "resolved_at": f.ResolvedAt,
		"note": f.Note}, func(w io.Writer) {
		fmt.Fprintf(w, "Feedback %s, a %s sent %s", f.ID, f.Kind, at(f.CreatedAt))
		if where := feedbackWhere(f.Org, f.App); where != "" {
			fmt.Fprintf(w, " about %s", where)
		}
		fmt.Fprintf(w, ", is %s", f.Status)
		if f.ResolvedAt != nil {
			fmt.Fprintf(w, " (resolved %s)", at(*f.ResolvedAt))
		}
		fmt.Fprintf(w, ".\n\n%s\n", f.Message)
		if f.Note != "" {
			fmt.Fprintf(w, "\nFrom the Whisk team: %s\n", f.Note)
		} else if f.Status == "open" {
			fmt.Fprintln(w, "\nThe Whisk team has not resolved it yet.")
		}
	})
	return nil
}

// feedbackStatusCmd sets a piece of feedback's status: resolve and reopen differ only in which.
func feedbackStatusCmd(s *session, name, status, short string) *cobra.Command {
	var note string
	cmd := &cobra.Command{
		Use:   name + ` <id> [--note "what changed"]`,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := s.client()
			if err != nil {
				return err
			}
			f, err := client.SetFeedbackStatus(s.ctx, strings.TrimSpace(args[0]), status, note)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"id": f.ID, "status": f.Status, "kind": f.Kind, "resolved_at": f.ResolvedAt, "note": f.Note}, func(w io.Writer) {
				fmt.Fprintf(w, "Feedback %s is %s.\n", f.ID, f.Status)
				if f.Note != "" {
					fmt.Fprintf(w, "Its sender reads: %s\n", f.Note)
				}
			})
			return nil
		},
	}
	cmd.Flags().StringVar(&note, "note", "", "what changed and what the sender can do now; the sender reads it with whisk feedback show")
	return cmd
}

const feedbackLong = `Sends feedback to the Whisk team: a bug, something that was hard, an idea, or something that
worked well. Whisk is built for coding agents, and this is how it gets better, so send it
whenever something gets in your way, without asking first. Say what you were doing, what
happened, what you expected and what would have helped. Never include a secret value, a token
or personal data; anything shaped like a credential is masked before it is stored.

The message is the arguments joined, or standard input when the only argument is "-". The CLI
adds its version, your platform and agent, and the org and app of this directory. It works
before whisk login too.`

// feedbackFlags are what a submission may say beyond the message.
type feedbackFlags struct {
	kind    string
	code    string
	command string
}

func (f *feedbackFlags) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.kind, "kind", "", "bug, difficulty (default), idea or praise")
	cmd.Flags().StringVar(&f.code, "code", "", "the error code you met, such as HEALTH_CHECK_FAILED")
	cmd.Flags().StringVar(&f.command, "command", "", `the command you ran, such as "whisk deploy"`)
}

func sendFeedback(s *session, f feedbackFlags, args []string) error {
	msg, err := feedbackMessage(args, s.env.Stdin)
	if err != nil {
		return err
	}
	in := api.FeedbackInput{Kind: f.kind, Message: msg, Context: feedbackContext(f, Version, runtime.GOOS, runtime.GOARCH, s.env.Getenv("WHISK_AGENT"))}
	if b, ok, _ := config.LoadBinding(s.env.Dir); ok {
		in.Org, in.App = b.Org, b.App
	}
	if s.orgFlag != "" {
		in.Org = s.orgFlag
	}
	if s.appFlag != "" {
		in.App = s.appFlag
	}
	client, _, err := s.client()
	if err != nil {
		// Feedback matters most when login is what failed.
		client = s.anonymousClient()
	}
	receipt, err := client.SendFeedback(s.ctx, in)
	if err != nil && client.Token != "" && isAuthFailure(err) {
		receipt, err = s.anonymousClient().SendFeedback(s.ctx, in)
	}
	if err != nil {
		return wrap(err)
	}
	s.printer.Result(map[string]any{"id": receipt.ID, "kind": receipt.Kind, "org": receipt.Org, "app": receipt.App,
		"redacted": receipt.Redacted, "created_at": receipt.CreatedAt, "message": receipt.Message}, func(w io.Writer) {
		fmt.Fprintf(w, "%s (feedback %s)\n", receipt.Message, receipt.ID)
		if receipt.Redacted > 0 {
			fmt.Fprintf(w, "%d value%s that looked like credentials %s masked before it was stored.\n", receipt.Redacted, plural(receipt.Redacted), wasWere(receipt.Redacted))
		}
	})
	return nil
}

// feedbackMessage is the message: the arguments joined, or standard input for "-".
func feedbackMessage(args []string, stdin io.Reader) (string, error) {
	msg := strings.TrimSpace(strings.Join(args, " "))
	if msg == "-" {
		b, err := io.ReadAll(io.LimitReader(stdin, 64<<10))
		if err != nil {
			return "", err
		}
		msg = strings.TrimSpace(string(decodeText(b)))
	}
	if msg == "" {
		return "", output.New("INVALID_REQUEST", "Feedback needs a message.",
			`Say what happened in quotes, for example whisk feedback "whisk deploy hung after the build" --kind bug, or pass - and write it on standard input.`, nil)
	}
	return msg, nil
}

// feedbackContext is what the CLI adds so the team can reproduce what the agent saw.
func feedbackContext(f feedbackFlags, version, goos, arch, agent string) map[string]string {
	ctx := map[string]string{"cli_version": version, "os": goos, "arch": arch}
	for k, v := range map[string]string{"agent": agent, "code": strings.ToUpper(strings.TrimSpace(f.code)), "command": strings.TrimSpace(f.command)} {
		if v != "" {
			ctx[k] = v
		}
	}
	return ctx
}

func isAuthFailure(err error) bool {
	for _, c := range []string{"AUTH_REQUIRED", "TOKEN_EXPIRED", "TOKEN_REVOKED", "TOKEN_SIGNATURE"} {
		if api.IsCode(err, c) {
			return true
		}
	}
	return false
}

// feedbackWhere is the org and app a piece names, as org/app.
func feedbackWhere(org, app string) string {
	if org != "" && app != "" {
		return org + "/" + app
	}
	return org
}

// ownFeedbackRows is a sender's list as a table: one line each, the message cut to fit.
func ownFeedbackRows(items []api.OwnFeedback) [][]string {
	rows := make([][]string, len(items))
	for i, f := range items {
		from := "your org"
		if f.SentByYou {
			from = "you"
		}
		rows[i] = []string{f.ID, at(f.CreatedAt), string(f.Kind), string(f.Status), orDash(feedbackWhere(f.Org, f.App)), from, truncate(strings.Join(strings.Fields(f.Message), " "), 60)}
	}
	return rows
}

// feedbackRows is everyone's list as a table: one line each, the message cut to fit.
func feedbackRows(items []api.Feedback) [][]string {
	rows := make([][]string, len(items))
	for i, f := range items {
		where := feedbackWhere(f.Org, f.App)
		from := f.Email
		if from == "" {
			from = string(f.ActorKind)
		} else if f.ActorKind == "agent" {
			from += " (agent)"
		}
		rows[i] = []string{f.ID, at(f.CreatedAt), string(f.Kind), string(f.Status), orDash(where), from, truncate(strings.Join(strings.Fields(f.Message), " "), 60)}
	}
	return rows
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func wasWere(n int) string {
	if n == 1 {
		return "was"
	}
	return "were"
}
