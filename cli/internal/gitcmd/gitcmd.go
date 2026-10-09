// Package gitcmd runs the git binary for whisk deploy and parses what it says. The Runner
// interface is the only effect; everything else is a pure function over git's output, so the
// deploy flow is testable with a fake runner and the parsers with strings.
package gitcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	werrors "github.com/whisk-run/contract/errors"
	"github.com/whisk-run/contract/run"
)

// Result is what one git invocation produced. A non-zero exit is a Result, not an error.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// OK reports a zero exit.
func (r Result) OK() bool { return r.ExitCode == 0 }

// Out is stdout without surrounding whitespace.
func (r Result) Out() string { return strings.TrimSpace(r.Stdout) }

// Runner runs git in a directory with extra environment variables.
type Runner interface {
	Run(ctx context.Context, dir string, env []string, args ...string) (Result, error)
}

// ErrNotInstalled is returned when no git binary is on PATH.
var ErrNotInstalled = errors.New("git is not installed or not on PATH")

// Timeout bounds one git invocation; the longest is a first push of a large repository over a
// slow link.
const Timeout = 30 * time.Minute

// Exec runs the real git binary. Stdin is closed so git never waits on a prompt.
type Exec struct{}

// Run implements Runner.
func (Exec) Run(ctx context.Context, dir string, env []string, args ...string) (Result, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return Result{}, ErrNotInstalled
	}
	cmd := run.Command(ctx, Timeout, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	res := Result{Stdout: out.String(), Stderr: errb.String()}
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		res.ExitCode = exit.ExitCode()
		if res.ExitCode == 0 {
			res.ExitCode = 1
		}
	default:
		return res, err
	}
	return res, nil
}

// TokenVar is the environment variable the credential helper reads the token from. The token
// is never on the command line and never written to .git/config.
const TokenVar = "WHISK_GIT_TOKEN"

// Credential returns the environment and the -c arguments that hand a token to git for one
// invocation against gitURL: the configured helpers are cleared and an inline helper answers
// with the token from the environment, for gitURL's scheme and host only. A pushurl, an
// insteadOf rule or a redirect that sends git elsewhere finds no helper there, so the token
// goes nowhere but the platform's git host. Only https is accepted (http on the machine itself,
// for a local stack). GIT_TERMINAL_PROMPT=0 makes a failed authentication fail instead of
// asking.
func Credential(token, gitURL string) (env []string, args []string, err error) {
	origin, err := CredentialOrigin(gitURL)
	if err != nil {
		return nil, nil, err
	}
	env = []string{TokenVar + "=" + token, "GIT_TERMINAL_PROMPT=0"}
	args = []string{
		"-c", "credential.helper=",
		"-c", "credential." + origin + `.helper=!f() { echo username=whisk; echo "password=$` + TokenVar + `"; }; f`,
	}
	return env, args, nil
}

// CredentialOrigin is the scheme and host git matches credential settings on, for a git URL a
// token may be sent to: https, or http to the machine itself.
func CredentialOrigin(gitURL string) (string, error) {
	u, err := url.Parse(gitURL)
	if err != nil || !reHost.MatchString(u.Host) || u.User != nil {
		return "", fmt.Errorf("the platform gave a git address that is not a plain https URL: %q", gitURL)
	}
	switch {
	case u.Scheme == "https":
	case u.Scheme == "http" && loopback(u.Hostname()):
	default:
		return "", fmt.Errorf("refusing to send the token to %s: the platform's git address must use https", u.Scheme+"://"+u.Host)
	}
	return u.Scheme + "://" + u.Host, nil
}

// reHost is a host name or bracketed IP address with an optional port: the origin becomes part
// of a git -c key, where anything else (an '=' above all) would change the setting.
var reHost = regexp.MustCompile(`^(([A-Za-z0-9-]+\.)*[A-Za-z0-9-]+|\[[0-9A-Fa-f:.]+\])(:[0-9]+)?$`)

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// SchannelArgs are the -c arguments that make one git invocation verify TLS with the Windows
// certificate store instead of the OpenSSL bundle Git for Windows ships. Machines whose
// trusted roots live only in the Windows store (an enterprise root, an inspecting proxy or
// antivirus) fail with OpenSSL and succeed with schannel.
var SchannelArgs = []string{"-c", "http.sslBackend=schannel"}

// StallArgs are the -c arguments that end a networked git command (push, clone, fetch) whose
// transfer runs below 1000 bytes a second for 60 seconds, so a connection that stalls fails
// instead of hanging.
var StallArgs = []string{"-c", "http.lowSpeedLimit=1000", "-c", "http.lowSpeedTime=60"}

// Networked is the arguments of a git command that talks to the platform: the credential's -c
// arguments, StallArgs, then the command and its arguments, in a new slice.
func Networked(credential []string, command ...string) []string {
	out := make([]string, 0, len(credential)+len(StallArgs)+len(command))
	out = append(out, credential...)
	out = append(out, StallArgs...)
	return append(out, command...)
}

// Sideband is what the server said during a push: lines prefixed "whisk:" on git's sideband.
// The post-receive hook prints "whisk: deploy <id>"; a pre-receive rejection prints the error
// object, either as one JSON line or as "whisk: <CODE>: <message>" followed by
// "whisk: fix: <fix>". Lines is every whisk line in order, for display.
type Sideband struct {
	DeployID string
	Error    *werrors.Detail
	Lines    []string
}

var (
	reAnsi   = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	reCoded  = regexp.MustCompile(`^([A-Z][A-Z0-9_]+): (.*)$`)
	reDeploy = regexp.MustCompile(`^deploy ([A-Za-z0-9_-]+)$`)
)

// stripAnsi removes terminal colour codes until none is left, so one hidden inside another
// ("\x1b[\x1b[A0A") does not survive into a line the CLI prints.
func stripAnsi(s string) string {
	for {
		next := reAnsi.ReplaceAllString(s, "")
		if next == s {
			return s
		}
		s = next
	}
}

// ParseSideband extracts the whisk lines from git's stderr.
func ParseSideband(stderr string) Sideband {
	var sb Sideband
	for _, raw := range strings.FieldsFunc(stderr, func(r rune) bool { return r == '\n' || r == '\r' }) {
		line := strings.TrimSpace(stripAnsi(raw))
		line = strings.TrimPrefix(line, "remote: ")
		line = strings.TrimSpace(line)
		payload, ok := strings.CutPrefix(line, "whisk:")
		if !ok {
			continue
		}
		payload = strings.TrimSpace(payload)
		if payload == "" {
			continue
		}
		sb.Lines = append(sb.Lines, payload)
		switch {
		case strings.HasPrefix(payload, "{"):
			var body werrors.Body
			if err := json.Unmarshal([]byte(payload), &body); err == nil && body.Error.Code != "" {
				d := body.Error
				sb.Error = &d
			}
		case reDeploy.MatchString(payload):
			sb.DeployID = reDeploy.FindStringSubmatch(payload)[1]
		case strings.HasPrefix(payload, "fix: "):
			if sb.Error != nil && sb.Error.Fix == "" {
				sb.Error.Fix = strings.TrimPrefix(payload, "fix: ")
			}
		case strings.HasPrefix(payload, "docs: "):
			if sb.Error != nil && sb.Error.Docs == "" {
				sb.Error.Docs = strings.TrimPrefix(payload, "docs: ")
			}
		case reCoded.MatchString(payload):
			m := reCoded.FindStringSubmatch(payload)
			if sb.Error == nil {
				sb.Error = &werrors.Detail{Code: m[1], Message: m[2], Docs: werrors.DocsBase + m[1]}
			}
		}
	}
	return sb
}

// PushOutcome is what happened to the pushed ref.
type PushOutcome int

// Outcomes of a push.
const (
	PushUnknown  PushOutcome = iota
	PushUpdated              // the remote ref moved
	PushUpToDate             // nothing to push
	PushRejected             // the remote refused
)

// ParsePush reads git push --porcelain output: one tab-separated line per ref, whose first
// character says what happened. summary is git's own words for that ref.
func ParsePush(stdout string) (outcome PushOutcome, summary string) {
	for _, line := range strings.Split(stdout, "\n") {
		parts := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(parts) < 2 || len(parts[0]) != 1 {
			continue
		}
		summary = ""
		if len(parts) >= 3 {
			summary = strings.TrimSpace(parts[2])
		}
		switch parts[0] {
		case "=":
			return PushUpToDate, summary
		case "!":
			return PushRejected, summary
		case " ", "+", "*", "-":
			return PushUpdated, summary
		}
	}
	if strings.Contains(stdout, "Everything up-to-date") {
		return PushUpToDate, "up to date"
	}
	return PushUnknown, ""
}

// ParseStatus turns git status --porcelain output into the paths that differ from HEAD.
func ParseStatus(stdout string) []string {
	var paths []string
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimRight(line, "\r")
		if len(line) < 4 {
			continue
		}
		paths = append(paths, strings.TrimSpace(line[3:]))
	}
	return paths
}

// Failure classifies a failed push that carried no whisk error: a certificate git does not
// trust, a network fault, a refused credential, a rejected ref, or something else. kind is one
// of "tls", "network", "auth", "rejected", "other". A certificate failure is checked first
// because git reports it as "unable to access", which would otherwise read as the network. A
// connection that dropped part-way ("the remote end hung up", "unexpected disconnect") is the
// network too, checked last so a refused credential or a rejected ref keeps its own kind.
func Failure(stderr string, outcome PushOutcome) (kind string) {
	low := strings.ToLower(stderr)
	switch {
	case strings.Contains(low, "ssl certificate problem"), strings.Contains(low, "unable to get local issuer certificate"),
		strings.Contains(low, "server certificate verification failed"), strings.Contains(low, "certificate verify failed"):
		return "tls"
	case strings.Contains(low, "could not resolve host"), strings.Contains(low, "failed to connect"),
		strings.Contains(low, "connection refused"), strings.Contains(low, "connection reset"),
		strings.Contains(low, "timed out"), strings.Contains(low, "unable to access"):
		return "network"
	case strings.Contains(low, "authentication failed"), strings.Contains(low, "403"),
		strings.Contains(low, "401"), strings.Contains(low, "invalid username or password"),
		strings.Contains(low, "could not read username"):
		return "auth"
	case outcome == PushRejected, strings.Contains(low, "[remote rejected]"), strings.Contains(low, "[rejected]"):
		return "rejected"
	case strings.Contains(low, "the remote end hung up"), strings.Contains(low, "unexpected disconnect"),
		strings.Contains(low, "early eof"), strings.Contains(low, "empty reply from server"),
		strings.Contains(low, "operation too slow"):
		// The connection dropped part-way, or stalled below http.lowSpeedLimit (StallArgs),
		// with no refusal from either side.
		return "network"
	}
	return "other"
}

// LastLines returns the last n non-empty lines of a text.
func LastLines(text string, n int) []string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	kept := lines[:0]
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			kept = append(kept, strings.TrimRight(l, "\r"))
		}
	}
	if len(kept) > n {
		kept = kept[len(kept)-n:]
	}
	return kept
}
