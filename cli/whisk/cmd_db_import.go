package whisk

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/output"
)

// dumpMagic is how every pg_dump custom-format archive starts.
const dumpMagic = "PGDMP"

// dbImportCmd loads a dump of the database an app is moving from (CLI.md §5.9, CONTROL-PLANE.md
// §6.18 "db import"): the file is uploaded over HTTPS with this token and loaded by a restore,
// beside the live database or swapped in.
func dbImportCmd(s *session) *cobra.Command {
	var swap, wait bool
	c := &cobra.Command{
		Use:   "import <dump file> [--swap] [--wait]",
		Short: "Load a pg_dump of another database into the app's database, beside it or swapped in",
		Long: `Loads a dump of the database an app is moving from. Make it with pg_dump -Fc (custom format) from
PostgreSQL 18 or older; a plain SQL dump is refused. The file is uploaded with this sign-in and
loaded into a fresh database beside the live one, named in the result, so the data can be
checked first with whisk db query --database <name>. With --swap the loaded database becomes
the live one and the previous one is kept for 7 days. Owners and grants in the dump are left
out: everything belongs to the app. The uploaded file is removed once it is loaded. Audited.

The load runs in the background and prints its id. --wait follows it until it is done or
failed; whisk restore show <id> reads it later.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			if !filepath.IsAbs(path) && s.env.Dir != "" {
				path = filepath.Join(s.env.Dir, path)
			}
			f, size, err := openDump(path)
			if err != nil {
				return err
			}
			defer f.Close()
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			auth, err := client.AuthoriseImport(s.ctx, org, app, size)
			if err != nil {
				return wrap(err)
			}
			report := uploadProgress(s.env.Stderr, size, s.quiet || s.json)
			if err := client.UploadDump(s.ctx, auth, f, size, report); err != nil {
				return output.New("PLATFORM_UNAVAILABLE", "The dump did not upload: "+err.Error()+".",
					"Run the same whisk db import again; nothing was loaded and the live database is untouched.", map[string]any{"import": auth.ID})
			}
			r, err := client.ImportDatabase(s.ctx, org, app, auth.ID, swap)
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
					fmt.Fprintf(w, "It loads in the background; follow it with whisk restore show %s --wait.\n", r.ID)
				}
			})
			return nil
		},
	}
	c.Flags().BoolVar(&swap, "swap", false, "make the loaded database the live one, keeping the previous one for 7 days")
	c.Flags().BoolVar(&wait, "wait", false, "follow the load until it is done or failed")
	return c
}

// openDump opens the file and checks it is a pg_dump custom-format archive, before anything is
// uploaded.
func openDump(path string) (*os.File, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, output.New("INVALID_REQUEST", "Cannot read "+path+": "+err.Error()+".", "Pass the path of a pg_dump -Fc file.", nil)
	}
	info, err := f.Stat()
	if err == nil && info.IsDir() {
		err = fmt.Errorf("it is a directory")
	}
	head := make([]byte, len(dumpMagic))
	if err == nil {
		_, err = io.ReadFull(f, head)
	}
	if err == nil && string(head) != dumpMagic {
		err = fmt.Errorf("it is not a pg_dump custom-format archive")
	}
	if err == nil {
		_, err = f.Seek(0, io.SeekStart)
	}
	if err != nil {
		f.Close()
		return nil, 0, output.New("INVALID_REQUEST", "Cannot import "+path+": "+err.Error()+".",
			"Make the dump in custom format: pg_dump -Fc --no-owner --no-acl -f app.dump \"$OLD_DATABASE_URL\", then whisk db import app.dump.", map[string]any{"file": path})
	}
	return f, info.Size(), nil
}

// uploadProgress prints how much of the dump has gone, every tenth of it, on log.
func uploadProgress(log io.Writer, size int64, quiet bool) func(int64) {
	next := int64(0)
	return func(sent int64) {
		if quiet || size == 0 {
			return
		}
		if pct := sent * 100 / size; pct >= next {
			fmt.Fprintf(log, "upload: %d%% of %d MB\n", pct, (size+(1<<20)-1)>>20)
			next = pct/10*10 + 10
		}
	}
}
