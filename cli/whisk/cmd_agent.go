package whisk

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract"
	"github.com/whisk-run/contract/errors"
	"github.com/whisk-run/contract/manifest"
)

func versionCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the CLI, API and conventions versions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s.printer.Result(map[string]any{"conventions": contract.Version}, func(w io.Writer) {
				fmt.Fprintf(w, "whisk %s (api %s, conventions %d)\n", Version, output.APIVersion, contract.Version)
			})
			return nil
		},
	}
}

func skillCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "skill",
		Short: "Print the skill document an agent reads first",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			s.printer.Result(map[string]any{"skill": contract.Skill, "conventions": contract.Version}, func(w io.Writer) {
				fmt.Fprint(w, contract.Skill)
			})
			return nil
		},
	}
}

func errorsCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "errors <CODE>",
		Short: "Explain an error code and its fix",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			code := strings.ToUpper(args[0])
			entry, ok := errors.Lookup(code)
			if !ok {
				return output.New("NOT_FOUND", code+" is not a known error code.", "Run whisk errors with one of the codes from errors.md; codes are upper snake case.", map[string]any{"known": errors.Codes()})
			}
			section, _ := errors.Section(code)
			s.printer.Result(map[string]any{
				"code": entry.Code, "status": entry.Status, "surfaces": entry.Surfaces,
				"when": entry.When, "fix": entry.Fix, "docs": errors.DocsBase + code, "example": entry.Example,
			}, func(w io.Writer) { fmt.Fprint(w, section) })
			return nil
		},
	}
}

func schemaCmd(s *session) *cobra.Command {
	var graph bool
	c := &cobra.Command{
		Use:   "schema",
		Short: "Print the whisk.yaml JSON schema (or the graph schema with --graph)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			src := contract.ManifestSchema
			if graph {
				src = contract.GraphSchema
			}
			fmt.Fprint(s.env.Stdout, string(src))
			if !strings.HasSuffix(string(src), "\n") {
				fmt.Fprintln(s.env.Stdout)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&graph, "graph", false, "print graph.schema.json instead")
	return c
}

// cronEntry is one cron function as whisk cron list prints it.
type cronEntry struct {
	Name   string   `json:"name"`
	Cron   string   `json:"cron"`
	TZ     string   `json:"tz"`
	RunsAs string   `json:"runs_as,omitempty"`
	Next   []string `json:"next"`
}

// deployedCrons is the cron triggers of the deployed functions, one entry each.
func deployedCrons(fns []api.Function) []cronEntry {
	var out []cronEntry
	for _, f := range fns {
		for _, t := range f.Triggers {
			if t.Cron == "" {
				continue
			}
			e := cronEntry{Name: f.Name, Cron: t.Cron, TZ: t.TZ, RunsAs: t.RunsAs, Next: []string{}}
			if e.TZ == "" {
				e.TZ = "UTC"
			}
			if t.NextRun != nil {
				e.Next = []string{t.NextRun.UTC().Format(time.RFC3339)}
			}
			out = append(out, e)
		}
	}
	return out
}

// cronRows is the table rows: the next run in local time, and the schedule a plan runs it as
// beside the declared one when the two differ.
func cronRows(entries []cronEntry) [][]string {
	rows := make([][]string, len(entries))
	for i, e := range entries {
		next := "never"
		if len(e.Next) > 0 {
			t, _ := time.Parse(time.RFC3339, e.Next[0])
			next = t.Local().Format("2006-01-02 15:04 MST")
		}
		cron := e.Cron
		if e.RunsAs != "" && e.RunsAs != e.Cron {
			cron += " (runs as " + e.RunsAs + ")"
		}
		rows[i] = []string{e.Name, cron, e.TZ, next}
	}
	return rows
}

func (s *session) printDeployedCrons(org, app string, fns []api.Function) error {
	entries := deployedCrons(fns)
	s.printer.Result(map[string]any{"org": org, "app": app, "source": "deployed", "functions": entries}, func(w io.Writer) {
		if len(entries) == 0 {
			fmt.Fprintf(w, "%s/%s has no cron functions deployed.\n", org, app)
			return
		}
		s.printer.Table(w, []string{"FUNCTION", "CRON", "TZ", "NEXT RUN"}, cronRows(entries))
	})
	return nil
}

func cronCmd(s *session) *cobra.Command {
	cron := &cobra.Command{Use: "cron", Short: "Scheduled functions"}
	list := &cobra.Command{
		Use:   "list",
		Short: "List the app's cron functions as deployed, or from whisk.yaml before a deploy, with their next run times",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// What is deployed is what runs, so an app this directory is bound to, with a
			// token, is read from the platform; only without either is the local whisk.yaml read.
			if org, app, err := s.target(); err == nil {
				if client, _, err := s.client(); err == nil {
					fns, err := client.ListFunctions(s.ctx, org, app)
					if err != nil {
						return wrap(err)
					}
					return s.printDeployedCrons(org, app, fns)
				}
			}
			m, err := loadManifest(s.env.Dir)
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			var entries []cronEntry
			for _, fn := range m.Functions {
				if fn.Cron == "" {
					continue
				}
				tz := fn.TZ
				if tz == "" {
					tz = "UTC"
				}
				loc, err := time.LoadLocation(tz)
				if err != nil {
					loc = time.UTC
				}
				e := cronEntry{Name: fn.Name, Cron: fn.Cron, TZ: tz}
				after := now
				for i := 0; i < 3; i++ {
					next, ok := manifest.NextCron(fn.Cron, loc, after)
					if !ok {
						break
					}
					e.Next = append(e.Next, next.UTC().Format(time.RFC3339))
					after = next
				}
				entries = append(entries, e)
			}
			s.printer.Result(map[string]any{"source": "whisk.yaml", "functions": entries}, func(w io.Writer) {
				fmt.Fprintln(w, "From whisk.yaml in this directory, not what is deployed: bind the directory (whisk use <org>/<app>) and sign in to see the deployed schedules.")
				if len(entries) == 0 {
					fmt.Fprintln(w, "No cron functions in whisk.yaml.")
					return
				}
				s.printer.Table(w, []string{"FUNCTION", "CRON", "TZ", "NEXT RUN"}, cronRows(entries))
			})
			return nil
		},
	}
	run := &cobra.Command{
		Use:   "run <function>",
		Short: "Start one run of a cron function now, as its schedule would; prints the run to follow",
		Example: `  whisk cron run nightly-margin           check a cron function right after a deploy
  whisk runs show <id>                    then follow the run it prints`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			r, err := client.RunCron(s.ctx, org, app, args[0])
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "function": args[0], "run": r}, func(w io.Writer) {
				fmt.Fprintln(w, cronRunStarted(args[0], r))
			})
			return nil
		},
	}
	cron.AddCommand(list, run)
	return cron
}

// cronRunStarted is what whisk cron run says it started: the run to follow, or, when the engine
// had not listed the run yet, the event that starts it.
func cronRunStarted(function string, r api.Run) string {
	if r.ID == "" {
		return fmt.Sprintf("Started %s by its event %s, as its schedule would; the engine has not listed the run yet. Find it with whisk runs list --event %s.", function, r.EventID, r.EventID)
	}
	return fmt.Sprintf("Started %s as %s (%s), as its schedule would. Follow it with whisk runs show %s, or whisk runs list %s.", function, r.ID, r.Status, r.ID, function)
}

// loadManifest reads and validates whisk.yaml in dir, as an agent-readable error.
func loadManifest(dir string) (manifest.Manifest, error) {
	src, err := os.ReadFile(filepath.Join(dir, "whisk.yaml"))
	if err != nil {
		return manifest.Manifest{}, output.New("MANIFEST_INVALID", "whisk.yaml was not found in "+dir+".", "Run whisk init to write one, or run the command from the app directory.", nil)
	}
	m, err := manifest.Parse(src)
	if err != nil {
		ps, _ := err.(manifest.Problems)
		problems := make([]map[string]string, 0, len(ps))
		for _, p := range ps {
			problems = append(problems, map[string]string{"path": p.Path, "message": p.Message, "code": p.Code})
		}
		code := "MANIFEST_INVALID"
		if len(ps) > 0 {
			code = ps.Code()
		}
		return m, output.New(code, err.Error(), "", map[string]any{"problems": problems})
	}
	return m, nil
}
