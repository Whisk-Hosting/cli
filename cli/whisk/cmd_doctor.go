package whisk

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/doctor"
	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/config"
	"github.com/whisk-run/cli/internal/output"
	rules "github.com/whisk-run/contract/doctor"
)

func doctorCmd(s *session) *cobra.Command {
	var fix bool
	c := &cobra.Command{
		Use:   "doctor",
		Short: "Check the repository against the conventions before pushing",
		Long: `Runs every rule from doctor-rules.md locally. When the directory is bound to an app and a token is
available, the platform is asked what only it knows: plan limits and, on the Business plan, a
check of the local lockfiles before they are deployed and the live image's package findings.
Exit 3 when any error remains.
--fix applies the safe fixes and reports what changed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := doctor.Options{Dir: s.env.Dir, Fix: fix}
			if b, ok, _ := config.LoadBinding(s.env.Dir); ok || (s.orgFlag != "" && s.appFlag != "") {
				org, app := b.Org, b.App
				if s.orgFlag != "" && s.appFlag != "" {
					org, app = s.orgFlag, s.appFlag
				}
				if client, _, err := s.client(); err == nil {
					opts.Validate = func(ctx context.Context, m []byte) (api.Validation, error) {
						return client.Validate(ctx, org, app, m)
					}
					opts.Packages = func(ctx context.Context) (api.Packages, error) {
						return client.GetPackages(ctx, org, app)
					}
					opts.CheckLockfiles = func(ctx context.Context, files []api.Lockfile) (api.PackageCheck, error) {
						return client.CheckPackages(ctx, org, app, files)
					}
					if ok {
						// W006 checks the binding itself, whatever --org and --app say.
						opts.BoundGitURL = func(ctx context.Context) (string, error) {
							a, err := client.GetApp(ctx, b.Org, b.App)
							return a.GitURL, err
						}
					}
				}
			}
			rep, err := doctor.Run(s.ctx, opts)
			if err != nil {
				return err
			}
			result := map[string]any{"ok": rep.OK(), "errors": rep.Errors, "warnings": rep.Warnings, "findings": nonNil(rep.Findings), "fixed": nonNilStrings(rep.Fixed), "skipped": nonNilStrings(rep.Skipped)}
			if rep.Plan != "" {
				result["plan"] = rep.Plan
			}
			if rep.OK() {
				s.printer.Result(result, func(w io.Writer) { printReport(s.printer, w, rep) })
				return nil
			}
			// --json prints one object: the error, with the whole report as its details.
			if !s.printer.JSON {
				printReport(s.printer, s.env.Stdout, rep)
			}
			e := output.New("DOCTOR_FAILED", strconv.Itoa(rep.Errors)+" error(s) must be fixed before deploying.",
				"Fix each finding (details.findings, or the list above; the fix says how); whisk doctor --fix applies the safe ones.", result)
			e.Exit = output.ExitValidation
			return e
		},
	}
	c.Flags().BoolVar(&fix, "fix", false, "apply the safe fixes")
	return c
}

func nonNil(f []doctor.Finding) []doctor.Finding {
	if f == nil {
		return []doctor.Finding{}
	}
	return f
}

func nonNilStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func printReport(p output.Printer, w io.Writer, rep doctor.Report) {
	for _, n := range rep.Fixed {
		fmt.Fprintln(w, p.Good("fixed")+"  "+n)
	}
	for _, f := range rep.Findings {
		level := p.Warn("warning")
		if f.Level == rules.Error {
			level = p.Bad("error")
		}
		where := ""
		if f.File != "" {
			where = f.File
			if f.Line > 0 {
				where += ":" + strconv.Itoa(f.Line)
			}
			where = "  " + p.Dim(where)
		}
		fmt.Fprintf(w, "%s %s%s\n    %s\n    %s\n", p.Bold(f.Rule), level, where, f.Message, p.Dim("fix: "+f.Fix))
	}
	for _, sk := range rep.Skipped {
		fmt.Fprintln(w, p.Dim(sk))
	}
	switch {
	case rep.Errors == 0 && rep.Warnings == 0:
		fmt.Fprintln(w, p.Good("Doctor found nothing to fix."))
	default:
		fmt.Fprintf(w, "%d error(s), %d warning(s).\n", rep.Errors, rep.Warnings)
	}
}
