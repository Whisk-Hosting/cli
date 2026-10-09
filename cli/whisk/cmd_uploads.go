package whisk

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
)

// uploadsCmd is the app's uploaded files (CLI.md §5.9): signed links to private images and
// video, and the key they are signed with.
func uploadsCmd(s *session) *cobra.Command {
	uploads := &cobra.Command{Use: "uploads", Short: "Signed links to an app's private images and video"}

	var expires time.Duration
	link := &cobra.Command{
		Use:   "link <path>... [--expires 1h]",
		Short: "Sign /.whisk/img/<id>?w=… or /.whisk/media/<id> so it opens without a sign-in until it expires",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			out, err := client.SignMedia(s.ctx, org, app, args, int64(expires/time.Second))
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "expires_at": out.ExpiresAt, "links": out.Links}, func(w io.Writer) {
				for _, l := range out.Links {
					if l.URL != "" {
						fmt.Fprintln(w, l.URL)
					} else {
						fmt.Fprintln(w, l.Path)
					}
				}
				fmt.Fprintf(w, "Anyone with a link can open it until %s.\n", out.ExpiresAt.Local().Format("2 Jan 15:04 MST"))
			})
			return nil
		},
	}
	link.Flags().DurationVar(&expires, "expires", 0, "how long the links work, 1m to 12h (default 1h)")

	var now bool
	rotate := &cobra.Command{
		Use:   "rotate-key [--now]",
		Short: "Give the app a new signing key; --now also stops every link already issued",
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
			k, err := client.RotateMediaLinkKey(s.ctx, org, app, now)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "key": k}, func(w io.Writer) {
				if now {
					fmt.Fprintf(w, "New signing key (generation %d). Every link issued before now has stopped working.\n", k.Generation)
				} else {
					fmt.Fprintf(w, "New signing key (generation %d). Links issued before keep working until they expire, at the latest %s.\n",
						k.Generation, k.PreviousUntil.Local().Format("2 Jan 15:04 MST"))
				}
			})
			return nil
		},
	}
	rotate.Flags().BoolVar(&now, "now", false, "stop every link already issued at once")

	uploads.AddCommand(link, rotate)
	return uploads
}
