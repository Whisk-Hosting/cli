// Package whisk is the command tree. Run executes one invocation the way the binary does, so
// the harness can drive the CLI as a library and tests can assert its JSON.
package whisk

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/whisk-run/contract/tokensig"

	"github.com/whisk-run/cli/internal/api"
	"github.com/whisk-run/cli/internal/config"
	"github.com/whisk-run/cli/internal/gitcmd"
	"github.com/whisk-run/cli/internal/output"
)

// Version is set by the build (-ldflags "-X github.com/whisk-run/cli/whisk.Version=...").
var Version = "dev"

// Released is the Unix time of the build's commit, set by make cli-dist
// (-X github.com/whisk-run/cli/whisk.Released=...), so whisk update can refuse an older release.
var Released = ""

// Env is the process environment an invocation runs in, made explicit for tests.
type Env struct {
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	Getenv     func(string) string
	Dir        string // working directory
	IsTerminal bool   // stdin and stdout are a terminal
	ConfigDir  string // overrides the config directory when set
	// Store overrides the credential store when set (tests).
	Store config.Store
	// Git runs git for whisk deploy; nil runs the git binary (tests pass a fake).
	Git gitcmd.Runner
	// Executable is the running binary's path for whisk update; empty means os.Executable().
	Executable string
	// GOOS is the operating system the CLI behaves as; empty means runtime.GOOS (tests set it).
	GOOS string
	// Now is the clock; nil means time.Now (tests set it).
	Now func() time.Time
}

// session is what every command receives: flags, config, output and lazy API access.
type session struct {
	ctx     context.Context
	env     Env
	cfg     config.Config
	cfgDir  string
	printer output.Printer
	update  *output.UpdateCheck
	store   config.Store
	git     gitcmd.Runner

	json    bool
	quiet   bool
	profile string
	orgFlag string
	appFlag string
}

// Run executes args and returns the exit code. Nothing is written except through env.
func Run(ctx context.Context, args []string, env Env) int {
	if env.Getenv == nil {
		env.Getenv = os.Getenv
	}
	if env.Stdin == nil {
		env.Stdin = strings.NewReader("")
	}
	s := &session{ctx: ctx, env: env, git: env.Git, update: &output.UpdateCheck{Current: Version}}
	if s.git == nil {
		s.git = gitcmd.Exec{}
	}
	// A flag or command error surfaces before the session is set up; the printer must already
	// have its streams, or the error itself would crash.
	s.printer = output.Printer{Out: env.Stdout, Err: env.Stderr, Version: Version, Update: s.update}
	root := newRoot(s)
	root.SetArgs(args)
	root.SetIn(env.Stdin)
	root.SetOut(env.Stdout)
	root.SetErr(env.Stderr)
	if err := root.ExecuteContext(ctx); err != nil {
		if !s.printer.JSON && hasJSONFlag(args) {
			// A parse error stops cobra before prepare runs; --json still gets JSON.
			s.printer.JSON, s.printer.Quiet = true, true
		}
		code := s.printer.Fail(unknownInput(err))
		s.noticeUpdate()
		return code
	}
	s.noticeUpdate()
	return output.ExitOK
}

// noticeUpdate tells a human, on stderr after the command, that the platform serves a newer
// CLI. JSON output carries the same as cli_update instead.
func (s *session) noticeUpdate() {
	if line := s.update.Sentence(); line != "" && !s.printer.JSON && !s.printer.Quiet {
		fmt.Fprintln(s.env.Stderr, line)
	}
}

// unknownInput turns cobra's unknown command or flag into UNKNOWN_COMMAND. The usual cause is a
// CLI older than the docs the agent is reading, so the fix names whisk update and this version.
func unknownInput(err error) error {
	msg := err.Error()
	if !strings.HasPrefix(msg, "unknown command") && !strings.HasPrefix(msg, "unknown flag") && !strings.HasPrefix(msg, "unknown shorthand flag") {
		return err
	}
	first, _, _ := strings.Cut(msg, "\n")
	return output.New("UNKNOWN_COMMAND", "This whisk ("+Version+") has no such command or flag: "+strings.TrimSpace(first)+".",
		"Run whisk update to install the platform's current CLI, then retry; run whisk --help for what this build offers.",
		map[string]any{"whisk_cli": Version})
}

// hasJSONFlag reports whether --json appears before any -- terminator.
func hasJSONFlag(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
}

func newRoot(s *session) *cobra.Command {
	root := &cobra.Command{
		Use:           "whisk",
		Short:         "Deploy business apps to Whisk",
		Long:          "whisk is the command coding agents drive and humans occasionally type. Every command has --json; exit codes carry meaning (0 ok, 1 error, 2 needs a human, 3 validation, 4 auth, 5 unavailable, 6 deploy failed). Whisk already provides sign-in, the database, jobs, secrets, storage, video, email, error tracking and logs: use them instead of adding outside services (https://skill.whisk.run/SKILL.md, §1).",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if !s.json && !machineProtocol[cmd.Name()] {
				s.env.Stdout, s.env.Stderr = output.Terminal(s.env.Stdout), output.Terminal(s.env.Stderr)
			}
			return s.prepare()
		},
	}
	pf := root.PersistentFlags()
	pf.BoolVar(&s.json, "json", false, "machine output: one JSON object on stdout")
	pf.BoolVar(&s.quiet, "quiet", false, "no progress on stderr")
	pf.StringVar(&s.profile, "profile", "", "credential profile (or WHISK_PROFILE)")
	pf.StringVar(&s.orgFlag, "org", "", "org slug, instead of .whisk/app.json")
	pf.StringVar(&s.appFlag, "app", "", "app slug, instead of .whisk/app.json")

	root.AddCommand(
		versionCmd(s), updateCmd(s), skillCmd(s), errorGroupsCmd(s, errorsCmd(s)), schemaCmd(s), cronCmd(s), webhooksCmd(s),
		logsCmd(s), tracesCmd(s), mcpCmd(s),
		loginCmd(s), logoutCmd(s), whoamiCmd(s), accountCmd(s), orgsCmd(s), useCmd(s), cloneCmd(s), gitCredentialCmd(s),
		initCmd(s), appsCmd(s), doctorCmd(s), devCmd(s),
		deployCmd(s), deploysCmd(s), rollbackCmd(s), statusCmd(s), scanCmd(s), openCmd(s),
		secretsCmd(s), domainsCmd(s), cdnCmd(s), envsCmd(s), githubCmd(s),
		membersCmd(s), accessCmd(s), deployKeysCmd(s), customersCmd(s), clientsCmd(s),
		agentTokenCmd(s), tokensCmd(s), feedbackCmd(s),
		functionsCmd(s), runsCmd(s), eventsCmd(s), approvalsCmd(s),
		dbCmd(s), restoreCmd(s), exportCmd(s), billingCmd(s), operatorCmd(s),
	)
	root.CompletionOptions.HiddenDefaultCmd = false
	return root
}

// machineProtocol names the commands whose stdout is a protocol for another program, not text
// for a person, so it is left exactly as written: the MCP server and git's credential helper.
var machineProtocol = map[string]bool{"mcp": true, "git-credential": true}

// prepare loads config and builds the printer once flags are parsed.
func (s *session) prepare() error {
	dir := s.env.ConfigDir
	if dir == "" {
		dir = config.Dir(s.env.Getenv)
	}
	s.cfgDir = dir
	cfg, err := config.Load(dir)
	if err != nil {
		return err
	}
	if v := s.env.Getenv("WHISK_API"); v != "" {
		cfg.API = v
	}
	if s.profile == "" {
		s.profile = s.env.Getenv("WHISK_PROFILE")
	}
	if s.profile == "" {
		s.profile = cfg.Profile
	}
	s.cfg = cfg
	s.store = s.env.Store
	if s.store == nil {
		s.store = config.OpenStore(dir)
	}
	s.printer = output.Printer{JSON: s.json, Quiet: s.quiet || s.json, Color: s.env.IsTerminal && !s.json && s.env.Getenv("NO_COLOR") == "", Out: s.env.Stdout, Err: s.env.Stderr, Version: Version, Update: s.update}
	return nil
}

// credential returns the token in use: WHISK_TOKEN, else the profile's stored credential.
func (s *session) credential() (config.Credential, bool, error) {
	if t := s.env.Getenv("WHISK_TOKEN"); t != "" {
		return config.Credential{Token: t, API: s.cfg.API}, true, nil
	}
	return s.store.Get(s.profile)
}

// client returns an API client, or the AUTH_REQUIRED error when there is no token.
func (s *session) client() (*api.Client, config.Credential, error) {
	cred, ok, err := s.credential()
	if err != nil {
		return nil, cred, err
	}
	if !ok || cred.Token == "" {
		return nil, cred, output.New("AUTH_REQUIRED", "No token for profile "+s.profile+".", "Run whisk login, or set WHISK_TOKEN.", map[string]any{"profile": s.profile})
	}
	base := s.cfg.API
	if cred.API != "" {
		if s.env.Getenv("WHISK_TOKEN") == "" && s.env.Getenv("WHISK_API") != "" && !sameAPI(cred.API, s.cfg.API) {
			// A stored token goes only to the platform that issued it: WHISK_API set by a
			// repository's environment file or a tool's config must not carry it elsewhere.
			return nil, cred, output.New("AUTH_REQUIRED", "The token stored for profile "+s.profile+" was issued by "+cred.API+", and WHISK_API points at "+s.cfg.API+"; it is not sent there.",
				"Run whisk login with WHISK_API set to sign in to "+s.cfg.API+" under another profile (--profile or WHISK_PROFILE), set WHISK_TOKEN to a token for it, or unset WHISK_API.",
				map[string]any{"profile": s.profile, "token_api": cred.API, "api": s.cfg.API})
		}
		base = cred.API
	}
	c := s.newClient(base, cred.Token)
	if cred.PrivateKey != "" && s.env.Getenv("WHISK_TOKEN") == "" {
		key, err := tokensig.ParsePrivate(cred.PrivateKey)
		if err != nil {
			return nil, cred, output.New("AUTH_REQUIRED", "The key stored with profile "+s.profile+"'s token cannot be read: "+err.Error()+".",
				"Run whisk login again on this computer.", map[string]any{"profile": s.profile})
		}
		c.Key = key
	}
	return c, cred, nil
}

// Credential is a profile's stored token, for a program driving the CLI as a library.
type Credential = config.Credential

// MemoryStore is a credential store in memory holding one profile's credential, for a program
// driving the CLI as a library with a device-bound token and its key (the harness): set it as
// Env.Store and nothing reaches the keychain or the disk.
func MemoryStore(profile string, c Credential) config.Store {
	return config.NewMemoryStore(profile, c)
}

// sameAPI compares two API origins, ignoring a trailing slash and case.
func sameAPI(a, b string) bool {
	return strings.EqualFold(strings.TrimRight(a, "/"), strings.TrimRight(b, "/"))
}

// newClient builds an API client that reports the platform's served CLI version.
func (s *session) newClient(base, token string) *api.Client {
	c := api.New(base, token, s.env.Getenv("WHISK_AGENT"), Version)
	c.OnCLIVersion = s.update.SetLatest
	return c
}

// anonymousClient talks to the API without a token (device flow).
func (s *session) anonymousClient() *api.Client {
	return s.newClient(s.cfg.API, "")
}

// target resolves which org and app a command acts on: flags, else .whisk/app.json.
func (s *session) target() (org, app string, err error) {
	if s.orgFlag != "" && s.appFlag != "" {
		return s.orgFlag, s.appFlag, nil
	}
	b, ok, err := config.LoadBinding(s.env.Dir)
	if err != nil {
		return "", "", err
	}
	if !ok && s.appFlag == "" {
		return "", "", output.New("INVALID_REQUEST", "This directory is not bound to an app.", "Run whisk use <org>/<app>, whisk init, or pass --org and --app.", nil)
	}
	if !ok {
		// --app alone names an app of the one org the token was approved for, or of the
		// caller's only org.
		org, err := s.org()
		return org, s.appFlag, err
	}
	org, app = b.Org, b.App
	if s.orgFlag != "" {
		org = s.orgFlag
	}
	if s.appFlag != "" {
		app = s.appFlag
	}
	return org, app, nil
}

// wrap turns an API transport failure into the platform-unavailable error and passes
// platform errors through with their code.
// needsHumanLogin turns BUSINESS_NOT_IN_LOGIN into the NEEDS_HUMAN block (pure): the login is
// used in a business it does not cover, and only the person, signed in, adds it, at the link the
// platform sent. The block keeps that link as details.url, so it prints plainly, and names the
// platform's code as details.reason. nil for any other error, or one without a link.
func needsHumanLogin(ae *api.Error) *output.Error {
	if ae.Code != "BUSINESS_NOT_IN_LOGIN" {
		return nil
	}
	link, _ := ae.Details["url"].(string)
	if link == "" {
		return nil
	}
	name, _ := ae.Details["org_name"].(string)
	if name == "" {
		name, _ = ae.Details["org"].(string)
	}
	details := map[string]any{"reason": ae.Code}
	for k, v := range ae.Details {
		details[k] = v
	}
	return &output.Error{Code: "NEEDS_HUMAN", Message: ae.Message,
		Fix:     "Ask the person to open this link signed in, check the login is theirs and tap Add " + name + ", then run the command again. No new login is needed.",
		Docs:    "https://skill.whisk.run/errors/BUSINESS_NOT_IN_LOGIN",
		Details: details, Status: ae.Status}
}

func wrap(err error) error {
	if err == nil {
		return nil
	}
	var ae *api.Error
	if errors.As(err, &ae) {
		if e := needsHumanLogin(ae); e != nil {
			return e
		}
		return &output.Error{Code: ae.Code, Message: ae.Message, Fix: ae.Fix, Docs: ae.Docs, Details: ae.Details, Status: ae.Status}
	}
	var u *api.Unavailable
	if errors.As(err, &u) {
		return output.New("PLATFORM_UNAVAILABLE", "The platform could not be reached: "+u.Err.Error(), "Retry with backoff. Check WHISK_API if you meant another endpoint.", nil)
	}
	return err
}
