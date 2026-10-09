package whisk

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/config"
	"github.com/whisk-run/cli/internal/gitcmd"
	"github.com/whisk-run/cli/internal/output"
)

func cloneCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:   "clone <org>/<app> [directory]",
		Short: "Copy an app's code from Whisk into a new directory, ready for git pull and whisk deploy",
		Long: `Clones the app's repository into the directory (default: the app's slug) with the remote named
"whisk", binds the directory to the app, and makes plain git pull, fetch and push work there by
pointing git's credential helper for the git host at whisk git-credential. The token stays in the
credential store; nothing secret is written to .git/config.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := config.ParseRef(args[0])
			if err != nil {
				return output.New("INVALID_REQUEST", err.Error(), "Pass <org>/<app>, for example acme/crm.", nil)
			}
			dir := app
			if len(args) == 2 {
				dir = args[1]
			}
			return runClone(s, org, app, dir)
		},
	}
}

func runClone(s *session, org, app, dir string) error {
	client, cred, err := s.client()
	if err != nil {
		return err
	}
	a, err := client.GetApp(s.ctx, org, app)
	if err != nil {
		return wrap(err)
	}
	if a.GitURL == "" {
		return output.New("PLATFORM_UNAVAILABLE", fmt.Sprintf("The platform returned no repository URL for %s/%s.", org, app), "The app's repository is not ready yet; retry in a moment. whisk apps info shows git_url once it exists.", nil)
	}
	target := dir
	if !filepath.IsAbs(target) {
		target = filepath.Join(s.env.Dir, target)
	}
	if !emptyOrMissing(target) {
		return output.New("INVALID_REQUEST", fmt.Sprintf("%s already exists and is not empty.", dir), "Pass a directory that does not exist yet, for example whisk clone "+org+"/"+app+" "+app+"-2.", map[string]any{"dir": dir})
	}
	exe, err := s.executable()
	if err != nil {
		return err
	}
	s.printer.Progress("Cloning %s/%s into %s.", org, app, dir)
	secret, err := s.gitSecret(client, cred)
	if err != nil {
		return err
	}
	env, cargs, err := gitcmd.Credential(secret, a.GitURL)
	if err != nil {
		return output.New("PLATFORM_UNAVAILABLE", err.Error()+".", "Check WHISK_API points at the Whisk platform you meant, then retry.", map[string]any{"git_url": a.GitURL})
	}
	args := gitcmd.Networked(cargs, "clone", "--quiet", "--origin", "whisk", "--", a.GitURL, target)
	res, err := s.gitRun(env, args...)
	if err != nil {
		return err
	}
	schannel := false
	if !res.OK() && s.goos() == "windows" && gitcmd.Failure(res.Stderr, gitcmd.PushUnknown) == "tls" {
		s.printer.Progress("git's OpenSSL does not trust %s; retrying with the Windows certificate store.", gitHost(a.GitURL))
		_ = os.RemoveAll(target)
		res, err = s.gitRun(env, append(append([]string{}, gitcmd.SchannelArgs...), args...)...)
		if err != nil {
			return err
		}
		schannel = true
	}
	if !res.OK() {
		return cloneFailed(res, gitHost(a.GitURL), s.goos(), schannel)
	}
	for _, c := range cloneConfig(gitOrigin(a.GitURL), exe, schannel) {
		r, err := s.git.Run(s.ctx, target, nil, append([]string{"config", "--local"}, c...)...)
		if err != nil {
			return fmt.Errorf("git config: %v", err)
		}
		if !r.OK() {
			return gitFailed("git config", r)
		}
	}
	if _, ok, _ := config.LoadBinding(target); !ok {
		if err := config.SaveBinding(target, config.Binding{Org: org, App: app, API: s.cfg.API}); err != nil {
			return err
		}
	}
	sha := ""
	if r, err := s.git.Run(s.ctx, target, nil, "rev-parse", "--verify", "--quiet", "HEAD"); err == nil && r.OK() {
		sha = r.Out()
	}
	s.printer.Result(map[string]any{"org": org, "app": app, "dir": target, "git_url": a.GitURL, "commit_sha": sha}, func(w io.Writer) {
		if sha == "" {
			fmt.Fprintf(w, "Cloned %s/%s into %s. The app has no code yet; add some and run whisk deploy.\n", org, app, dir)
			return
		}
		fmt.Fprintf(w, "Cloned %s/%s into %s at %s. git pull and git push work there; whisk deploy puts changes live.\n", org, app, dir, short(sha))
	})
	return nil
}

// emptyOrMissing reports whether dir does not exist or is an empty directory.
func emptyOrMissing(dir string) bool {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return true
	}
	return err == nil && len(entries) == 0
}

// gitOrigin is the scheme and host of a git URL, the scope git matches credential settings on.
func gitOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return u.Scheme + "://" + u.Host
}

// cloneConfig is the repository configuration a clone gets, as git config arguments in order:
// the credential helpers for the git host are reset (so a stored password from elsewhere is
// never offered) and then whisk git-credential is the one helper, by the running binary's
// path; with schannel, git verifies TLS with the Windows certificate store from then on.
func cloneConfig(origin, exe string, schannel bool) [][]string {
	key := "credential." + origin + ".helper"
	helper := `!"` + filepath.ToSlash(exe) + `" git-credential`
	out := [][]string{{"--add", key, ""}, {"--add", key, helper}}
	if schannel {
		out = append(out, []string{"http.sslBackend", "schannel"})
	}
	return out
}

// cloneFailed turns a failed git clone into the error an agent can act on.
func cloneFailed(res gitcmd.Result, host, goos string, triedSchannel bool) error {
	tail := gitcmd.LastLines(res.Stderr, 10)
	details := map[string]any{"git": tail}
	switch gitcmd.Failure(res.Stderr, gitcmd.PushUnknown) {
	case "tls":
		e := tlsUntrusted(host, goos, triedSchannel, details)
		e.Fix = strings.ReplaceAll(e.Fix, "whisk deploy again", "whisk clone again")
		return e
	case "network":
		return output.New("PLATFORM_UNAVAILABLE", "git clone could not reach the platform: "+lastLine(tail), "Retry with backoff. Check WHISK_API if you meant another endpoint.", details)
	case "auth":
		return output.New("AUTH_REQUIRED", "The platform refused the token for git clone.", "Run whisk login again and approve the code for this app (or the whole org), then retry.", details)
	}
	return fmt.Errorf("git clone failed (exit %d): %s", res.ExitCode, lastLine(tail))
}

func gitCredentialCmd(s *session) *cobra.Command {
	return &cobra.Command{
		Use:    "git-credential <get|store|erase>",
		Short:  "Answer git's credential requests for the Whisk git host with the stored token",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "get" {
				// The token lives in whisk's own store; git has nothing to store or erase.
				_, _ = io.Copy(io.Discard, s.env.Stdin)
				return nil
			}
			req := parseCredentialRequest(s.env.Stdin)
			if p := req["protocol"]; p != "" && p != "https" {
				// Only https is answered, so nothing secret travels in clear.
				return nil
			}
			client, cred, err := s.client()
			if err != nil {
				// Saying nothing lets git fall through to its own failure, which names the host.
				return nil
			}
			secret, err := s.gitSecret(client, cred)
			if err != nil {
				fmt.Fprintf(s.env.Stderr, "whisk: %v\n", err)
				return nil
			}
			_, err = io.WriteString(s.env.Stdout, credentialAnswer(req, secret))
			return err
		},
	}
}

// gitSecret is what git is given as the password (CONTROL-PLANE.md §6.3): the token itself when
// it is a plain bearer (WHISK_TOKEN, a token from before device keys), or, for a token bound to
// this computer, a ten-minute git password traded for it through a signed request, because git
// cannot sign and the git host refuses a bound token on its own.
func (s *session) gitSecret(client *api.Client, cred config.Credential) (string, error) {
	if client.Key == nil {
		return cred.Token, nil
	}
	pw, err := client.GitPassword(s.ctx)
	if err != nil {
		return "", wrap(err)
	}
	return pw.Password, nil
}

// parseCredentialRequest reads git's key=value lines up to a blank line or the end.
func parseCredentialRequest(r io.Reader) map[string]string {
	req := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" {
			break
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			req[k] = v
		}
	}
	return req
}

// credentialAnswer is what whisk git-credential get prints: the username the git host expects
// and the git password (gitSecret). Only https is answered, so it never travels in clear.
func credentialAnswer(req map[string]string, token string) string {
	if p := req["protocol"]; p != "" && p != "https" {
		return ""
	}
	return "username=whisk\npassword=" + token + "\n"
}
