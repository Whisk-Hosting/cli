package whisk

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
)

// queryCmd runs SQL on the app's database through the API (CLI.md §5.9). Reads and additions
// run at once; SQL that would change or remove existing data comes back as a preview and a
// code (CONFIRM_REQUIRED), which --confirm runs.
func queryCmd(s *session) *cobra.Command {
	var environment, database, file, confirm string
	var limit int
	c := &cobra.Command{
		Use:   `query ["<sql>" | --file script.sql | --confirm <code>] [--env name] [--database name] [--limit 1000]`,
		Short: "Run SQL on the app's database and print the rows",
		Long: `Runs SQL on the environment's database on the platform, in one transaction, and prints the
last statement's rows and what the SQL did. Values come back as PostgreSQL prints them.

Reads and additions (select, insert, create) run at once. SQL that would change or remove
existing data (update, delete, upserts, drop, alter, truncate, create or replace) is tried,
rolled back and answered with CONFIRM_REQUIRED: the effect, and a code. Run
whisk db query --confirm <code> within ten minutes to do it; a restore point is taken first
and the SQL commits only if it does the same again.

Statements run for at most 30 seconds, at most --limit rows are printed (10000 at most), and
every call is audited.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := api.QueryRequest{Environment: environment, Database: database, Confirm: confirm, Limit: limit}
			sql, err := querySQL(s, args, file)
			if err != nil {
				return err
			}
			switch {
			case confirm != "" && sql != "":
				return output.New("INVALID_REQUEST", "--confirm runs the SQL its preview held back, so it takes no SQL of its own.", "Run whisk db query --confirm <code> alone.", nil)
			case confirm == "" && sql == "":
				return output.New("INVALID_REQUEST", "There is no SQL to run.", `Pass the SQL as the argument, a file with --file, or - to read it from stdin.`, nil)
			}
			req.SQL = sql
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			res, err := client.QueryDatabase(s.ctx, org, app, req)
			if err != nil {
				return wrap(err)
			}
			printQuery(s, org, app, res)
			return nil
		},
	}
	c.Flags().StringVar(&environment, "env", "", "the environment; production when omitted")
	c.Flags().StringVar(&database, "database", "", "one of the environment's restored copies instead of its database")
	c.Flags().StringVar(&file, "file", "", "read the SQL from this file")
	c.Flags().StringVar(&confirm, "confirm", "", "run the SQL a preview held back, by its code")
	c.Flags().IntVar(&limit, "limit", 1000, "the most rows to print")
	return c
}

// querySQL is the SQL from the argument, --file, or stdin when the argument is -.
func querySQL(s *session, args []string, file string) (string, error) {
	if file != "" && len(args) > 0 {
		return "", output.New("INVALID_REQUEST", "Give the SQL as the argument or with --file, not both.", "Drop one of them.", nil)
	}
	switch {
	case file != "":
		path := file
		if !filepath.IsAbs(path) {
			path = filepath.Join(s.env.Dir, path)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", output.New("INVALID_REQUEST", "The file could not be read: "+err.Error(), "Check the path.", map[string]any{"file": file})
		}
		return string(decodeText(raw)), nil
	case len(args) == 1 && args[0] == "-":
		raw, err := io.ReadAll(s.env.Stdin)
		if err != nil {
			return "", err
		}
		return string(decodeText(raw)), nil
	case len(args) == 1:
		return args[0], nil
	}
	return "", nil
}

func printQuery(s *session, org, app string, res api.QueryResult) {
	v := map[string]any{"org": org, "app": app, "environment": res.Environment, "database": res.Database, "columns": res.Columns, "rows": res.Rows,
		"row_count": res.RowCount, "truncated": res.Truncated, "command": res.Command, "effect": res.Effect, "duration_ms": res.DurationMS}
	if res.RestorePoint != nil {
		v["restore_point"] = res.RestorePoint
	}
	s.printer.Result(v, func(w io.Writer) { printRows(s, w, res) })
}

// printRows prints a result as a table with a closing line, or the command tag alone.
func printRows(s *session, w io.Writer, res api.QueryResult) {
	if len(res.Columns) == 0 {
		fmt.Fprintln(w, res.Command)
	} else {
		rows := make([][]string, 0, len(res.Rows))
		for _, r := range res.Rows {
			cells := make([]string, len(r))
			for i, v := range r {
				if v == nil {
					cells[i] = "null"
				} else {
					cells[i] = strings.ReplaceAll(*v, "\n", `\n`)
				}
			}
			rows = append(rows, cells)
		}
		s.printer.Table(w, res.Columns, rows)
		if res.Truncated {
			fmt.Fprintf(w, "(%d rows, first %d shown; raise --limit for more)\n", res.RowCount, len(res.Rows))
		} else {
			fmt.Fprintf(w, "(%d rows)\n", res.RowCount)
		}
	}
	if res.RestorePoint != nil {
		fmt.Fprintf(w, "Restore point %s taken first. %s\n", res.RestorePoint.ID, undoLine(*res.RestorePoint))
	}
}

// dbSchemaCmd prints the tables, columns and keys in one call (CLI.md §5.9).
func dbSchemaCmd(s *session) *cobra.Command {
	var environment, database string
	c := &cobra.Command{
		Use:   "schema [--env name] [--database name]",
		Short: "Print the database's tables, columns, keys and estimated row counts",
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
			sc, err := client.DatabaseSchema(s.ctx, org, app, environment, database)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "environment": sc.Environment, "database": sc.Database, "tables": sc.Tables, "truncated": sc.Truncated}, func(w io.Writer) {
				if len(sc.Tables) == 0 {
					fmt.Fprintf(w, "%s has no tables yet.\n", sc.Database)
					return
				}
				for _, t := range sc.Tables {
					rows := "rows not counted yet"
					if t.RowEstimate != nil {
						rows = fmt.Sprintf("about %d rows", *t.RowEstimate)
					}
					if t.Kind != "table" {
						rows = strings.ReplaceAll(t.Kind, "_", " ")
					}
					fmt.Fprintf(w, "%s (%s)\n", s.printer.Bold(t.Schema+"."+t.Name), rows)
					for _, col := range t.Columns {
						line := "  " + col.Name + " " + col.Type
						if !col.Nullable {
							line += " not null"
						}
						if col.Default != nil {
							line += " default " + *col.Default
						}
						fmt.Fprintln(w, line)
					}
					for _, k := range t.Constraints {
						fmt.Fprintln(w, "  "+s.printer.Dim(k.Definition))
					}
				}
				if sc.Truncated {
					fmt.Fprintf(w, "(first %d tables shown)\n", len(sc.Tables))
				}
			})
			return nil
		},
	}
	c.Flags().StringVar(&environment, "env", "", "the environment; production when omitted")
	c.Flags().StringVar(&database, "database", "", "one of the environment's restored copies instead of its database")
	return c
}

// shellCmd is an interactive SQL prompt over db query, needing nothing but the CLI: each
// statement ending in a semicolon runs, and one that would change or remove existing data
// shows its effect and runs only when yes is typed.
func shellCmd(s *session) *cobra.Command {
	var environment, database string
	c := &cobra.Command{
		Use:   "shell [--env name] [--database name]",
		Short: "Type SQL at a prompt; each statement runs through whisk db query",
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
			name := environment
			if name == "" {
				name = "production"
			}
			fmt.Fprintf(s.env.Stderr, "SQL on %s/%s (%s). End each statement with ; and press Ctrl-D to leave.\n", org, app, name)
			in := bufio.NewScanner(s.env.Stdin)
			in.Buffer(make([]byte, 64<<10), 1<<20)
			var buf strings.Builder
			prompt := func() {
				if buf.Len() == 0 {
					fmt.Fprint(s.env.Stderr, name+"=> ")
				} else {
					fmt.Fprint(s.env.Stderr, name+"-> ")
				}
			}
			prompt()
			for in.Scan() {
				buf.WriteString(in.Text())
				buf.WriteString("\n")
				if !strings.HasSuffix(strings.TrimSpace(in.Text()), ";") {
					prompt()
					continue
				}
				sql := buf.String()
				buf.Reset()
				s.shellRun(client, org, app, api.QueryRequest{SQL: sql, Environment: environment, Database: database}, in)
				prompt()
			}
			fmt.Fprintln(s.env.Stderr)
			return nil
		},
	}
	c.Flags().StringVar(&environment, "env", "", "the environment; production when omitted")
	c.Flags().StringVar(&database, "database", "", "one of the environment's restored copies instead of its database")
	return c
}

// shellRun runs one statement at the prompt, asking before a destructive one.
func (s *session) shellRun(client *api.Client, org, app string, req api.QueryRequest, in *bufio.Scanner) {
	res, err := client.QueryDatabase(s.ctx, org, app, req)
	var ae *api.Error
	if errors.As(err, &ae) && ae.Code == "CONFIRM_REQUIRED" {
		code, _ := ae.Details["code"].(string)
		fmt.Fprintln(s.env.Stderr, ae.Message)
		fmt.Fprint(s.env.Stderr, "Type yes to do it (a restore point is taken first): ")
		if !in.Scan() || strings.TrimSpace(strings.ToLower(in.Text())) != "yes" {
			fmt.Fprintln(s.env.Stderr, "Not run; nothing was changed.")
			return
		}
		res, err = client.QueryDatabase(s.ctx, org, app, api.QueryRequest{Confirm: code})
	}
	if err != nil {
		if e, ok := wrap(err).(*output.Error); ok {
			s.printer.Block(s.env.Stderr, e)
		} else {
			fmt.Fprintln(s.env.Stderr, err)
		}
		return
	}
	printRows(s, s.env.Stdout, res)
}
