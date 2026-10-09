package whisk

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
)

// githubStatusText is a link's status in plain words.
func githubStatusText(l api.GitHubLink) string {
	if l.Status == "broken" {
		if l.Problem == "" {
			return "broken"
		}
		return "broken: " + l.Problem
	}
	return "copying both ways"
}

// githubBranchText is a branch's state in plain words.
func githubBranchText(state string) string {
	switch state {
	case "in_sync":
		return "in step"
	case "waiting":
		return "catching up"
	case "refused":
		return "refused"
	}
	return orDash(state)
}

// GitHub's own rules for the two words: an account is letters, digits and hyphens, a
// repository letters, digits, '.', '_' and '-'.
var (
	reGitHubOwner = regexp.MustCompile(`^[A-Za-z0-9-]{1,39}$`)
	reGitHubName  = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
)

// normalizeRepo accepts owner/name, a GitHub URL or a clone URL, and answers owner/name.
func normalizeRepo(v string) (string, bool) {
	r := strings.TrimSpace(v)
	r = strings.TrimSuffix(strings.TrimSuffix(r, "/"), ".git")
	for _, prefix := range []string{"https://github.com/", "http://github.com/", "git@github.com:", "github.com/"} {
		r = strings.TrimPrefix(r, prefix)
	}
	owner, name, ok := strings.Cut(r, "/")
	if !ok || !reGitHubOwner.MatchString(owner) || !reGitHubName.MatchString(name) || name == "." || name == ".." {
		return "", false
	}
	return owner + "/" + name, true
}

// isNotFound reports whether the platform answered NOT_FOUND.
func isNotFound(err error) bool {
	var e *api.Error
	return errors.As(err, &e) && e.Code == "NOT_FOUND"
}

// printGitHubLink is the human view of a link: where it is, whether it works, each branch.
func printGitHubLink(p output.Printer, w io.Writer, app string, l api.GitHubLink) {
	fmt.Fprintf(w, "%s is copied to GitHub at %s (%s).\n", app, l.Repo, l.HTMLURL)
	fmt.Fprintf(w, "Status: %s\n", githubStatusText(l))
	fmt.Fprintf(w, "Last in step: %s\n", when(l.SyncedAt))
	if l.Status == "broken" {
		fmt.Fprintln(w, "Nothing is copied while the link is broken. A person mends it by linking the repository again.")
	}
	if len(l.Branches) == 0 {
		fmt.Fprintln(w, "No branches have been compared yet.")
		return
	}
	rows := make([][]string, len(l.Branches))
	refused := false
	for i, b := range l.Branches {
		rows[i] = []string{b.Name, orDash(short(b.Whisk)), orDash(short(b.GitHub)), githubBranchText(string(b.State))}
		refused = refused || b.State == "refused"
	}
	p.Table(w, []string{"BRANCH", "WHISK", "GITHUB", "STATE"}, rows)
	if refused {
		fmt.Fprintln(w, "A refused commit was not taken into Whisk; its commit status on GitHub says why. The next commit on that branch is tried.")
	}
}

// githubCmd is the app's optional copy on GitHub (CLI.md §5.7, CONTROL-PLANE.md §6.26).
func githubCmd(s *session) *cobra.Command {
	statusRun := func(cmd *cobra.Command, args []string) error {
		org, app, err := s.target()
		if err != nil {
			return err
		}
		client, _, err := s.client()
		if err != nil {
			return err
		}
		link, err := client.GitHubLink(s.ctx, org, app)
		if isNotFound(err) {
			settings := appPage(s.dashboard(), org, app, "/settings")
			s.printer.Result(map[string]any{"org": org, "app": app, "linked": false, "settings_url": settings}, func(w io.Writer) {
				fmt.Fprintf(w, "%s is not linked to GitHub. Its code lives only in Whisk.\n", app)
				fmt.Fprintf(w, "A person links it under GitHub at %s, or runs whisk github link owner/name while signed in as themselves.\n", settings)
			})
			return nil
		}
		if err != nil {
			return wrap(err)
		}
		s.printer.Result(map[string]any{"org": org, "app": app, "linked": true, "github": link}, func(w io.Writer) {
			printGitHubLink(s.printer, w, app, link)
		})
		return nil
	}

	github := &cobra.Command{
		Use:   "github",
		Short: "The app's optional two-way copy on GitHub",
		Long: `An app may keep a two-way copy of its code in a GitHub repository the business owns.
Whisk's repository stays the source: deploys are built from it, and changes made on either side
reach the other. Installing Whisk on GitHub and linking a repository are done by a person,
because a link lets whoever can push to that repository deploy the app.

With no subcommand, whisk github prints the link's status.`,
		Args: cobra.NoArgs,
		RunE: statusRun,
	}

	status := &cobra.Command{
		Use:   "status",
		Short: "The link, whether it works, and each branch's state",
		Args:  cobra.NoArgs,
		RunE:  statusRun,
	}

	repos := &cobra.Command{
		Use:   "repos",
		Short: "GitHub accounts Whisk is installed on and the repositories it reaches",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, err := s.org()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			ov, err := client.GitHubOverview(s.ctx, org)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "set_up": ov.SetUp, "installations": ov.Installations}, func(w io.Writer) {
				if !ov.SetUp {
					fmt.Fprintln(w, "Copying to GitHub is not available on this platform yet.")
					return
				}
				if len(ov.Installations) == 0 {
					fmt.Fprintf(w, "Whisk is not installed on any GitHub account for %s. A person runs whisk github install, or installs it from the org's settings in the dashboard.\n", org)
					return
				}
				for i, in := range ov.Installations {
					if i > 0 {
						fmt.Fprintln(w)
					}
					fmt.Fprintf(w, "GitHub account %s (%s)\n", in.Account, strings.ToLower(orDash(in.AccountType)))
					if in.Problem != "" {
						fmt.Fprintf(w, "Problem: %s\n", in.Problem)
					}
					if len(in.Repositories) == 0 {
						fmt.Fprintln(w, "Whisk reaches no repositories on this account yet.")
					} else {
						rows := make([][]string, len(in.Repositories))
						for j, r := range in.Repositories {
							visibility := "public"
							if r.Private {
								visibility = "private"
							}
							rows[j] = []string{r.FullName, visibility, orDash(r.LinkedApp)}
						}
						s.printer.Table(w, []string{"REPOSITORY", "VISIBILITY", "LINKED TO"}, rows)
					}
					if in.SettingsURL != "" {
						fmt.Fprintf(w, "Choose which repositories Whisk reaches at %s\n", in.SettingsURL)
					}
				}
			})
			return nil
		},
	}

	link := &cobra.Command{
		Use:   "link <owner/name>",
		Short: "Link the app to a GitHub repository (a person, signed in)",
		Long: `Links the app to a repository an installation of Whisk reaches. The repository must be
empty or hold this app's history. Only a person signed in as themselves can link, because a
link lets whoever can push to the repository deploy the app; an agent gets NEEDS_HUMAN with the
page where a person does it.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, ok := normalizeRepo(args[0])
			if !ok {
				return output.New("INVALID_REQUEST", fmt.Sprintf("%q is not a GitHub repository.", args[0]),
					"Pass it as owner/name, for example acme/crm. whisk github repos lists the ones Whisk reaches.", map[string]any{"repo": args[0]})
			}
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			l, err := client.LinkGitHub(s.ctx, org, app, repo)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "linked": true, "github": l}, func(w io.Writer) {
				fmt.Fprintf(w, "Linked %s to %s. The first copy is under way; whisk github shows how it is going.\n", app, l.Repo)
				printGitHubLink(s.printer, w, app, l)
			})
			return nil
		},
	}

	sync := &cobra.Command{
		Use:   "sync",
		Short: "Compare the app with its GitHub copy now",
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
			if err := client.SyncGitHub(s.ctx, org, app); err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "queued": true}, func(w io.Writer) {
				fmt.Fprintln(w, "Queued a sync with GitHub. whisk github shows each branch once it has run.")
			})
			return nil
		},
	}

	unlink := &cobra.Command{
		Use:   "unlink",
		Short: "Stop copying the app to GitHub",
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
			if err := client.UnlinkGitHub(s.ctx, org, app); err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "unlinked": true}, func(w io.Writer) {
				fmt.Fprintf(w, "Unlinked %s from GitHub. Nothing changed on GitHub or in the app's code: the repository keeps what it has, and Whisk stops copying to it.\n", app)
			})
			return nil
		},
	}

	var noBrowser bool
	install := &cobra.Command{
		Use:   "install",
		Short: "Print the page where a person installs Whisk on their GitHub account",
		Long: `Answers the GitHub page where a person installs Whisk's GitHub App and chooses which
repositories it may reach. GitHub then returns them to the app's settings, or the org's when the
directory is not bound to an app. Only a person signed in as themselves can install; an agent
gets NEEDS_HUMAN with the page where a person does it.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			org, err := s.org()
			if err != nil {
				return err
			}
			app := s.appIfBound()
			client, _, err := s.client()
			if err != nil {
				return err
			}
			u, err := client.GitHubInstallURL(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			if !noBrowser && s.env.IsTerminal {
				openBrowser(u)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "install_url": u}, func(w io.Writer) {
				fmt.Fprintln(w, "Open this page to install Whisk on GitHub and choose the repositories it may reach:")
				fmt.Fprintln(w, u)
			})
			return nil
		},
	}
	install.Flags().BoolVar(&noBrowser, "no-browser", false, "print the URL only")

	github.AddCommand(status, repos, link, sync, unlink, install)
	return github
}
