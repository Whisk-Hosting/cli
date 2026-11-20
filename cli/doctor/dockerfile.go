package doctor

import (
	"bytes"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/parser"
)

// instruction is one Dockerfile instruction as BuildKit's own parser reads it, the parser the
// build runs, so doctor and the build agree on where an instruction starts and ends: a parser
// directive (# escape=), a continued line with comments inside it and a here-document are read
// as the build reads them.
type instruction struct {
	// Line is the line the instruction starts on.
	Line int
	// Keyword is the instruction in upper case: FROM, RUN, CMD.
	Keyword string
	// Text is the keyword and the rest of the instruction on one line, single spaced, with the
	// commands of a RUN's here-documents after it.
	Text string
	// Flags are the instruction's own flags, such as a RUN's --mount=type=secret,id=NAME.
	Flags []string
	// Args are the exec form's words (Exec) or the shell form's command as one word.
	Args []string
	// Exec is the JSON form of CMD, ENTRYPOINT and RUN, which runs without a shell.
	Exec bool
}

// instructionLines is a Dockerfile's instructions in order, none when it does not parse (the
// build fails on it with the parser's own message). Pure.
func instructionLines(src []byte) []instruction {
	res, err := parser.Parse(bytes.NewReader(src))
	if err != nil || res.AST == nil {
		return nil
	}
	var out []instruction
	for _, n := range res.AST.Children {
		fields := strings.Fields(n.Original)
		if len(fields) == 0 {
			continue
		}
		in := instruction{Line: n.StartLine, Keyword: strings.ToUpper(n.Value), Flags: n.Flags, Exec: n.Attributes["json"]}
		fields[0] = in.Keyword
		for a := n.Next; a != nil; a = a.Next {
			in.Args = append(in.Args, a.Value)
		}
		for _, h := range n.Heredocs {
			for _, line := range strings.Split(h.Content, "\n") {
				if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "#") {
					fields = append(fields, ";", line)
				}
			}
		}
		in.Text = strings.Join(fields, " ")
		out = append(out, in)
	}
	return out
}

// finalStage is the instructions of a Dockerfile's last stage, from its FROM.
func finalStage(ins []instruction) []instruction {
	start := 0
	for i, in := range ins {
		if in.Keyword == "FROM" {
			start = i
		}
	}
	return ins[start:]
}

// stageEnv is the variables a stage's ENV instructions set, the last value of each, which a
// shell-form CMD or ENTRYPOINT reads when the container starts.
func stageEnv(ins []instruction) map[string]string {
	env := map[string]string{}
	for _, in := range ins {
		if in.Keyword != "ENV" {
			continue
		}
		// The parser answers name, value and separator for each variable.
		for i := 0; i+1 < len(in.Args); i += 3 {
			env[in.Args[i]] = unquoted(in.Args[i+1])
		}
	}
	return env
}

// unquoted is an ENV value without the quotes around it.
func unquoted(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}

// mountsSecret reports whether a RUN of the Dockerfile mounts the named build secret: a
// --mount flag of type secret whose id or env is the name. Pure.
func mountsSecret(dockerfile, name string) bool {
	for _, in := range instructionLines([]byte(dockerfile)) {
		if in.Keyword != "RUN" {
			continue
		}
		for _, flag := range in.Flags {
			opts, ok := strings.CutPrefix(flag, "--mount=")
			if !ok {
				continue
			}
			secret, named := false, false
			for _, opt := range strings.Split(opts, ",") {
				k, v, _ := strings.Cut(opt, "=")
				switch {
				case k == "type" && v == "secret":
					secret = true
				case (k == "id" || k == "env") && v == name:
					named = true
				}
			}
			if secret && named {
				return true
			}
		}
	}
	return false
}
