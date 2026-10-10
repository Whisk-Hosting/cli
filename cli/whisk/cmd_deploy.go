package whisk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/cli/doctor"
	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/gitcmd"
	"github.com/whisk-run/cli/internal/output"
	werrors "github.com/whisk-run/contract/errors"
	"github.com/whisk-run/contract/status"
)

// deployOptions are whisk deploy's flags. CommitSet says whether --commit was given at all,
// because its default depends on whether the directory is a repository yet.
type deployOptions struct {
	Env       string
	Branch    string
	NoWait    bool
	Force     bool
	Commit    bool
	CommitSet bool
	Message   string
}

func deployCmd(s *session) *cobra.Command {
	var opts deployOptions
	c := &cobra.Command{
		Use:   "deploy",
		Short: "Push the repository to Whisk and follow the deploy until it is live",
		Long: `Runs doctor (errors abort; --force skips it), makes sure the directory is a git repository with
everything committed (--commit commits what is not ignored as "whisk deploy"; it is the default for
a directory that is not a repository yet; -m "what changed" commits it with that message instead),
points the remote "whisk" at the app's repository, pushes
with the token as the credential (never written to .git/config), and streams the deploy's phases
until it is live or failed. With --json every event is one JSON line, then one summary object.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.CommitSet = cmd.Flags().Changed("commit")
			return runDeploy(s, opts)
		},
	}
	c.Flags().StringVar(&opts.Env, "env", "", "production or preview (default: production from main or master, preview from any other branch)")
	c.Flags().StringVar(&opts.Branch, "branch", "", "the local branch to push (default: the current one)")
	c.Flags().BoolVar(&opts.NoWait, "no-wait", false, "print the deploy id and exit once the push is accepted")
	c.Flags().BoolVar(&opts.Force, "force", false, "skip doctor")
	c.Flags().BoolVar(&opts.Commit, "commit", false, "commit everything not ignored as \"whisk deploy\" before pushing (default true when the directory is not a repository yet)")
	c.Flags().StringVarP(&opts.Message, "message", "m", "", "one line saying what changed, shown with the deploy; commits uncommitted changes with it (implies --commit)")
	return c
}

func runDeploy(s *session, opts deployOptions) error {
	org, app, err := s.target()
	if err != nil {
		return err
	}
	client, cred, err := s.client()
	if err != nil {
		return err
	}
	if !opts.Force {
		if err := s.doctorGate(org, app, client); err != nil {
			return err
		}
	}
	a, err := client.GetApp(s.ctx, org, app)
	if err != nil {
		return wrap(err)
	}
	if a.GitURL == "" {
		return output.New("PLATFORM_UNAVAILABLE", fmt.Sprintf("The platform returned no repository URL for %s/%s.", org, app), "The app's repository is not ready yet; retry in a moment. whisk apps info shows git_url once it exists.", nil)
	}
	if err := s.checkBoundRemote(org, app, a.GitURL); err != nil {
		return err
	}
	repo, err := s.prepareRepo(opts)
	if err != nil {
		return err
	}
	plan, err := planPush(opts.Env, opts.Branch, repo.Branch)
	if err != nil {
		return err
	}
	sha := repo.SHA
	if plan.Src != "HEAD" && plan.Src != repo.Branch {
		res, err := s.gitRun(nil, "rev-parse", "--verify", "--quiet", "refs/heads/"+plan.Src)
		if err != nil {
			return err
		}
		if !res.OK() {
			return output.New("INVALID_REQUEST", fmt.Sprintf("There is no local branch named %s.", plan.Src), "Pass --branch with a branch that exists (git branch lists them).", map[string]any{"branch": plan.Src})
		}
		sha = res.Out()
	}
	if err := s.ensureRemote(a.GitURL); err != nil {
		return err
	}
	s.printer.Progress("Pushing %s (%s) to %s/%s as %s.", plan.Src, short(sha), org, app, plan.Environment)
	if note := previewNote(plan.Environment); note != "" {
		s.printer.Progress("%s", note)
	}
	secret, err := s.gitSecret(client, cred)
	if err != nil {
		return err
	}
	sb, outcome, err := s.push(secret, plan, a.GitURL)
	if err != nil {
		return s.pushFailed(client, org, app, plan.Environment, err)
	}
	var d api.Deploy
	if strings.HasPrefix(plan.Environment, "preview:") {
		// A push only stores a branch; the preview starts on request, or answers the deploy
		// already following the branch (CONTROL-PLANE.md §6.3).
		started, err := client.StartPreview(s.ctx, org, app, plan.Dst)
		if err != nil {
			return wrap(err)
		}
		d = started.Deploy
	} else if d, err = s.locateDeploy(client, org, app, sb, outcome, sha, plan.Environment); err != nil {
		return err
	}
	if opts.NoWait {
		result := map[string]any{"deploy_id": d.ID, "status": d.Status, "commit_sha": sha, "branch": plan.Src, "environment": plan.Environment}
		note := previewNote(plan.Environment)
		if note != "" {
			result["note"] = note
		}
		s.printer.Result(result, func(w io.Writer) {
			fmt.Fprintf(w, "Deploy %s queued for %s (%s). Follow it with: whisk deploys info %s\n", d.ID, plan.Environment, short(sha), d.ID)
			if note != "" {
				fmt.Fprintln(w, note)
			}
		})
		return nil
	}
	return s.waitForDeploy(client, org, app, d)
}

// doctorGate runs doctor before a push and refuses to continue on errors (exit 3).
func (s *session) doctorGate(org, app string, client *api.Client) error {
	rep, err := doctor.Run(s.ctx, doctor.Options{Dir: s.env.Dir, Validate: func(ctx context.Context, m []byte) (api.Validation, error) {
		return client.Validate(ctx, org, app, m)
	}})
	if err != nil {
		return err
	}
	if rep.OK() {
		if rep.Warnings > 0 {
			s.printer.Progress("Doctor: %d warning(s); whisk doctor lists them.", rep.Warnings)
		}
		return nil
	}
	if !s.json {
		printReport(s.printer, s.env.Stderr, rep)
	}
	return &output.Error{Code: "DOCTOR_FAILED", Message: fmt.Sprintf("%d error(s) must be fixed before deploying.", rep.Errors), Fix: "Fix each finding (whisk doctor shows them; --fix applies the safe ones), or pass --force to push anyway.", Exit: output.ExitValidation, Details: map[string]any{"errors": rep.Errors, "warnings": rep.Warnings, "findings": nonNil(rep.Findings)}}
}

// gitRun runs git in the app directory; a missing binary is a CLI error with a fix.
func (s *session) gitRun(env []string, args ...string) (gitcmd.Result, error) {
	res, err := s.git.Run(s.ctx, s.env.Dir, env, args...)
	if errors.Is(err, gitcmd.ErrNotInstalled) {
		return res, output.New("INVALID_REQUEST", "git is not installed or not on PATH.", "Install git (https://git-scm.com) and run whisk deploy again.", nil)
	}
	if err != nil {
		return res, fmt.Errorf("git %s: %v", strings.Join(args, " "), err)
	}
	return res, nil
}

// goos is the operating system deploy behaves as: Env.GOOS in tests, else the real one.
func (s *session) goos() string {
	if s.env.GOOS != "" {
		return s.env.GOOS
	}
	return runtime.GOOS
}

func (s *session) now() time.Time {
	if s.env.Now != nil {
		return s.env.Now()
	}
	return time.Now()
}

func gitFailed(what string, res gitcmd.Result) error {
	msg := strings.TrimSpace(res.Stderr)
	if msg == "" {
		msg = strings.TrimSpace(res.Stdout)
	}
	return fmt.Errorf("%s failed (exit %d): %s", what, res.ExitCode, msg)
}

// repoState is what prepareRepo found or made.
type repoState struct {
	Fresh     bool   // git init ran
	Committed int    // files committed by this run
	SHA       string // HEAD
	Branch    string // "" when HEAD is detached
}

// prepareRepo makes the directory a repository with a commit to push: git init when needed,
// and a "whisk deploy" commit of everything not ignored when --commit allows it.
func (s *session) prepareRepo(opts deployOptions) (repoState, error) {
	var st repoState
	top, err := s.gitRun(nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return st, err
	}
	if !top.OK() {
		s.printer.Progress("Not a git repository yet; running git init.")
		res, err := s.gitRun(nil, "init", "--quiet")
		if err != nil {
			return st, err
		}
		if !res.OK() {
			return st, gitFailed("git init", res)
		}
		st.Fresh = true
	}
	status, err := s.gitRun(nil, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return st, err
	}
	if !status.OK() {
		return st, gitFailed("git status", status)
	}
	dirty, local := splitLocal(gitcmd.ParseStatus(status.Stdout))
	if len(local) > 0 {
		s.printer.Progress("Leaving %d file(s) under .whisk/dev/ out of the commit: %s. They are whisk dev's local files (secrets.env holds values); whisk doctor --fix adds .whisk/dev/ to .gitignore, and git rm -r --cached .whisk/dev takes files already committed out of the repository.", len(local), listPreview(local, 5))
	}
	commit := st.Fresh || strings.TrimSpace(opts.Message) != ""
	if opts.CommitSet {
		commit = opts.Commit
	}
	message := commitMessage(opts.Message)
	if len(dirty) > 0 {
		if !commit {
			return st, output.New("INVALID_REQUEST",
				fmt.Sprintf("%d file(s) have uncommitted changes: %s.", len(dirty), listPreview(dirty, 5)),
				"Commit them, or run whisk deploy -m \"<what changed, for the people who use the app>\" to commit everything not ignored with that message.",
				map[string]any{"uncommitted": dirty})
		}
		if res, err := s.gitRun(nil, append([]string{"add", "--all"}, stagePathspec...)...); err != nil {
			return st, err
		} else if !res.OK() {
			return st, gitFailed("git add", res)
		}
		args := append(s.identityArgs(), "commit", "--quiet", "-m", message)
		res, err := s.gitRun(nil, args...)
		if err != nil {
			return st, err
		}
		if !res.OK() {
			return st, gitFailed("git commit", res)
		}
		st.Committed = len(dirty)
		s.printer.Progress("Committed %d file(s) as \"%s\".", len(dirty), message)
	} else if strings.TrimSpace(opts.Message) != "" {
		s.printer.Progress("Nothing to commit, so the deploy shows the last commit's message, not --message.")
	}
	head, err := s.gitRun(nil, "rev-parse", "--verify", "--quiet", "HEAD")
	if err != nil {
		return st, err
	}
	if !head.OK() {
		return st, output.New("INVALID_REQUEST", "The repository has no commits and nothing to commit.", "Add the app's files to this directory and run whisk deploy again.", nil)
	}
	st.SHA = head.Out()
	if br, err := s.gitRun(nil, "symbolic-ref", "--short", "--quiet", "HEAD"); err == nil && br.OK() {
		st.Branch = br.Out()
	}
	return st, nil
}

// stagePathspec is what whisk deploy's commit stages: the whole work tree except whisk dev's
// local files under .whisk/dev/, wherever the app sits in the repository, so secrets.env is
// never committed even where .gitignore does not cover it and even when it is tracked already.
var stagePathspec = []string{"--", ":(top)", ":(top,exclude,glob)**/.whisk/dev/**"}

// splitLocal separates git status paths into those whisk deploy may commit and whisk dev's
// local files under a .whisk/dev/ folder, which it never commits. Pure.
func splitLocal(paths []string) (commit, local []string) {
	for _, p := range paths {
		if localOnly(p) {
			local = append(local, p)
		} else {
			commit = append(commit, p)
		}
	}
	return commit, local
}

// localOnly reports whether a git status path (possibly quoted, possibly "old -> new") lies
// under a .whisk/dev/ folder at any depth. Pure.
func localOnly(p string) bool {
	for _, part := range strings.Split(p, " -> ") {
		part = "/" + strings.Trim(strings.TrimSpace(part), `"`)
		if strings.Contains(part, "/.whisk/dev/") {
			return true
		}
	}
	return false
}

// commitMessage is the message whisk deploy commits with: the one given, trimmed, or
// "whisk deploy". Pure.
func commitMessage(given string) string {
	if m := strings.TrimSpace(given); m != "" {
		return m
	}
	return "whisk deploy"
}

// identityArgs supplies a committer when the machine has none configured, so a first deploy
// from a fresh machine does not stop at "please tell me who you are".
func (s *session) identityArgs() []string {
	if res, err := s.gitRun(nil, "config", "user.email"); err == nil && res.OK() && res.Out() != "" {
		return nil
	}
	return []string{"-c", "user.name=whisk", "-c", "user.email=whisk@localhost"}
}

func listPreview(items []string, n int) string {
	if len(items) <= n {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:n], ", ") + fmt.Sprintf(" and %d more", len(items)-n)
}

// pushPlan is which local ref goes to which remote branch, and the environment that makes.
type pushPlan struct {
	Src         string // local branch, or HEAD when detached
	Dst         string // remote branch
	Environment string // production | preview:<branch>
}

// planPush decides the push from the flags and the current branch. Only main, master or a
// detached HEAD deploys production, pushed to main; any other branch is a preview, pushed under
// its own name, and asking for production from one is PRODUCTION_NEEDS_MAIN. Without --env,
// main, master and a detached HEAD mean production and any other branch means a preview.
func planPush(envFlag, branchFlag, current string) (pushPlan, error) {
	branch := branchFlag
	if branch == "" {
		branch = current
	}
	if branch != "" && !branchOK(branch) {
		return pushPlan{}, output.New("INVALID_REQUEST", fmt.Sprintf("%q is not a branch name git accepts.", branch), "Pass --branch with a branch that exists (git branch lists them).", map[string]any{"branch": branch})
	}
	mainline := branch == "" || branch == "main" || branch == "master"
	env := envFlag
	if env == "" {
		env = "preview"
		if mainline {
			env = "production"
		}
	}
	switch env {
	case "production":
		if !mainline {
			return pushPlan{}, output.New("PRODUCTION_NEEDS_MAIN", fmt.Sprintf("Only main goes live; %s is a branch.", branch), fmt.Sprintf("Merge %s into main, then run whisk deploy from main. To show it without putting it live, run whisk deploy from %s for a preview.", branch, branch), map[string]any{"branch": branch})
		}
		src := branch
		if src == "" {
			src = "HEAD"
		}
		return pushPlan{Src: src, Dst: "main", Environment: "production"}, nil
	case "preview":
		if branch == "" {
			return pushPlan{}, output.New("INVALID_REQUEST", "A preview needs a branch and HEAD is detached.", "Check out a branch or pass --branch <name>.", nil)
		}
		if mainline {
			return pushPlan{}, output.New("INVALID_REQUEST", fmt.Sprintf("A preview cannot be made from %s; that branch is production.", branch), "Pass --branch <name> with a feature branch, or drop --env preview to deploy production.", map[string]any{"branch": branch})
		}
		return pushPlan{Src: branch, Dst: branch, Environment: "preview:" + branch}, nil
	}
	return pushPlan{}, output.New("INVALID_REQUEST", fmt.Sprintf("--env %q is not an environment.", envFlag), "Pass --env production or --env preview.", nil)
}

// branchOK is git check-ref-format's rule for a branch name. It keeps a branch one ref of the
// refspec: never an option of git push such as --receive-pack=<program>, never a second refspec.
func branchOK(b string) bool {
	switch {
	case b == "@", strings.HasPrefix(b, "-"), strings.HasPrefix(b, "/"), strings.HasSuffix(b, "/"),
		strings.HasSuffix(b, "."), strings.HasSuffix(b, ".lock"), strings.Contains(b, ".."),
		strings.Contains(b, "@{"), strings.Contains(b, "//"), strings.ContainsAny(b, " ~^:?*[\\"),
		strings.ContainsFunc(b, func(r rune) bool { return r < 0x20 || r == 0x7f }):
		return false
	}
	for _, seg := range strings.Split(b, "/") {
		if strings.HasPrefix(seg, ".") {
			return false
		}
	}
	return true
}

// previewNote is what a deploy to a preview says about production, which it leaves alone, and
// how its changes reach production: through main. It is empty for production.
func previewNote(environment string) string {
	branch, ok := strings.CutPrefix(environment, "preview:")
	if !ok {
		return ""
	}
	return fmt.Sprintf("This is a preview of branch %s; production is unchanged. To put it live, merge %s into main and run whisk deploy from main.", branch, branch)
}

// ensureRemote points the remote "whisk" at the app's repository.
// checkBoundRemote refuses to deploy when the target came from .whisk/app.json and the
// checkout's remote whisk points at another app's repository. A binding can arrive with the
// repository; the remote is what this machine last pushed to or cloned from, so a disagreement
// means the binding was not chosen here. whisk use confirms a binding and points the remote at
// it; --org and --app name the target explicitly and are not checked.
func (s *session) checkBoundRemote(org, app, gitURL string) error {
	if s.orgFlag != "" || s.appFlag != "" {
		return nil
	}
	cur, err := s.gitRun(nil, "remote", "get-url", "whisk")
	if err != nil {
		return err
	}
	if !cur.OK() || !gitcmd.RemoteDisagrees(cur.Out(), gitURL) {
		return nil
	}
	ref := org + "/" + app
	return output.New("INVALID_REQUEST",
		fmt.Sprintf("This directory is bound to %s in .whisk/app.json, but its git remote whisk points at %s, which is not that app's repository.", ref, gitcmd.Redact(cur.Out())),
		fmt.Sprintf("If %s is the app this code belongs to, run whisk use %s to confirm it (that points the remote at it) and deploy again; otherwise run whisk use <org>/<app> with the right app. A .whisk/app.json that came with the repository is not trusted on its own.", ref, ref),
		map[string]any{"reason": "binding_mismatch", "bound": ref, "remote": gitcmd.Redact(cur.Out()), "git_url": gitURL})
}

func (s *session) ensureRemote(gitURL string) error {
	cur, err := s.gitRun(nil, "remote", "get-url", "whisk")
	if err != nil {
		return err
	}
	var res gitcmd.Result
	switch {
	case !cur.OK():
		res, err = s.gitRun(nil, "remote", "add", "whisk", gitURL)
	case cur.Out() != gitURL:
		res, err = s.gitRun(nil, "remote", "set-url", "whisk", gitURL)
	default:
		return nil
	}
	if err != nil {
		return err
	}
	if !res.OK() {
		return gitFailed("git remote", res)
	}
	return nil
}

// push runs git push with the token handed over through the environment, and turns a
// rejection into the error object the server printed on the sideband.
func (s *session) push(token string, plan pushPlan, gitURL string) (gitcmd.Sideband, gitcmd.PushOutcome, error) {
	env, cargs, err := gitcmd.Credential(token, gitURL)
	if err != nil {
		return gitcmd.Sideband{}, gitcmd.PushUnknown, output.New("PLATFORM_UNAVAILABLE", err.Error()+".", "Check WHISK_API points at the Whisk platform you meant, then retry.", map[string]any{"git_url": gitURL})
	}
	args := gitcmd.Networked(cargs, "push", "--porcelain", "whisk", plan.Src+":refs/heads/"+plan.Dst)
	res, err := s.gitRun(env, args...)
	if err != nil {
		return gitcmd.Sideband{}, gitcmd.PushUnknown, err
	}
	retried := false
	if !res.OK() && s.goos() == "windows" && gitcmd.Failure(res.Stderr, gitcmd.PushUnknown) == "tls" {
		// Git for Windows verifies with its own OpenSSL bundle, which lacks roots that only the
		// Windows store has. Push once more through schannel before calling it a failure.
		s.printer.Progress("git's OpenSSL does not trust %s; retrying with the Windows certificate store.", gitHost(gitURL))
		res, err = s.gitRun(env, append(append([]string{}, gitcmd.SchannelArgs...), args...)...)
		if err != nil {
			return gitcmd.Sideband{}, gitcmd.PushUnknown, err
		}
		retried = true
		if res.OK() {
			s.printer.Progress("Pushed through the Windows certificate store. Run git config http.sslBackend schannel in this repository to make git do that itself.")
		}
	}
	sb := gitcmd.ParseSideband(res.Stderr)
	outcome, summary := gitcmd.ParsePush(res.Stdout)
	for _, line := range sb.Lines {
		if !strings.HasPrefix(line, "deploy ") && !strings.HasPrefix(line, "{") {
			s.printer.Progress("whisk: %s", line)
		}
	}
	if res.OK() && outcome != gitcmd.PushRejected {
		return sb, outcome, nil
	}
	if sb.Error != nil {
		return sb, outcome, hookError(sb.Error)
	}
	tail := gitcmd.LastLines(res.Stderr, 10)
	details := map[string]any{"git": tail, "summary": summary}
	switch gitcmd.Failure(res.Stderr, outcome) {
	case "tls":
		return sb, outcome, tlsUntrusted(gitHost(gitURL), s.goos(), retried, details)
	case "network":
		return sb, outcome, output.New("PLATFORM_UNAVAILABLE", "git push could not reach the platform: "+lastLine(tail), "Retry with backoff. Check WHISK_API if you meant another endpoint.", details)
	case "auth":
		return sb, outcome, output.New("AUTH_REQUIRED", "The platform refused the token for git push.", "Run whisk login again and approve the code for this app (or the whole org), then retry.", details)
	case "rejected":
		return sb, outcome, &output.Error{Code: "INVALID_REQUEST", Message: "The platform rejected the push: " + orDash(summary) + ".", Fix: "Read the server's lines in details.git, fix the cause, and run whisk deploy again.", Docs: werrors.DocsBase + "INVALID_REQUEST", Details: details, Exit: output.ExitValidation}
	}
	return sb, outcome, fmt.Errorf("git push failed (exit %d): %s", res.ExitCode, lastLine(tail))
}

// tlsUntrusted is GIT_TLS_UNTRUSTED: git refused the certificate of the platform's git host.
// On Windows the usual cause is Git's bundled OpenSSL, fixed by schannel; when schannel was
// already tried, or elsewhere, something on the machine is presenting its own certificate.
func tlsUntrusted(host, goos string, triedSchannel bool, details map[string]any) *output.Error {
	details["host"] = host
	msg := "git does not trust the certificate of " + host + "."
	var fix string
	switch {
	case goos == "windows" && !triedSchannel:
		fix = "Run git config --global http.sslBackend schannel so git verifies with the Windows certificate store, then run whisk deploy again."
	case goos == "windows":
		msg = "Neither git's OpenSSL nor the Windows certificate store trusts the certificate of " + host + "."
		fix = "Something on this machine or network (an HTTPS-inspecting proxy or antivirus) is presenting its own certificate. Add its root certificate to the Windows store or point git's http.sslCAInfo at it, then run whisk deploy again."
	default:
		fix = "Update the system CA certificates. If an HTTPS-inspecting proxy or antivirus is in the path, point git's http.sslCAInfo at a bundle that includes its root certificate, then run whisk deploy again."
	}
	return output.New("GIT_TLS_UNTRUSTED", msg, fix, details)
}

// gitHost is the host of a git URL, or the URL itself when it does not parse.
func gitHost(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		return u.Host
	}
	return raw
}

func lastLine(lines []string) string {
	if len(lines) == 0 {
		return "no output"
	}
	return lines[len(lines)-1]
}

// hookError turns a pre-receive rejection into the CLI error. Codes the catalogue maps to an
// exit code keep it; anything else is a validation failure (exit 3).
func hookError(d *werrors.Detail) *output.Error {
	e := &output.Error{Code: d.Code, Message: d.Message, Fix: d.Fix, Docs: d.Docs, Details: d.Details}
	if e.Docs == "" {
		e.Docs = werrors.DocsBase + d.Code
	}
	if output.ExitCode(e) == output.ExitError {
		e.Exit = output.ExitValidation
	}
	return e
}

// locateDeploy finds the deploy the push started: from the sideband, else the newest deploy
// for the pushed commit (up to 30 seconds). A coded error on the sideband with no deploy is
// the server refusing it, returned at once. When nothing was pushed because the remote already
// had the commit, the existing deploy is reused if it is live or running, else it is deployed
// again; one that has sat queued or building past deployStallLimit is DEPLOY_IN_PROGRESS.
func (s *session) locateDeploy(client *api.Client, org, app string, sb gitcmd.Sideband, outcome gitcmd.PushOutcome, sha, environment string) (api.Deploy, error) {
	if sb.DeployID != "" {
		d, err := client.GetDeploy(s.ctx, org, app, sb.DeployID)
		if err == nil {
			return d, nil
		}
		if api.IsCode(err, "NOT_FOUND") {
			return api.Deploy{ID: sb.DeployID, CommitSHA: sha, Environment: environment}, nil
		}
		return d, wrap(err)
	}
	if sb.Error != nil {
		// The push was stored but post-receive refused its deploy and said why: waiting for a
		// deploy that will not come would only end in PLATFORM_UNAVAILABLE.
		return api.Deploy{}, hookError(sb.Error)
	}
	find := func() (api.Deploy, bool, error) {
		deploys, err := client.ListDeploys(s.ctx, org, app, 20)
		if err != nil {
			return api.Deploy{}, false, wrap(err)
		}
		d, ok := latestForCommit(deploys, sha, environment)
		return d, ok, nil
	}
	if outcome == gitcmd.PushUpToDate {
		d, ok, err := find()
		if err != nil {
			return d, err
		}
		if ok && d.Status == status.DeployLive {
			s.printer.Progress("Nothing new to push; %s is deploy %s (%s).", short(sha), d.ID, d.Status)
			return d, nil
		}
		if ok && !api.Terminal(d.Status) {
			// The commit's deploy is on its way: follow it, unless it has sat queued or
			// building so long that waiting on it would never end.
			if waited, stuck := stalled(d, s.now()); stuck {
				return d, deployInProgress(d, waited, "", nil)
			}
			s.printer.Progress("Nothing new to push; %s is deploy %s (%s for %s); following it.", short(sha), d.ID, d.Status, spanWords(s.now().Sub(phaseSince(d))))
			return d, nil
		}
		s.printer.Progress("Nothing new to push; deploying %s again.", short(sha))
		d, err = client.CreateDeploy(s.ctx, org, app, api.CreateDeployRequest{CommitSHA: sha, Environment: environment})
		return d, wrap(err)
	}
	deadline := time.Now().Add(30 * time.Second)
	for {
		d, ok, err := find()
		if err != nil {
			return d, err
		}
		if ok {
			return d, nil
		}
		if time.Now().After(deadline) {
			return api.Deploy{}, output.New("PLATFORM_UNAVAILABLE", fmt.Sprintf("The push succeeded but no deploy for %s appeared within 30 seconds.", short(sha)), "Run whisk deploys list to see whether it started; if it did not, run whisk deploy again.", map[string]any{"commit_sha": sha, "environment": environment})
		}
		if err := s.sleep(time.Second); err != nil {
			return api.Deploy{}, err
		}
	}
}

// latestForCommit picks the newest deploy of a commit in an environment (lists are newest
// first).
func latestForCommit(deploys []api.Deploy, sha, environment string) (api.Deploy, bool) {
	for _, d := range deploys {
		if d.CommitSHA == sha && (environment == "" || d.Environment == environment) {
			return d, true
		}
	}
	return api.Deploy{}, false
}

func (s *session) sleep(d time.Duration) error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// waitForDeploy follows a deploy to its end and reports the outcome: the URL (and a NEEDS_HUMAN
// block for unset secrets) when live, exit 2 when blocked on secrets, exit 6 otherwise.
func (s *session) waitForDeploy(client *api.Client, org, app string, d api.Deploy) error {
	if api.Terminal(d.Status) {
		ev, err := s.eventFromDeploy(client, org, app, d)
		if err != nil {
			return err
		}
		printEvent(s, ev)
		return s.finishDeploy(client, org, app, ev, d.Environment)
	}
	ev, err := s.followDeploy(client, org, app, d.ID)
	if err != nil {
		return err
	}
	return s.finishDeploy(client, org, app, ev, d.Environment)
}

// followDeploy streams the deploy's events, reconnecting when the stream ends early or drops,
// until an event says done. Five failed connections in a row, with no event between them, end
// it with PLATFORM_UNAVAILABLE.
func (s *session) followDeploy(client *api.Client, org, app, id string) (api.DeployEvent, error) {
	seen := map[string]bool{}
	var last api.DeployEvent
	unavailable := 0
	for {
		got := false
		done, err := client.DeployEvents(s.ctx, org, app, id, func(ev api.DeployEvent) error {
			last, got = ev, true
			key := ev.Phase.Phase + "|" + ev.Phase.At.UTC().Format(time.RFC3339Nano)
			if !seen[key] {
				seen[key] = true
				printEvent(s, ev)
			}
			return nil
		})
		if done {
			return last, nil
		}
		if err != nil {
			var u *api.Unavailable
			if !errors.As(err, &u) {
				return last, wrap(err)
			}
			if got {
				unavailable = 0
			}
			unavailable++
			if unavailable >= 5 {
				return last, wrap(err)
			}
		}
		d, derr := client.GetDeploy(s.ctx, org, app, id)
		if derr == nil && api.Terminal(d.Status) {
			ev, err := s.eventFromDeploy(client, org, app, d)
			if err != nil {
				return last, err
			}
			key := ev.Phase.Phase + "|" + ev.Phase.At.UTC().Format(time.RFC3339Nano)
			if !seen[key] {
				printEvent(s, ev)
			}
			return ev, nil
		}
		if err := s.sleep(time.Second); err != nil {
			return last, err
		}
	}
}

// eventFromDeploy builds the terminal event from a deploy record, for deploys that were
// already finished or whose stream dropped: the URL comes from the environment and the unset
// secrets from the app.
func (s *session) eventFromDeploy(client *api.Client, org, app string, d api.Deploy) (api.DeployEvent, error) {
	ev := api.DeployEvent{DeployID: d.ID, Status: d.Status, Error: d.Error, Done: true, Phase: api.Phase{Phase: string(d.Status), At: time.Now()}}
	if n := len(d.Phases); n > 0 {
		ev.Phase = d.Phases[n-1]
	}
	if d.Status != status.DeployLive {
		return ev, nil
	}
	a, err := client.GetApp(s.ctx, org, app)
	if err != nil {
		return ev, wrap(err)
	}
	ev.URL = a.URL
	ev.Unset = a.UnsetSecrets
	ev.StartMS = d.StartMS
	if d.Environment != "" && d.Environment != "production" {
		envs, err := client.ListEnvironments(s.ctx, org, app)
		if err != nil {
			return ev, wrap(err)
		}
		for _, e := range envs {
			if e.Name == d.Environment && e.URL != "" {
				ev.URL = e.URL
			}
		}
	}
	return ev, nil
}

// printEvent prints one phase: a JSON line for agents, a timestamped line for humans.
func printEvent(s *session, ev api.DeployEvent) {
	if s.json {
		s.printer.Line(ev, "")
		return
	}
	detail := ""
	if ev.Phase.Detail != "" {
		detail = "  " + ev.Phase.Detail
	}
	s.printer.Progress("  %s  %s%s", ev.Phase.At.Local().Format("15:04:05"), ev.Phase.Phase, detail)
}

// finishDeploy reports the terminal event. A live preview says so, and that production is
// unchanged, so nobody mistakes it for a release. A deploy a newer one replaced did not fail:
// the newer one carries the change on (CLI.md §5.4).
func (s *session) finishDeploy(client *api.Client, org, app string, ev api.DeployEvent, environment string) error {
	switch ev.Status {
	case status.DeployLive:
		unset := nonNilStrings(ev.Unset)
		result := map[string]any{"deploy_id": ev.DeployID, "status": "live", "url": ev.URL, "unset_secrets": unset}
		if environment != "" {
			result["environment"] = environment
		}
		if ev.StartMS != nil {
			result["start_ms"] = *ev.StartMS
		}
		label := "Live:"
		note := previewNote(environment)
		if note != "" {
			label = "Preview live:"
			result["note"] = note
		}
		var block *output.Error
		if len(unset) > 0 {
			block = needsHumanSecrets(unset, secretsURL(s.dashboard(), org, app))
			result["needs_human"] = block.Body()["error"]
		}
		warnings := ev.Warnings
		if warnings == nil {
			warnings = []api.Warning{}
		}
		result["warnings"] = warnings
		// The addresses to paste into each webhook's provider, when the app declares any.
		hooks := s.webhookURLs(client, org, app)
		if len(hooks) > 0 {
			result["webhooks"] = hooks
		}
		s.printer.Result(result, func(w io.Writer) {
			fmt.Fprintf(w, "%s %s\n", s.printer.Good(label), ev.URL)
			if ev.StartMS != nil {
				fmt.Fprintf(w, "Started in %s\n", output.Seconds(*ev.StartMS))
			}
			for _, h := range hooks {
				fmt.Fprintf(w, "Webhook %s: %s\n", h["name"], h["url"])
			}
			for _, wa := range warnings {
				fmt.Fprintf(w, "%s %s\n  %s\n", s.printer.Warn(wa.Code+":"), wa.Message, wa.Fix)
			}
			if note != "" {
				fmt.Fprintln(w, note)
			}
			if block != nil {
				s.printer.Block(w, block)
			}
		})
		return nil
	case status.DeploySuperseded:
		result := map[string]any{"deploy_id": ev.DeployID, "status": ev.Status, "note": "A newer deploy of this environment replaced this one. Follow it with whisk deploys."}
		if environment != "" {
			result["environment"] = environment
		}
		s.printer.Result(result, func(w io.Writer) {
			fmt.Fprintln(w, "Replaced by a newer deploy.")
			fmt.Fprintln(w, "Follow it with: whisk deploys")
		})
		return nil
	case status.DeployBlocked:
		grants, unset := blockedDetail(ev.Error)
		if len(ev.Unset) > 0 {
			unset = ev.Unset
		}
		if len(grants) == 0 {
			grants = s.grantsNeeded(client, org, app, ev.DeployID)
		}
		if len(grants) > 0 {
			return grantsNeededBlock(ev.DeployID, grants, unset, connectionsURL(s.dashboard(), org, app), secretsURL(s.dashboard(), org, app))
		}
		if len(unset) == 0 {
			// Best effort: the deploy is blocked either way, and that is the error to report,
			// so a failed lookup only leaves the secret names out of it.
			if a, err := client.GetApp(s.ctx, org, app); err == nil {
				unset = a.UnsetSecrets
			} else {
				s.printer.Progress("Could not list the unset secrets: %v", err)
			}
		}
		e := needsHumanSecrets(unset, secretsURL(s.dashboard(), org, app))
		e.Message = fmt.Sprintf("Deploy %s is blocked until %d secret(s) have a value: %s.", ev.DeployID, len(unset), strings.Join(unset, ", "))
		e.Details["deploy_id"] = ev.DeployID
		return e
	case status.DeployFailed, status.DeployRolledBack, status.DeployCancelled,
		status.DeployQueued, status.DeployBuilding, status.DeploySnapshotting, status.DeployMigrating,
		status.DeployStarting, status.DeployHealthChecking, status.DeploySwitching, status.DeployDraining:
	}
	return s.deployFailure(client, org, app, ev)
}

// grantsNeeded is what a blocked deploy waits on a person to grant, read from the deploy when
// its event did not carry it. Best effort, as the unset secrets are: a failed read leaves the
// deploy reported as waiting on secrets.
func (s *session) grantsNeeded(client *api.Client, org, app, id string) []api.GrantNeeded {
	d, err := client.GetDeploy(s.ctx, org, app, id)
	if err != nil {
		s.printer.Progress("Could not read what the deploy waits on: %v", err)
		return nil
	}
	return d.GrantsNeeded
}

// deployFailure is exit 6: the code and fix from the deploy record, with the last 40 lines
// of the build log when the build is what failed.
func (s *session) deployFailure(client *api.Client, org, app string, ev api.DeployEvent) error {
	d, _ := client.GetDeploy(s.ctx, org, app, ev.DeployID)
	detail := ev.Error
	if detail == nil {
		detail = d.Error
	}
	e := &output.Error{Exit: output.ExitDeploy, Details: map[string]any{}}
	if detail != nil {
		e.Code, e.Message, e.Fix, e.Docs = detail.Code, detail.Message, detail.Fix, detail.Docs
		if api.Platform(detail) {
			// Whisk's side: retry or wait, which is what exit 5 means (CLI.md §3).
			e.Exit = output.ExitUnavailable
		}
		for k, v := range detail.Details {
			e.Details[k] = v
		}
	} else {
		e.Code = "DEPLOY_FAILED"
		e.Message = fmt.Sprintf("Deploy %s ended with status %s and no error was recorded.", ev.DeployID, ev.Status)
		e.Fix = "Run whisk deploys info " + ev.DeployID + " for the phases, fix the cause, and run whisk deploy again."
	}
	e.Details["deploy_id"] = ev.DeployID
	e.Details["status"] = ev.Status
	lines := s.logExcerpt(client, org, app, d, e)
	if len(lines) > 0 {
		e.Details["log"] = lines
		if !s.json {
			fmt.Fprintf(s.env.Stderr, "--- last %d line(s) of the log ---\n%s\n---\n", len(lines), strings.Join(lines, "\n"))
		}
	}
	return e
}

// deployWhy is a failed deploy's code in a listing, marked when it failed on Whisk's side.
func deployWhy(e *werrors.Detail) string {
	switch {
	case e == nil:
		return ""
	case api.Platform(e):
		return e.Code + " (Whisk's side)"
	}
	return e.Code
}

// webhookURLs is each declared webhook's name and the address its provider sends to (for the
// inbox, the email address mail for the app goes to), or none when the app declares none or they cannot be read (the deploy is live either way).
func (s *session) webhookURLs(client *api.Client, org, app string) []map[string]string {
	m, err := loadManifest(s.env.Dir)
	if err != nil || (len(m.Webhooks) == 0 && m.Inbox == nil) {
		return nil
	}
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	sources, err := client.ListWebhooks(ctx, org, app)
	if err != nil {
		return nil
	}
	out := make([]map[string]string, 0, len(sources))
	for _, src := range sources {
		out = append(out, map[string]string{"name": src.Name, "url": src.URL})
	}
	return out
}

// logExcerpt is the last 40 lines that explain a failure: the build log for a build failure,
// else what the error object carries in details.log.
func (s *session) logExcerpt(client *api.Client, org, app string, d api.Deploy, e *output.Error) []string {
	if (e.Code == "BUILD_FAILED" || e.Code == "BUILD_TIMEOUT") && d.BuildID != "" {
		ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
		defer cancel()
		if text, err := client.BuildLog(ctx, org, app, d.BuildID); err == nil && strings.TrimSpace(text) != "" {
			return gitcmd.LastLines(text, 40)
		}
	}
	lines := errorLines(e.Details)
	if len(lines) > 40 {
		lines = lines[len(lines)-40:]
	}
	return lines
}

// ---- deploys, rollback ----------------------------------------------------------------------

func deploysCmd(s *session) *cobra.Command {
	deploys := &cobra.Command{Use: "deploys", Short: "Deploys of the app"}
	var limit int
	list := &cobra.Command{
		Use:   "list",
		Short: "List recent deploys, newest first",
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
			all, err := client.ListDeploys(s.ctx, org, app, limit)
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "deploys": all}, func(w io.Writer) {
				if len(all) == 0 {
					fmt.Fprintln(w, "No deploys yet. Run whisk deploy.")
					return
				}
				rows := make([][]string, len(all))
				for i, d := range all {
					rows[i] = []string{d.ID, string(d.Status), d.Environment, short(d.CommitSHA), orDash(d.Branch), orDash(clip(d.Message, 50)), orDash(d.TriggeredByLabel), at(d.CreatedAt), deployWhy(d.Error)}
				}
				s.printer.Table(w, []string{"DEPLOY", "STATUS", "ENV", "COMMIT", "BRANCH", "CHANGE", "BY", "WHEN", "WHY"}, rows)
			})
			return nil
		},
	}
	list.Flags().IntVar(&limit, "limit", 20, "how many to show")

	info := &cobra.Command{
		Use:   "info <id>",
		Short: "Show one deploy with its phases",
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
			d, err := client.GetDeploy(s.ctx, org, app, args[0])
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "deploy": d}, func(w io.Writer) { printDeploy(s.printer, w, d) })
			return nil
		},
	}

	cancel := &cobra.Command{
		Use:   "cancel <id>",
		Short: "Cancel a deploy that has not finished",
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
			d, err := client.CancelDeploy(s.ctx, org, app, args[0])
			if err != nil {
				return wrap(err)
			}
			s.printer.Result(map[string]any{"org": org, "app": app, "deploy": d}, func(w io.Writer) {
				fmt.Fprintf(w, "Deploy %s is %s.\n", d.ID, d.Status)
			})
			return nil
		},
	}
	logCmd := &cobra.Command{
		Use:   "log <id>",
		Short: "Print a deploy's whole build log, or the app's last lines when it failed to start",
		Long: `Prints the build log of the deploy's build, in full. For a deploy whose app failed to start
(HEALTH_CHECK_FAILED, CONTAINER_CRASHED) or to migrate, it also prints the lines the deploy's
error carries. --json gives {"deploy_id", "build_id", "build_log", "app_log"}.`,
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
			d, err := client.GetDeploy(s.ctx, org, app, args[0])
			if err != nil {
				return wrap(err)
			}
			buildLog := ""
			if d.BuildID != "" {
				if buildLog, err = client.BuildLog(s.ctx, org, app, d.BuildID); err != nil && !api.IsCode(err, "NOT_FOUND") {
					return wrap(err)
				}
			}
			appLog := []string{}
			if d.Error != nil {
				appLog = errorLines(d.Error.Details)
			}
			s.printer.Result(map[string]any{"deploy_id": d.ID, "build_id": d.BuildID, "build_log": buildLog, "app_log": appLog}, func(w io.Writer) {
				if strings.TrimSpace(buildLog) == "" {
					fmt.Fprintln(w, "No build log is kept for this deploy.")
				} else {
					fmt.Fprint(w, strings.TrimRight(buildLog, "\n")+"\n")
				}
				if len(appLog) > 0 {
					fmt.Fprintf(w, "--- the app's last %d line(s) ---\n%s\n", len(appLog), strings.Join(appLog, "\n"))
				}
			})
			return nil
		},
	}
	deploys.AddCommand(list, info, cancel, logCmd)
	return deploys
}

func printDeploy(p output.Printer, w io.Writer, d api.Deploy) {
	fmt.Fprintf(w, "%s  %s  %s\n  commit %s%s  by %s  created %s\n", p.Bold(d.ID), d.Status, d.Environment, short(d.CommitSHA), branchSuffix(d.Branch), orDash(d.TriggeredByLabel), at(d.CreatedAt))
	if d.Message != "" {
		fmt.Fprintf(w, "  %s\n", d.Message)
	}
	for _, ph := range d.Phases {
		detail := ""
		if ph.Detail != "" {
			detail = "  " + ph.Detail
		}
		fmt.Fprintf(w, "  %s  %s%s\n", ph.At.Local().Format("15:04:05"), ph.Phase, detail)
	}
	if d.Error != nil {
		fmt.Fprintf(w, "  %s: %s\n    %s\n", p.Bad(d.Error.Code), d.Error.Message, p.Dim("fix: "+d.Error.Fix))
	}
}

func branchSuffix(branch string) string {
	if branch == "" {
		return ""
	}
	return " (" + branch + ")"
}

func rollbackCmd(s *session) *cobra.Command {
	var envName string
	var noWait bool
	c := &cobra.Command{
		Use:   "rollback [<deploy-id>]",
		Short: "Make an earlier deploy live again (default: the previous live one)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			org, app, err := s.target()
			if err != nil {
				return err
			}
			client, _, err := s.client()
			if err != nil {
				return err
			}
			req := api.CreateDeployRequest{Environment: envName}
			if len(args) == 1 {
				req.DeployID = args[0]
			}
			d, err := client.CreateDeploy(s.ctx, org, app, req)
			if err != nil {
				return wrap(err)
			}
			s.printer.Progress("Rolling back to %s (deploy %s).", short(d.CommitSHA), d.ID)
			if noWait {
				s.printer.Result(map[string]any{"deploy_id": d.ID, "status": d.Status, "commit_sha": d.CommitSHA, "environment": d.Environment}, func(w io.Writer) {
					fmt.Fprintf(w, "Rollback queued as deploy %s. Follow it with: whisk deploys info %s\n", d.ID, d.ID)
				})
				return nil
			}
			return s.waitForDeploy(client, org, app, d)
		},
	}
	c.Flags().StringVar(&envName, "env", "", "environment to roll back (default production)")
	c.Flags().BoolVar(&noWait, "no-wait", false, "print the deploy id and exit")
	return c
}

// clip shortens s to at most n characters for a table cell, ending a cut one with "…". Pure.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n-1])) + "…"
}

// errorLines is the log a deploy error's details carry, under log (or log_tail on a deploy
// recorded before details.log), as strings. Pure.
func errorLines(details map[string]any) []string {
	raw, ok := details["log"].([]any)
	if !ok {
		raw, _ = details["log_tail"].([]any)
	}
	out := make([]string, 0, len(raw))
	for _, l := range raw {
		if s, ok := l.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
