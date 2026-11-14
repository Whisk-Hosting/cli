package whisk

import (
	"context"
	"errors"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/whisk-run/contract/apitypes"
)

// Commands agents reach for (CLI.md §7). When a command line fails before any command runs (a
// command or flag this build lacks, a required flag left out, the wrong arguments), the CLI tells
// Whisk its shape: the command it reached, the unknown word or flag names, the names of the
// flags given, how many arguments, the CLI version and whether an agent ran it. No argument or
// flag value goes, nor anything a command did. WHISK_NO_TELEMETRY=1 or DO_NOT_TRACK=1 turns it
// off. The pure half is missOf, agentOf and reportsOff; reportMiss is the shell that sends.

// missDeadline bounds the whole report, so a failed command line never waits on it for long.
const missDeadline = 2 * time.Second

// usageFault is an error cobra raised reading the command line, with the command it reached:
// a flag that would not parse, or arguments the command's Args refused (kind MissArgs).
type usageFault struct {
	cmd  *cobra.Command
	kind apitypes.CLIMissKind
	err  error
}

func (u *usageFault) Error() string { return u.err.Error() }
func (u *usageFault) Unwrap() error { return u.err }

// markUsage makes every flag and argument error in the tree a usageFault naming its command.
func markUsage(root *cobra.Command) {
	root.SetFlagErrorFunc(func(c *cobra.Command, err error) error { return &usageFault{cmd: c, err: err} })
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if check := c.Args; check != nil {
			c.Args = func(cmd *cobra.Command, args []string) error {
				if err := check(cmd, args); err != nil {
					return &usageFault{cmd: cmd, kind: apitypes.MissArgs, err: err}
				}
				return nil
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}

var (
	commandWord   = regexp.MustCompile(`^[a-z][a-z-]{0,31}$`)
	flagWord      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)
	unknownCmd    = regexp.MustCompile(`^unknown command "([^"]*)" for `)
	quotedName    = regexp.MustCompile(`"([^"]+)"`)
	flagGroup     = regexp.MustCompile(`flags in the group \[([^\]]*)\]`)
	shorthandName = regexp.MustCompile(`'(.)'`)
)

// missOf is the report for a command line that failed with err, or false when err is not one
// the command line caused. Pure: it reads the command tree and the arguments, never a value.
func missOf(err error, root *cobra.Command, args []string) (apitypes.CLIMiss, bool) {
	kind, word, cmd := classify(err)
	if kind == "" {
		return apitypes.CLIMiss{}, false
	}
	found, rest, _ := root.Find(args)
	if cmd == nil {
		cmd = found
	}
	if cmd == nil {
		cmd = root
	}
	n := argCount(cmd, rest)
	if kind == apitypes.MissUnknownCommand && n > 0 {
		n-- // the unknown word itself
	}
	return apitypes.CLIMiss{Kind: kind, Command: commandPath(root, cmd), Word: word, Flags: flagNames(cmd, args), Args: n}, true
}

// helpMiss is the report for a command line that printed a command's help instead of running
// one: a word after a command that only groups others (whisk deploys lst), which cobra answers
// with that command's help. args is what the help was shown for; nil when none was. Asking for
// help (--help, -h) is not a miss. Pure.
func helpMiss(root *cobra.Command, args []string) (apitypes.CLIMiss, bool) {
	if args == nil || slices.Contains(args, "--help") || slices.Contains(args, "-h") {
		return apitypes.CLIMiss{}, false
	}
	cmd, rest, err := root.Find(args)
	if err != nil || cmd == nil || cmd.Runnable() || !cmd.HasSubCommands() {
		return apitypes.CLIMiss{}, false
	}
	word := firstArg(cmd, rest)
	if word == "" || word == "help" {
		return apitypes.CLIMiss{}, false
	}
	if w := strings.ToLower(word); commandWord.MatchString(w) {
		word = w
	} else {
		word = ""
	}
	return apitypes.CLIMiss{Kind: apitypes.MissUnknownCommand, Command: commandPath(root, cmd), Word: word,
		Flags: flagNames(cmd, args), Args: max(argCount(cmd, rest)-1, 0)}, true
}

// firstArg is the first argument in rest that is neither a flag nor a flag's value.
func firstArg(cmd *cobra.Command, rest []string) string {
	for i := range rest {
		if argCount(cmd, rest[:i+1]) == 1 {
			return rest[i]
		}
	}
	return ""
}

// classify is the kind of miss err is, the word or flags it names, and the command it reached
// when the error says. An error a command returned is none of them.
func classify(err error) (apitypes.CLIMissKind, string, *cobra.Command) {
	var cmd *cobra.Command
	var uf *usageFault
	if errors.As(err, &uf) {
		cmd = uf.cmd
		if uf.kind == apitypes.MissArgs {
			return apitypes.MissArgs, "", cmd
		}
	}
	var notExist *pflag.NotExistError
	var needsValue *pflag.ValueRequiredError
	var badValue *pflag.InvalidValueError
	var badSyntax *pflag.InvalidSyntaxError
	msg := err.Error()
	switch {
	case errors.As(err, &notExist):
		name := notExist.GetSpecifiedName()
		if notExist.GetSpecifiedShortnames() != "" {
			if m := shorthandName.FindStringSubmatch(msg); m != nil {
				return apitypes.MissUnknownFlag, flagToken(m[1], true), cmd
			}
			return apitypes.MissUnknownFlag, "", cmd
		}
		return apitypes.MissUnknownFlag, flagToken(name, false), cmd
	case errors.As(err, &needsValue) && needsValue.GetFlag() != nil:
		return apitypes.MissFlagValue, flagToken(needsValue.GetFlag().Name, false), cmd
	case errors.As(err, &badValue) && badValue.GetFlag() != nil:
		return apitypes.MissFlagValue, flagToken(badValue.GetFlag().Name, false), cmd
	case errors.As(err, &badSyntax):
		return apitypes.MissUnknownFlag, "", cmd
	case strings.HasPrefix(msg, "unknown flag: --"):
		return apitypes.MissUnknownFlag, flagToken(strings.TrimSpace(strings.TrimPrefix(firstLine(msg), "unknown flag: --")), false), cmd
	case strings.HasPrefix(msg, "unknown command "):
		if m := unknownCmd.FindStringSubmatch(msg); m != nil {
			if w := strings.ToLower(m[1]); commandWord.MatchString(w) {
				return apitypes.MissUnknownCommand, w, cmd
			}
		}
		return apitypes.MissUnknownCommand, "", cmd
	case strings.HasPrefix(msg, "required flag(s) "):
		return apitypes.MissMissingFlag, flagTokens(quotedName.FindAllStringSubmatch(msg, -1)), cmd
	case flagGroup.MatchString(msg):
		m := flagGroup.FindStringSubmatch(msg)
		names := strings.Fields(m[1])
		groups := make([][]string, len(names))
		for i, n := range names {
			groups[i] = []string{"", n}
		}
		return apitypes.MissFlagGroup, flagTokens(groups), cmd
	}
	if uf != nil {
		return apitypes.MissFlagValue, "", cmd
	}
	return "", "", nil
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// flagToken writes a flag name as it is typed, or "" when it is not shaped like one.
func flagToken(name string, short bool) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if !flagWord.MatchString(name) {
		return ""
	}
	if short {
		return "-" + name
	}
	return "--" + name
}

// flagTokens joins the names in regexp submatches (group 1) as typed flags.
func flagTokens(matches [][]string) string {
	out := []string{}
	for _, m := range matches {
		if t := flagToken(m[1], false); t != "" {
			out = append(out, t)
		}
	}
	return strings.Join(out, " ")
}

// commandPath is cmd's path below root: "deploys list", or "" for root itself.
func commandPath(root, cmd *cobra.Command) string {
	return strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), root.Name()))
}

// flagNames are the flags given, by long name where cmd knows them, each once and sorted. A
// value given after a flag, or after "=", is never read.
func flagNames(cmd *cobra.Command, args []string) []string {
	fs := cmd.Flags()
	out := []string{}
	for _, a := range args {
		if a == "--" {
			break
		}
		switch {
		case strings.HasPrefix(a, "--"):
			name, _, _ := strings.Cut(a[2:], "=")
			if t := flagToken(name, false); t != "" {
				out = append(out, t)
			}
		case strings.HasPrefix(a, "-") && len(a) > 1:
			short, _, _ := strings.Cut(a[1:], "=")
			for _, r := range short {
				if f := fs.ShorthandLookup(string(r)); f != nil {
					out = append(out, "--"+f.Name)
					if f.NoOptDefVal == "" {
						break // the rest of the group is this flag's value
					}
				} else if t := flagToken(string(r), true); t != "" {
					out = append(out, t)
					break
				}
			}
		}
	}
	slices.Sort(out)
	out = slices.Compact(out)
	if len(out) > 20 {
		out = out[:20]
	}
	return out
}

// argCount is how many arguments rest holds once flags and the values they take are left out.
func argCount(cmd *cobra.Command, rest []string) int {
	fs := cmd.Flags()
	n := 0
	for i := 0; i < len(rest); i++ {
		a := rest[i]
		if a == "--" {
			return n + len(rest) - i - 1
		}
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			if strings.Contains(a, "=") {
				continue
			}
			var f *pflag.Flag
			if strings.HasPrefix(a, "--") {
				f = fs.Lookup(a[2:])
			} else {
				f = fs.ShorthandLookup(a[len(a)-1:])
			}
			if f != nil && f.NoOptDefVal == "" {
				i++ // its value
			}
			continue
		}
		n++
	}
	return n
}

// knownAgents are the coding agents that say so in the environment of the shells they run.
var knownAgents = []struct{ env, name string }{
	{"CLAUDECODE", "claude-code"},
	{"CODEX_SANDBOX", "codex"},
	{"CURSOR_AGENT", "cursor"},
	{"GEMINI_CLI", "gemini-cli"},
	{"OPENCODE", "opencode"},
}

var notAgentChar = regexp.MustCompile(`[^a-z0-9-]+`)

// agentOf names the coding agent running the CLI: WHISK_AGENT as a short lower-case name, else
// one of knownAgents, else "". Pure over getenv.
func agentOf(getenv func(string) string) string {
	if v := strings.Trim(notAgentChar.ReplaceAllString(strings.ToLower(strings.TrimSpace(getenv("WHISK_AGENT"))), "-"), "-"); v != "" {
		if len(v) > 32 {
			v = strings.TrimRight(v[:32], "-")
		}
		return v
	}
	for _, a := range knownAgents {
		if on(getenv(a.env)) {
			return a.name
		}
	}
	return ""
}

// reportsOff reports whether the person turned the reports off: WHISK_NO_TELEMETRY or the
// common DO_NOT_TRACK set to anything but 0 or false. Pure over getenv.
func reportsOff(getenv func(string) string) bool {
	return on(getenv("WHISK_NO_TELEMETRY")) || on(getenv("DO_NOT_TRACK"))
}

func on(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v != "" && v != "0" && v != "false"
}

// reportMiss sends m with the caller's token when there is one, so Whisk can leave its own
// businesses out of the counts, and without one otherwise. Whatever happens is silent: the
// command line already failed with its own error.
func (s *session) reportMiss(m apitypes.CLIMiss) {
	if !s.env.ReportMisses || reportsOff(s.env.Getenv) {
		return
	}
	m.Version, m.OS, m.Arch, m.Agent, m.Terminal = Version, s.goos(), runtime.GOARCH, agentOf(s.env.Getenv), s.env.IsTerminal
	ctx, cancel := context.WithTimeout(s.ctx, missDeadline)
	defer cancel()
	if s.cfgDir == "" {
		// The command line failed before prepare ran; keep the printer that wrote the error.
		printer := s.printer
		err := s.prepare()
		s.printer = printer
		if err != nil {
			return
		}
	}
	c, _, err := s.client()
	if err != nil {
		c = s.anonymousClient()
	}
	_ = c.ReportCLIMiss(ctx, m)
}
