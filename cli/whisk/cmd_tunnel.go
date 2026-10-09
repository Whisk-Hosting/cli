package whisk

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/dev"
	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/run/runhttp"
)

// tunnelCmd forwards a provider's webhooks to the app running on this machine (CLI.md §5.10).
func tunnelCmd(s *session) *cobra.Command {
	var port int
	var minutes int
	c := &cobra.Command{
		Use:   "tunnel <source>",
		Short: "Forward a webhook source's deliveries to the app running here",
		Long: `Prints the URL to paste into the provider and, for an hour, forwards every verified delivery
the platform records for that source to the local gate — the same body, the provider's own
headers, and the same X-Whisk-Webhook-Id the deployed app receives. Deliveries are replayed from
the platform's record, so the deployed app keeps receiving them and nothing about the source
changes.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			sources, err := client.ListWebhooks(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			src, ok := findSource(sources, name)
			if !ok {
				return output.New("WEBHOOK_SOURCE_UNKNOWN",
					app+" has no webhook source called "+name+".",
					"Run whisk webhooks list to see the app's sources, or declare one under webhooks: in whisk.yaml.",
					map[string]any{"source": name, "known": sourceNames(sources)})
			}
			target := fmt.Sprintf("http://127.0.0.1:%d", port+1)
			log := s.env.Stderr
			fmt.Fprintf(log, "tunnel: paste this into the provider: %s\n", src.URL)
			fmt.Fprintf(log, "tunnel: forwarding verified deliveries to %s%s for %dm\n", target, src.Handler, minutes)

			forwarder := runhttp.Client(30 * time.Second)
			ctx, stop := signal.NotifyContext(s.ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()
			n, err := dev.Tunnel(ctx, dev.TunnelOptions{
				Source: src.Name, Handler: src.Handler, Target: target,
				Lifetime: time.Duration(minutes) * time.Minute,
				Deliveries: func(ctx context.Context) ([]dev.Delivery, error) {
					events, err := client.WebhookEvents(ctx, org, app, src.Name, dev.TunnelBacklog)
					if err != nil {
						return nil, err
					}
					return deliveries(events), nil
				},
				Post: func(ctx context.Context, d dev.Delivery) (int, error) {
					return dev.PostDelivery(ctx, forwarder, target, src.Handler, d)
				},
				Log: log,
			})
			if err != nil {
				return err
			}
			s.printer.Result(
				map[string]any{"source": src.Name, "url": src.URL, "handler": src.Handler, "forwarded": n},
				func(w io.Writer) {
					fmt.Fprintf(w, "tunnel: closed after forwarding %d deliveries to %s\n", n, src.Handler)
				})
			return nil
		},
	}
	f := c.Flags()
	f.IntVar(&port, "port", 3000, "the whisk dev edge port; deliveries go to port+1, its webhook ingress")
	f.IntVar(&minutes, "minutes", 60, "how long to forward for")
	return c
}

// deliveries is the stored events as the tunnel reads them.
func deliveries(events []api.WebhookEvent) []dev.Delivery {
	out := make([]dev.Delivery, 0, len(events))
	for _, e := range events {
		out = append(out, dev.Delivery{
			ID: e.ID, ReceivedAt: e.ReceivedAt, Verified: e.Verified, Headers: e.Headers, Body: e.Body,
		})
	}
	return out
}

func findSource(sources []api.Source, name string) (api.Source, bool) {
	for _, s := range sources {
		if strings.EqualFold(s.Name, name) {
			return s, true
		}
	}
	return api.Source{}, false
}

func sourceNames(sources []api.Source) []string {
	out := make([]string, 0, len(sources))
	for _, s := range sources {
		out = append(out, s.Name)
	}
	return out
}
