package whisk

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
)

// scanCmd is whisk scan (CLI.md §5, CONTROL-PLANE.md §6.28): the app's packages checked against
// public vulnerability lists, with the version that fixes each finding.
func scanCmd(s *session) *cobra.Command {
	var now bool
	c := &cobra.Command{
		Use:   "scan",
		Short: "Known vulnerabilities in the live app's packages and the versions that fix them (Business plan)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return wrap(err)
			}
			if now {
				queued, err := client.ScanPackages(s.ctx, org, app)
				if err != nil {
					return wrap(err)
				}
				s.printer.Progress("scan: checking %s's packages (%s)", app, queued.ID)
				done, err := followScan(s.ctx, client, org, app, queued.ID, scanEvery, 10*time.Minute)
				if err != nil {
					return wrap(err)
				}
				if done.Status != "done" {
					return output.New("PLATFORM_UNAVAILABLE", "The package check "+done.ID+" ended "+string(done.Status)+": "+orDash(done.Error),
						"Run whisk scan --now again later; whisk scan shows the newest check that finished.", map[string]any{"id": done.ID, "status": done.Status})
				}
			}
			p, err := client.GetPackages(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			result := map[string]any{"org": org, "app": app, "included": p.Included, "scan": p.Scan, "pending": p.Pending}
			if !p.Included {
				result["note"] = scanNotIncluded
			}
			s.printer.Result(result, func(w io.Writer) { printPackages(s.printer, w, app, p) })
			return nil
		},
	}
	c.Flags().BoolVar(&now, "now", false, "check the live build again now and wait for the result")
	return c
}

// scanEvery is how often --now asks after its check.
var scanEvery = 3 * time.Second

const scanNotIncluded = "Package scanning is included in the Business plan."

// followScan asks after a check until it ends or the wait runs out.
func followScan(ctx context.Context, client *api.Client, org, app, id string, every, limit time.Duration) (api.PackageScan, error) {
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	for {
		p, err := client.GetPackageScan(ctx, org, app, id)
		if err != nil {
			return p, err
		}
		switch p.Status {
		case "done", "failed", "skipped":
			return p, nil
		}
		select {
		case <-ctx.Done():
			return p, output.New("PLATFORM_UNAVAILABLE", "The package check "+id+" has not finished.",
				"Run whisk scan in a few minutes for its result.", map[string]any{"id": id, "status": p.Status})
		case <-time.After(every):
		}
	}
}

// scanSummary is a scan's counts in words: "2 to fix now, 14 in all (1 critical, 1 high, ...)".
func scanSummary(c api.PackageCounts) string {
	if c.Total() == 0 {
		return "no known vulnerabilities"
	}
	parts := []string{}
	for _, p := range []struct {
		n    int
		word string
	}{{c.Critical, "critical"}, {c.High, "high"}, {c.Medium, "medium"}, {c.Low, "low"}, {c.Unknown, "unknown"}} {
		if p.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", p.n, p.word))
		}
	}
	return fmt.Sprintf("%d to fix now, %d in all (%s)", c.Attention, c.Total(), strings.Join(parts, ", "))
}

// printPackages is the human view: the summary, a row per finding, and what to do.
func printPackages(p output.Printer, w io.Writer, app string, pk api.Packages) {
	if !pk.Included {
		fmt.Fprintln(w, scanNotIncluded)
		return
	}
	if pk.Pending != nil {
		fmt.Fprintf(w, "A check is %s (%s).\n", pk.Pending.Status, pk.Pending.ID)
	}
	if pk.Scan == nil {
		fmt.Fprintf(w, "No check of %s has finished yet. It runs after each production deploy and daily.\n", app)
		return
	}
	sc := pk.Scan
	fmt.Fprintf(w, "%s: %s. Checked %s on %s.\n", app, scanSummary(sc.Counts), at(sc.FinishedAt), short(sc.CommitSHA))
	if len(sc.Findings) == 0 {
		return
	}
	rows := make([][]string, len(sc.Findings))
	for i, f := range sc.Findings {
		where := f.Path
		if where == "" {
			where = string(f.Where)
		}
		rows[i] = []string{string(f.Severity), f.Package, f.Version, orDash(f.Fixed), f.ID, where}
	}
	p.Table(w, []string{"SEVERITY", "PACKAGE", "INSTALLED", "FIXED IN", "ID", "FOUND IN"}, rows)
	if sc.Counts.Total() > len(sc.Findings) {
		fmt.Fprintf(w, "The %d most serious of %d are shown.\n", len(sc.Findings), sc.Counts.Total())
	}
	fmt.Fprintln(w, "Update each package to its FIXED IN version or newer (a base image package by moving to a newer base image), deploy, then run whisk scan --now.")
}

// packagesLine is whisk status's line about the newest check, or "" when there is nothing to say.
func packagesLine(pk api.Packages) string {
	if !pk.Included || pk.Scan == nil {
		return ""
	}
	if pk.Scan.Counts.Attention == 0 {
		return "packages: nothing to fix now (whisk scan lists all)"
	}
	return fmt.Sprintf("packages: %d known vulnerabilit%s to fix now; run whisk scan", pk.Scan.Counts.Attention, map[bool]string{true: "y", false: "ies"}[pk.Scan.Counts.Attention == 1])
}
