package whisk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/apitypes"
)

// exportPoll is how often --wait asks how an export is going.
var exportPoll = 3 * time.Second

// exportCmd starts the org's export and, with --wait, follows it to its link (CLI.md §5.9).
func exportCmd(s *session) *cobra.Command {
	var wait bool
	c := &cobra.Command{
		Use:   "export",
		Short: "Export the whole org as one archive: repositories, databases and audit log",
		Long: `Starts an export of the whole org and prints its id: every app's repository as a git bundle,
every database as a pg_dump, the secrets' names (never their values), the audit log and the
manifest history, in one archive in the org's own bucket. It works in every state the org can
be in, frozen included.

With --wait it follows the export, or the one already running, and prints a link that lives
until the export expires, seven days after it finishes. Owners and admins.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, err := s.org()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			e, err := client.StartExport(s.ctx, org)
			if err != nil {
				id, running := runningExport(err)
				if !wait || !running {
					return wrap(err)
				}
				fmt.Fprintf(s.env.Stderr, "export: following the export already running, %s\n", id)
				e = api.Export{ID: id}
			}
			if !wait {
				s.printer.Result(exportResult(org, e), func(w io.Writer) {
					fmt.Fprintf(w, "Export %s started. Run whisk export --wait to follow it to its link.\n", e.ID)
				})
				return nil
			}
			final, err := followExport(s.ctx, client, org, e.ID, s.env.Stderr, exportPoll)
			if err != nil {
				return wrap(err)
			}
			if final.Status != apitypes.ExportDone {
				return exportFailed(final)
			}
			s.printer.Result(exportResult(org, final), func(w io.Writer) {
				until := ""
				if !final.ExpiresAt.IsZero() {
					until = ", downloadable until " + final.ExpiresAt.Local().Format("2 Jan 2006 15:04")
				}
				fmt.Fprintf(w, "Export %s is ready: %s%s.\n%s\n", final.ID, exportSize(final.Bytes), until, final.URL)
			})
			return nil
		},
	}
	c.Flags().BoolVar(&wait, "wait", false, "follow the export, or the one already running, and print its link when it is ready")
	return c
}

func exportResult(org string, e api.Export) map[string]any {
	out := map[string]any{"org": org, "id": e.ID, "status": e.Status}
	if e.URL != "" {
		out["url"] = e.URL
	}
	if !e.ExpiresAt.IsZero() {
		out["expires_at"] = e.ExpiresAt
	}
	if e.Bytes > 0 {
		out["bytes"] = e.Bytes
	}
	if len(e.Contents) > 0 {
		out["contents"] = e.Contents
	}
	return out
}

// runningExport is the id of the export already running, when that is why a start was refused.
func runningExport(err error) (string, bool) {
	var e *api.Error
	if !errors.As(err, &e) || e.Code != "EXPORT_IN_PROGRESS" {
		return "", false
	}
	id, _ := e.Details["job_id"].(string)
	return id, id != ""
}

// followExport asks after an export until it finishes, saying so each time its state changes.
func followExport(ctx context.Context, client *api.Client, org, id string, log io.Writer, every time.Duration) (api.Export, error) {
	var last apitypes.ExportStatus
	for {
		e, err := client.GetExport(ctx, org, id)
		if err != nil {
			return e, err
		}
		if e.Status != last {
			fmt.Fprintf(log, "export: %s is %s\n", id, e.Status)
			last = e.Status
		}
		switch e.Status {
		case apitypes.ExportDone, apitypes.ExportFailed, apitypes.ExportExpired:
			return e, nil
		}
		select {
		case <-ctx.Done():
			return e, ctx.Err()
		case <-time.After(every):
		}
	}
}

// exportFailed is the error a failed export recorded, so an agent branches on what stopped it.
func exportFailed(e api.Export) error {
	if e.Error == nil {
		return output.New("EXPORT_FAILED", "Export "+e.ID+" ended "+string(e.Status)+" without a link.",
			"Start the export again with whisk export --wait.", map[string]any{"id": e.ID, "status": e.Status})
	}
	details := map[string]any{"id": e.ID}
	for k, v := range e.Error.Details {
		details[k] = v
	}
	return &output.Error{Code: e.Error.Code, Message: e.Error.Message, Fix: e.Error.Fix, Docs: e.Error.Docs, Details: details}
}

func exportSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d bytes", n)
}
