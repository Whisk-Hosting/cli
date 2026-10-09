package whisk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/cli/internal/safefile"
)

// exportFileName is where an export lands by default: named for the day, in UTC, it was made.
func exportFileName(exportedAt time.Time) string {
	if exportedAt.IsZero() {
		return "whisk-account-export.json"
	}
	return "whisk-account-" + exportedAt.UTC().Format(time.DateOnly) + ".json"
}

// accountCmd is the person's own account (CLI.md §5.1): a copy of their data, and deleting it.
// Both need the token whisk login made on this computer; the platform refuses a CI token, an
// agent identity and an operator read token.
func accountCmd(s *session) *cobra.Command {
	account := &cobra.Command{Use: "account", Short: "Your own account: download your data, or delete it"}

	var out string
	export := &cobra.Command{
		Use:   "export [--out file]",
		Short: "Save everything Whisk keeps about you as a JSON file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := s.client()
			if err != nil {
				return err
			}
			data, raw, err := client.ExportAccount(s.ctx)
			if err != nil {
				return wrap(err)
			}
			path := out
			if path == "" {
				path = exportFileName(data.ExportedAt)
			}
			if !filepath.IsAbs(path) && s.env.Dir != "" {
				path = filepath.Join(s.env.Dir, path)
			}
			var pretty bytes.Buffer
			if err := json.Indent(&pretty, raw, "", "  "); err != nil {
				return err
			}
			pretty.WriteByte('\n')
			// The file holds where you signed in from, so only you may read it.
			if err := safefile.WriteFile(path, pretty.Bytes(), 0o600); err != nil {
				return output.New("INVALID_REQUEST", "Could not write "+path+": "+err.Error()+".", "Pass --out with a file in a folder you can write to.", nil)
			}
			s.printer.Result(map[string]any{"file": path, "exported_at": data.ExportedAt, "memberships": len(data.Memberships), "sessions": len(data.Sessions), "audit": len(data.Audit)}, func(w io.Writer) {
				fmt.Fprintf(w, "Saved your data to %s: %d businesses, %d sign-ins, %d audit entries.\n", path, len(data.Memberships), len(data.Sessions), len(data.Audit))
			})
			return nil
		},
	}
	export.Flags().StringVar(&out, "out", "", "file to write (default whisk-account-<date>.json here)")

	var yes bool
	del := &cobra.Command{
		Use:   "delete --yes",
		Short: "Delete your account: you leave every business and your sign-ins and tokens end",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return output.New("NEEDS_HUMAN", "Deleting your account signs you out everywhere, takes you out of every business and ends every agent token, this one included. It cannot be undone.", "Run again with --yes once you, the person, have decided.", nil)
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			if err := client.DeleteAccount(s.ctx); err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"deleted": true}, func(w io.Writer) {
				fmt.Fprintln(w, "Your account is deleted. This computer's token no longer works; whisk logout clears it.")
			})
			return nil
		},
	}
	del.Flags().BoolVar(&yes, "yes", false, "you, the person, have decided")
	account.AddCommand(export, del)
	return account
}
