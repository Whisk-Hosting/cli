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
	"github.com/whisk-run/cli/internal/config"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/cli/internal/safefile"
	"github.com/whisk-run/cli/internal/stack"
	"github.com/whisk-run/cli/scaffold"
	"github.com/whisk-run/cli/templates"
	"github.com/whisk-run/contract/manifest"
)

func initCmd(s *session) *cobra.Command {
	var template, name string
	var noTemplate, noCreate bool
	c := &cobra.Command{
		Use:   "init",
		Short: "Write whisk.yaml for this directory (or copy a template into an empty one) and create the app",
		Long: `Detects the stack (package.json, pyproject.toml, go.mod, Dockerfile), writes a commented
whisk.yaml, adds .whisk/dev/ and .env* to .gitignore and, when --template is given into an empty
directory, copies the template. It then creates the app on the platform and binds the directory,
unless --no-create. Safe to run again: existing files are kept.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := s.env.Dir
			slug := name
			if slug == "" {
				var err error
				if slug, err = scaffold.Slug(filepath.Base(dir)); err != nil {
					return output.New("INVALID_REQUEST", err.Error(), "Pass --name <slug>.", nil)
				}
			} else if err := scaffold.CheckSlug(slug); err != nil {
				return output.New("INVALID_REQUEST", err.Error(), "Choose 3 to 40 characters of a-z, 0-9 and single hyphens.", nil)
			}
			var wrote, kept []string
			empty, err := scaffold.IsEmptyDir(dir)
			if err != nil {
				return err
			}
			manifestPath := filepath.Join(dir, "whisk.yaml")
			_, manifestErr := os.Stat(manifestPath)
			hasManifest := manifestErr == nil
			switch {
			case template != "" && !noTemplate && empty:
				paths, err := templates.Write(dir, template, slug)
				if err != nil {
					return output.New("INVALID_REQUEST", err.Error(), templateFix(err), map[string]any{"templates": templates.List()})
				}
				wrote = append(wrote, paths...)
				hasManifest = true
			case template != "" && !noTemplate && !empty && !hasManifest:
				code, _ := scaffold.CodeFiles(dir)
				return output.New("INVALID_REQUEST", "A template is only copied into a folder with no code, and this one holds "+namesList(code)+".",
					"If that is the app's code, run whisk init without --template to write whisk.yaml for it. Otherwise move those files out, or run whisk init --template "+template+" in a new folder. Dot-files and CLAUDE.md or AGENTS.md may stay.",
					map[string]any{"files": code})
			}
			if !hasManifest {
				files := scaffold.ListFiles(dir)
				st := stack.Detect(files, func(p string) []byte { b, _ := os.ReadFile(filepath.Join(dir, p)); return b })
				if err := safefile.Write(dir, "whisk.yaml", []byte(scaffold.Manifest(slug, st)), 0o644); err != nil {
					return err
				}
				wrote = append(wrote, "whisk.yaml")
			} else if !contains(wrote, "whisk.yaml") {
				kept = append(kept, "whisk.yaml")
			}
			m, err := loadManifest(dir)
			if err != nil {
				return err
			}
			if m.Name != slug && name != "" {
				return output.New("INVALID_REQUEST", fmt.Sprintf("whisk.yaml names the app %q but --name says %q.", m.Name, slug), "Change name in whisk.yaml or drop --name.", nil)
			}
			slug = m.Name
			existing, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
			merged, added := scaffold.MergeGitignore(string(existing))
			if len(added) > 0 {
				if err := safefile.Write(dir, ".gitignore", []byte(merged), 0o644); err != nil {
					return err
				}
				wrote = append(wrote, ".gitignore ("+strings.Join(added, ", ")+")")
			}
			if len(m.Functions) > 0 {
				for _, fn := range m.Functions {
					if _, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(fn.Graph))); err == nil {
						continue
					}
					if err := writeExampleGraph(dir, fn); err != nil {
						return err
					}
					wrote = append(wrote, fn.Graph)
				}
			}

			result := map[string]any{"name": slug, "wrote": wrote, "kept": kept}
			binding, bound, _ := config.LoadBinding(dir)
			var app *createdApp
			if !noCreate && !bound {
				created, err := createApp(s, slug)
				if err != nil {
					// One object under --json: the error, carrying what was written beside its own details.
					if !s.printer.JSON {
						fmt.Fprintf(s.env.Stdout, "Wrote %s for %s.\n", strings.Join(wrote, ", "), slug)
					}
					return withInitResult(err, withKeys(result, map[string]any{"created": false}))
				}
				app = &created
				binding = config.Binding{Org: created.OrgSlug, App: created.Slug, API: s.cfg.API}
				if err := config.SaveBinding(dir, binding); err != nil {
					return err
				}
				wrote = append(wrote, config.BindingPath)
				result["wrote"] = wrote
				result["app"] = created.App
				result["hostname"] = created.Hostname
				bound = true
			}
			result["created"] = app != nil
			if bound {
				result["org"] = binding.Org
				result["binding"] = binding
			}
			result["next"] = "whisk doctor, then whisk deploy"
			s.printer.Result(result, func(w io.Writer) {
				if len(wrote) > 0 {
					fmt.Fprintf(w, "Wrote %s.\n", strings.Join(wrote, ", "))
				}
				if len(kept) > 0 {
					fmt.Fprintf(w, "Kept %s.\n", strings.Join(kept, ", "))
				}
				switch {
				case app != nil:
					fmt.Fprintf(w, "Created app %s/%s: https://%s\n", app.OrgSlug, app.Slug, app.Hostname)
				case bound:
					fmt.Fprintf(w, "Bound to %s/%s.\n", binding.Org, binding.App)
				default:
					fmt.Fprintln(w, "App not created (--no-create); run whisk apps create "+slug+" when ready.")
				}
				fmt.Fprintln(w, "Next: whisk doctor, then whisk deploy.")
			})
			return nil
		},
	}
	c.Flags().StringVar(&template, "template", "", "copy a template into an empty directory: ts, py or go")
	c.Flags().StringVar(&name, "name", "", "app slug (default: the directory name)")
	c.Flags().BoolVar(&noTemplate, "no-template", false, "never copy a template")
	c.Flags().BoolVar(&noCreate, "no-create", false, "write files only; do not create the app on the platform")
	return c
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// withInitResult carries what init wrote into the error that stopped it, as details.init, so
// --json still prints one object and the agent knows the files are there.
func withInitResult(err error, result map[string]any) error {
	e, ok := err.(*output.Error)
	if !ok {
		e = &output.Error{Code: "CLI_ERROR", Message: err.Error(), Fix: "The files are written; fix what the message names and run whisk init again, which keeps them."}
	}
	out := *e
	out.Details = map[string]any{}
	for k, v := range e.Details {
		out.Details[k] = v
	}
	out.Details["init"] = result
	return &out
}

func withKeys(m map[string]any, extra map[string]any) map[string]any {
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func writeExampleGraph(dir string, fn manifest.Function) error {
	src := fmt.Sprintf(`# Declared graph for %s. One entry per step the code runs, in order; a decision is an id
# ending in ? with branches instead of next. whisk doctor checks it against the code.
steps:
  - id: prepare
    label: Gather what the step needs
    next: record
  - id: record
    label: Write the result
`, fn.Name)
	return safefile.Write(dir, fn.Graph, []byte(src), 0o644)
}

// createdApp is an app with its org slug, which the API's app object carries only as an id.
type createdApp struct {
	api.App
	OrgSlug string
}

func createApp(s *session, slug string) (createdApp, error) {
	client, _, err := s.client()
	if err != nil {
		return createdApp{}, err
	}
	org, err := s.org()
	if err != nil {
		return createdApp{}, err
	}
	app, err := client.CreateApp(s.ctx, org, api.CreateAppRequest{Slug: slug, Name: slug})
	if err != nil {
		return createdApp{}, wrap(err)
	}
	return createdApp{App: app, OrgSlug: org}, nil
}

func appsCmd(s *session) *cobra.Command {
	apps := &cobra.Command{Use: "apps", Short: "Apps in the org"}
	create := &cobra.Command{
		Use:   "create <slug>",
		Short: "Create an app: its repository and hostname come back",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := scaffold.CheckSlug(args[0]); err != nil {
				return output.New("INVALID_REQUEST", err.Error(), "Choose 3 to 40 characters of a-z, 0-9 and single hyphens.", nil)
			}
			created, err := createApp(s, args[0])
			if err != nil {
				return err
			}
			s.printer.Result(map[string]any{"app": created.App, "org": created.OrgSlug}, func(w io.Writer) {
				fmt.Fprintf(w, "Created %s/%s: https://%s\nBind a directory with: whisk use %s/%s\n", created.OrgSlug, created.Slug, created.Hostname, created.OrgSlug, created.Slug)
			})
			return nil
		},
	}

	list := &cobra.Command{
		Use:   "list",
		Short: "List the org's apps",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := s.client()
			if err != nil {
				return err
			}
			org, err := s.org()
			if err != nil {
				return err
			}
			all, err := client.ListApps(s.ctx, org)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "apps": all}, func(w io.Writer) {
				rows := make([][]string, len(all))
				for i, a := range all {
					rows[i] = []string{a.Slug, string(a.Status), a.Region, "https://" + a.Hostname}
				}
				s.printer.Table(w, []string{"APP", "STATUS", "REGION", "URL"}, rows)
			})
			return nil
		},
	}

	info := &cobra.Command{
		Use:   "info [<slug>]",
		Short: "Show one app",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				s.appFlag = args[0]
			}
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			a, err := client.GetApp(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": a}, func(w io.Writer) {
				fmt.Fprintf(w, "%s/%s  %s  %s\n  https://%s\n  git %s\n  created %s\n", org, a.Slug, a.Status, a.Region, a.Hostname, a.GitURL, a.CreatedAt.Local().Format("2006-01-02 15:04"))
				for _, line := range memoryLines(a.Memory, time.Now()) {
					fmt.Fprintln(w, "  "+line)
				}
			})
			return nil
		},
	}

	var yes bool
	del := &cobra.Command{
		Use:   "delete <slug>",
		Short: "Delete an app and everything it holds",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s.appFlag = args[0]
			org, app, err := s.target()
			if err != nil {
				return err
			}
			if !yes {
				return output.New("NEEDS_HUMAN", fmt.Sprintf("Deleting %s/%s stops it now and removes its repository, database, files and deploys after 7 days; whisk apps restore brings it back until then.", org, app), "Run again with --yes once a human has confirmed.", map[string]any{"org": org, "app": app})
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			if err := client.DeleteApp(s.ctx, org, app); err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "deleted": true}, func(w io.Writer) {
				fmt.Fprintf(w, "Deleted %s/%s. whisk apps deleted lists it for 7 days, and whisk apps restore brings it back.\n", org, app)
			})
			return nil
		},
	}
	del.Flags().BoolVar(&yes, "yes", false, "a human has confirmed")
	deleted := &cobra.Command{
		Use:   "deleted",
		Short: "List deleted apps that can still be restored",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := s.client()
			if err != nil {
				return err
			}
			org, err := s.org()
			if err != nil {
				return err
			}
			all, err := client.ListDeletedApps(s.ctx, org)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "apps": all}, func(w io.Writer) {
				if len(all) == 0 {
					fmt.Fprintln(w, "No deleted apps to restore.")
					return
				}
				rows := make([][]string, len(all))
				for i, a := range all {
					rows[i] = []string{a.Slug, a.ID, a.DeletedAt.Local().Format("2006-01-02 15:04"), a.ReleaseAt.Local().Format("2006-01-02 15:04")}
				}
				s.printer.Table(w, []string{"APP", "ID", "DELETED", "GONE AFTER"}, rows)
			})
			return nil
		},
	}
	restore := &cobra.Command{
		Use:   "restore <id>",
		Short: "Bring back a deleted app, stopped, until its 7 days are up",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := s.client()
			if err != nil {
				return err
			}
			org, err := s.org()
			if err != nil {
				return err
			}
			a, err := client.RestoreDeletedApp(s.ctx, org, args[0])
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": a, "restored": true}, func(w io.Writer) {
				fmt.Fprintf(w, "Restored %s/%s. It is stopped: run whisk deploy to start it.\n", org, a.Slug)
			})
			return nil
		},
	}
	apps.AddCommand(create, list, info, del, deleted, restore)
	return apps
}

// memoryLines is the memory part of whisk apps info: the limit, now and peak, a warning when the
// peak is near the limit, and the last time the app was killed for going over it. /tmp is
// memory, so it is named where it matters.
func memoryLines(m *api.AppMemory, now time.Time) []string {
	if m == nil || m.LimitBytes <= 0 {
		return nil
	}
	mib := func(b int64) int64 { return b >> 20 }
	pct := m.PeakBytes * 100 / m.LimitBytes
	out := []string{fmt.Sprintf("memory %d MiB now, peak %d MiB of %d MiB (%d%%); /tmp counts", mib(m.UsedBytes), mib(m.PeakBytes), mib(m.LimitBytes), pct)}
	if m.LastOutOfMemoryAt != nil && now.Sub(*m.LastOutOfMemoryAt) < 7*24*time.Hour {
		out = append(out, fmt.Sprintf("killed for memory %s ago (APP_OUT_OF_MEMORY): reduce peak memory or keep less in /tmp", now.Sub(*m.LastOutOfMemoryAt).Round(time.Minute)))
	} else if pct >= 80 {
		out = append(out, "near the memory limit: over it the app is killed without SIGTERM; reduce peak memory or keep less in /tmp")
	}
	return out
}

// templateFix is the fix for a template that could not be written: a file of the template
// already exists (a .gitignore, a README.md), or the template name is unknown. Pure.
func templateFix(err error) string {
	if strings.Contains(err.Error(), " exists;") {
		return "Move that file aside (the template writes its own), run whisk init --template again, then merge anything you need back."
	}
	return "Use --template ts, py or go."
}

// namesList is up to five names, then how many more. Pure.
func namesList(names []string) string {
	if len(names) <= 5 {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:5], ", "), len(names)-5)
}
