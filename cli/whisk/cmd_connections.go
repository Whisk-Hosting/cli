package whisk

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/apitypes"
	"github.com/whisk-run/contract/connect"
	werrors "github.com/whisk-run/contract/errors"
)

// connectionsCmd is the app's connections: outside systems it reaches through Whisk's broker
// without holding their credential (CLI.md §5.6, CONTRACT.md §3.1, CONTROL-PLANE.md §6.8). A
// person grants, resumes and revokes them in the dashboard; the CLI lists, explains, pauses and
// sets their limits.
func connectionsCmd(s *session) *cobra.Command {
	connections := &cobra.Command{Use: "connections", Short: "Outside systems the app reaches through Whisk's broker"}

	list := &cobra.Command{
		Use:   "list",
		Short: "Each connection: grants by environment, state, what waits, calls today",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			all, err := client.ListConnections(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			url := connectionsURL(s.dashboard(), org, app)
			s.printer.Result(map[string]any{"org": org, "app": app, "connections": all, "url": url}, func(w io.Writer) {
				if len(all) == 0 {
					fmt.Fprintln(w, "This app declares no connections. Declare one under connections: in whisk.yaml (CONTRACT.md §3.1).")
					return
				}
				s.printer.Table(w, []string{"NAME", "HOST", "PRODUCTION", "PREVIEWS", "WAITS FOR", "CALLS TODAY"}, connectionRows(all))
				fmt.Fprintf(w, "A person grants, resumes and revokes them at %s\n", url)
			})
			return nil
		},
	}

	show := &cobra.Command{
		Use:   "show NAME",
		Short: "Whisk's summary of what a grant allows, and how each secret is used",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			all, err := client.ListConnections(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			c, ok := findConnection(all, args[0])
			if !ok {
				return output.New("NOT_FOUND", fmt.Sprintf("%s declares no connection named %s.", app, args[0]),
					"Run whisk connections list to see the app's connections.", map[string]any{"connection": args[0], "org": org, "app": app})
			}
			url := connectionsURL(s.dashboard(), org, app)
			s.printer.Result(map[string]any{"org": org, "app": app, "connection": c, "url": url}, func(w io.Writer) {
				fmt.Fprintln(w, addressLine(c.Name, c.Summary))
				if c.Summary.Insecure {
					fmt.Fprintf(w, "%s the address is not https, so calls and keys cross the network readable.\n", s.printer.Warn("Warning:"))
				}
				if c.Summary.TokenHost != "" {
					fmt.Fprintf(w, "It first fetches a token from %s.\n", c.Summary.TokenHost)
				}
				if c.Summary.SignsBody {
					fmt.Fprintln(w, "Its recipe signs each request's body.")
				}
				fmt.Fprintln(w)
				fmt.Fprintln(w, "What the app may do:")
				s.printer.Table(w, []string{"  ACTION", "METHOD", "PATH", "THE APP CALLS THIS"}, operationRows(c.Summary.Operations))
				fmt.Fprintln(w)
				if len(c.Summary.Secrets) == 0 {
					fmt.Fprintln(w, "Keys: none. The recipe sends no credential.")
				} else {
					fmt.Fprintln(w, "How each key is used:")
					s.printer.Table(w, []string{"  KEY", "USE"}, secretRows(c.Summary.Secrets))
				}
				for _, warn := range secretWarnings(c.Summary.Secrets) {
					fmt.Fprintf(w, "%s %s\n", s.printer.Warn("Warning:"), warn)
				}
				fmt.Fprintln(w, "The app may send anything these addresses accept.")
				fmt.Fprintln(w)
				if c.Summary.AI != nil {
					fmt.Fprintln(w, "AI spend cap, in US dollars:")
					s.printer.Table(w, []string{"  WINDOW", "CAP", "SPENT", "CALLS"}, aiWindowRows(c.AI))
					fmt.Fprintln(w, "What each answer costs:")
					s.printer.Table(w, []string{"  NUMBER AT", "PRICE"}, aiPartRows(c.Summary.AI.Parts))
					for _, line := range aiUnreadableLines(c.AI) {
						fmt.Fprintf(w, "%s %s\n", s.printer.Warn("Stopped:"), line)
					}
					fmt.Fprintln(w)
				}
				for _, env := range []apitypes.ConnectionEnvironment{apitypes.ConnectionProduction, apitypes.ConnectionPreviews} {
					fmt.Fprintf(w, "%s: %s\n", env, grantLine(c, env))
				}
				if len(c.Unset) > 0 {
					fmt.Fprintf(w, "Keys with no value yet: %s. Set them at %s\n", strings.Join(c.Unset, ", "), pasteURL(secretsURL(s.dashboard(), org, app), c.Unset))
				}
				fmt.Fprintf(w, "A person grants, resumes or revokes it at %s\n", url)
			})
			return nil
		},
	}

	grant := &cobra.Command{
		Use:   "grant NAME",
		Short: "The link where a person grants it; exits 2, always",
		Long: `Only a person grants a connection, freshly signed in, on the app's
connections page. The CLI never grants, so this prints the NEEDS_HUMAN block with that page
and exits 2. Relay it to the human and say plainly what each operation lets the app do.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.target()
			if err != nil {
				return err
			}
			return grantBlock(args[0], org, app, connectionsURL(s.dashboard(), org, app))
		},
	}

	var env string
	var all bool
	pause := &cobra.Command{
		Use:   "pause NAME [--env production|previews] | pause --all",
		Short: "Stop a connection's calls now, or every connection's in the business with --all",
		Long: `Pausing only takes access away, so an agent may: it is the first thing to do when an app may be
misbehaving. Calls stop at once; a call already on its way finishes. Without --env the
connection is paused in every environment. --all pauses every connection of every app in the
business. Resuming is a person's, in the dashboard.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name, environment, err := pausePlan(args, all, env)
			if err != nil {
				return err
			}
			if all {
				client, org, err := s.orgClient()
				if err != nil {
					return err
				}
				out, err := client.PauseAllConnections(s.ctx, org)
				if err != nil {
					return wrap(err)
				}
				s.printer.Result(map[string]any{"org": org, "all": true, "changed": out.Changed}, func(w io.Writer) {
					fmt.Fprintln(w, pauseSentence("", "", out.Changed))
				})
				return nil
			}
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			out, err := client.PauseConnection(s.ctx, org, app, name, environment)
			if err != nil {
				return wrap(err)
			}
			url := connectionsURL(s.dashboard(), org, app)
			s.printer.Result(map[string]any{"org": org, "app": app, "name": name, "environment": string(environment), "changed": out.Changed, "url": url}, func(w io.Writer) {
				fmt.Fprintln(w, pauseSentence(name, environment, out.Changed))
				if out.Changed > 0 {
					fmt.Fprintf(w, "An owner or admin resumes it at %s\n", url)
				}
			})
			return nil
		},
	}
	pause.Flags().StringVar(&env, "env", "", "production or previews (default: every environment)")
	pause.Flags().BoolVar(&all, "all", false, "pause every connection of every app in the business")

	var lim connect.SpendCap
	var calls apitypes.ConnectionLimitsRequest
	var limitEnv string
	limit := &cobra.Command{
		Use:   "limit NAME [--calls-per-minute N] [--calls-per-day N] [--per-month USD] [--per-day USD] [--per-week USD] [--total USD] [--env production|previews]",
		Short: "Set how many calls a connection may make, or how much an AI connection may spend",
		Long: `Sets a connection's limits, in force from the next call with no push: calls refused at
the old limit go on at once if the new one is higher.

--calls-per-minute and --calls-per-day are how many calls the app may make through the
connection (600 a minute and 100,000 a day unless someone changed them; at most 1,000 a minute
and 1,000,000 a day). Raise them to fit the app's work, and keep them below what the outside
system itself allows. A limit you leave out keeps its value.

--per-day, --per-week, --per-month and --total set an AI connection's spending limit in US
dollars. Ask your person how much the app may spend first and set what they choose; they can
change it themselves on the app's page.

Without --env it is production's.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spend := lim != (connect.SpendCap{})
			environment, err := limitPlan(lim, spend, calls.LimitPerMinute, calls.LimitPerDay, limitEnv)
			if err != nil {
				return err
			}
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			result := map[string]any{"org": org, "app": app, "name": args[0], "environment": string(environment)}
			var sentences []string
			if calls.LimitPerMinute != 0 || calls.LimitPerDay != 0 {
				calls.Environment = environment
				out, err := client.SetConnectionLimits(s.ctx, org, app, args[0], calls)
				if err != nil {
					return wrap(err)
				}
				g, _ := grantIn(out, environment)
				result["limit_per_minute"], result["limit_per_day"] = g.LimitPerMinute, g.LimitPerDay
				sentences = append(sentences, callsSentence(args[0], environment, g))
			}
			if spend {
				out, err := client.SetSpendCap(s.ctx, org, app, args[0], apitypes.SpendCapRequest{Environment: environment, SpendCap: lim})
				if err != nil {
					return wrap(err)
				}
				result["spend_cap"], result["ai"] = lim, out.AI
				sentences = append(sentences, limitSentence(args[0], environment, lim))
			}
			s.printer.Result(result, func(w io.Writer) {
				for _, line := range sentences {
					fmt.Fprintln(w, line)
				}
			})
			return nil
		},
	}
	limit.Flags().IntVar(&calls.LimitPerMinute, "calls-per-minute", 0, "calls a minute, 1 to 1,000")
	limit.Flags().IntVar(&calls.LimitPerDay, "calls-per-day", 0, "calls a day (UTC), 1 to 1,000,000")
	limit.Flags().Float64Var(&lim.PerDay, "per-day", 0, "AI spending: US dollars a day (UTC)")
	limit.Flags().Float64Var(&lim.PerWeek, "per-week", 0, "AI spending: US dollars a week, from Monday (UTC)")
	limit.Flags().Float64Var(&lim.PerMonth, "per-month", 0, "AI spending: US dollars a month (UTC)")
	limit.Flags().Float64Var(&lim.Total, "total", 0, "AI spending: US dollars in all")
	limit.Flags().StringVar(&limitEnv, "env", "production", "production or previews")

	connections.AddCommand(list, show, grant, pause, limit)
	return connections
}

// connectionsURL is the app's Keys tab, where a person allows, resumes and turns off each
// outside system (DASHBOARD.md §4.7).
func connectionsURL(dash, org, app string) string {
	return appPage(dash, org, app, "/secrets")
}

func findConnection(all []api.Connection, name string) (api.Connection, bool) {
	for _, c := range all {
		if c.Name == name {
			return c, true
		}
	}
	return api.Connection{}, false
}

func grantIn(c api.Connection, env apitypes.ConnectionEnvironment) (api.ConnectionGrant, bool) {
	for _, g := range c.Grants {
		if g.Environment == env {
			return g, true
		}
	}
	return api.ConnectionGrant{}, false
}

// grantState is one environment's grant in a word or two, as the dashboard names them:
// granted, waits for you, paused, revoked, not granted. A grant of an earlier version says so.
func grantState(c api.Connection, env apitypes.ConnectionEnvironment) string {
	g, ok := grantIn(c, env)
	switch {
	case !ok && c.Waiting && env == apitypes.ConnectionProduction:
		return "waits for you"
	case !ok:
		return "not granted"
	case g.State == apitypes.ConnectionRevoked:
		return "revoked"
	case g.State == apitypes.ConnectionPaused:
		return "paused"
	case !g.Current && c.Waiting && env == apitypes.ConnectionProduction:
		return "waits for you"
	case !g.Current:
		return "granted, older version"
	}
	return "granted"
}

// grantLine is one environment's grant with its limits and today's calls, for show.
func grantLine(c api.Connection, env apitypes.ConnectionEnvironment) string {
	state := grantState(c, env)
	g, ok := grantIn(c, env)
	if !ok {
		return state
	}
	line := fmt.Sprintf("%s; granted by %s on %s; up to %d calls a minute and %d a day; %d calls and %d refused today",
		state, orDash(g.GrantedBy), at(g.GrantedAt), g.LimitPerMinute, g.LimitPerDay, g.CallsToday, g.RefusedToday)
	if len(g.Missing) > 0 {
		line += "; not yet granted: " + operationList(g.Missing)
	}
	return line
}

// waitsFor names what keeps a connection from working: a person's grant, keys with no value.
func waitsFor(c api.Connection) string {
	var parts []string
	if c.Waiting {
		parts = append(parts, "a grant")
	}
	if len(c.Unset) > 0 {
		parts = append(parts, "keys "+strings.Join(c.Unset, ", "))
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, "; ")
}

func callsToday(c api.Connection) int {
	n := 0
	for _, g := range c.Grants {
		n += g.CallsToday
	}
	return n
}

// connectionRows are the list table's rows.
func connectionRows(all []api.Connection) [][]string {
	rows := make([][]string, len(all))
	for i, c := range all {
		rows[i] = []string{c.Name, orDash(c.Summary.Host), grantState(c, apitypes.ConnectionProduction), grantState(c, apitypes.ConnectionPreviews), waitsFor(c), fmt.Sprint(callsToday(c))}
	}
	return rows
}

// addressLine says where the connection's calls go, from the rules the broker enforces.
func addressLine(name string, sum connect.Summary) string {
	line := fmt.Sprintf("%s sends requests to %s:%s", name, orDash(sum.Host), orDash(sum.Port))
	if sum.BasePath != "" && sum.BasePath != "/" {
		line += ", below " + sum.BasePath
	}
	return line + "."
}

// operationRows are show's operations: the method in words, the method, the path pattern, and
// the app's own label for it.
func operationRows(ops []connect.SummaryOperation) [][]string {
	rows := make([][]string, len(ops))
	for i, o := range ops {
		verb := o.Verb
		if verb == "" {
			verb = o.Method
		}
		rows[i] = []string{"  " + verb, o.Method, o.Path, orDash(o.Label)}
	}
	return rows
}

// secretUse is how a recipe uses one key, in words.
func secretUse(u connect.Use) string {
	if !u.Sent {
		return "only signs; it never leaves Whisk"
	}
	return "sent to " + hostList(u.To)
}

func hostList(hosts []string) string {
	if len(hosts) == 0 {
		return "the outside system"
	}
	return strings.Join(hosts, ", ")
}

func secretRows(uses []connect.Use) [][]string {
	rows := make([][]string, len(uses))
	for i, u := range uses {
		rows[i] = []string{"  " + u.Name, secretUse(u)}
	}
	return rows
}

// secretWarnings warns once for every key the recipe sends (BROKER.md §4).
func secretWarnings(uses []connect.Use) []string {
	var out []string
	for _, u := range uses {
		if u.Sent {
			out = append(out, fmt.Sprintf("%s is sent to %s with every request. That system could record it.", u.Name, hostList(u.To)))
		}
	}
	return out
}

// grantBlock is grant's answer: the NEEDS_HUMAN block with the connections page, exit 2.
func grantBlock(name, org, app, page string) *output.Error {
	return &output.Error{
		Code:    "NEEDS_HUMAN",
		Message: fmt.Sprintf("Granting %s is for a person, freshly signed in, in the dashboard; the CLI never grants.", name),
		Fix:     fmt.Sprintf("Ask an owner or admin to open %s, review what %s may do and grant it. Tell them plainly what each operation lets the app do (whisk connections show %s). A deploy waiting on it starts when it is granted.", page, name, name),
		Docs:    "https://skill.whisk.run/errors/NEEDS_HUMAN",
		Details: map[string]any{"url": page, "name": name, "org": org, "app": app},
	}
}

// pausePlan checks pause's arguments: NAME with an optional --env, or --all alone.
func pausePlan(args []string, all bool, env string) (string, apitypes.ConnectionEnvironment, error) {
	if all {
		if len(args) > 0 || env != "" {
			return "", "", output.New("INVALID_REQUEST", "--all pauses every connection in the business, so it takes no NAME and no --env.",
				"Run whisk connections pause --all, or whisk connections pause NAME [--env production|previews].", nil)
		}
		return "", "", nil
	}
	if len(args) == 0 {
		return "", "", output.New("INVALID_REQUEST", "Name the connection to pause, or pass --all.",
			"Run whisk connections pause NAME, or whisk connections pause --all to stop every connection in the business.", nil)
	}
	switch apitypes.ConnectionEnvironment(env) {
	case "", apitypes.ConnectionProduction, apitypes.ConnectionPreviews:
		return args[0], apitypes.ConnectionEnvironment(env), nil
	}
	return "", "", output.New("INVALID_REQUEST", fmt.Sprintf("--env %q is not an environment a connection is granted for.", env),
		"Pass --env production or --env previews, or leave it out to pause every environment.", map[string]any{"env": env})
}

// limitPlan checks limit's flags: call limits, a spending limit or both, each within bounds,
// and an environment. spend is whether any spending window was given. Pure.
func limitPlan(lim connect.SpendCap, spend bool, perMinute, perDay int, env string) (apitypes.ConnectionEnvironment, error) {
	calls := perMinute != 0 || perDay != 0
	switch {
	case !calls && !spend:
		return "", output.New("INVALID_REQUEST", "Name a new limit: --calls-per-minute or --calls-per-day, or for an AI connection a spending limit.",
			"Run whisk connections limit NAME --calls-per-minute <calls>, or whisk connections limit NAME --per-month <dollars> with the amount your person chose.", nil)
	case perMinute < 0 || perDay < 0:
		return "", output.New("INVALID_REQUEST", "Call limits are whole numbers from 1: up to 1,000 a minute and 1,000,000 a day.",
			"Pass --calls-per-minute and --calls-per-day within those bounds, or leave one out to keep it.", map[string]any{"calls_per_minute": perMinute, "calls_per_day": perDay})
	}
	if spend {
		if problems := connect.CapProblems(lim); len(problems) > 0 {
			return "", output.New("INVALID_REQUEST", "A spending limit names at least one of --per-day, --per-week, --per-month or --total, each "+problems[0].Message+".",
				"Ask your person how much the app may spend, then run whisk connections limit NAME --per-month <dollars>.", nil)
		}
	}
	switch e := apitypes.ConnectionEnvironment(env); e {
	case apitypes.ConnectionProduction, apitypes.ConnectionPreviews:
		return e, nil
	}
	return "", output.New("INVALID_REQUEST", fmt.Sprintf("--env %q is not an environment a connection is allowed in.", env),
		"Pass --env production or --env previews.", map[string]any{"env": env})
}

// callsSentence says what a grant's call limits are now. Pure.
func callsSentence(name string, env apitypes.ConnectionEnvironment, g api.ConnectionGrant) string {
	return fmt.Sprintf("%s may now make up to %s calls a minute and %s a day in %s.", name, thousands(g.LimitPerMinute), thousands(g.LimitPerDay), env)
}

// limitSentence says what a limit is now, window by window.
func limitSentence(name string, env apitypes.ConnectionEnvironment, lim connect.SpendCap) string {
	words := map[string]string{connect.PeriodDay: "a day", connect.PeriodWeek: "a week", connect.PeriodMonth: "a month", connect.PeriodTotal: "in all"}
	caps := lim.Caps()
	var parts []string
	for _, p := range connect.Periods {
		if v, ok := caps[p]; ok {
			parts = append(parts, fmt.Sprintf("US$%.2f %s", v, words[p]))
		}
	}
	return fmt.Sprintf("%s may now spend %s in %s.", name, strings.Join(parts, ", "), env)
}

// pauseSentence says what a pause did. name "" is pause --all.
func pauseSentence(name string, env apitypes.ConnectionEnvironment, changed int) string {
	grants := "grants"
	if changed == 1 {
		grants = "grant"
	}
	switch {
	case name == "" && changed == 0:
		return "No connection in the business was active, so nothing changed."
	case name == "":
		return fmt.Sprintf("Paused every connection in the business (%d %s). Calls stop at once; a call already on its way finishes.", changed, grants)
	}
	where := "every environment"
	if env != "" {
		where = string(env)
	}
	if changed == 0 {
		return fmt.Sprintf("%s had no active grant in %s, so nothing changed.", name, where)
	}
	return fmt.Sprintf("Paused %s in %s (%d %s). Calls stop at once; a call already on its way finishes.", name, where, changed, grants)
}

// operationList names operations as method, path and the app's label.
func operationList(ops []connect.Operation) string {
	parts := make([]string, len(ops))
	for i, o := range ops {
		parts[i] = o.Method + " " + o.Path
		if o.Name != "" {
			parts[i] += " (" + o.Name + ")"
		}
	}
	return strings.Join(parts, ", ")
}

// allowPage is where an owner or admin allows what a production deploy waits on, every kind at
// once with the keys it needs: the blocked deploy's Allow step (CONTROL-PLANE.md §6.5).
func allowPage(dash, org, app, deployID string) string {
	return appPage(dash, org, app, "/deploys?approve="+url.QueryEscape(deployID))
}

// grantChange says what one waiting grant would allow that the last one did not: for a shared
// secret or wider access, the words the control plane gave.
func grantChange(g api.GrantNeeded) string {
	if g.Kind == apitypes.GrantSecret || g.Kind == apitypes.GrantAccess {
		if c := strings.TrimSuffix(g.Change, "."); len(c) > 1 && c[0] >= 'A' && c[0] <= 'Z' && (c[1] < 'A' || c[1] > 'Z') {
			return strings.ToLower(c[:1]) + c[1:]
		} else if c != "" {
			return c
		}
		return string(g.Kind) + " " + g.Name
	}
	var what []string
	if g.RecipeChanged {
		what = append(what, "a new address or recipe")
	}
	if len(g.NewOperations) > 0 {
		what = append(what, "new operations "+operationList(g.NewOperations))
	}
	s := g.Name
	if g.Environment != "" {
		s += " (" + string(g.Environment) + ")"
	}
	if len(what) == 0 {
		return s
	}
	return s + " with " + strings.Join(what, " and ")
}

// blockedDetail reads what a blocked deploy's error names: the grants it waits on and the
// secrets with no value (GRANT_NEEDED's details.grants and details.unset_secrets).
func blockedDetail(d *werrors.Detail) ([]api.GrantNeeded, []string) {
	if d == nil || d.Code != "GRANT_NEEDED" || d.Details == nil {
		return nil, nil
	}
	var grants []api.GrantNeeded
	if raw, err := json.Marshal(d.Details["grants"]); err == nil {
		_ = json.Unmarshal(raw, &grants)
	}
	var unset []string
	if raw, err := json.Marshal(d.Details["unset_secrets"]); err == nil {
		_ = json.Unmarshal(raw, &unset)
	}
	return grants, unset
}

// grantsNeededBlock is the deploy's answer when it waits on a person (CLI.md §5.4): GRANT_NEEDED
// naming each thing it waits on, what changed and the Allow step where an owner or admin allows
// it and pastes any keys it also needs, exit 2.
func grantsNeededBlock(deployID string, grants []api.GrantNeeded, unset []string, page string) *output.Error {
	listed := make([]api.GrantNeeded, len(grants))
	names := make([]string, len(grants))
	changes := make([]string, len(grants))
	for i, g := range grants {
		if g.NewOperations == nil {
			g.NewOperations = []connect.Operation{}
		}
		listed[i], names[i], changes[i] = g, g.Name, grantChange(g)
	}
	unset = nonNilStrings(unset)
	message := fmt.Sprintf("Deploy %s waits for an owner or admin to allow: %s.", deployID, strings.Join(changes, "; "))
	fix := fmt.Sprintf("Ask an owner or admin to allow it at %s: it takes one step there, signed in. Tell them plainly what each item lets the app do. Do not try to allow it yourself: only a person, freshly signed in, can. The deploy starts when it is allowed.", page)
	if len(unset) > 0 {
		fix += fmt.Sprintf(" They paste %s on the same page.", strings.Join(unset, ", "))
	}
	return &output.Error{
		Code:    "GRANT_NEEDED",
		Message: message,
		Fix:     fix,
		Docs:    werrors.DocsBase + "GRANT_NEEDED",
		Details: map[string]any{"deploy_id": deployID, "grants": listed, "names": names, "unset_secrets": unset, "url": page},
	}
}

// keysPage is where a blocked deploy's keys are pasted: the page the control plane named (a
// production deploy's Allow step), else the app's keys page with a field open for each key.
func keysPage(d *werrors.Detail, secretsPage string, names []string) string {
	if d != nil && d.Code == "SECRET_UNSET" && d.Details != nil {
		if u, ok := d.Details["url"].(string); ok && strings.Contains(u, "?approve=") {
			return u
		}
	}
	return pasteURL(secretsPage, names)
}

// aiWindowRows are an AI connection's windows: each cap, what was spent, and whether calls run.
func aiWindowRows(a *apitypes.ConnectionAI) [][]string {
	if a == nil {
		return [][]string{}
	}
	rows := make([][]string, 0, len(a.Windows))
	for _, w := range a.Windows {
		calls := "running"
		switch {
		case w.Stopped && w.Until != nil:
			calls = "stopped until " + when(w.Until)
		case w.Stopped:
			calls = "stopped until the cap is raised"
		}
		rows = append(rows, []string{"  " + string(w.Period), fmt.Sprintf("%.2f", w.Cap), fmt.Sprintf("%.2f", w.Spent), calls})
	}
	return rows
}

// aiPartRows are an AI connection's cost parts: where each number is read and its price.
func aiPartRows(parts []connect.AIPart) [][]string {
	rows := make([][]string, 0, len(parts))
	for _, p := range parts {
		price := fmt.Sprintf("US$%g per million", p.USDPerMillion)
		if p.USDEach > 0 {
			price = fmt.Sprintf("US$%g each", p.USDEach)
		}
		rows = append(rows, []string{"  " + strings.Join(p.At, " or "), price})
	}
	return rows
}

// aiUnreadableLines say which deploys' calls stopped because an answer named no cost part.
func aiUnreadableLines(a *apitypes.ConnectionAI) []string {
	if a == nil {
		return nil
	}
	out := make([]string, 0, len(a.Unreadable))
	for _, u := range a.Unreadable {
		out = append(out, fmt.Sprintf("deploy %s: an answer to %q at %s named none of the cost parts. Fix ai.cost in whisk.yaml and deploy again.", u.DeployID, u.Operation, at(u.At)))
	}
	return out
}
