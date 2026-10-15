package whisk

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
)

// dbCmd is the app's database from outside the app (CLI.md §5.9): SQL and the schema, run on
// the platform through the API, the connection string, and a restore point.
func dbCmd(s *session) *cobra.Command {
	db := &cobra.Command{Use: "db", Short: "The app's database: SQL, the schema, a restore point"}

	var environment string
	var private bool
	url := &cobra.Command{
		Use:   "url [--env production|preview:<branch>] [--private]",
		Short: "Print the connection string where it works from here; audited",
		Long: `Prints the environment's connection string, to be used within 15 minutes. Where the platform
runs no public database proxy, the string's address is inside the app's network and does not
answer from outside, so this refuses with DB_URL_PRIVATE and names whisk db query and whisk db
schema, which run on the platform. --private prints the inside address anyway.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			u, err := client.DatabaseURL(s.ctx, org, app, environment)
			if err != nil {
				return wrap(err)
			}
			if u.Private() && !private {
				return output.New("DB_URL_PRIVATE", "The "+u.Environment+" database's address is inside the platform, so it does not answer from this machine.",
					"Use whisk db query \"<sql>\" for rows as JSON or whisk db schema for the tables; both run on the platform.",
					map[string]any{"environment": u.Environment})
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "environment": u.Environment, "url": u.URL, "reachable": u.Reachable, "expires_at": u.ExpiresAt.UTC().Format(time.RFC3339)}, func(w io.Writer) {
				fmt.Fprintln(w, u.URL)
				fmt.Fprintf(s.env.Stderr, "valid until %s; every use of this string is audited\n", at(u.ExpiresAt))
			})
			return nil
		},
	}
	url.Flags().StringVar(&environment, "env", "", "the environment; production when omitted")
	url.Flags().BoolVar(&private, "private", false, "print the address inside the platform even though it does not answer from outside")

	var snapEnv string
	snapshot := &cobra.Command{
		Use:   "snapshot [--env name]",
		Short: "Record a restore point now, to restore to with whisk restore --at",
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
			snap, err := client.SnapshotDatabase(s.ctx, org, app, snapEnv)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "snapshot": snap}, func(w io.Writer) {
				fmt.Fprintf(w, "Restore point %s recorded at %s (%s).\n", snap.ID, snap.CreatedAt.UTC().Format(time.RFC3339), snap.Environment)
				fmt.Fprintln(w, undoLine(snap))
			})
			return nil
		},
	}
	snapshot.Flags().StringVar(&snapEnv, "env", "", "the environment; production when omitted")

	db.AddCommand(queryCmd(s), dbSchemaCmd(s), shellCmd(s), url, snapshot, dbImportCmd(s))
	return db
}

// restoreCmd is self-service point-in-time restore (CLI.md §5.9, CONTROL-PLANE.md §6.18).
func restoreCmd(s *session) *cobra.Command {
	var atFlag, environment string
	var swap, wait bool
	c := &cobra.Command{
		Use:   "restore --at <RFC 3339 time> [--swap] [--env name] [--wait]",
		Short: "Restore the database to a point in time, beside the live one or swapped in",
		Long: `Restores the environment's database to the moment given by --at into a fresh database
beside the live one, named in the result, so the data can be inspected first. With --swap the
restored database becomes the live one and the previous one is kept for 7 days. Both are
audited. Paid plans; the time must be inside the plan's retention window.

A restore runs in the background and prints its id. --wait follows it until it is done or failed,
with progress on stderr; whisk restore show <id> reads it later and whisk restore list lists the
app's restores.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if atFlag == "" {
				return output.New("INVALID_REQUEST", "--at names the moment to restore to.", "Pass an RFC 3339 time, for example --at 2026-09-07T14:13:00Z, or a duration ago like 2h.", nil)
			}
			at, err := parseSince(atFlag, time.Now())
			if err != nil {
				return output.New("INVALID_REQUEST", fmt.Sprintf("--at %q is not a time or a duration.", atFlag), "Pass an RFC 3339 time like 2026-09-07T14:13:00Z, or a duration ago like 2h or 3d.", nil)
			}
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			r, err := client.RestoreDatabase(s.ctx, org, app, environment, at, swap)
			if err != nil {
				return wrap(err)
			}
			if wait && !restoreEnded(r.Status) {
				if r, err = followRestore(s.ctx, client, org, app, r.ID, s.env.Stderr, restorePoll); err != nil {
					return wrap(err)
				}
			}
			if r.Status == "failed" {
				return restoreFailed(r)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "restore": r}, func(w io.Writer) {
				printRestore(w, r)
				if !restoreEnded(r.Status) {
					fmt.Fprintf(w, "It runs in the background; follow it with whisk restore show %s --wait.\n", r.ID)
				}
			})
			return nil
		},
	}
	c.Flags().StringVar(&atFlag, "at", "", "the moment to restore to: RFC 3339, or a duration ago (2h, 3d)")
	c.Flags().BoolVar(&swap, "swap", false, "make the restored database the live one")
	c.Flags().StringVar(&environment, "env", "", "the environment; production when omitted")
	c.Flags().BoolVar(&wait, "wait", false, "follow the restore until it is done or failed")

	var showWait bool
	show := &cobra.Command{
		Use:   "show <id> [--wait]",
		Short: "Show one restore: its status, the database it restored into, and its error",
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
			var r api.Restore
			if showWait {
				r, err = followRestore(s.ctx, client, org, app, args[0], s.env.Stderr, restorePoll)
			} else {
				r, err = client.GetRestore(s.ctx, org, app, args[0])
			}
			if err != nil {
				return wrap(err)
			}
			if r.Status == "failed" {
				return restoreFailed(r)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "restore": r}, func(w io.Writer) { printRestore(w, r) })
			return nil
		},
	}
	show.Flags().BoolVar(&showWait, "wait", false, "follow the restore until it is done or failed")

	list := &cobra.Command{
		Use:   "list",
		Short: "List the app's restores, newest first",
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
			rs, err := client.ListRestores(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			if rs == nil {
				rs = []api.Restore{}
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "restores": rs}, func(w io.Writer) {
				if len(rs) == 0 {
					fmt.Fprintln(w, "No restores yet.")
					return
				}
				for _, r := range rs {
					fmt.Fprintln(w, restoreRow(r))
				}
			})
			return nil
		},
	}
	c.AddCommand(show, list)
	return c
}

// restorePoll is how often --wait asks how a restore is going.
var restorePoll = 3 * time.Second

// restoreEnded is whether a restore status is final. Pure.
func restoreEnded(status string) bool { return status == "done" || status == "failed" }

// followRestore reads the restore until it ends, naming each new status on log.
func followRestore(ctx context.Context, client *api.Client, org, app, id string, log io.Writer, every time.Duration) (api.Restore, error) {
	last := ""
	for {
		r, err := client.GetRestore(ctx, org, app, id)
		if err != nil {
			return r, err
		}
		if r.Status != last {
			fmt.Fprintf(log, "restore: %s is %s\n", id, r.Status)
			last = r.Status
		}
		if restoreEnded(r.Status) {
			return r, nil
		}
		select {
		case <-ctx.Done():
			return r, ctx.Err()
		case <-time.After(every):
		}
	}
}

// restoreFailed is the error a failed restore recorded, with its id, so an agent branches on what
// stopped it. Pure.
func restoreFailed(r api.Restore) error {
	if r.Error == nil {
		return output.New("RESTORE_FAILED", "Restore "+r.ID+" failed without saying why.",
			"Run whisk restore show "+r.ID+" again; if it still says nothing, report it with whisk feedback --kind bug --code RESTORE_FAILED.", map[string]any{"restore": r.ID})
	}
	details := map[string]any{"restore": r.ID}
	for k, v := range r.Error.Details {
		details[k] = v
	}
	return &output.Error{Code: r.Error.Code, Message: r.Error.Message, Fix: r.Error.Fix, Details: details}
}

// restoreSource is what a restore loads: a point in the app's history, or an imported dump.
// Pure.
func restoreSource(r api.Restore) string {
	if r.Import != "" {
		return "from import " + r.Import
	}
	return "to " + r.At.UTC().Format(time.RFC3339)
}

// restoreRow is one line of whisk restore list. Pure.
func restoreRow(r api.Restore) string {
	line := fmt.Sprintf("%s  %-8s %s %s", r.ID, r.Status, r.Environment, restoreSource(r))
	if r.TargetDatabase != "" {
		line += " into " + r.TargetDatabase
	}
	if r.Error != nil {
		line += "  " + r.Error.Code
	}
	return line
}

func printRestore(w io.Writer, r api.Restore) {
	fmt.Fprintf(w, "Restore %s %s: %s %s", r.ID, r.Status, r.Environment, restoreSource(r))
	if r.TargetDatabase != "" {
		fmt.Fprintf(w, " into %s", r.TargetDatabase)
	}
	fmt.Fprintln(w, ".")
	switch {
	case r.Swap && r.Status == "done":
		fmt.Fprintln(w, "The restored database is the live one; the previous one is kept for 7 days.")
	case r.Swap:
		fmt.Fprintln(w, "The restored database becomes the live one when the restore finishes; the previous one is kept for 7 days.")
	default:
		fmt.Fprintln(w, "The live database is untouched. Inspect the restored one with whisk db query --database <name>, then run again with --swap to switch to it.")
	}
}

// undoLine says how to return to a restore point, or that this plan cannot: a server that does
// not say is taken to allow it, as before plans were reported. Pure.
func undoLine(snap api.Snapshot) string {
	if snap.Restorable != nil && !*snap.Restorable {
		return "This plan has no self-service restore, so this point cannot be returned to with whisk restore; copy rows you may need first with whisk db query \"select ...\" --json."
	}
	return "Return to it with: whisk restore --at " + snap.CreatedAt.UTC().Format(time.RFC3339)
}
