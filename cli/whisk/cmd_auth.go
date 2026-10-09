package whisk

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/contract/tokensig"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/config"
	"github.com/whisk-run/cli/internal/gitcmd"
	"github.com/whisk-run/cli/internal/output"
)

func loginCmd(s *session) *cobra.Command {
	var noWait, wait bool
	var resume, agent string
	c := &cobra.Command{
		Use:   "login",
		Short: "Sign in with a device code; the human approves in the browser",
		Long: `Requests a device code and prints the URL and the code for the human to approve.

At a terminal it waits for the approval. Anywhere else (an agent's shell, whisk mcp) it does not
wait: it exits 2 with a NEEDS_HUMAN error carrying url, user_code and device_code, so the agent
can show them to the human; once they have approved, whisk login --resume <device_code> on the
same computer collects the token. --resume waits until the approval arrives or the code
expires (10 minutes); if it is interrupted, run it again. --wait waits even off a terminal, and
--no-wait never waits. While waiting, the URL and code are always on stderr, also with --json.

--agent names the coding agent running the login ("Claude Code", "Codex"), so the approval page,
deploys and the token list say who acted; WHISK_AGENT stands in when it is not given. The person
approving can change the name.

The token is bound to this computer: login makes a key pair, sends the public key with the
code request, and keeps the private key with the token. Every request is signed with it, so a
copied token is useless on its own.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client := s.anonymousClient()
			var dc api.DeviceCode
			var privateKey string
			var deadline time.Time
			if resume != "" {
				pending, ok, err := s.store.Get(config.PendingProfile(s.profile))
				if err != nil {
					return err
				}
				if !ok || pending.PrivateKey == "" || pending.PendingCode != codeHash(resume) {
					return output.New("INVALID_REQUEST", "This computer holds no key for that device code, so its token could not be used here.",
						"Run whisk login on the computer that will use the token, and whisk login --resume <device_code> on that same computer.", map[string]any{"profile": s.profile})
				}
				privateKey = pending.PrivateKey
				dc = api.DeviceCode{DeviceCode: resume, Interval: 2}
				deadline = resumeDeadline(pending.ExpiresAt, time.Now())
				s.waiting("Waiting for the human to approve this login (the code expires at %s). If this is interrupted, run whisk login --resume %s again.",
					deadline.Local().Format("15:04"), resume)
			} else {
				publicKey, priv, err := tokensig.NewKey()
				if err != nil {
					return fmt.Errorf("making this computer's key: %v", err)
				}
				privateKey = priv
				if dc, err = client.RequestDeviceCode(s.ctx, api.DeviceCodeRequest{PublicKey: publicKey, Agent: agentName(agent, s.env.Getenv), DeviceName: deviceName(), OS: runtime.GOOS + "/" + runtime.GOARCH}); err != nil {
					return wrap(err)
				}
				deadline = time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)
				// The key waits beside the profile's credential until the token is collected, so a
				// login that is stopped while it waits can still be finished with --resume.
				if err := s.store.Set(config.PendingProfile(s.profile), config.Credential{API: s.cfg.API, PrivateKey: privateKey, PendingCode: codeHash(dc.DeviceCode), ExpiresAt: deadline}); err != nil {
					return fmt.Errorf("keeping this computer's key: %v", err)
				}
				openBrowser(dc.VerificationURL)
				if !loginWaits(s.env.IsTerminal, noWait, wait) {
					return &output.Error{Code: "NEEDS_HUMAN",
						Message: fmt.Sprintf("Approve this login: open %s and enter the code %s.", dc.VerificationURL, dc.UserCode),
						Fix:     "Show the human the URL and the code, then run whisk login --resume " + dc.DeviceCode + " once they have approved; it waits for the approval.",
						Docs:    "https://skill.whisk.run/errors/NEEDS_HUMAN",
						Details: map[string]any{"url": dc.VerificationURL, "user_code": dc.UserCode, "device_code": dc.DeviceCode, "expires_in": dc.ExpiresIn,
							"resume": "whisk login --resume " + dc.DeviceCode}}
				}
				s.waiting("Open %s and enter the code %s. Waiting for approval; if this is interrupted, run whisk login --resume %s.", dc.VerificationURL, dc.UserCode, dc.DeviceCode)
			}
			interval := time.Duration(dc.Interval) * time.Second
			for {
				tok, err := client.PollDeviceToken(s.ctx, dc.DeviceCode)
				if err != nil {
					return wrap(err)
				}
				if tok.Status == "approved" {
					if tok.Token == "" {
						return output.New("TOKEN_EXPIRED", "The token for this device code was already collected.", "Run whisk login again.", nil)
					}
					// An approved answer carries all three; zero values stand in if one is missing.
					org, user, expires := valueOf(tok.Org), valueOf(tok.User), valueOf(tok.ExpiresAt)
					cred := config.Credential{Token: tok.Token, API: s.cfg.API, Org: org.Slug, OrgID: org.ID, User: user.Email, Scopes: tok.Scopes, ExpiresAt: expires, PrivateKey: privateKey}
					if err := s.store.Set(s.profile, cred); err != nil {
						return fmt.Errorf("storing the token: %v", err)
					}
					_ = s.store.Delete(config.PendingProfile(s.profile))
					// A login for the whole account comes back with no org: it acts as the person
					// in every org they belong to.
					allOrgs := tok.Org == nil
					result := map[string]any{"user": user, "all_orgs": allOrgs, "scopes": tok.Scopes, "expires_at": expires, "profile": s.profile, "stored_in": s.store.Where(), "bound": true, "device_name": deviceName()}
					if !allOrgs {
						result["org"] = org
					}
					s.printer.Result(result, func(w io.Writer) {
						what := "org " + org.Slug
						if allOrgs {
							what = "all your orgs"
						}
						fmt.Fprintf(w, "Signed in as %s (%s), profile %s, token stored in %s and bound to this computer.\n", user.Email, what, s.profile, s.store.Where())
					})
					return nil
				}
				if time.Now().After(deadline) {
					_ = s.store.Delete(config.PendingProfile(s.profile))
					return output.New("TOKEN_EXPIRED", "The device code expired before it was approved.", "Run whisk login again for a new code.", nil)
				}
				select {
				case <-s.ctx.Done():
					return s.ctx.Err()
				case <-time.After(interval):
				}
			}
		},
	}
	c.Flags().BoolVar(&noWait, "no-wait", false, "print the URL and code, exit 2, do not poll (the default off a terminal)")
	c.Flags().BoolVar(&wait, "wait", false, "wait for the approval even when not at a terminal")
	c.Flags().StringVar(&resume, "resume", "", "wait for an existing device code's approval and collect its token")
	c.Flags().StringVar(&agent, "agent", "", `the coding agent's own name, such as "Claude Code" (else WHISK_AGENT)`)
	return c
}

// loginWaits decides whether a fresh login waits for the approval: at a terminal a person is
// there to approve, anywhere else (an agent's shell, whisk mcp) the code goes back at once as
// NEEDS_HUMAN, so a shell timeout never swallows it. --no-wait and --wait override. Pure.
func loginWaits(terminal, noWait, wait bool) bool {
	switch {
	case noWait:
		return false
	case wait:
		return true
	}
	return terminal
}

// resumeDeadline is when a resumed code stops being worth polling: the expiry its login kept,
// or the platform's ten minutes from now for a code kept before expiries were. Pure.
func resumeDeadline(kept, now time.Time) time.Time {
	if kept.IsZero() {
		return now.Add(10 * time.Minute)
	}
	return kept
}

// codeHash is how a waiting login remembers its device code: never the code itself.
func codeHash(deviceCode string) string {
	sum := sha256.Sum256([]byte(deviceCode))
	return hex.EncodeToString(sum[:])
}

// agentName is the name the coding agent goes by on the approval page (pure over getenv):
// --agent, else WHISK_AGENT, else Claude Code when its shell says so (CLAUDECODE=1), else "".
func agentName(flag string, getenv func(string) string) string {
	if name := strings.TrimSpace(flag); name != "" {
		return name
	}
	if name := strings.TrimSpace(getenv("WHISK_AGENT")); name != "" {
		return name
	}
	if getenv("CLAUDECODE") == "1" {
		return "Claude Code"
	}
	return ""
}

// deviceName is this computer's name for the approval page; "" when the system has none.
func deviceName() string {
	name, err := os.Hostname()
	if err != nil {
		return ""
	}
	return name
}

func logoutCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke this profile's token and forget it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cred, ok, err := s.store.Get(s.profile)
			if err != nil {
				return err
			}
			if !ok {
				s.printer.Result(map[string]any{"revoked": false, "profile": s.profile}, func(w io.Writer) {
					fmt.Fprintf(w, "No token stored for profile %s.\n", s.profile)
				})
				return nil
			}
			revoked := true
			client := s.newClient(cred.API, cred.Token)
			if key, err := tokensig.ParsePrivate(cred.PrivateKey); err == nil {
				client.Key = key
			}
			if err := client.RevokeSelf(s.ctx); err != nil {
				revoked = false
				s.printer.Progress("The platform could not revoke the token (%v); it is forgotten locally and expires on its own.", err)
			}
			if err := s.store.Delete(s.profile); err != nil {
				return err
			}
			s.printer.Result(map[string]any{"revoked": revoked, "profile": s.profile}, func(w io.Writer) {
				fmt.Fprintf(w, "Logged out of profile %s.\n", s.profile)
			})
			return nil
		},
	}
}

func whoamiCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the user, orgs, scopes and expiry of the token in use",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, cred, err := s.client()
			if err != nil {
				return err
			}
			me, err := client.Whoami(s.ctx)
			if err != nil {
				return wrap(err)
			}
			source := s.store.Where()
			if s.env.Getenv("WHISK_TOKEN") != "" {
				source = "WHISK_TOKEN"
			}
			// Bound to this device: the token is bound and this computer holds its key (a request
			// with a bound token and no key would not have been answered at all).
			token := valueOf(me.Token)
			bound := token.Bound && client.Key != nil
			out := map[string]any{"user": me.User, "orgs": me.Orgs, "token": token, "profile": s.profile, "api": client.Base, "token_source": source, "bound_to_this_device": bound}
			if me.Identity != nil {
				out["identity"] = me.Identity
			}
			s.printer.Result(out, func(w io.Writer) {
				fmt.Fprintf(w, "%s (%s)\n", me.User.Email, me.User.Name)
				for _, o := range me.Orgs {
					fmt.Fprintf(w, "  org %s (%s, %s plan, %s)\n", o.Slug, o.Role, o.Plan, o.Status)
				}
				if me.Identity != nil {
					fmt.Fprintf(w, "  agent identity %s, acting for %s: %s\n", me.Identity.Name, me.User.Email, strings.Join(me.Identity.Categories, ", "))
				}
				fmt.Fprintf(w, "  token %s scopes %s, expires %s\n", token.Kind, strings.Join(token.Scopes, ","), token.ExpiresAt.Local().Format("2006-01-02 15:04"))
				if bound {
					fmt.Fprintf(w, "  bound to this device (%s): every request is signed with its key\n", orDash(token.DeviceName))
				} else {
					fmt.Fprintf(w, "  not bound to a device: the token works wherever it is copied\n")
				}
				fmt.Fprintf(w, "  profile %s from %s, api %s\n", s.profile, source, client.Base)
				_ = cred
			})
			return nil
		},
	}
}

func orgsCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "orgs",
		Short: "List the orgs the token can see",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, err := s.client()
			if err != nil {
				return err
			}
			me, err := client.Whoami(s.ctx)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"orgs": me.Orgs}, func(w io.Writer) {
				rows := make([][]string, len(me.Orgs))
				for i, o := range me.Orgs {
					rows[i] = []string{o.Slug, o.Name, string(o.Role), o.Plan, string(o.Status)}
				}
				s.printer.Table(w, []string{"ORG", "NAME", "ROLE", "PLAN", "STATUS"}, rows)
			})
			return nil
		},
	}
}

func useCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "use <org>/<app>",
		Short: "Bind the current directory to an app (.whisk/app.json)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := config.ParseRef(args[0])
			if err != nil {
				return output.New("INVALID_REQUEST", err.Error(), "Pass <org>/<app>, for example acme/crm.", nil)
			}
			verified, gitURL := false, ""
			if client, _, err := s.client(); err == nil {
				if a, err := client.GetApp(s.ctx, org, app); err != nil {
					if api.IsCode(err, "NOT_FOUND") {
						return wrap(err)
					}
					s.printer.Progress("Could not confirm the app with the platform (%v); binding anyway.", err)
				} else {
					verified, gitURL = true, a.GitURL
				}
			}
			b := config.Binding{Org: org, App: app, API: s.cfg.API}
			if err := config.SaveBinding(s.env.Dir, b); err != nil {
				return err
			}
			repointed := verified && s.repointRemote(gitURL)
			s.printer.Result(map[string]any{"org": org, "app": app, "api": s.cfg.API, "verified": verified, "file": config.BindingPath, "remote_updated": repointed}, func(w io.Writer) {
				fmt.Fprintf(w, "Bound this directory to %s/%s in %s.\n", org, app, config.BindingPath)
				if repointed {
					fmt.Fprintf(w, "Pointed the git remote whisk at %s/%s's repository.\n", org, app)
				}
			})
			return nil
		},
	}
}

// repointRemote points an existing remote whisk at gitURL when it names another repository,
// so whisk deploy accepts the binding whisk use just confirmed. A directory that is not a
// repository, or has no remote whisk yet, is left alone: the first deploy adds the remote.
// It reports whether it changed the remote.
func (s *session) repointRemote(gitURL string) bool {
	if gitURL == "" {
		return false
	}
	cur, err := s.git.Run(s.ctx, s.env.Dir, nil, "remote", "get-url", "whisk")
	if err != nil || !cur.OK() || !gitcmd.RemoteDisagrees(cur.Out(), gitURL) {
		return false
	}
	res, err := s.git.Run(s.ctx, s.env.Dir, nil, "remote", "set-url", "whisk", gitURL)
	if err != nil || !res.OK() {
		s.printer.Progress("Could not point the git remote whisk at %s; whisk deploy will refuse until it does (git remote set-url whisk %s).", gitURL, gitURL)
		return false
	}
	return true
}

// waiting tells whoever runs the CLI what it is waiting for, on stderr, also with --json (whose
// stdout carries one object at the end); only --quiet silences it.
func (s *session) waiting(format string, args ...any) {
	if s.quiet {
		return
	}
	fmt.Fprintf(s.env.Stderr, format+"\n", args...)
}

// valueOf is what p points at, or T's zero value when the API left it out.
func valueOf[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
