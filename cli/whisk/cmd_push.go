package whisk

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/apitypes"
	"github.com/whisk-run/contract/endpoints"
)

// pushCmd is the app's push notifications (CLI.md §5.8, CONTROL-PLANE.md §6.42): the devices its
// people turned them on for, a test send, and what became of a send.
func pushCmd(s *session) *cobra.Command {
	push := &cobra.Command{Use: "push", Short: "The push notifications the app sends"}

	var user string
	devices := &cobra.Command{
		Use:   "devices",
		Short: "The devices people turned the app's notifications on for",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			list, err := api.Send(s.ctx, client, endpoints.ListPushDevices, apitypes.PushDeviceQuery{User: user}, org, app)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "devices": list.Items}, func(w io.Writer) {
				if len(list.Items) == 0 {
					fmt.Fprintln(w, "No devices.")
					return
				}
				rows := make([][]string, len(list.Items))
				for i, d := range list.Items {
					ok := "never"
					if d.LastOKAt != nil {
						ok = d.LastOKAt.Local().Format(time.DateTime)
					}
					rows[i] = []string{d.User, d.Label, d.Service, d.CreatedAt.Local().Format(time.DateTime), ok}
				}
				s.printer.Table(w, []string{"USER", "DEVICE", "SERVICE", "ADDED", "LAST DELIVERED"}, rows)
			})
			return nil
		},
	}
	devices.Flags().StringVar(&user, "user", "", "only this person's devices, by the app's key for them")

	var in apitypes.SendPushRequest
	var to []string
	send := &cobra.Command{
		Use:   "send --to <user> --title <title>",
		Short: "Send a notification to people's devices, to try it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			list := make([]any, len(to))
			for i, u := range to {
				list[i] = u
			}
			in.To = list
			sent, err := api.Send(s.ctx, client, endpoints.SendPush, in, org, app)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "send": sent}, func(w io.Writer) {
				if sent.Devices == 0 {
					fmt.Fprintf(w, "%s: nobody named has a device with notifications on.\n", sent.ID)
					return
				}
				fmt.Fprintf(w, "%s: going to %d devices. whisk push status %s says what became of it.\n", sent.ID, sent.Devices, sent.ID)
			})
			return nil
		},
	}
	send.Flags().StringSliceVar(&to, "to", nil, "the person, by the app's key for them; repeat for more")
	send.Flags().StringVar(&in.Title, "title", "", "the notification's title")
	send.Flags().StringVar(&in.Body, "body", "", "its text")
	send.Flags().StringVar(&in.URL, "url", "", "the page a tap opens, a path such as /tickets/42")
	send.Flags().StringVar(&in.FallbackEmail, "fallback-email", "", "an address to email when no device takes it")
	_ = send.MarkFlagRequired("to")
	_ = send.MarkFlagRequired("title")

	status := &cobra.Command{
		Use:   "status <id>",
		Short: "What became of a notification: each device's copy delivered, retrying, gone, refused or expired",
		Long: `Reads what became of a notification by the id POST .../push/send answered, for the bound app:
each device's copy, how many times it was tried and the push service's answer. gone means the
device's subscription expired or was withdrawn, and Whisk forgot it. A send is kept 7 days.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			got, err := api.Call(s.ctx, client, endpoints.GetPushSend, org, app, args[0])
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "send": got}, func(w io.Writer) { printPushSend(s.printer, w, got) })
			return nil
		},
	}
	push.AddCommand(devices, send, status)
	return push
}

func printPushSend(p output.Printer, w io.Writer, send apitypes.PushSend) {
	fmt.Fprintf(w, "%s %s\n", send.ID, send.Status)
	if len(send.Deliveries) == 0 {
		return
	}
	rows := make([][]string, len(send.Deliveries))
	for i, d := range send.Deliveries {
		at := ""
		if d.At != nil {
			at = d.At.Local().Format(time.DateTime)
		}
		rows[i] = []string{d.User, d.Device, d.Status, fmt.Sprint(d.Attempts), at, d.Detail}
	}
	fmt.Fprintln(w)
	p.Table(w, []string{"USER", "DEVICE", "STATUS", "TRIES", "AT", "DETAIL"}, rows)
}
