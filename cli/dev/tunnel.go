package dev

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/whisk-run/contract/run"
)

// Forwarding a provider's webhooks to the app running on this machine (CLI.md §5.10). The
// platform already receives, verifies and stores every delivery, so a tunnel does not intercept
// anything: it reads the record and replays each verified delivery to the local gate, with the
// body, the headers and the webhook id the deployed app would see. The deployed app keeps
// receiving them too, and nothing about the source changes.

// Delivery is one stored delivery as the tunnel needs it.
type Delivery struct {
	ID         string
	ReceivedAt time.Time
	Verified   bool
	Headers    map[string]string
	Body       string
}

// TunnelOptions is one run of the tunnel.
type TunnelOptions struct {
	// Source is the webhook source's name, and Handler the route in the app it is delivered to.
	Source  string
	Handler string
	// Target is where the local gate listens, e.g. http://127.0.0.1:3001.
	Target string
	// Lifetime is how long the tunnel forwards for; zero means an hour.
	Lifetime time.Duration
	// Every is how often the platform is asked for new deliveries; zero means two seconds.
	Every time.Duration
	// Deliveries reads the source's stored deliveries, newest first.
	Deliveries func(ctx context.Context) ([]Delivery, error)
	// Post sends one delivery to the local gate and answers the status it got.
	Post func(ctx context.Context, d Delivery) (int, error)
	// Log is where each forward is reported.
	Log io.Writer
}

// TunnelDefaults are the shape of a run when the caller says nothing.
const (
	TunnelLifetime = time.Hour
	TunnelInterval = 2 * time.Second
	// TunnelBacklog is how many deliveries are read each time. A tunnel is for one developer
	// watching one provider, so a hundred is more than a burst.
	TunnelBacklog = 100
)

// Tunnel forwards until the lifetime runs out or the context ends. It answers how many
// deliveries it forwarded, so a caller can say so.
func Tunnel(ctx context.Context, o TunnelOptions) (int, error) {
	lifetime, every := o.Lifetime, o.Every
	if lifetime <= 0 {
		lifetime = TunnelLifetime
	}
	if every <= 0 {
		every = TunnelInterval
	}
	ctx, cancel := context.WithTimeout(ctx, lifetime)
	defer cancel()

	// Everything already stored is history, not news: a tunnel forwards what arrives while it
	// is open. Without that history every stored delivery would look new and be replayed, so
	// a tunnel that cannot read it does not open.
	known, err := o.Deliveries(ctx)
	if err != nil {
		return 0, err
	}
	seen := map[string]bool{}
	for _, d := range known {
		seen[d.ID] = true
	}

	forwarded := 0
	run.Every(ctx, "webhook tunnel", every, tunnelRun, func(ctx context.Context) error {
		list, err := o.Deliveries(ctx)
		if err != nil {
			if ctx.Err() == nil {
				fmt.Fprintf(o.Log, "tunnel: reading deliveries: %v\n", err)
			}
			return nil
		}
		for _, d := range NewDeliveries(list, seen) {
			seen[d.ID] = true
			status, err := o.Post(ctx, d)
			switch {
			case err != nil && ctx.Err() != nil:
				return nil
			case err != nil:
				fmt.Fprintf(o.Log, "tunnel: %s -> %s: %v\n", d.ID, o.Handler, err)
			default:
				forwarded++
				fmt.Fprintf(o.Log, "tunnel: %s -> %s %d\n", d.ID, o.Handler, status)
			}
		}
		return nil
	})
	return forwarded, nil
}

// tunnelRun bounds one read and forward: a full backlog of deliveries, each posted to the
// local gate with its own deadline.
const tunnelRun = 10 * time.Minute

// NewDeliveries is the verified deliveries the tunnel has not forwarded yet, oldest first so
// the app sees them in the order the provider sent them. It is pure, which is where the
// tunnel's one real decision lives.
func NewDeliveries(list []Delivery, seen map[string]bool) []Delivery {
	var out []Delivery
	for _, d := range list {
		if seen[d.ID] || !d.Verified {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ReceivedAt.Equal(out[j].ReceivedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].ReceivedAt.Before(out[j].ReceivedAt)
	})
	return out
}

// PostDelivery sends one delivery to the local gate exactly as the platform would to the
// deployed app: the provider's own headers, the platform's webhook id, and the raw body.
func PostDelivery(ctx context.Context, client *http.Client, target, handler string, d Delivery) (int, error) {
	url := strings.TrimRight(target, "/") + "/" + strings.TrimLeft(handler, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte(d.Body)))
	if err != nil {
		return 0, err
	}
	for k, v := range d.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("X-Whisk-Webhook-Id", d.ID)
	req.Header.Set("X-Whisk-Delivery", "tunnel")
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	return res.StatusCode, nil
}
