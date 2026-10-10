// Package output is the CLI's voice: JSON on stdout for agents, tables and sentences for
// humans, errors as the platform's error object, exit codes from CLI.md §3.
package output

import (
	"encoding/json"
	stderrors "errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/whisk-run/contract/errors"
)

// APIVersion is the API version the CLI speaks; every JSON result carries it.
const APIVersion = "v1"

// Exit codes, CLI.md §3.
const (
	ExitOK          = 0
	ExitError       = 1
	ExitNeedsHuman  = 2
	ExitValidation  = 3
	ExitAuth        = 4
	ExitUnavailable = 5
	ExitDeploy      = 6
)

// Error is a CLI or platform error with a stable code, a sentence and a fix. It is printed as
// the error object from CONTROL-PLANE.md §9 and mapped to an exit code.
type Error struct {
	Code    string
	Message string
	Fix     string
	Docs    string
	Details map[string]any
	Status  int // HTTP status when the platform produced it, else 0
	Exit    int // 0 means derive from the code
}

func (e *Error) Error() string { return e.Message }

// New builds an Error for a catalogued code, taking the fix from errors.md when none is given.
func New(code, message, fix string, details map[string]any) *Error {
	body := errors.New(code, message, fix, details)
	return &Error{Code: body.Error.Code, Message: body.Error.Message, Fix: body.Error.Fix, Docs: body.Error.Docs, Details: body.Error.Details}
}

// Body is the JSON shape of the error.
func (e *Error) Body() map[string]any {
	details := e.Details
	if details == nil {
		details = map[string]any{}
	}
	docs := e.Docs
	if _, known := errors.Lookup(e.Code); docs == "" && known {
		docs = errors.DocsBase + e.Code
	}
	return map[string]any{"error": map[string]any{
		"code": e.Code, "message": e.Message, "fix": e.Fix, "docs": docs, "details": details,
	}}
}

var exitByCode = map[string]int{
	"NEEDS_HUMAN":                     ExitNeedsHuman,
	"SECRET_VALUE_NEEDS_HUMAN":        ExitNeedsHuman,
	"MANIFEST_INVALID":                ExitValidation,
	"MANIFEST_UNKNOWN_KEY":            ExitValidation,
	"CONVENTIONS_VERSION_UNSUPPORTED": ExitValidation,
	"DOCTOR_FAILED":                   ExitValidation,
	"SECRET_IN_COMMIT":                ExitValidation,
	"REPO_TOO_LARGE":                  ExitValidation,
	"PLAN_LIMIT_PREVIEWS":             ExitValidation,
	"PRODUCTION_NEEDS_MAIN":           ExitValidation,
	"AUTH_REQUIRED":                   ExitAuth,
	"TOKEN_EXPIRED":                   ExitAuth,
	"TOKEN_REVOKED":                   ExitAuth,
	"TOKEN_SCOPE":                     ExitAuth,
	"AGENT_CATEGORY":                  ExitAuth,
	"AGENT_PAUSED":                    ExitAuth,
	"TOKEN_INVALID":                   ExitAuth,
	"FORBIDDEN_ROLE":                  ExitAuth,
	"PLATFORM_UNAVAILABLE":            ExitUnavailable,
	"UPSTREAM_UNAVAILABLE":            ExitUnavailable,
	"NODE_DOWN":                       ExitUnavailable,
	"RESTORE_POINT_NOT_ARCHIVED":      ExitUnavailable,
	"BUILD_FAILED":                    ExitDeploy,
	"BUILD_TIMEOUT":                   ExitDeploy,
	"SECRET_IN_IMAGE":                 ExitDeploy,
	"IMAGE_PULL_FAILED":               ExitDeploy,
	"HEALTH_CHECK_FAILED":             ExitDeploy,
	"CONTAINER_CRASHED":               ExitDeploy,
	"MIGRATE_FAILED":                  ExitDeploy,
	"PACKAGE_MALICIOUS":               ExitDeploy,
	"DEPLOY_CANCELLED":                ExitDeploy,
	"PLATFORM_DEPLOY_FAILED":          ExitUnavailable,
}

// ExitCode maps an error to the exit code an agent reads.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var e *Error
	if !stderrors.As(err, &e) {
		return ExitError
	}
	if e.Exit != 0 {
		return e.Exit
	}
	if code, ok := exitByCode[e.Code]; ok {
		return code
	}
	return ExitError
}

// Printer renders results. JSON mode prints exactly one object on stdout; human mode prints
// prose and tables. Progress goes to stderr and is silenced by Quiet.
type Printer struct {
	JSON    bool
	Quiet   bool
	Color   bool
	Out     io.Writer
	Err     io.Writer
	Version string
	// Update is shared by every copy of the printer in one invocation; the API client fills
	// in the version the platform serves (CLI.md §4).
	Update *UpdateCheck
}

// UpdateCheck compares the running CLI with the one the platform serves. Responses may arrive
// on several goroutines (a log stream beside a poll), so the served version is set under a lock.
type UpdateCheck struct {
	Current string

	mu     sync.Mutex
	latest string
}

// SetLatest records the version the platform serves.
func (u *UpdateCheck) SetLatest(v string) {
	u.mu.Lock()
	u.latest = v
	u.mu.Unlock()
}

// Outdated reports whether the platform serves a different CLI from this one. Versions are
// release names, not ordered numbers, so different means older: the platform always serves
// its newest release. A development build ("dev") never reports.
func Outdated(current, latest string) bool {
	return latest != "" && current != "" && current != "dev" && current != latest
}

// Notice is the cli_update object for JSON output, or nil when the CLI is current or the
// platform has not said.
func (u *UpdateCheck) Notice() map[string]any {
	if u == nil {
		return nil
	}
	u.mu.Lock()
	latest := u.latest
	u.mu.Unlock()
	if !Outdated(u.Current, latest) {
		return nil
	}
	return map[string]any{"current": u.Current, "latest": latest, "fix": "Run whisk update, then retry anything that failed as an unknown command."}
}

// Sentence is the stderr line a human sees when the CLI is outdated, or "".
func (u *UpdateCheck) Sentence() string {
	n := u.Notice()
	if n == nil {
		return ""
	}
	return fmt.Sprintf("whisk %s is available (this is %s). Run whisk update.", n["latest"], u.Current)
}

// Result prints a command's result. In JSON mode the object gains whisk_cli and api; in human
// mode `human` is called to print prose instead.
func (p Printer) Result(v map[string]any, human func(w io.Writer)) {
	if p.JSON {
		out := map[string]any{"whisk_cli": p.Version, "api": APIVersion}
		for k, val := range v {
			out[k] = val
		}
		if n := p.Update.Notice(); n != nil {
			out["cli_update"] = n
		}
		enc := json.NewEncoder(p.Out)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(out)
		return
	}
	if human != nil {
		human(p.Out)
	}
}

// Line prints one line of streaming JSON (one object per line) or, in human mode, the text.
// An empty text prints nothing in human mode.
func (p Printer) Line(v any, text string) {
	if p.JSON {
		enc := json.NewEncoder(p.Out)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(v)
		return
	}
	if text != "" {
		fmt.Fprintln(p.Out, text)
	}
}

// Block writes an error for a human: the code and message, the fix, and the URL from details
// when there is one. Fail uses it; commands use it for a NEEDS_HUMAN block after a result that
// succeeded (a live deploy with unset secrets).
func (p Printer) Block(w io.Writer, e *Error) {
	fmt.Fprintln(w, p.paint("31", e.Code)+": "+e.Message)
	if e.Fix != "" {
		fmt.Fprintln(w, "  "+e.Fix)
	}
	if url, ok := e.Details["url"].(string); ok && url != "" {
		fmt.Fprintln(w, "  "+url)
	}
}

// Progress prints a progress sentence to stderr unless quiet.
func (p Printer) Progress(format string, args ...any) {
	if p.Quiet {
		return
	}
	fmt.Fprintf(p.Err, format+"\n", args...)
}

// Fail prints an error: the JSON error object on stdout in JSON mode, or message and fix on
// stderr for a human. It returns the exit code.
func (p Printer) Fail(err error) int {
	var e *Error
	if !stderrors.As(err, &e) {
		e = &Error{Code: "CLI_ERROR", Message: err.Error(), Fix: "Read the message; if it names a platform problem, retry with backoff."}
	}
	if p.JSON {
		body := e.Body()
		if n := p.Update.Notice(); n != nil {
			body["cli_update"] = n
		}
		enc := json.NewEncoder(p.Out)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(body)
	} else {
		p.Block(p.Err, e)
	}
	return ExitCode(e)
}

// Table prints aligned columns.
func (p Printer) Table(w io.Writer, header []string, rows [][]string) {
	widths := make([]int, len(header))
	for i, h := range header {
		widths[i] = len(h)
	}
	for _, r := range rows {
		for i, c := range r {
			if i < len(widths) && len(c) > widths[i] {
				widths[i] = len(c)
			}
		}
	}
	line := func(cells []string) string {
		parts := make([]string, len(cells))
		for i, c := range cells {
			if i == len(cells)-1 {
				parts[i] = c
			} else {
				parts[i] = c + strings.Repeat(" ", widths[i]-len(c))
			}
		}
		return strings.TrimRight(strings.Join(parts, "  "), " ")
	}
	fmt.Fprintln(w, p.paint("1", line(header)))
	for _, r := range rows {
		fmt.Fprintln(w, line(r))
	}
}

func (p Printer) paint(code, s string) string {
	if !p.Color {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// Bold and Dim style a fragment for human output.
func (p Printer) Bold(s string) string { return p.paint("1", s) }
func (p Printer) Dim(s string) string  { return p.paint("2", s) }
func (p Printer) Warn(s string) string { return p.paint("33", s) }
func (p Printer) Bad(s string) string  { return p.paint("31", s) }
func (p Printer) Good(s string) string { return p.paint("32", s) }

// SortedKeys is a helper for deterministic human output of maps.
func SortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Seconds writes a duration in milliseconds as seconds with one decimal, the way start times
// are shown ("7.3 s"), rounded to the nearest tenth; under a tenth it is "0.1 s", never
// "0.0 s". Pure.
func Seconds(ms int64) string {
	tenths := max((ms+50)/100, 1)
	return fmt.Sprintf("%d.%d s", tenths/10, tenths%10)
}
