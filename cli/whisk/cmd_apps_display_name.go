package whisk

import (
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/output"
)

// MaxDisplayName is the longest display name the platform keeps (CONTROL-PLANE.md §4.6).
const MaxDisplayName = 60

// displayNameCmd sets the name an app is shown by: in the dashboard, and on its customers'
// sign-in pages and emails (CLI.md §5.2, CONTROL-PLANE.md §4.6). Its address stays as it is.
func displayNameCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "display-name <name>",
		Short: "Set the name the app is shown by, on its sign-in pages and emails too; its address stays",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := strings.TrimSpace(args[0])
			if problem := displayNameProblem(name); problem != "" {
				return output.New("INVALID_REQUEST", problem, fmt.Sprintf("Pass a name of 1 to %d characters, such as \"Job Tracker\".", MaxDisplayName), nil)
			}
			org, ref, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			app, err := client.PatchApp(s.ctx, org, ref, name)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app}, func(w io.Writer) {
				fmt.Fprintf(w, "%s/%s is shown as %q: https://%s\n", org, app.Slug, app.Name, app.Hostname)
			})
			return nil
		},
	}
}

// displayNameProblem says what is wrong with a display name, "" when nothing is. Pure.
func displayNameProblem(name string) string {
	switch {
	case name == "":
		return "The display name cannot be empty."
	case len([]rune(name)) > MaxDisplayName:
		return fmt.Sprintf("The display name is over %d characters.", MaxDisplayName)
	case strings.IndexFunc(name, unicode.IsControl) >= 0:
		return "The display name holds a control character."
	}
	return ""
}
