package whisk

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/inbound"
	"github.com/whisk-run/contract/run/runhttp"
)

// inboxCmd is the app's inbound email (CLI.md §5.8, CONTROL-PLANE.md §6.12 "Receiving"): its
// address, a business's own receiving domains, the latest messages, and a test message to the
// local stub. Each message is an event of the inbox source, so whisk webhooks events inbox and
// whisk webhooks replay inbox <id> work on them too.
func inboxCmd(s *session) *cobra.Command {
	inbox := &cobra.Command{
		Use:   "inbox",
		Short: "The app's email address and the messages it received",
		Long: `Shows the address mail for the app goes to, the business's own receiving domains, and the
latest messages with how their delivery went. Declare the inbox in whisk.yaml first:

  inbox:
    handler: /inbound/email
    allow_from: ["@lab.example"]

Each message is stored with its original and attachments in the app's storage and delivered to the
handler like a webhook: whisk webhooks events inbox lists them and whisk webhooks replay inbox <id>
sends one again.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			in, err := client.GetInbox(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "inbox": in}, func(w io.Writer) { printInbox(s.printer, w, in) })
			return nil
		},
	}

	domains := &cobra.Command{Use: "domains", Short: "A business's own domain whose mail reaches this app"}
	add := &cobra.Command{
		Use:   "add <domain>",
		Short: "Receive mail at a domain of the business's own, such as results.yourbusiness.com",
		Long: `Registers the domain for receiving and prints the MX record to publish. Use a subdomain that
receives no other mail: its MX points at Whisk's provider, so mail to any address at it reaches
this app. Run whisk inbox domains verify <id> once the record is published.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			d, err := client.AddInboxDomain(s.ctx, org, app, args[0])
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "domain": d}, func(w io.Writer) { printInboxDomain(s.printer, w, d) })
			return nil
		},
	}
	verify := &cobra.Command{
		Use:   "verify <id>",
		Short: "Check a receiving domain's records; it stays pending until they are published",
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
			d, err := client.VerifyInboxDomain(s.ctx, org, app, args[0])
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "domain": d}, func(w io.Writer) { printInboxDomain(s.printer, w, d) })
			return nil
		},
	}
	remove := &cobra.Command{
		Use:   "remove <id>",
		Short: "Stop a receiving domain reaching this app (by id, from whisk inbox)",
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
			if err := client.RemoveInboxDomain(s.ctx, org, app, args[0]); err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "removed": args[0]}, func(w io.Writer) {
				fmt.Fprintf(w, "Removed %s. Mail to it no longer reaches the app; remove its MX record too.\n", args[0])
			})
			return nil
		},
	}
	domains.AddCommand(add, verify, remove)

	var port int
	var from, to, subject, text, attach string
	send := &cobra.Command{
		Use:   "send [message.eml]",
		Short: "Send a test message to the app running under whisk dev",
		Long: `Hands a message to the local stub's inbox, which stores it and delivers it to the inbox handler
the way the platform does. Pass a saved .eml file, or compose one with the flags:

  whisk inbox send results.eml
  whisk inbox send --from results@lab.example --subject "Batch 7" --text "See attached." --attach b7.pdf

It never sends real mail; to reach the deployed app, send mail to the address whisk inbox prints.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := loadManifest(s.env.Dir)
			if err != nil {
				return err
			}
			if m.Inbox == nil {
				return output.New("INBOX_NOT_DECLARED", m.Name+" declares no inbox, so it receives no email.",
					"Add inbox: with handler: /inbound/email (the route that receives each message) to whisk.yaml and restart whisk dev.", map[string]any{"app": m.Name})
			}
			var raw []byte
			if len(args) == 1 {
				if raw, err = readCapped(args[0], inbound.MaxMessageBytes); err != nil {
					return err
				}
			} else {
				if to == "" {
					to = inbound.Address(m.Name, "dev", "in.localhost")
				}
				var file []byte
				if attach != "" {
					if file, err = readCapped(attach, inbound.MaxMessageBytes); err != nil {
						return err
					}
				}
				raw = composeMessage(from, to, subject, text, attach, file, time.Now())
			}
			got, err := sendToStub(s.ctx, fmt.Sprintf("http://127.0.0.1:%d/v1/stub/inbox", port+1), raw)
			if err != nil {
				return err
			}
			s.printer.Result(got, func(w io.Writer) {
				if delivered, _ := got["delivered"].(bool); delivered {
					fmt.Fprintf(w, "Stored message %v and queued it for %s. Its original is at %v.\n", got["id"], m.Inbox.Handler, rawKey(got))
					return
				}
				fmt.Fprintf(w, "Message %v was kept but not delivered: %v.\n", got["id"], got["reason"])
			})
			return nil
		},
	}
	f := send.Flags()
	f.IntVar(&port, "port", 3000, "the edge port whisk dev was started with; the stub's api is port+1")
	f.StringVar(&from, "from", "Test sender <sender@example.com>", "From of a composed message")
	f.StringVar(&to, "to", "", "To of a composed message; the app's local address by default")
	f.StringVar(&subject, "subject", "Test message", "Subject of a composed message")
	f.StringVar(&text, "text", "This is a test message from whisk inbox send.", "plain text body of a composed message")
	f.StringVar(&attach, "attach", "", "a file to attach to a composed message")

	inbox.AddCommand(domains, send)
	return inbox
}

func printInbox(p output.Printer, w io.Writer, in api.Inbox) {
	if !in.Declared {
		fmt.Fprintln(w, "This app has no inbox. Add inbox: with handler: /inbound/email to whisk.yaml and push.")
		return
	}
	fmt.Fprintf(w, "Address: %s\nHandler: %s\n", in.Address, in.Handler)
	if len(in.AllowFrom) > 0 {
		fmt.Fprintf(w, "Accepts mail from: %s\n", strings.Join(in.AllowFrom, ", "))
	} else {
		fmt.Fprintln(w, "Accepts mail from: anyone")
	}
	if !in.Available {
		fmt.Fprintln(w, "Whisk is not set up to receive email yet, so nothing reaches this address for now.")
	} else if !in.Included {
		fmt.Fprintln(w, "This business's plan does not include an inbox, so mail to this address is dropped. Team and above include one.")
	}
	if len(in.Domains) > 0 {
		rows := make([][]string, len(in.Domains))
		for i, d := range in.Domains {
			rows[i] = []string{d.ID, d.Domain, string(d.Status)}
		}
		fmt.Fprintln(w)
		p.Table(w, []string{"DOMAIN ID", "DOMAIN", "STATUS"}, rows)
	}
	fmt.Fprintln(w)
	if len(in.Recent) == 0 {
		fmt.Fprintln(w, "No messages yet.")
		return
	}
	rows := make([][]string, len(in.Recent))
	for i, m := range in.Recent {
		status := string(m.Status)
		if m.Reason != "" {
			status += " (" + m.Reason + ")"
		}
		rows[i] = []string{m.ID, m.ReceivedAt.Local().Format(time.RFC3339), m.From, m.Subject, status}
	}
	p.Table(w, []string{"MESSAGE", "RECEIVED", "FROM", "SUBJECT", "DELIVERY"}, rows)
}

func printInboxDomain(p output.Printer, w io.Writer, d api.InboxDomain) {
	if d.Status == "verified" {
		fmt.Fprintf(w, "%s is verified: mail to any address at it reaches this app.\n", d.Domain)
		return
	}
	fmt.Fprintf(w, "Publish these DNS records for %s, then run whisk inbox domains verify %s:\n", d.Domain, d.ID)
	rows := make([][]string, len(d.DNSRecords))
	for i, r := range d.DNSRecords {
		rows[i] = []string{r.Type, r.Name, r.Value}
	}
	p.Table(w, []string{"TYPE", "NAME", "VALUE"}, rows)
}

// readCapped reads a local file of at most limit bytes.
func readCapped(path string, limit int64) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, output.New("INVALID_REQUEST", path+" cannot be read: "+err.Error(), "Pass the path of a file that exists.", map[string]any{"path": path})
	}
	if info.Size() > limit {
		return nil, output.New("INVALID_REQUEST", fmt.Sprintf("%s is %d bytes; a message is at most %d.", path, info.Size(), limit),
			"Send a smaller message; the platform drops larger ones too.", map[string]any{"path": path, "limit_bytes": limit})
	}
	return os.ReadFile(path)
}

// composeMessage writes a plain message, with one attachment when file is set. Pure.
func composeMessage(from, to, subject, text, filename string, file []byte, now time.Time) []byte {
	var b strings.Builder
	header := func(k, v string) {
		fmt.Fprintf(&b, "%s: %s\r\n", k, strings.NewReplacer("\r", " ", "\n", " ").Replace(v))
	}
	header("From", from)
	header("To", to)
	header("Subject", subject)
	header("Date", now.UTC().Format(time.RFC1123Z))
	header("Message-ID", fmt.Sprintf("<%d.whisk-inbox-send@localhost>", now.UnixNano()))
	header("MIME-Version", "1.0")
	if file == nil {
		header("Content-Type", "text/plain; charset=utf-8")
		b.WriteString("\r\n" + text + "\r\n")
		return []byte(b.String())
	}
	const boundary = "whisk-inbox-send"
	header("Content-Type", "multipart/mixed; boundary="+boundary)
	b.WriteString("\r\n--" + boundary + "\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" + text + "\r\n")
	name := inbound.SafeName(baseName(filename))
	b.WriteString("--" + boundary + "\r\nContent-Type: application/octet-stream\r\n")
	b.WriteString("Content-Disposition: attachment; filename=\"" + name + "\"\r\nContent-Transfer-Encoding: base64\r\n\r\n")
	b.WriteString(base64Lines(file))
	b.WriteString("--" + boundary + "--\r\n")
	return []byte(b.String())
}

// base64Lines is data in base64, in lines of 76 characters as MIME asks. Pure.
func base64Lines(data []byte) string {
	enc := base64.StdEncoding.EncodeToString(data)
	var b strings.Builder
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc + "\r\n")
	return b.String()
}

func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

// sendToStub posts the raw message to the local stub and answers what it said.
func sendToStub(ctx context.Context, url string, raw []byte) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "message/rfc822")
	resp, err := runhttp.Client(30 * time.Second).Do(req)
	if err != nil {
		return nil, output.New("DEV_NOT_RUNNING", "Nothing answered at "+url+": "+err.Error(),
			"Start the app with whisk dev in another terminal, or pass --port with the port it was started on.", map[string]any{"url": url})
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		var e struct {
			Error struct {
				Code    string         `json:"code"`
				Message string         `json:"message"`
				Fix     string         `json:"fix"`
				Details map[string]any `json:"details"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &e) == nil && e.Error.Code != "" {
			return nil, output.New(e.Error.Code, e.Error.Message, e.Error.Fix, e.Error.Details)
		}
		return nil, output.New("DEV_NOT_RUNNING", fmt.Sprintf("%s answered %d, not the stub's inbox.", url, resp.StatusCode),
			"Check that whisk dev is running for this app, and pass --port with the port it was started on.", map[string]any{"url": url, "status": resp.StatusCode})
	}
	out := map[string]any{}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, output.New("DEV_NOT_RUNNING", url+" did not answer as the stub's inbox does.",
			"Check that whisk dev is running for this app, and pass --port with the port it was started on.", map[string]any{"url": url})
	}
	return out, nil
}

func rawKey(got map[string]any) any {
	if r, ok := got["raw"].(map[string]any); ok {
		return r["key"]
	}
	return "-"
}
