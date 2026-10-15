package whisk

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/cli/internal/safefile"
	"github.com/whisk-run/contract/manifest"
	"github.com/whisk-run/contract/webhook"
)

// webhooksCmd is the app's webhook sources: the URLs to paste into providers, what arrived, and
// what was delivered (CLI.md §5.8, CONTROL-PLANE.md §6.10).
func webhooksCmd(s *session) *cobra.Command {
	webhooks := &cobra.Command{Use: "webhooks", Short: "Webhook sources the app receives"}

	list := &cobra.Command{
		Use:   "list",
		Short: "List the app's sources with the URL to paste into each provider",
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
			sources, err := client.ListWebhooks(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "webhooks": sources}, func(w io.Writer) {
				if len(sources) == 0 {
					fmt.Fprintln(w, "This app receives no webhooks. Declare one under webhooks: in whisk.yaml, or run whisk webhooks add.")
					return
				}
				rows := make([][]string, len(sources))
				for i, src := range sources {
					rows[i] = []string{src.Name, src.Preset, src.URL, secretText(src), deliveryText(src)}
				}
				s.printer.Table(w, []string{"SOURCE", "PRESET", "URL", "SECRET", "LAST 100"}, rows)
			})
			return nil
		},
	}

	var preset, secret, handler string
	add := &cobra.Command{
		Use:   "add <name>",
		Short: "Write a webhook source into whisk.yaml",
		Long: `Adds the source to the manifest. A source names a route the app must answer, so it
belongs to the app's code: the platform declares it at the next push, and the URL to paste into
the provider appears in whisk webhooks list once that push has landed.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if preset == "" {
				return output.New("INVALID_REQUEST", "A source needs a preset.", "Pass --preset stripe, or run whisk webhooks presets to see them all.", nil)
			}
			if _, known := webhook.Presets()[preset]; !known && preset != webhook.PresetHMAC && preset != webhook.PresetToken {
				return output.New("INVALID_REQUEST", preset+" is not a webhook preset.", "Run whisk webhooks presets to see them all.", map[string]any{"preset": preset})
			}
			if handler == "" {
				handler = "/hooks/" + name
			}
			if secret == "" && preset != webhook.PresetToken {
				return output.New("INVALID_REQUEST", "A signed source needs the name of its signing secret.",
					"Pass --secret STRIPE_WEBHOOK_SECRET; declare it under secrets: too, and a person sets its value in the dashboard.", nil)
			}
			m, err := loadManifest(s.env.Dir)
			if err != nil {
				return err
			}
			if _, exists := m.WebhookByName(name); exists {
				return output.New("INVALID_REQUEST", "whisk.yaml already declares a source called "+name+".",
					"Edit the entry in whisk.yaml, or choose another name.", map[string]any{"source": name})
			}
			path := filepath.Join(s.env.Dir, "whisk.yaml")
			updated, err := addWebhookEntry(path, name, preset, secret, handler)
			if err != nil {
				return err
			}
			s.printer.Result(map[string]any{"source": name, "preset": preset, "handler": handler, "secret": secret, "manifest": path, "secrets": updated},
				func(w io.Writer) {
					fmt.Fprintf(w, "Added %s to whisk.yaml, handled at %s.\n", name, handler)
					if updated {
						fmt.Fprintf(w, "Added %s to secrets: as well; a person sets its value in the dashboard.\n", secret)
					}
					fmt.Fprintf(w, "Write the %s route, then push. whisk webhooks list then prints the URL to paste into the provider.\n", handler)
				})
			return nil
		},
	}
	add.Flags().StringVar(&preset, "preset", "", "how deliveries are verified: stripe, github, hmac, token…")
	add.Flags().StringVar(&secret, "secret", "", "name of the signing secret (not needed for the token preset)")
	add.Flags().StringVar(&handler, "handler", "", "route in this app that receives clean events; /hooks/<name> by default")

	var limit int
	events := &cobra.Command{
		Use:   "events <source>",
		Short: "What a source received, newest first",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			list, err := client.WebhookEvents(s.ctx, org, app, args[0], limit)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "source": args[0], "events": list}, func(w io.Writer) {
				if len(list) == 0 {
					fmt.Fprintf(w, "Nothing has arrived at %s yet.\n", args[0])
					return
				}
				rows := make([][]string, len(list))
				for i, e := range list {
					rows[i] = []string{e.ID, e.ReceivedAt.Local().Format(time.RFC3339), verifiedText(e), string(e.Status), fmt.Sprint(e.Attempts), deliveryWhy(e)}
				}
				s.printer.Table(w, []string{"EVENT", "RECEIVED", "VERIFIED", "DELIVERY", "TRIES", "WHY"}, rows)
			})
			return nil
		},
	}
	events.Flags().IntVar(&limit, "limit", 20, "how many deliveries to show")

	replay := &cobra.Command{
		Use:   "replay <source> <event-id>",
		Short: "Send a stored delivery to the app again",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			event, err := client.ReplayWebhookEvent(s.ctx, org, app, args[0], args[1])
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"event": event}, func(w io.Writer) {
				fmt.Fprintf(w, "Queued %s for delivery again. It keeps its id, so the app can recognise the repeat.\n", event.ID)
			})
			return nil
		},
	}

	presets := &cobra.Command{
		Use:   "presets",
		Short: "List the signature presets the platform verifies",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			all := webhook.Presets()
			names := make([]string, 0, len(all))
			for n := range all {
				names = append(names, n)
			}
			sort.Strings(names)
			type row struct {
				Name      string `json:"name"`
				Header    string `json:"header"`
				Algorithm string `json:"algorithm"`
				Encoding  string `json:"encoding"`
				Payload   string `json:"payload"`
				Tolerance int    `json:"tolerance_seconds"`
			}
			var rows []row
			for _, n := range names {
				p := all[n]
				rows = append(rows, row{Name: n, Header: p.Header, Algorithm: p.Algorithm, Encoding: p.Encoding, Payload: p.Payload, Tolerance: p.ToleranceSeconds})
			}
			s.printer.Result(map[string]any{"presets": rows}, func(w io.Writer) {
				table := make([][]string, len(rows))
				for i, r := range rows {
					algo, header := r.Algorithm, r.Header
					switch r.Name {
					case webhook.PresetToken:
						algo, header = "secret in the URL", "-"
					case webhook.PresetHMAC:
						algo, header = "hmac, settings in the manifest", "-"
					}
					tol := "-"
					if r.Tolerance > 0 {
						tol = fmt.Sprintf("%ds", r.Tolerance)
					}
					table[i] = []string{r.Name, header, algo, tol}
				}
				s.printer.Table(w, []string{"PRESET", "SIGNATURE HEADER", "ALGORITHM", "TOLERANCE"}, table)
			})
			return nil
		},
	}

	webhooks.AddCommand(list, add, events, replay, presets)
	return webhooks
}

// addWebhookEntry appends a source to whisk.yaml, and the signing secret to secrets: when the
// manifest does not already name it. The file is re-parsed before it is written, so a bad edit
// is never left on disk.
func addWebhookEntry(path, name, preset, secret, handler string) (addedSecret bool, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false, output.New("MANIFEST_INVALID", "whisk.yaml was not found at "+path+".", "Run the command from the app directory.", nil)
	}
	text := strings.TrimRight(string(raw), "\n")
	current, err := manifest.Parse([]byte(text))
	if err != nil {
		return false, output.New("MANIFEST_INVALID", "whisk.yaml does not parse, so it cannot be edited.", "Run whisk doctor to see what is wrong.", nil)
	}

	if secret != "" && !namesInclude(current.Secrets, secret) {
		text, addedSecret = addToSecrets(text, secret), true
	}
	entry := fmt.Sprintf("  - name: %s\n    preset: %s\n", name, preset)
	if secret != "" {
		entry += fmt.Sprintf("    secret: %s\n", secret)
	}
	entry += fmt.Sprintf("    handler: %s\n", handler)
	if idx := blockIndex(text, "webhooks:"); idx >= 0 {
		text = insertAfterBlock(text, idx, entry)
	} else {
		text += "\n\nwebhooks:\n" + entry
	}
	text = strings.TrimRight(text, "\n") + "\n"
	if _, err := manifest.Parse([]byte(text)); err != nil {
		return false, output.New("MANIFEST_INVALID", "The edit would leave whisk.yaml invalid, so nothing was written.",
			"Add the source by hand under webhooks: in whisk.yaml.", map[string]any{"error": err.Error()})
	}
	return addedSecret, safefile.WriteFile(path, []byte(text), 0o644)
}

// addToSecrets adds a name to the manifest's secrets list, whether it is written inline or as
// a block, and writes a new list when there is none.
func addToSecrets(text, secret string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "secrets:") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "secrets:"))
		switch {
		case strings.HasPrefix(rest, "[") && strings.HasSuffix(rest, "]"):
			inner := strings.TrimSpace(rest[1 : len(rest)-1])
			if inner == "" {
				lines[i] = "secrets: [" + secret + "]"
			} else {
				lines[i] = "secrets: [" + inner + ", " + secret + "]"
			}
			return strings.Join(lines, "\n")
		case rest == "":
			lines = append(lines[:i+1], append([]string{"  - " + secret}, lines[i+1:]...)...)
			return strings.Join(lines, "\n")
		}
	}
	return text + "\n\nsecrets: [" + secret + "]"
}

// blockIndex is the line a top-level key starts on, or -1.
func blockIndex(text, key string) int {
	for i, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, key) {
			return i
		}
	}
	return -1
}

// insertAfterBlock puts an entry at the end of the block that starts at line idx, which is
// where the last indented or blank line before the next top-level key is.
func insertAfterBlock(text string, idx int, entry string) string {
	lines := strings.Split(text, "\n")
	end := len(lines)
	for i := idx + 1; i < len(lines); i++ {
		if lines[i] != "" && !strings.HasPrefix(lines[i], " ") && !strings.HasPrefix(lines[i], "\t") {
			end = i
			break
		}
	}
	for end > idx+1 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	tail := append([]string{}, lines[end:]...)
	out := append(lines[:end], strings.Split(strings.TrimRight(entry, "\n"), "\n")...)
	return strings.Join(append(out, tail...), "\n")
}

func namesInclude(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// secretText says whether a source can verify anything yet.
func secretText(src api.Source) string {
	switch {
	case src.Preset == webhook.PresetToken:
		return "token in the URL"
	case src.Secret == "":
		return "none declared"
	case !src.SecretSet:
		return src.Secret + " (unset)"
	default:
		return src.Secret
	}
}

// deliveryText is how the last hundred deliveries went.
func deliveryText(src api.Source) string {
	if src.Events == 0 {
		return "nothing yet"
	}
	if src.Unverified == 0 {
		return fmt.Sprintf("%d received, all verified", src.Events)
	}
	return fmt.Sprintf("%d received, %.0f%% unverified", src.Events, src.Unverified*100)
}

func verifiedText(e api.WebhookEvent) string {
	if e.Verified {
		return "yes"
	}
	if e.Reason != "" {
		return "no (" + e.Reason + ")"
	}
	return "no"
}

// deliveryWhy is why the event's latest delivery failed, cut to 80 characters for the table:
// the handler's status and answer, or the connection error; empty when none failed. Pure.
func deliveryWhy(e api.WebhookEvent) string {
	if e.LastError == nil {
		return ""
	}
	why := strings.Join(strings.Fields(e.LastError.Error), " ")
	if e.LastError.Status > 0 {
		why = strings.TrimSpace(fmt.Sprintf("HTTP %d %s", e.LastError.Status, why))
	}
	if r := []rune(why); len(r) > 80 {
		why = string(r[:79]) + "…"
	}
	return why
}
