package whisk

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/cli/internal/textenc"
)

// functionsCmd is what the app declares and what its runs have proved (CLI.md §5.8).
func functionsCmd(s *session) *cobra.Command {
	functions := &cobra.Command{Use: "functions", Short: "Workflow functions the app declares"}

	list := &cobra.Command{
		Use:   "list",
		Short: "List the app's functions with their triggers and drift",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			fns, err := client.ListFunctions(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "functions": fns}, func(w io.Writer) {
				if len(fns) == 0 {
					fmt.Fprintln(w, "This app declares no functions. Add one under functions: in whisk.yaml.")
					return
				}
				rows := make([][]string, len(fns))
				for i, f := range fns {
					rows[i] = []string{f.Name, triggerText(f.Triggers), ranText(f), driftText(f.Drift)}
				}
				s.printer.Table(w, []string{"FUNCTION", "TRIGGER", "RUNS", "DRIFT"}, rows)
			})
			return nil
		},
	}

	graph := &cobra.Command{
		Use:   "graph <name>",
		Short: "Show a function's declared steps beside the ones its runs took",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			f, err := client.FunctionGraph(s.ctx, org, app, args[0])
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"function": f}, func(w io.Writer) {
				fmt.Fprintf(w, "%s: %s\n", f.Name, triggerText(f.Triggers))
				fmt.Fprintf(w, "declared: %s\n", orNone(strings.Join(f.Declared, " → ")))
				fmt.Fprintf(w, "observed: %s\n", orNone(strings.Join(f.Observed, " → ")))
				if len(f.Drift.Unexpected) > 0 {
					fmt.Fprintf(w, "ran but not declared: %s\n", strings.Join(f.Drift.Unexpected, ", "))
				}
				if len(f.Drift.NeverRan) > 0 {
					fmt.Fprintf(w, "declared but never ran: %s\n", strings.Join(f.Drift.NeverRan, ", "))
				}
				if !api.Drifted(f.Drift) {
					fmt.Fprintln(w, "No drift: every declared step has run and nothing else has.")
				}
			})
			return nil
		},
	}

	functions.AddCommand(list, graph)
	return functions
}

// runsCmd reads what the app's functions did (CLI.md §5.8).
func runsCmd(s *session) *cobra.Command {
	runs := &cobra.Command{Use: "runs", Short: "Runs of the app's functions"}

	var event, function, status, since string
	list := &cobra.Command{
		Use:   "list [<function>] [--status failed|running|parked] [--since 1h] [--event id]",
		Short: "List the app's runs, newest first: all, one function's, or the runs one event started",
		Example: `  whisk runs list nightly-margin          the latest runs of one function, cron ones included
  whisk runs list --status failed --since 1d
  whisk runs list --event 01J8Z…          what one sent event started`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				if function != "" && function != args[0] {
					return output.New("INVALID_REQUEST", fmt.Sprintf("Two functions given: %s and --function %s.", args[0], function), "Name the function once, as whisk runs list <function>.", nil)
				}
				function = args[0]
			}
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			var list []api.Run
			result := map[string]any{"org": org, "app": app}
			if event != "" {
				list, err = client.RunsOfEvent(s.ctx, org, app, event)
				result["event"] = event
				list = runsOfFunction(list, function)
			} else {
				f := api.RunFilter{Function: function, Status: status}
				if since != "" {
					if f.Since, err = parseSince(since, time.Now()); err != nil {
						return sinceFlagError(since)
					}
				}
				list, err = client.ListRuns(s.ctx, org, app, f)
			}
			if err != nil {
				return wrap(err)
			}
			result["runs"] = list
			s.printer.Result(result, func(w io.Writer) {
				if len(list) == 0 {
					if event != "" {
						fmt.Fprintln(w, "No run started for that event. A function triggers on the event name it declares.")
					} else {
						fmt.Fprintln(w, "No runs match. A function runs when its event is sent or its cron fires.")
					}
					return
				}
				rows := make([][]string, len(list))
				for i, r := range list {
					rows[i] = []string{r.ID, r.Function, r.Status, when(r.StartedAt), strings.Join(r.Steps, " → "), runWhy(r)}
				}
				s.printer.Table(w, []string{"RUN", "FUNCTION", "STATUS", "STARTED", "STEPS", "WHY"}, rows)
			})
			return nil
		},
	}
	list.Flags().StringVar(&event, "event", "", "only the runs this event started")
	list.Flags().StringVar(&function, "function", "", "only runs of this function")
	list.Flags().StringVar(&status, "status", "", "only runs in this status: running, failed, parked, completed, cancelled")
	list.Flags().StringVar(&since, "since", "", "only runs started in the last duration (30m, 6h, 7d) or since an RFC 3339 time")

	replay := &cobra.Command{
		Use:   "replay <id>",
		Short: "Run a failed or parked run again; prints the new run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			r, err := client.ReplayRun(s.ctx, org, app, args[0])
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"replayed": args[0], "run": r}, func(w io.Writer) {
				fmt.Fprintf(w, "Replayed %s as %s (%s). Follow it with whisk runs show %s.\n", args[0], r.ID, r.Status, r.ID)
			})
			return nil
		},
	}

	cancel := &cobra.Command{
		Use:   "cancel <id>",
		Short: "Stop a running or parked run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			r, err := client.CancelRun(s.ctx, org, app, args[0])
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"run": r}, func(w io.Writer) {
				fmt.Fprintf(w, "Cancelled %s (%s).\n", r.ID, r.Status)
			})
			return nil
		},
	}

	show := &cobra.Command{
		Use:   "show <id>",
		Short: "Show one run: its steps, timing and output",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			r, err := client.GetRun(s.ctx, org, app, args[0])
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"run": r}, func(w io.Writer) {
				fmt.Fprintf(w, "%s %s (%s)\n", r.ID, r.Function, r.Status)
				if t := runTrigger(r); t != "" {
					fmt.Fprintf(w, "started: %s\n", t)
				}
				fmt.Fprintf(w, "steps: %s\n", orNone(strings.Join(r.Steps, " → ")))
				if r.StartedAt != nil && r.EndedAt != nil {
					fmt.Fprintf(w, "took: %s\n", r.EndedAt.Sub(*r.StartedAt).Round(time.Millisecond))
				}
				if r.Error != nil {
					side := "the app's side"
					if api.Platform(r.Error) {
						side = "Whisk's side, not your code"
					}
					fmt.Fprintf(w, "failed: %s (%s): %s\n", r.Error.Code, side, r.Error.Message)
					fmt.Fprintf(w, "fix: %s\n", r.Error.Fix)
				}
				if r.RerunRunID != "" {
					fmt.Fprintf(w, "Whisk ran it again as %s. Follow it with whisk runs show %s.\n", r.RerunRunID, r.RerunRunID)
				}
				if r.RerunOf != "" {
					fmt.Fprintf(w, "Whisk started this run again after it interrupted %s.\n", r.RerunOf)
				}
				if r.Output != nil && r.Error == nil {
					if raw, err := json.Marshal(r.Output); err == nil {
						fmt.Fprintf(w, "output: %s\n", truncate(string(raw), 400))
					}
				}
			})
			return nil
		},
	}

	runs.AddCommand(list, show, replay, cancel)
	return runs
}

// runTrigger is what started a run, in words.
func runTrigger(r api.Run) string {
	switch {
	case r.Manual:
		return "by hand (whisk cron run), as its schedule would"
	case r.Trigger == "cron":
		return "on its schedule"
	case r.Trigger == "replay":
		return "by a replay"
	case r.Trigger != "":
		return "on the event " + r.Trigger
	}
	return ""
}

// runWhy is a failed run's cause in a listing: its code, marked when it was Whisk's side, and
// the run Whisk started in its place. A run that did not fail says when it is a re-run or was
// started by hand.
func runWhy(r api.Run) string {
	if r.Error == nil {
		switch {
		case r.RerunOf != "":
			return "re-run of " + r.RerunOf
		case r.Manual:
			return "started by hand"
		}
		return ""
	}
	why := r.Error.Code
	if api.Platform(r.Error) {
		why += " (Whisk's side)"
	}
	if r.RerunRunID != "" {
		why += ", re-run as " + r.RerunRunID
	}
	return why
}

// runsOfFunction keeps one function's runs; an empty name keeps them all.
func runsOfFunction(runs []api.Run, function string) []api.Run {
	if function == "" {
		return runs
	}
	out := make([]api.Run, 0, len(runs))
	for _, r := range runs {
		if r.Function == function {
			out = append(out, r)
		}
	}
	return out
}

// eventsCmd puts an event on the app's queue (CLI.md §5.8).
func eventsCmd(s *session) *cobra.Command {
	events := &cobra.Command{Use: "events", Short: "The app's queue"}

	var data, dataFile, dedupe string
	send := &cobra.Command{
		Use:   "send <name>",
		Short: "Send an event into the app's queue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			payload, err := eventData(data, dataFile, s.readInput)
			if err != nil {
				return err
			}
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			id, err := client.SendEvent(s.ctx, org, app, args[0], payload, dedupe)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"id": id, "name": args[0]}, func(w io.Writer) {
				fmt.Fprintf(w, "Sent %s as %s. See what it started with whisk runs list --event %s.\n", args[0], id, id)
			})
			return nil
		},
	}
	send.Flags().StringVar(&data, "data", "", "the event's data as JSON, or @file to read it from a file")
	send.Flags().StringVar(&dataFile, "data-file", "", "a file holding the event's data as JSON; - reads stdin")
	send.Flags().StringVar(&dedupe, "dedupe", "", "a key that suppresses a repeat for 24 hours")

	events.AddCommand(send)
	return events
}

// eventData is the event's data from --data or --data-file. "--data @path" means the same as
// "--data-file path", as curl spells it, and the path "-" is stdin. A file keeps JSON away from
// shells that rewrite quotes (PowerShell), so it is read in whatever encoding an editor or a
// redirect wrote: UTF-8 with or without a byte-order mark, or UTF-16 with one.
func eventData(data, file string, read func(path string) ([]byte, error)) (map[string]any, error) {
	if path, ok := strings.CutPrefix(data, "@"); ok {
		if file != "" {
			return nil, output.New("INVALID_REQUEST", "--data @file and --data-file both name a file.", "Pass one of them.", nil)
		}
		data, file = "", path
	}
	payload := map[string]any{}
	switch {
	case data != "" && file != "":
		return nil, output.New("INVALID_REQUEST", "--data and --data-file were both given.", "Pass the JSON inline with --data or in a file with --data-file, not both.", nil)
	case file != "":
		raw, err := read(file)
		if err != nil {
			return nil, output.New("INVALID_REQUEST", "Could not read "+file+": "+err.Error()+".", "Pass the path of a readable file with --data-file, or - for stdin.", map[string]any{"file": file})
		}
		if err := json.Unmarshal(decodeText(raw), &payload); err != nil {
			return nil, output.New("INVALID_REQUEST", file+" is not a JSON object: "+err.Error()+".", `Write one object to the file, for example {"id":42}.`, map[string]any{"file": file})
		}
	case data != "":
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			return nil, output.New("INVALID_REQUEST", "--data is not valid JSON.", `Pass an object, for example --data '{"id":42}'. In PowerShell, write it to a file and pass --data-file data.json.`, nil)
		}
	}
	return payload, nil
}

// decodeText turns file or stdin bytes into UTF-8 (textenc.Decode): a byte-order mark is dropped
// and UTF-16, what Windows PowerShell's > writes, is converted.
func decodeText(b []byte) []byte { return textenc.Decode(b) }

// readInput reads a file named on the command line, relative to the working directory; "-" is
// stdin.
func (s *session) readInput(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(s.env.Stdin)
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.env.Dir, path)
	}
	return os.ReadFile(path)
}

// approvalsCmd lists what the app is waiting on (CLI.md §5.8). Deciding is a person's act, in
// the dashboard, so the CLI prints the link rather than the buttons.
func approvalsCmd(s *session) *cobra.Command {
	approvals := &cobra.Command{Use: "approvals", Short: "Decisions the app's workflows are waiting on"}

	var status string
	list := &cobra.Command{
		Use:   "list",
		Short: "List approvals, pending by default",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			list, err := client.ListApprovals(s.ctx, org, app, status)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "approvals": list}, func(w io.Writer) {
				if len(list) == 0 {
					fmt.Fprintln(w, "Nothing is waiting for a decision.")
					return
				}
				rows := make([][]string, len(list))
				for i, a := range list {
					rows[i] = []string{a.Title, a.To, string(a.Status), a.URL}
				}
				s.printer.Table(w, []string{"WAITING FOR", "WHO DECIDES", "STATUS", "OPEN"}, rows)
				fmt.Fprintln(w, "\nA person decides an approval; open the link and choose.")
			})
			return nil
		},
	}
	list.Flags().StringVar(&status, "status", "pending", "pending, approved, rejected or expired; empty for all")

	approvals.AddCommand(list)
	return approvals
}

// triggerText is how a function's triggers read in a table. A cron reads in the zone it was
// declared in (UTC when none), with its next run in that zone too, so "5-21" is never read
// against a UTC clock.
func triggerText(triggers []api.Trigger) string {
	var out []string
	for _, t := range triggers {
		switch {
		case t.Event != "":
			out = append(out, "on "+t.Event)
		case t.Cron != "":
			out = append(out, cronText(t))
		}
	}
	return orNone(strings.Join(out, ", "))
}

// cronText is one cron trigger: the expression, its zone, the schedule the plan runs it on when
// that differs, and the next run shown in the zone.
func cronText(t api.Trigger) string {
	zone := t.TZ
	if zone == "" {
		zone = "UTC"
	}
	text := t.Cron + " " + zone
	if t.RunsAs != "" {
		text += " (runs as " + t.RunsAs + " on this plan)"
	}
	if t.NextRun == nil {
		return text
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return text + " (next " + t.NextRun.UTC().Format("2006-01-02 15:04Z") + ")"
	}
	return text + " (next " + t.NextRun.In(loc).Format("2006-01-02 15:04 MST") + ")"
}

func ranText(f api.Function) string {
	if f.Ran {
		return "yes"
	}
	return "never"
}

func driftText(d api.Drift) string {
	switch {
	case len(d.Unexpected) > 0 && len(d.NeverRan) > 0:
		return fmt.Sprintf("%d extra, %d unrun", len(d.Unexpected), len(d.NeverRan))
	case len(d.Unexpected) > 0:
		return fmt.Sprintf("%d not declared", len(d.Unexpected))
	case len(d.NeverRan) > 0:
		return fmt.Sprintf("%d never ran", len(d.NeverRan))
	default:
		return "none"
	}
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
