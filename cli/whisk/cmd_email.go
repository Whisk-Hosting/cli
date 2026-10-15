package whisk

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/apitypes"
)

// emailCmd is the app's sent mail (CLI.md §5.8, CONTROL-PLANE.md §6.12 "Delivery"): what became
// of a message, by the id its send answered.
func emailCmd(s *session) *cobra.Command {
	email := &cobra.Command{Use: "email", Short: "The mail the app sends"}
	status := &cobra.Command{
		Use:   "status <id>",
		Short: "What became of a message the app sent: sent, delayed, delivered, failed, bounced or complained",
		Long: `Reads what became of a message by the id POST .../email/send answered, for the bound app:
its status, the provider's words for it (a bounce's reason, a delay's kind) and each step with its
time. delivered means the receiving server accepted the message; whether it went to the inbox or
the spam folder cannot be known, and only a complaint (the recipient marked it as spam) is
reported. A message's delivery is kept 30 days.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			send, err := client.EmailSend(s.ctx, org, app, args[0])
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "send": send}, func(w io.Writer) { printEmailSend(s.printer, w, send) })
			return nil
		},
	}
	email.AddCommand(status)
	return email
}

// emailSendSays is the sentence a person reads for each status. Pure.
var emailSendSays = map[apitypes.EmailSendStatus]string{
	apitypes.EmailSendSent:       "sent: the provider took it and has said nothing since",
	apitypes.EmailSendDelayed:    "delayed: the receiving server has not accepted it yet; the provider keeps trying",
	apitypes.EmailSendDelivered:  "delivered: the receiving server accepted it (inbox or spam folder cannot be known)",
	apitypes.EmailSendFailed:     "failed: the provider could not send it",
	apitypes.EmailSendBounced:    "bounced: the receiving server refused it; stop mailing this address",
	apitypes.EmailSendComplained: "complained: the recipient marked it as spam; stop mailing this address",
}

func printEmailSend(p output.Printer, w io.Writer, send apitypes.EmailSend) {
	says, ok := emailSendSays[send.Status]
	if !ok {
		says = string(send.Status)
	}
	fmt.Fprintf(w, "%s %s\n", send.ID, says)
	if send.Detail != "" {
		fmt.Fprintf(w, "Detail: %s\n", send.Detail)
	}
	rows := make([][]string, len(send.Events))
	for i, e := range send.Events {
		rows[i] = []string{e.At.Local().Format(time.DateTime), string(e.Status)}
	}
	if len(rows) > 0 {
		fmt.Fprintln(w)
		p.Table(w, []string{"AT", "STATUS"}, rows)
	}
}
