package doctor

import (
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// shellCommand is one simple command of a shell text: its words as the shell would hand them
// to the program, on one line, and the line of the text it is on.
type shellCommand struct {
	Text string
	Line int
}

// printers are commands that only print or do nothing, so what their words say is not run.
var printers = map[string]bool{"echo": true, "printf": true, ":": true, "true": true, "false": true}

// shells run the command after -c.
var shells = map[string]bool{"sh": true, "bash": true, "dash": true, "ash": true, "zsh": true}

// shellCommands is each simple command a shell text runs, in order, read from its syntax tree:
// the commands either side of &&, ||, ; and |, inside if, for, while, case, a subshell and a
// function, and the command a shell runs with -c. A command that only prints (echo, printf) is
// left out, so words inside its text are never read as a command. A variable is its value in
// env when env has it, and is kept as written when not. Text that is not shell (it does not
// parse) is one command as written. Pure.
func shellCommands(text string, env map[string]string) []shellCommand {
	return shellCommandsAt(text, env, 0, 0)
}

func shellCommandsAt(text string, env map[string]string, offset, depth int) []shellCommand {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(text), "")
	if err != nil {
		if t := strings.Join(strings.Fields(text), " "); t != "" {
			return []shellCommand{{Text: t, Line: offset + 1}}
		}
		return nil
	}
	var out []shellCommand
	syntax.Walk(file, func(n syntax.Node) bool {
		call, ok := n.(*syntax.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		words := make([]string, 0, len(call.Args))
		for _, w := range call.Args {
			words = append(words, wordText(w, env))
		}
		line := offset + int(call.Args[0].Pos().Line())
		name := words[0][strings.LastIndex(words[0], "/")+1:]
		switch {
		case printers[name]:
			return false
		case shells[name] && depth < 3:
			for i := 1; i+1 < len(words); i++ {
				if words[i] == "-c" || (strings.HasPrefix(words[i], "-") && strings.HasSuffix(words[i], "c") && !strings.HasPrefix(words[i], "--")) {
					out = append(out, shellCommandsAt(words[i+1], env, line-1, depth+1)...)
					return false
				}
			}
		}
		out = append(out, shellCommand{Text: strings.Join(words, " "), Line: line})
		return true
	})
	return out
}

// wordText is a word as the program would get it: quotes taken off, a variable replaced by its
// value in env, and anything worked out when it runs (a command substitution, arithmetic) kept
// as written.
func wordText(w *syntax.Word, env map[string]string) string {
	var b strings.Builder
	var parts func(ps []syntax.WordPart)
	parts = func(ps []syntax.WordPart) {
		for _, p := range ps {
			switch x := p.(type) {
			case *syntax.Lit:
				b.WriteString(unescaped(x.Value))
			case *syntax.SglQuoted:
				b.WriteString(x.Value)
			case *syntax.DblQuoted:
				parts(x.Parts)
			case *syntax.ParamExp:
				if v, ok := env[x.Param.Value]; ok && x.Exp == nil && x.Repl == nil && x.Slice == nil && !x.Length && !x.Excl {
					b.WriteString(v)
				} else {
					b.WriteString(printed(p))
				}
			default:
				b.WriteString(printed(p))
			}
		}
	}
	parts(w.Parts)
	return b.String()
}

// unescaped is an unquoted literal without the backslashes that escape its characters.
func unescaped(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			if s[i] == '\n' {
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// printed is a part of a word as it is written.
func printed(n syntax.Node) string {
	var b strings.Builder
	if syntax.NewPrinter().Print(&b, n) != nil {
		return ""
	}
	return b.String()
}
