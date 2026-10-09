package whisk

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/output"
	"github.com/whisk-run/contract/apitypes"
)

func findMember(members []api.Member, email string) (api.Member, bool) {
	for _, m := range members {
		if strings.EqualFold(m.Email, email) && m.Status != "removed" {
			return m, true
		}
	}
	return api.Member{}, false
}

func findGroup(groups []api.Group, name string) (api.Group, bool) {
	for _, g := range groups {
		if strings.EqualFold(g.Name, name) || g.ID == name {
			return g, true
		}
	}
	return api.Group{}, false
}

func membersCmd(s *session) *cobra.Command {
	members := &cobra.Command{Use: "members", Short: "People in the org"}

	list := &cobra.Command{
		Use:   "list",
		Short: "List members and guests, invited ones included",
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
			all, err := client.ListMembers(s.ctx, org)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "members": all}, func(w io.Writer) {
				rows := make([][]string, len(all))
				for i, m := range all {
					rows[i] = []string{m.Email, orDash(m.Name), string(m.Role), orDash(string(m.Kind)), string(m.Status), fmt.Sprint(len(m.Groups))}
				}
				s.printer.Table(w, []string{"EMAIL", "NAME", "ROLE", "KIND", "STATUS", "GROUPS"}, rows)
			})
			return nil
		},
	}

	var role, name string
	var guest bool
	invite := &cobra.Command{
		Use:   "invite <email> --role developer [--guest]",
		Short: "Invite a person; prints the link they redeem",
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
			req := api.Invite{Email: strings.TrimSpace(args[0]), Name: name, Role: role}
			if guest {
				req.Kind = "guest"
			}
			m, err := client.InviteMember(s.ctx, org, req)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "member": m, "invite_url": m.InviteURL}, func(w io.Writer) {
				fmt.Fprintf(w, "Invited %s as %s%s.\n", m.Email, m.Role, kindSuffix(m.Kind))
				if m.InviteURL != "" {
					fmt.Fprintf(w, "Send them this link: %s\n", m.InviteURL)
				}
			})
			return nil
		},
	}
	invite.Flags().StringVar(&role, "role", "member", "owner, admin, developer, billing or member")
	invite.Flags().StringVar(&name, "name", "", "the person's name")
	invite.Flags().BoolVar(&guest, "guest", false, "invite as a guest (outside the company)")

	remove := &cobra.Command{
		Use:   "remove <email>",
		Short: "Remove a person from the org, every app and every group at once",
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
			all, err := client.ListMembers(s.ctx, org)
			if err != nil {
				return wrap(err)
			}
			m, ok := findMember(all, args[0])
			if !ok {
				return output.New("NOT_FOUND", fmt.Sprintf("%s is not a member of %s.", args[0], org), "whisk members list shows who is.", map[string]any{"email": args[0]})
			}
			if err := client.RemoveMember(s.ctx, org, m.ID); err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "email": m.Email, "removed": true}, func(w io.Writer) {
				fmt.Fprintf(w, "Removed %s from %s.\n", m.Email, org)
			})
			return nil
		},
	}
	members.AddCommand(list, invite, remove)
	return members
}

func kindSuffix(kind apitypes.MemberKind) string {
	if kind == apitypes.KindGuest {
		return " (guest)"
	}
	return ""
}

// subjectFlags name who a grant is for.
type subjectFlags struct {
	Group    string
	User     string
	Everyone bool
	Audience string
}

// resolveSubject turns the flags into a grant, looking names and emails up in the org.
func (s *session) resolveSubject(client *api.Client, org string, f subjectFlags) (api.Grant, error) {
	chosen := 0
	for _, on := range []bool{f.Group != "", f.User != "", f.Everyone} {
		if on {
			chosen++
		}
	}
	if chosen != 1 {
		return api.Grant{}, output.New("INVALID_REQUEST", "Name exactly one subject.", "Pass --group <name>, --user <email> or --everyone.", nil)
	}
	if f.Audience != "team" && f.Audience != "customer" {
		return api.Grant{}, output.New("INVALID_REQUEST", fmt.Sprintf("--audience %q is not an audience.", f.Audience), "Pass --audience team or --audience customer.", nil)
	}
	switch {
	case f.Everyone:
		return api.Grant{SubjectKind: apitypes.SubjectEveryone, Audience: apitypes.Audience(f.Audience), Label: "Everyone in the org"}, nil
	case f.Group != "":
		groups, err := client.ListGroups(s.ctx, org)
		if err != nil {
			return api.Grant{}, wrap(err)
		}
		g, ok := findGroup(groups, f.Group)
		if !ok {
			return api.Grant{}, output.New("NOT_FOUND", fmt.Sprintf("%s has no group named %s.", org, f.Group), "Create the group in the dashboard under People, or check the name.", map[string]any{"group": f.Group})
		}
		return api.Grant{SubjectKind: apitypes.SubjectGroup, SubjectID: g.ID, Audience: apitypes.Audience(f.Audience), Label: g.Name}, nil
	default:
		members, err := client.ListMembers(s.ctx, org)
		if err != nil {
			return api.Grant{}, wrap(err)
		}
		m, ok := findMember(members, f.User)
		if !ok {
			return api.Grant{}, output.New("NOT_FOUND", fmt.Sprintf("%s is not a member of %s.", f.User, org), "Invite them first with whisk members invite "+f.User+".", map[string]any{"email": f.User})
		}
		return api.Grant{SubjectKind: apitypes.SubjectUser, SubjectID: m.UserID, Audience: apitypes.Audience(f.Audience), Label: m.Email}, nil
	}
}

func sameSubject(a, b api.Grant) bool {
	return a.SubjectKind == b.SubjectKind && a.SubjectID == b.SubjectID
}

// withGrant adds a grant unless an identical one exists.
func withGrant(grants []api.Grant, g api.Grant) []api.Grant {
	for _, x := range grants {
		if sameSubject(x, g) && x.Audience == g.Audience {
			return grants
		}
	}
	return append(append([]api.Grant{}, grants...), g)
}

// withoutSubject removes every grant for the subject (any audience when audience is "").
func withoutSubject(grants []api.Grant, g api.Grant, audience string) (kept []api.Grant, removed int) {
	kept = []api.Grant{}
	for _, x := range grants {
		if sameSubject(x, g) && (audience == "" || x.Audience == apitypes.Audience(audience)) {
			removed++
			continue
		}
		kept = append(kept, x)
	}
	return kept, removed
}

func printAccess(p output.Printer, w io.Writer, a api.Access) {
	if len(a.Grants) == 0 {
		fmt.Fprintln(w, "Nobody can open this app. Grant access with whisk access grant --everyone, --group <name> or --user <email>.")
		return
	}
	rows := make([][]string, len(a.Grants))
	for i, g := range a.Grants {
		rows[i] = []string{string(g.SubjectKind), orDash(g.Label), string(g.Audience)}
	}
	p.Table(w, []string{"KIND", "WHO", "AUDIENCE"}, rows)
}

func accessCmd(s *session) *cobra.Command {
	access := &cobra.Command{Use: "access", Short: "Who can open the app"}

	show := &cobra.Command{
		Use:   "show",
		Short: "List the grants",
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
			a, err := client.GetAccess(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "access": a}, func(w io.Writer) { printAccess(s.printer, w, a) })
			return nil
		},
	}

	addSubjectFlags := func(c *cobra.Command, f *subjectFlags, withAudienceDefault bool) {
		c.Flags().StringVar(&f.Group, "group", "", "a group by name")
		c.Flags().StringVar(&f.User, "user", "", "a member by email")
		c.Flags().BoolVar(&f.Everyone, "everyone", false, "everyone in the org")
		def := ""
		if withAudienceDefault {
			def = "team"
		}
		c.Flags().StringVar(&f.Audience, "audience", def, "team or customer")
	}

	var grantFlags subjectFlags
	grant := &cobra.Command{
		Use:   "grant --group <name> | --user <email> | --everyone [--audience team|customer]",
		Short: "Let a group, a person or everyone open the app",
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
			g, err := s.resolveSubject(client, org, grantFlags)
			if err != nil {
				return err
			}
			cur, err := client.GetAccess(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			next := withGrant(cur.Grants, g)
			if len(next) == len(cur.Grants) {
				s.printer.Result(map[string]any{"org": org, "app": app, "access": cur, "changed": false}, func(w io.Writer) {
					fmt.Fprintf(w, "%s can already open %s as %s.\n", orDash(g.Label), app, g.Audience)
				})
				return nil
			}
			updated, err := client.SetAccess(s.ctx, org, app, next)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "access": updated, "changed": true, "granted": g}, func(w io.Writer) {
				fmt.Fprintf(w, "%s can now open %s as %s.\n", orDash(g.Label), app, g.Audience)
			})
			return nil
		},
	}
	addSubjectFlags(grant, &grantFlags, true)

	var revokeFlags subjectFlags
	revoke := &cobra.Command{
		Use:   "revoke --group <name> | --user <email> | --everyone [--audience team|customer]",
		Short: "Stop a group, a person or everyone opening the app",
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
			f := revokeFlags
			audience := f.Audience
			if audience == "" {
				f.Audience = "team"
			}
			g, err := s.resolveSubject(client, org, f)
			if err != nil {
				return err
			}
			cur, err := client.GetAccess(s.ctx, org, app)
			if err != nil {
				return wrap(err)
			}
			next, removed := withoutSubject(cur.Grants, g, audience)
			if removed == 0 {
				return output.New("NOT_FOUND", fmt.Sprintf("%s has no grant on %s.", orDash(g.Label), app), "whisk access show lists the grants.", map[string]any{"subject": g})
			}
			updated, err := client.SetAccess(s.ctx, org, app, next)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "access": updated, "changed": true, "revoked": g, "removed": removed}, func(w io.Writer) {
				fmt.Fprintf(w, "%s can no longer open %s.\n", orDash(g.Label), app)
			})
			return nil
		},
	}
	addSubjectFlags(revoke, &revokeFlags, false)

	access.AddCommand(show, grant, revoke)
	return access
}

func deployKeysCmd(s *session) *cobra.Command {
	keys := &cobra.Command{Use: "deploy-keys", Short: "Deploy tokens for git push from CI"}
	var label string
	create := &cobra.Command{
		Use:   "create [--label text]",
		Short: "Mint a deploy token; its value is shown once",
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
			k, err := client.CreateDeployKey(s.ctx, org, app, label)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "token": k.Token, "value": k.Value, "git_url": k.GitURL}, func(w io.Writer) {
				fmt.Fprintf(w, "Deploy key %s (%s), shown once:\n%s\nGit: %s\nUse it as the password with username whisk, or set WHISK_TOKEN in CI and run whisk deploy.\n", k.Token.ID, k.Token.Label, k.Value, k.GitURL)
			})
			return nil
		},
	}
	create.Flags().StringVar(&label, "label", "", "what the key is for, e.g. \"github actions\"")
	keys.AddCommand(create)
	return keys
}
