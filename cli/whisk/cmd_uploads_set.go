package whisk

import (
	"fmt"
	"io"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/apitypes"
	"github.com/whisk-run/contract/endpoints"

	"github.com/spf13/cobra"
)

// uploadsSetCmd changes who may open an upload and the address its file address names as
// canonical (CLI.md §5.9).
func uploadsSetCmd(s *session) *cobra.Command {
	var public, private bool
	var canonical, sitePath string
	c := &cobra.Command{
		Use:   "set <id> [--public | --private] [--path <path>] [--canonical <url>]",
		Short: "Make an upload public or private, give it a path on the site, or name its canonical address",
		Long: `Make an upload public or private, give it a path on the site, or name its canonical address.

--path is where the file answers on the app's site when the app has nothing there, such as
/posters/install.jpg, so a website links it by that path and old links keep working; --path ""
names none.

--canonical names the page or file search engines should credit, sent as
Link: <url>; rel="canonical" from /.whisk/file/<id>, such as the address a PDF had
on the old website. --canonical "" names none.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := apitypes.PatchUploadRequest{}
			switch {
			case public:
				req.Visibility = "public"
			case private:
				req.Visibility = "private"
			}
			if cmd.Flags().Changed("canonical") {
				req.Canonical = &canonical
			}
			if cmd.Flags().Changed("path") {
				req.Path = &sitePath
			}
			if req.Visibility == "" && req.Canonical == nil && req.Path == nil {
				return &output.Error{Code: "INVALID_REQUEST", Message: "Nothing to change.",
					Fix: "Pass --public, --private, --path <path> or --canonical <url>, such as: whisk uploads set " + args[0] + " --canonical https://example.com/spec-sheet.pdf"}
			}
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			u, err := api.Send(s.ctx, client, endpoints.PatchUpload, req, org, app, args[0])
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "upload": u}, func(w io.Writer) {
				printUpload(w, u)
				if u.Canonical != "" {
					fmt.Fprintf(w, "  canonical %s\n", u.Canonical)
				}
			})
			return nil
		},
	}
	c.Flags().BoolVar(&public, "public", false, "anyone may open it, and other websites may link or embed it")
	c.Flags().BoolVar(&private, "private", false, "only people signed in to the app, or a signed link")
	c.Flags().StringVar(&sitePath, "path", "", `the path on the site it answers at, such as /posters/install.jpg; "" for none`)
	c.Flags().StringVar(&canonical, "canonical", "", `the address its file address names as canonical; "" for none`)
	c.MarkFlagsMutuallyExclusive("public", "private")
	return c
}
