package whisk

import (
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
)

// Managed apps (MANAGED-APPS.md §11, CLI.md §5.2): apps in the business whose code and releases
// are Whisk's. The business adds a copy of a product, sets what the product lets it set, links
// it to one of its own apps, and pauses or resumes it. A product whose copies are forks
// (FORKS.md) is added the same way, and its app is the business's own code, which follows the
// product's releases.

// settingName is an environment variable's name, the form a product's settings take.
var settingName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// parseSettings reads NAME=value words into settings. With removable, NAME= (an empty value)
// removes the setting, as PATCH .../managed reads it; without, an empty value is refused. A name
// given twice is refused. Pure.
func parseSettings(words []string, removable bool) (map[string]string, error) {
	out := map[string]string{}
	for _, w := range words {
		name, value, ok := strings.Cut(w, "=")
		if !ok || !settingName.MatchString(name) {
			return nil, output.New("INVALID_REQUEST", fmt.Sprintf("%q is not a setting: write NAME=value.", w),
				"Name the setting as the product or its variant lists it (whisk managed list), then = and its value, as in LIMIT_PER_SECOND=5.", map[string]any{"setting": w})
		}
		if value == "" && !removable {
			return nil, output.New("INVALID_REQUEST", name+" has no value.",
				"Give it a value, or leave it out to keep the product's default.", map[string]any{"setting": name})
		}
		if _, seen := out[name]; seen {
			return nil, output.New("INVALID_REQUEST", name+" is set twice.", "Set each setting once.", map[string]any{"setting": name})
		}
		out[name] = value
	}
	return out, nil
}

// parseLinks reads role=app words: the role the product names and the business's app, by slug
// or id. A role given twice is refused. Pure.
func parseLinks(words []string) (map[string]string, error) {
	out := map[string]string{}
	for _, w := range words {
		role, app, ok := strings.Cut(w, "=")
		role, app = strings.TrimSpace(role), strings.TrimSpace(app)
		if !ok || role == "" || app == "" {
			return nil, output.New("INVALID_REQUEST", fmt.Sprintf("%q is not a link: write role=app.", w),
				"Name the role the product lists (whisk managed list) and one of your apps, as in --link shop=shop.", map[string]any{"link": w})
		}
		if _, seen := out[role]; seen {
			return nil, output.New("INVALID_REQUEST", "The "+role+" link is named twice.", "Link each role to one app.", map[string]any{"role": role})
		}
		out[role] = app
	}
	return out, nil
}

// resolveLinks turns each link's app, named by slug or id, into the app's id, which the API
// takes (MANAGED-APPS.md §9: nothing is chosen by slug). Pure.
func resolveLinks(org string, links map[string]string, apps []api.App) (map[string]string, error) {
	out := map[string]string{}
	for _, role := range slices.Sorted(maps.Keys(links)) {
		ref := links[role]
		i := slices.IndexFunc(apps, func(a api.App) bool { return a.Slug == ref || a.ID == ref })
		if i < 0 {
			return nil, output.New("INVALID_REQUEST", fmt.Sprintf("%s has no app called %s to link as %s.", org, ref, role),
				"Name one of the business's apps by its slug or id (whisk apps list).", map[string]any{"role": role, "app": ref})
		}
		out[role] = apps[i].ID
	}
	return out, nil
}

// linksText is a copy's links as role=app, the app by slug where the list has it, by id
// otherwise. Pure.
func linksText(links map[string]string, apps []api.App) string {
	if len(links) == 0 {
		return "-"
	}
	parts := make([]string, 0, len(links))
	for _, role := range slices.Sorted(maps.Keys(links)) {
		name := links[role]
		if i := slices.IndexFunc(apps, func(a api.App) bool { return a.ID == name }); i >= 0 {
			name = apps[i].Slug
		}
		parts = append(parts, role+"="+name)
	}
	return strings.Join(parts, ", ")
}

// monthText is a product's monthly price per copy. Whisk's prices are in US dollars, shown as
// whisk billing shows money. Pure.
func monthText(cents *int64) string {
	switch {
	case cents == nil:
		return "-"
	case *cents == 0:
		return "included"
	}
	return money(*cents, "usd") + " a month"
}

// unavailableText is why a business cannot add a product, in plain words. Pure.
func unavailableText(p api.ManagedProduct) string {
	if p.Available {
		return "yes"
	}
	switch p.Unavailable {
	case "plan":
		return "no: not in this plan"
	case "trial":
		return "no: not during the free trial"
	case "client":
		return "no: not for a business its agency pays for"
	case "no_release":
		return "no: no release ready yet"
	case "retired":
		return "no: no longer offered"
	case "":
		return "no"
	}
	return "no: " + p.Unavailable
}

// variantsText is a product's variants, each by the name --variant takes, with the settings a
// copy of that variant may set besides the product's in brackets. Pure.
func variantsText(p api.ManagedProduct) string {
	if len(p.Variants) == 0 {
		return "-"
	}
	names := make([]string, len(p.Variants))
	for i, v := range p.Variants {
		names[i] = v.Name
		if len(v.Settings) > 0 {
			names[i] += " (" + strings.Join(v.Settings, ", ") + ")"
		}
	}
	return strings.Join(names, ", ")
}

// settingsText is a product's settings, the link key marked: an app may link a copy for each of
// its values (MANAGED-APPS.md §4). Pure.
func settingsText(p api.ManagedProduct) string {
	names := make([]string, len(p.Settings))
	for i, s := range p.Settings {
		names[i] = s
		if s == p.LinkKey {
			names[i] += " (a copy per value)"
		}
	}
	return listText(names)
}

// listText is a list of names, or - when there are none. Pure.
func listText(names []string) string {
	if len(names) == 0 {
		return "-"
	}
	return strings.Join(names, ", ")
}

// appStatusText is an app's status in whisk apps list, with managed for a copy, paused for a
// paused app, held for a copy Whisk holds on its release, and fork for an app that follows a
// product, with how many of its releases the app's code has not taken. Pure.
func appStatusText(a api.App) string {
	words := []string{string(a.Status)}
	if a.Managed != nil {
		words = append(words, "managed")
		if a.Managed.Held {
			words = append(words, "held")
		}
	}
	if f := a.Fork; f != nil {
		words = append(words, "fork")
		if f.Behind > 0 {
			words = append(words, fmt.Sprintf("%d behind", f.Behind))
		}
	}
	if a.PausedAt != nil {
		words = append(words, "paused")
	}
	return strings.Join(words, ", ")
}

// releaseText is the release a copy runs, by its short commit. Pure.
func releaseText(m *api.AppManaged) string {
	if m == nil || m.Release == nil || m.Release.Commit == "" {
		return "-"
	}
	return shortCommit(m.Release.Commit)
}

func shortCommit(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// managedLines is what whisk apps info adds for a copy or a paused app. Pure.
func managedLines(a api.App, apps []api.App) []string {
	var out []string
	if m := a.Managed; m != nil {
		line := "managed by Whisk: " + m.Name + " (" + m.Product + ")"
		if m.Variant != "" {
			line += ", variant " + m.Variant
		}
		out = append(out, line)
		if m.Release != nil {
			out = append(out, fmt.Sprintf("release %s, live since %s", shortCommit(m.Release.Commit), when(&m.Release.LiveAt)))
		}
		if m.Held {
			out = append(out, "held on its release by Whisk")
		}
		out = append(out, "links "+linksText(m.Links, apps))
		names := slices.Sorted(maps.Keys(m.Settings))
		settings := make([]string, len(names))
		for i, n := range names {
			settings[i] = n + "=" + m.Settings[n]
		}
		out = append(out, "settings "+listText(settings))
	}
	if a.PausedAt != nil {
		out = append(out, "paused since "+when(a.PausedAt)+"; whisk resume "+a.Slug+" resumes it")
	}
	return out
}

// forkEmpty reports whether a fork has no code of its own yet: it holds no release and nothing
// of the business's. Pure.
func forkEmpty(f *api.AppFork) bool { return f.Release == 0 && !f.Changed }

// forkText is where a fork stands against its product's releases, in a few words. Pure.
func forkText(f *api.AppFork) string {
	code := "as Whisk made it"
	if f.Changed {
		code = "with changes of its own"
	}
	switch {
	case forkEmpty(f):
		return "no code of its own yet"
	case f.Release == 0:
		return "code of its own that holds none of its releases"
	case f.Behind == 0:
		return fmt.Sprintf("release %d, the newest, %s", f.Release, code)
	}
	return fmt.Sprintf("release %d %s, %d behind release %d", f.Release, code, f.Behind, f.Newest)
}

// forkNext is what brings a fork its product's newest release: the update for code Whisk may
// move, the merge for code the business changed (whisk managed merge once the code holds a
// release), and "" for a fork that holds it. Pure.
func forkNext(slug string, f *api.AppFork) string {
	switch {
	case f.Behind == 0:
		return ""
	case f.Changed && f.Release > 0:
		return fmt.Sprintf("whisk managed merge in a checkout of its code brings release %d in, naming each change of the business's it meets, and whisk deploy puts it live", f.Newest)
	case f.Changed:
		return fmt.Sprintf("git fetch whisk, then git merge whisk/%s, brings release %d into its code, and whisk deploy puts it live", f.Branch, f.Newest)
	}
	return fmt.Sprintf("whisk managed update %s moves it to release %d", slug, f.Newest)
}

// forkLines is what whisk apps info adds for a fork. Pure.
func forkLines(a api.App) []string {
	f := a.Fork
	if f == nil {
		return nil
	}
	out := []string{
		fmt.Sprintf("follows %s (%s): %s", f.Name, f.Product, forkText(f)),
		fmt.Sprintf("its releases arrive on the branch %s, which only Whisk moves", f.Branch),
	}
	if next := forkNext(a.Slug, f); next != "" {
		out = append(out, next)
	}
	return out
}

// forkRow is a fork's row in whisk managed list: the release its code holds, the newest, whether
// the code is the business's own by now, and its status. Pure.
func forkRow(a api.App) []string {
	f := a.Fork
	if f == nil {
		return []string{a.Slug, "-", "-", "-", "-", appStatusText(a)}
	}
	release, code := fmt.Sprint(f.Release), "as released"
	switch {
	case forkEmpty(f):
		release, code = "-", "-"
	case f.Release == 0:
		release, code = "-", "changed"
	case f.Changed:
		code = "changed"
	}
	return []string{a.Slug, f.Product, release, fmt.Sprint(f.Newest), code, appStatusText(a)}
}

// kindText is what a copy of the product is, in a product's row. Pure.
func kindText(p api.ManagedProduct) string {
	if p.Kind == api.ProductForks {
		return "your own code"
	}
	return "run by Whisk"
}

// forksOffered reports whether the products include one whose copies are the business's own
// code. Pure.
func forksOffered(products []api.ManagedProduct) bool {
	return slices.ContainsFunc(products, func(p api.ManagedProduct) bool { return p.Kind == api.ProductForks })
}

// addHints are the closing lines of whisk managed list: how to add a product of each kind
// listed. Pure.
func addHints(products []api.ManagedProduct) []string {
	var out []string
	if slices.ContainsFunc(products, func(p api.ManagedProduct) bool { return p.Kind != api.ProductForks }) {
		out = append(out, "Add one with: whisk managed add <product> --link <role>=<app> [--variant v] [--set NAME=value]")
	}
	if forksOffered(products) {
		out = append(out, "Add one that is your own code with: whisk managed add <product>")
	}
	return out
}

// updateLine says what an update of a fork did. Pure.
func updateLine(org string, u api.ForkUpdate) string {
	a, f := u.App, u.App.Fork
	switch {
	case f == nil && u.Updated:
		return fmt.Sprintf("Moved %s/%s to its product's newest release.", org, a.Slug)
	case f == nil:
		return fmt.Sprintf("%s/%s has nothing to take.", org, a.Slug)
	case u.Updated:
		return fmt.Sprintf("Moved %s/%s to %s release %d.", org, a.Slug, f.Name, f.Release)
	case u.Deploy == nil:
		return fmt.Sprintf("%s/%s holds %s release %d, the newest, with changes of its own, so there is nothing to take.", org, a.Slug, f.Name, f.Release)
	}
	return fmt.Sprintf("%s/%s already holds %s release %d, the newest.", org, a.Slug, f.Name, f.Release)
}

// forkUpdated reports what an update of a fork did, and follows the deploy that carries the
// fork's code when there is one: the deploy the update started, or the one already live or on
// its way.
func (s *session) forkUpdated(client *api.Client, org, app string, u api.ForkUpdate, noWait bool) error {
	line := updateLine(org, u)
	if u.Deploy == nil {
		s.printer.Result(map[string]any{"org": org, "app": u.App, "updated": false}, func(w io.Writer) { fmt.Fprintln(w, line) })
		return nil
	}
	d := *u.Deploy
	if noWait {
		s.printer.Result(map[string]any{"org": org, "app": u.App, "updated": u.Updated, "deploy_id": d.ID, "status": d.Status, "commit_sha": d.CommitSHA, "environment": d.Environment}, func(w io.Writer) {
			fmt.Fprintln(w, line)
			fmt.Fprintf(w, "Follow its deploy with: whisk deploys info %s\n", d.ID)
		})
		return nil
	}
	s.printer.Progress("%s", line)
	s.forkUpdate = map[string]any{"updated": u.Updated, "fork": u.App.Fork}
	return s.waitForDeploy(client, org, app, d)
}

// addedLines is what whisk managed add says once the copy is made: the product by the name its
// copy carries, the variant and links when there are any, what a fork's business does next, and
// the copy's address. linked is the links in words, empty for none. Pure.
func addedLines(org, product string, a api.App, variant, linked string) []string {
	name := product
	if a.Managed != nil {
		name = a.Managed.Name
	}
	if a.Fork != nil {
		name = a.Fork.Name
	}
	line := fmt.Sprintf("Added %s to %s as %s", name, org, a.Slug)
	if variant != "" {
		line += " (" + variant + ")"
	}
	if linked != "" {
		line += ", linked " + linked
	}
	out := []string{line + ".", fmt.Sprintf("It is deploying the newest release now; whisk status --app %s shows when it is live.", a.Slug)}
	if f := a.Fork; f != nil {
		out = append(out,
			fmt.Sprintf("Its code is the business's own: whisk clone %s/%s brings it here, and whisk deploy puts a change live.", org, a.Slug),
			fmt.Sprintf("New releases of %s arrive on its branch %s, which only Whisk moves; whisk managed update %s takes one while the code is unchanged, and whisk managed merge brings one into changed code.", f.Name, f.Branch, a.Slug))
	}
	if a.Hostname != "" {
		out = append(out, "https://"+a.Hostname)
	}
	return out
}

// managedUpdateCmd is whisk managed update: a fork's unchanged code moves to its product's
// newest release and that deploys (FORKS.md §4).
func managedUpdateCmd(s *session) *cobra.Command {
	var noWait bool
	update := &cobra.Command{
		Use:   "update [<app>]",
		Short: "Move a fork's unchanged code to its product's newest release and deploy it",
		Long: `For an app that follows one of Whisk's products as the business's own code (default: the app
this directory is bound to). When its code is a release as Whisk made it, the code moves to the
product's newest release and that deploys. Code the business has changed is never moved: the
command answers FORK_CHANGED, and whisk managed merge in a checkout brings the release in. When the
code already holds the newest release, the command follows the deploy that carries it.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				s.appFlag = args[0]
			}
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			u, err := client.UpdateFork(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			return s.forkUpdated(client, org, app, u, noWait)
		},
	}
	update.Flags().BoolVar(&noWait, "no-wait", false, "print the deploy id and exit")
	return update
}

func managedCmd(s *session) *cobra.Command {
	listRun := func(*cobra.Command, []string) error { return s.managedList() }

	managed := &cobra.Command{
		Use:   "managed",
		Short: "Apps Whisk runs for the business: add, set up, list",
		Long: `A managed app is an app in the business whose code and releases are Whisk's: the business adds
a copy of a product, links it to one of its own apps, sets what the product lets it set, and
pauses or resumes it, and every Whisk release reaches the copy without anyone deploying. Its code
cannot be pushed, cloned or deployed (APP_MANAGED).

A product listed as your own code is added the same way and makes an ordinary app of the
business's, to clone, change and deploy as any other. Whisk's releases of the product arrive on
the app's branch "upstream", which only Whisk moves: whisk managed update takes one while the code
is unchanged, and whisk managed merge brings one into a checkout of changed code.

With no subcommand, whisk managed lists the products and the business's copies.`,
		Args: cobra.NoArgs,
		RunE: listRun,
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "The products the business may add, with their price, and the copies it has",
		Args:  cobra.NoArgs,
		RunE:  listRun,
	}

	var variant string
	var linkWords, setWords []string
	add := &cobra.Command{
		Use:   "add <product> [--link <role>=<app>] [--variant v] [--set NAME=value]",
		Short: "Add a copy of a product, linked to the business's own apps",
		Long: `Adds a copy of the product to the business and deploys its newest release. --link names, for
each role the product lists, one of the business's apps by slug or id; --variant picks the form
the product comes in when it has several, fixed for the copy's life; --set gives a setting the
product, or the chosen variant, lists a value. whisk managed list shows each product's roles, variants and settings.

A product listed as your own code takes only its name, and the app it makes is the business's to
clone, change and deploy.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			links, err := parseLinks(linkWords)
			if err != nil {
				return err
			}
			settings, err := parseSettings(setWords, false)
			if err != nil {
				return err
			}
			client, org, err := s.orgClient()
			if err != nil {
				return err
			}
			ids := map[string]string{}
			var apps []api.App
			if len(links) > 0 {
				if apps, err = client.ListApps(s.ctx, org); err != nil {
					return wrap(err)
				}
				if ids, err = resolveLinks(org, links, apps); err != nil {
					return err
				}
			}
			req := api.ManagedAddRequest{Product: args[0], Variant: variant, Links: ids}
			if len(settings) > 0 {
				req.Settings = settings
			}
			a, err := client.AddManaged(s.ctx, org, req)
			if err != nil {
				return wrap(err)
			}
			linked := ""
			if len(ids) > 0 {
				linked = linksText(ids, apps)
			}
			s.printer.Result(map[string]any{"org": org, "app": a}, func(w io.Writer) {
				for _, line := range addedLines(org, args[0], a, variant, linked) {
					fmt.Fprintln(w, line)
				}
			})
			return nil
		},
	}
	add.Flags().StringArrayVar(&linkWords, "link", nil, "role=app: link one of the business's apps, by slug or id, under a role the product lists")
	add.Flags().StringVar(&variant, "variant", "", "the form the product comes in, when it has several")
	add.Flags().StringArrayVar(&setWords, "set", nil, "NAME=value: a setting the product or the chosen variant lists")

	set := &cobra.Command{
		Use:   "set <app> NAME=value...",
		Short: "Change a copy's settings (NAME= removes one); it restarts on its release",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			changes, err := parseSettings(args[1:], true)
			if err != nil {
				return err
			}
			s.appFlag = args[0]
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			a, err := client.SetManagedSettings(s.ctx, org, app, changes)
			if err != nil {
				return wrap(err)
			}
			var setNames, removed []string
			for _, n := range slices.Sorted(maps.Keys(changes)) {
				if changes[n] == "" {
					removed = append(removed, n)
				} else {
					setNames = append(setNames, n)
				}
			}
			s.printer.Result(map[string]any{"org": org, "app": a, "set": orEmpty(setNames), "removed": orEmpty(removed)}, func(w io.Writer) {
				var parts []string
				if len(setNames) > 0 {
					parts = append(parts, "set "+strings.Join(setNames, ", "))
				}
				if len(removed) > 0 {
					parts = append(parts, "removed "+strings.Join(removed, ", "))
				}
				fmt.Fprintf(w, "Changed %s/%s: %s. It restarts on its current release to read them.\n", org, a.Slug, strings.Join(parts, "; "))
			})
			return nil
		},
	}
	link := &cobra.Command{
		Use:   "link <copy> <role>=<app>",
		Short: "Link a copy to one of the business's apps under a role; both restart to read it",
		Long: `Links the copy, under a role its product lists, to one of the business's apps by slug or id,
replacing the app linked under that role before. A copy left without a link it needs is paused
until it is linked again. The copy restarts to read the link, and the linked app does too when it
is running; a sleeping one reads it at its next start.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			links, err := parseLinks(args[1:])
			if err != nil {
				return err
			}
			role := slices.Collect(maps.Keys(links))[0]
			s.appFlag = args[0]
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			apps, err := client.ListApps(s.ctx, org)
			if err != nil {
				return wrap(err)
			}
			ids, err := resolveLinks(org, links, apps)
			if err != nil {
				return err
			}
			a, err := client.LinkManaged(s.ctx, org, app, role, ids[role])
			if err != nil {
				return wrap(err)
			}
			linked := linksText(map[string]string{role: ids[role]}, apps)
			s.printer.Result(map[string]any{"org": org, "app": a, "role": role, "linked": ids[role]}, func(w io.Writer) {
				fmt.Fprintf(w, "Linked %s/%s: %s. It restarts to read the link, and the linked app does too when it is running.\n", org, a.Slug, linked)
			})
			return nil
		},
	}
	managed.AddCommand(list, add, set, link, managedUpdateCmd(s), managedMergeCmd(s))
	return managed
}

// orEmpty is the list, or an empty one in place of nil, so JSON carries [] rather than null.
func orEmpty(names []string) []string {
	if names == nil {
		return []string{}
	}
	return names
}

// pauseCmd and resumeCmd pause and resume a copy (MANAGED-APPS.md §4).
func pauseCmd(s *session) *cobra.Command  { return pausing(s, true) }
func resumeCmd(s *session) *cobra.Command { return pausing(s, false) }

func pausing(s *session, paused bool) *cobra.Command {
	use, short := "resume <app>", "Resume a paused managed app"
	if paused {
		use, short = "pause <app>", "Pause a managed app: it sleeps and does no work until resumed"
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s.appFlag = args[0]
			client, org, app, err := s.appClient()
			if err != nil {
				return err
			}
			a, err := client.SetPaused(s.ctx, org, app, paused)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": a, "paused": a.PausedAt != nil}, func(w io.Writer) {
				if paused {
					fmt.Fprintf(w, "Paused %s/%s. It sleeps, every run ends without work and its address answers the paused page. Resume it with whisk resume %s.\n", org, a.Slug, a.Slug)
				} else {
					fmt.Fprintf(w, "Resumed %s/%s. It wakes on its next request, schedule or delivery, and moves to the newest release if it was behind.\n", org, a.Slug)
				}
			})
			return nil
		},
	}
}

// managedRefusal is APP_MANAGED for a managed app's copy, whose code is Whisk's: the platform
// gives it no repository, so a clone or deploy says why rather than that none is ready. nil for
// any other app. Pure.
func managedRefusal(a api.App) error {
	if a.Managed == nil {
		return nil
	}
	return output.New("APP_MANAGED",
		fmt.Sprintf("%s is the %s, which Whisk runs for the business, so its code cannot be pushed, cloned or deployed.", a.Slug, a.Managed.Name),
		fmt.Sprintf("Change its settings with whisk managed set %s NAME=value, pause it with whisk pause %s, or delete it.", a.Slug, a.Slug),
		map[string]any{"app": a.Slug, "product": a.Managed.Product})
}

// managedList lists the products the business may add and the copies it has.
func (s *session) managedList() error {
	client, org, err := s.orgClient()
	if err != nil {
		return err
	}
	m, err := client.ListManaged(s.ctx, org)
	if err != nil {
		return wrap(err)
	}
	if m.Products == nil {
		m.Products = []api.ManagedProduct{}
	}
	if m.Copies == nil {
		m.Copies = []api.App{}
	}
	if m.Forks == nil {
		m.Forks = []api.App{}
	}
	var apps []api.App
	if !s.printer.JSON && slices.ContainsFunc(m.Copies, func(a api.App) bool { return a.Managed != nil && len(a.Managed.Links) > 0 }) {
		// The links name apps by id; the list puts their slugs in a person's table. Without
		// it the ids are shown, so a failure here does not fail the command.
		apps, _ = client.ListApps(s.ctx, org)
	}
	s.printer.Result(map[string]any{"org": org, "products": m.Products, "copies": m.Copies, "forks": m.Forks}, func(w io.Writer) { s.managedTables(w, m, apps) })
	return nil
}

// productsTable is the products as whisk managed list shows them. What a copy is has a column
// of its own only when a product whose copies are the business's own code is listed. Pure.
func productsTable(products []api.ManagedProduct) (header []string, rows [][]string) {
	kinds := forksOffered(products)
	header = []string{"PRODUCT", "NAME", "PRICE", "VARIANTS", "LINKS", "SETTINGS", "AVAILABLE"}
	if kinds {
		header = slices.Insert(header, 2, "KIND")
	}
	rows = make([][]string, len(products))
	for i, p := range products {
		rows[i] = []string{p.Slug, p.Name, monthText(p.MonthCents), variantsText(p), listText(p.Links), settingsText(p), unavailableText(p)}
		if kinds {
			rows[i] = slices.Insert(rows[i], 2, kindText(p))
		}
	}
	return header, rows
}

// managedTables prints the products, the copies and the forks for a person. apps names the apps
// a copy's links name by id; without them the ids are shown.
func (s *session) managedTables(w io.Writer, m api.Managed, apps []api.App) {
	if len(m.Products) == 0 {
		fmt.Fprintln(w, "Whisk offers no managed apps to this business yet.")
	} else {
		header, rows := productsTable(m.Products)
		s.printer.Table(w, header, rows)
	}
	fmt.Fprintln(w)
	if len(m.Copies) == 0 && len(m.Forks) == 0 {
		fmt.Fprintln(w, "No copies yet.")
	}
	if len(m.Copies) > 0 {
		rows := make([][]string, len(m.Copies))
		for i, a := range m.Copies {
			product, variant, links := "-", "-", "-"
			if a.Managed != nil {
				product, variant, links = a.Managed.Product, orDash(a.Managed.Variant), linksText(a.Managed.Links, apps)
			}
			rows[i] = []string{a.Slug, product, variant, releaseText(a.Managed), appStatusText(a), links}
		}
		s.printer.Table(w, []string{"APP", "PRODUCT", "VARIANT", "RELEASE", "STATUS", "LINKS"}, rows)
	}
	if len(m.Forks) > 0 {
		if len(m.Copies) > 0 {
			fmt.Fprintln(w)
		}
		rows := make([][]string, len(m.Forks))
		for i, a := range m.Forks {
			rows[i] = forkRow(a)
		}
		s.printer.Table(w, []string{"APP", "PRODUCT", "RELEASE", "NEWEST", "CODE", "STATUS"}, rows)
	}
	for _, hint := range addHints(m.Products) {
		fmt.Fprintln(w, hint)
	}
}
