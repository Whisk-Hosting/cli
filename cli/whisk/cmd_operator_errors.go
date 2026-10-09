package whisk

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
)

// operatorErrorsCmd is Whisk's own error log (CLI.md §5.14, CONTROL-PLANE.md §6.31): every
// problem the platform's services logged and every crash in people's browsers, one row per
// problem, for an operator or the operator's agent to review and resolve like feedback.
func operatorErrorsCmd(s *session) *cobra.Command {
	var q api.PlatformErrorQuery
	root := &cobra.Command{
		Use:   "errors",
		Short: "Whisk's own errors, grouped, to review and resolve",
		Long: `Lists the problems the platform's services logged and the crashes people's browsers reported,
one row per problem, most recently seen first. Open ones by default; --status resolved, ignored or
all for the rest, --source for one service (whiskd.service, caddy.service) or browser.
Read one with whisk operator errors show <id>. Resolve one once it is fixed: it opens again if
it happens again. Ignore one that is noise: it keeps counting and stays quiet.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := s.client()
			if err != nil {
				return err
			}
			page, err := client.PlatformErrors(s.ctx, q)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"errors": page.Items, "next_cursor": page.NextCursor, "open": page.Open, "scanned_to": page.ScannedTo}, func(w io.Writer) {
				if len(page.Items) == 0 {
					fmt.Fprintf(w, "No %s errors. Scanned the platform's logs up to %s.\n", orAll(q.Status), ago(page.ScannedTo))
					return
				}
				rows := make([][]string, len(page.Items))
				for i, e := range page.Items {
					rows[i] = []string{e.ID, e.Status, strconv.FormatInt(e.Count, 10), ago(e.LastSeen), strings.TrimSuffix(e.Source, ".service"), e.Title}
				}
				s.printer.Table(w, []string{"ID", "STATUS", "COUNT", "LAST SEEN", "SOURCE", "ERROR"}, rows)
				fmt.Fprintf(w, "\n%d open. Scanned up to %s.", page.Open, ago(page.ScannedTo))
				if page.NextCursor != "" {
					fmt.Fprintf(w, " More with --cursor %s.", page.NextCursor)
				}
				fmt.Fprintln(w)
			})
			return nil
		},
	}
	f := root.Flags()
	f.StringVar(&q.Status, "status", "", "open (the default), resolved, ignored or all")
	f.StringVar(&q.Source, "source", "", "one service, such as whiskd.service, or browser")
	f.StringVar(&q.Cursor, "cursor", "", "the cursor the last page printed")
	f.IntVar(&q.Limit, "limit", 50, "how many to list")
	root.AddCommand(operatorErrorShowCmd(s),
		operatorErrorStatusCmd(s, "resolve", "resolved", "Mark an error fixed; it opens again if it happens again"),
		operatorErrorStatusCmd(s, "ignore", "ignored", "Mark an error as noise; it keeps counting and stays quiet"),
		operatorErrorStatusCmd(s, "reopen", "open", "Open a resolved or ignored error again"))
	return root
}

func orAll(status string) string {
	if status == "" {
		return "open"
	}
	if status == "all" {
		return "recorded"
	}
	return status
}

func operatorErrorShowCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "One error with its latest sample",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := s.client()
			if err != nil {
				return err
			}
			e, err := client.PlatformError(s.ctx, args[0])
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"error": e}, func(w io.Writer) { printPlatformError(w, e) })
			return nil
		},
	}
}

func printPlatformError(w io.Writer, e api.PlatformError) {
	fmt.Fprintf(w, "%s\n\n", e.Title)
	fmt.Fprintf(w, "Status:     %s\n", e.Status)
	fmt.Fprintf(w, "Source:     %s\n", e.Source)
	if e.Host != "" {
		fmt.Fprintf(w, "Host:       %s\n", e.Host)
	}
	fmt.Fprintf(w, "Seen:       %d times, first %s, last %s\n", e.Count, e.FirstSeen.Local().Format("2006-01-02 15:04"), ago(e.LastSeen))
	if e.Returned > 0 {
		fmt.Fprintf(w, "Came back:  %d times after it was resolved\n", e.Returned)
	}
	if e.Note != "" {
		fmt.Fprintf(w, "Note:       %s\n", e.Note)
	}
	fmt.Fprintf(w, "\nLatest:\n%s\n", e.Sample)
}

func operatorErrorStatusCmd(s *session, use, status, short string) *cobra.Command {
	var note string
	c := &cobra.Command{
		Use:   use + " <id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := s.client()
			if err != nil {
				return err
			}
			e, err := client.SetPlatformErrorStatus(s.ctx, args[0], status, note)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"error": e}, func(w io.Writer) {
				fmt.Fprintf(w, "%s is %s.\n", e.ID, e.Status)
				if e.Note != "" {
					fmt.Fprintf(w, "Note: %s\n", e.Note)
				}
			})
			return nil
		},
	}
	c.Flags().StringVar(&note, "note", "", "what was done, kept with the error")
	return c
}
