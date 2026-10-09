package whisk

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
)

// logsCmd is what the app printed (CLI.md §5.5). --json is the global one: one object with every
// line, its time, stream and parsed fields (one object per line under -f); --raw prints a JSON
// line as the object the app wrote.
func logsCmd(s *session) *cobra.Command {
	var follow, raw bool
	var since, env, grep string
	var limit int
	c := &cobra.Command{
		Use:   "logs",
		Short: "What the app printed",
		Long: `Reads the app's own lines. --grep takes free text, or field=value to match a field of a
JSON line; several terms all have to match. --since takes a duration (30m, 6h, 7d) or an RFC 3339
time. Values of declared secrets and platform credentials appear as [REDACTED:NAME]; values
under the manifest's env are plain configuration and appear as printed.

--json prints one object, {"org", "app", "lines": [{"at", "stream", "env", "line", "json"}]},
with "lines": [] when the window is empty; with -f it prints one such line object per line as it
arrives. --raw is for a person: each JSON line as the object the app wrote.`,
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
			q := api.LogQuery{Env: env, Grep: grep, Since: since, Limit: limit}
			if !follow {
				lines, err := client.Logs(s.ctx, org, app, q)
				if err != nil {
					return wrap(err)
				}
				if lines == nil {
					lines = []api.LogLine{}
				}
				s.printer.Result(map[string]any{"org": org, "app": app, "lines": lines}, func(w io.Writer) {
					if len(lines) == 0 {
						fmt.Fprintln(w, "Nothing in that window. The app prints to stdout and stderr; anything it prints appears here.")
						return
					}
					for _, l := range lines {
						fmt.Fprintln(w, formatLine(l, raw))
					}
				})
				return nil
			}
			ctx, stop := signal.NotifyContext(s.ctx, os.Interrupt, syscall.SIGTERM)
			defer stop()
			// --json follows as one object per line, the shape of an item of lines above.
			err = client.TailLogs(ctx, org, app, q, func(batch []api.LogLine) error {
				for _, l := range batch {
					s.printer.Line(l, formatLine(l, raw))
				}
				return nil
			})
			if err != nil && ctx.Err() == nil {
				return wrap(err)
			}
			return nil
		},
	}
	f := c.Flags()
	f.BoolVarP(&follow, "follow", "f", false, "keep the stream open and print lines as they arrive")
	f.BoolVar(&raw, "raw", false, "print each JSON line as the object the app wrote (for a person piping into jq; --json gives every line with its time and stream)")
	f.StringVar(&since, "since", "1h", "how far back to read: 30m, 6h, 7d, or an RFC 3339 time")
	f.StringVar(&env, "env", "", "one environment: production or preview:<branch>")
	f.StringVar(&grep, "grep", "", "free text, or field=value for a field of a JSON line")
	f.IntVar(&limit, "limit", 200, "how many lines to read when not following")
	c.AddCommand(logForwardingCmd(s))
	return c
}

// logForwardingCmd is where the org's logs are also sent: its own log services (CLI.md §5.5).
// Adding and removing one hands over a credential, so it is for an owner, signed in, in the
// dashboard; an agent reads how sending is going and asks for a test line.
func logForwardingCmd(s *session) *cobra.Command {
	c := &cobra.Command{
		Use:   "forwarding",
		Short: "Where the org's logs are also sent, and how sending is going",
		Long: `Lists the org's own log services that every line of every app is also sent to (any HTTPS
endpoint, Datadog or an OpenTelemetry collector), with how sending is going. An owner adds and
removes them in the dashboard; it is on the Business plan.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, err := s.org()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			list, err := client.ListLogDestinations(s.ctx, org)
			if err != nil {
				return wrap(err)
			}
			page := s.dashboard() + "/o/" + org + "/settings#logs"
			s.printer.Result(map[string]any{"org": org, "included": list.Included, "destinations": list.Items, "dashboard": page}, func(w io.Writer) {
				printLogDestinations(s, w, list, page)
			})
			return nil
		},
	}
	test := &cobra.Command{
		Use:   "test <id>",
		Short: "Send one destination a test line now",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, err := s.org()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			if err := client.TestLogDestination(s.ctx, org, args[0]); err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "id": args[0], "sent": true}, func(w io.Writer) {
				fmt.Fprintln(w, "The destination took the test line.")
			})
			return nil
		},
	}
	c.AddCommand(test)
	return c
}

func printLogDestinations(s *session, w io.Writer, list api.LogDestinations, page string) {
	if !list.Included {
		fmt.Fprintf(w, "Forwarding logs is on the Business plan. An owner changes the plan at %s.\n", strings.Replace(page, "/settings#logs", "/billing", 1))
		if len(list.Items) == 0 {
			return
		}
	}
	if len(list.Items) == 0 {
		fmt.Fprintf(w, "Logs are not forwarded anywhere. An owner adds a destination at %s.\n", page)
		return
	}
	rows := make([][]string, 0, len(list.Items))
	for _, d := range list.Items {
		last := "-"
		if !d.LastSentAt.IsZero() {
			last = d.LastSentAt.Local().Format("2006-01-02 15:04:05")
		}
		rows = append(rows, []string{d.ID, string(d.Kind), d.Where, string(d.Status), fmt.Sprint(d.LinesSent), last, orDash(d.LastError)})
	}
	s.printer.Table(w, []string{"ID", "KIND", "WHERE", "STATUS", "LINES SENT", "LAST SENT", "LAST ERROR"}, rows)
}

// formatLine is one line as a person reads it: the time, then for a JSON line its level, its
// message, the request it describes and its other fields as key=value; for any other line the
// stream when it is stderr and the line as printed. --json prints the app's own object instead,
// so a filter can be piped onto it.
func formatLine(l api.LogLine, asJSON bool) string {
	if asJSON && l.JSON != nil {
		if raw, err := json.Marshal(l.JSON); err == nil {
			return string(raw)
		}
	}
	prefix := l.At.Local().Format("15:04:05.000")
	if l.JSON == nil {
		if l.Stream == "stderr" {
			prefix += " stderr"
		}
		return prefix + "  " + l.Line
	}
	level := logLevel(l)
	if level == "" {
		level = "-"
	}
	return fmt.Sprintf("%s %-5s %s", prefix, strings.ToUpper(level), readableJSON(l.JSON))
}

// numberedLevels are pino's and bunyan's levels, written as numbers.
var numberedLevels = []struct {
	at   float64
	name string
}{{60, "fatal"}, {50, "error"}, {40, "warn"}, {30, "info"}, {20, "debug"}, {10, "trace"}}

// logLevel is a line's level: the platform's own lines are "platform", a JSON line's level field
// as a word, and a plain line on stderr an error.
func logLevel(l api.LogLine) string {
	if l.Stream == "platform" {
		return "platform"
	}
	for _, k := range []string{"level", "severity", "lvl"} {
		switch v := l.JSON[k].(type) {
		case string:
			return strings.ToLower(v)
		case float64:
			for _, n := range numberedLevels {
				if v >= n.at {
					return n.name
				}
			}
			return "trace"
		}
	}
	if l.Stream == "stderr" {
		return "error"
	}
	return ""
}

var (
	logQuietKeys   = []string{"level", "severity", "lvl", "time", "timestamp", "ts", "@timestamp", "pid", "hostname", "v", "app", "msg", "message"}
	logRequestKeys = []string{"method", "path", "url", "status", "statusCode", "status_code", "ms", "duration_ms", "responseTime", "response_time", "user", "req", "res"}
	// A request logger's own words for a request line; the request says it better.
	logGenericMessages = []string{"request", "request completed", "request finished", "http request", "incoming request", "response", "access"}
)

// readableJSON is a JSON line's message, request and other fields, in the dashboard's order
// (DASHBOARD.md §4.8): GET /reports 200 4.63s who, then the message, then key=value sorted by key.
func readableJSON(j map[string]any) string {
	str := func(vs ...any) string {
		for _, v := range vs {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
		return ""
	}
	num := func(vs ...any) (float64, bool) {
		for _, v := range vs {
			if n, ok := v.(float64); ok {
				return n, true
			}
		}
		return 0, false
	}
	obj := func(v any) map[string]any { m, _ := v.(map[string]any); return m }
	req, res, user := obj(j["req"]), obj(j["res"]), obj(j["user"])
	method := str(j["method"], req["method"])
	path := str(j["path"], j["url"], req["url"], req["path"])
	isRequest := method != "" && path != ""
	var parts []string
	if isRequest {
		parts = append(parts, strings.ToUpper(method), path)
		if status, ok := num(j["status"], j["statusCode"], j["status_code"], res["statusCode"], res["status"]); ok {
			parts = append(parts, fmt.Sprint(status))
		}
		if ms, ok := num(j["ms"], j["duration_ms"], j["responseTime"], j["response_time"]); ok && ms >= 0 {
			parts = append(parts, time.Duration(ms*float64(time.Millisecond)).Round(roundFor(ms)).String())
		}
		if who := str(j["user"], user["email"], user["id"]); who != "" {
			parts = append(parts, who)
		}
	}
	message := str(j["msg"], j["message"])
	if message != "" && !(isRequest && contains(logGenericMessages, strings.ToLower(message))) {
		parts = append(parts, message)
	}
	keys := make([]string, 0, len(j))
	for k, v := range j {
		if contains(logQuietKeys, k) || (isRequest && contains(logRequestKeys, k)) || v == nil || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts = append(parts, k+"="+logValue(j[k]))
	}
	return strings.Join(parts, " ")
}

// roundFor keeps three figures of a duration: 4.63s, 152ms.
func roundFor(ms float64) time.Duration {
	switch {
	case ms < 1:
		return time.Microsecond
	case ms < 1000:
		return time.Millisecond
	case ms < 10_000:
		return 10 * time.Millisecond
	default:
		return 100 * time.Millisecond
	}
}

// logValue is one field's value: text and an error's type and message quoted when they have
// spaces, so the line still splits on them, and anything else as its JSON.
func logValue(v any) string {
	var s string
	switch t := v.(type) {
	case string:
		s = t
	case map[string]any:
		if m, ok := t["message"].(string); ok && m != "" {
			kind, _ := t["type"].(string)
			if kind == "" {
				kind, _ = t["name"].(string)
			}
			s = strings.TrimPrefix(kind+": "+m, ": ")
			break
		}
		raw, _ := json.Marshal(t)
		return string(raw)
	default:
		raw, _ := json.Marshal(t)
		return string(raw)
	}
	if strings.ContainsAny(s, " \t\n\"") {
		return strconv.Quote(s)
	}
	return s
}

// errorGroupsCmd is the app's error groups, and the explanation of one platform error code.
// A code is upper case, so the two readings of "errors" never collide.
func errorGroupsCmd(s *session, explain *cobra.Command) *cobra.Command {
	var since, query string
	var limit int
	var resolve, ignore string
	c := &cobra.Command{
		Use:   "errors [CODE]",
		Short: "Error groups the app reported, or the meaning of one platform error code",
		Long: `With no argument, the app's error groups: what broke, how often, and when it was last
seen. With a code (upper case, like BUILD_FAILED), the platform's explanation of that code and how
to fix it.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				return explain.RunE(explain, args)
			}
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			if resolve != "" || ignore != "" {
				id, status := resolve, "resolved"
				if ignore != "" {
					id, status = ignore, "ignored"
				}
				if err := client.SetErrorStatus(s.ctx, org, app, id, status); err != nil {
					return wrap(err)
				}
				s.printer.Result(map[string]any{"id": id, "status": status}, func(w io.Writer) {
					fmt.Fprintf(w, "%s is %s.\n", id, status)
				})
				return nil
			}
			groups, err := client.ErrorGroups(s.ctx, org, app, query, since, limit)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "errors": groups}, func(w io.Writer) {
				if len(groups) == 0 {
					fmt.Fprintln(w, "Nothing has gone wrong. The app reports errors through SENTRY_DSN, which the platform sets for it.")
					return
				}
				rows := make([][]string, len(groups))
				for i, g := range groups {
					rows[i] = []string{g.ID, truncate(g.Title, 60), fmt.Sprint(g.Count), ago(g.LastSeen), g.Status}
				}
				s.printer.Table(w, []string{"ID", "ERROR", "COUNT", "LAST SEEN", "STATUS"}, rows)
			})
			return nil
		},
	}
	f := c.Flags()
	f.StringVar(&since, "since", "", "only groups seen since: 24h, 7d")
	f.StringVar(&query, "query", "", "only groups whose error says these words")
	f.IntVar(&limit, "limit", 25, "how many groups to show")
	f.StringVar(&resolve, "resolve", "", "mark a group resolved by id")
	f.StringVar(&ignore, "ignore", "", "mark a group ignored by id")
	return c
}

// ago is a last-seen time in words.
func ago(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
