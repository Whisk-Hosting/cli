package whisk

import (
	"net/url"
	"strings"

	"github.com/whisk-run/cli/internal/gitcmd"
)

// setupGitHelper makes whisk git's one credential helper for the platform's git host, in the
// user's global git config, so every plain git command against that host (an agent's git push,
// an editor's background fetch, a repository whisk clone did not make) signs in with the stored
// Whisk login. Nothing else is asked, so Git Credential Manager never opens its window for the
// host. Only that host's helpers change; nothing secret is written. It runs for the released
// binary only (Env.SetupGit), does nothing when the config is already right, and never stops
// the command it runs in: a failure is said once.
func (s *session) setupGitHelper(gitURL string) {
	if !s.env.SetupGit || gitURL == "" {
		return
	}
	origin, err := gitcmd.CredentialOrigin(gitURL)
	if err != nil || !strings.HasPrefix(origin, "https://") {
		// whisk git-credential answers https only; a local stack's http host is left alone.
		return
	}
	exe, err := s.executable()
	if err != nil {
		return
	}
	key := gitcmd.HelperKey(origin)
	cur, err := s.git.Run(s.ctx, s.env.Dir, nil, "config", "--global", "--get-all", key)
	if err != nil || (!cur.OK() && cur.ExitCode != 1) {
		// No git, or a global config git cannot read: whisk deploy hands git the token itself.
		return
	}
	var current []string
	if cur.OK() {
		current = gitcmd.ConfigValues(cur.Stdout)
	}
	plan := gitcmd.HelperPlan(key, current, gitcmd.HelperValues(exe))
	for _, args := range plan {
		res, err := s.git.Run(s.ctx, s.env.Dir, nil, append([]string{"config", "--global"}, args...)...)
		if err != nil || !res.OK() {
			s.printer.Progress("Could not make whisk git's sign-in for %s in your global git config (%s), so plain git commands there may ask for a password; whisk deploy is not affected. Run: git config --global --add %s %q",
				origin, lastLine(gitcmd.LastLines(res.Stderr, 1)), key, gitcmd.HelperValues(exe)[1])
			return
		}
	}
	if len(plan) > 0 {
		s.printer.Progress("git now signs in to %s with your Whisk login, so git push and pull there never ask for a password.", origin)
	}
}

// gitURLOf is the address of the platform's git host for its API address: the API is served at
// api.<domain> and git at git.<domain>. Empty when the API address does not follow that. Pure.
func gitURLOf(api string) string {
	u, err := url.Parse(api)
	if err != nil || u.Scheme != "https" || u.Port() != "" {
		return ""
	}
	rest, ok := strings.CutPrefix(u.Hostname(), "api.")
	if !ok || rest == "" {
		return ""
	}
	return "https://git." + rest
}
